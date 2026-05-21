package main

import (
	"drone-system/internal/models"
	"drone-system/internal/network"
	"encoding/json"
	"sync"
	"time"
)

func startDispatcher() {
	for {
		// Acelerador: A base agora checa a fila a cada 100ms em vez de 500ms!
		time.Sleep(100 * time.Millisecond)
		tryDispatch()
	}
}

func tryDispatch() {
	// 1. Pega a missão e verifica se há drone
	req, hasReq := pq.Peek()
	if !hasReq || req.Status != models.StatusPending {
		return
	}

	drone, hasDrone := state.idleDrone(cfg.baseID)
	if !hasDrone || pq.IsDroneAssigned(drone.ID) {
		return
	}

	// 2. Trava a missão e o drone LOCAMENTE primeiro, para que
	// o loop principal continue e não tente dar o mesmo drone pra outra coisa.
	pq.UpdateStatus(req.ID, models.StatusLocked, cfg.baseID, drone.ID)
	state.updateDroneStatus(drone.ID, models.DroneBusy)

	// 3. O SEGREDO DA VELOCIDADE: Abre uma Thread (goroutine)!
	// A Base NÃO congela mais esperando a rede. Ela despacha isso no background!
	go func(r models.Request, d models.Drone) {

		if !acquireLock(r.ID) {
			// Se alguém negou na rede, liberta o drone e devolve a missão
			state.updateDroneStatus(d.ID, models.DroneIdle)
			pq.UpdateStatus(r.ID, models.StatusPending, "", "")
			return
		}

		payload, _ := json.Marshal(models.DispatchPayload{
			RequestID:   r.ID,
			SectorID:    r.SectorID,
			Description: r.Description,
			Priority:    r.Priority,
			TargetX:     sectorCoordX(r.SectorID),
			TargetY:     sectorCoordY(r.SectorID),
		})

		msg := models.Message{Type: models.MsgDispatch, SenderID: cfg.baseID, LamportTS: clk.Tick(), Payload: payload}

		// Avisa o drone para ir trabalhar
		_, err := network.SendMessageWithReply(d.Addr, msg)
		if err != nil {
			state.updateDroneStatus(d.ID, models.DroneIdle)
			state.releaseLock(r.ID)
			broadcastLockRelease(r.ID)
			pq.UpdateStatus(r.ID, models.StatusPending, "", "")
			return
		}

		// Confirma o despacho!
		pq.UpdateStatus(r.ID, models.StatusAssigned, cfg.baseID, d.ID)
		r.Status = models.StatusAssigned
		r.AssignedTo = d.ID
		gossipQueueUpdate(r)
	}(req, drone)
}

func acquireLock(reqID string) bool {
	if !state.tryLock(reqID, cfg.baseID) {
		return false
	}

	peers := state.onlinePeers()
	if len(peers) == 0 {
		return true
	}

	payload, _ := json.Marshal(models.LockRequestPayload{RequestID: reqID, BaseID: cfg.baseID, LamportTS: clk.Tick()})
	lockMsg := models.Message{Type: models.MsgLockRequest, SenderID: cfg.baseID, LamportTS: clk.Get(), Payload: payload}

	// Busca o consenso na rede inteira simultaneamente (Paralelo)
	var wg sync.WaitGroup
	var mu sync.Mutex
	granted := 0

	for _, peer := range peers {
		wg.Add(1)
		go func(p models.Base) {
			defer wg.Done()
			reply, err := network.SendMessageWithReply(p.GossipAddr, lockMsg)
			if err != nil {
				return
			}
			var lockReply models.LockReplyPayload
			if err := json.Unmarshal(reply.Payload, &lockReply); err == nil && lockReply.Granted {
				mu.Lock()
				granted++
				mu.Unlock()
			}
		}(peer)
	}

	wg.Wait()

	success := (granted == len(peers))

	if !success {
		state.releaseLock(reqID)
		broadcastLockRelease(reqID)
	}

	return success
}

func broadcastLockRelease(reqID string) {
	payload, _ := json.Marshal(models.LockRequestPayload{RequestID: reqID, BaseID: cfg.baseID})
	msg := models.Message{Type: models.MsgLockRelease, SenderID: cfg.baseID, LamportTS: clk.Tick(), Payload: payload}
	network.BroadcastMessage(cfg.peerGossip, msg)
}

func sectorCoordX(sectorID string) float64 {
	coords := map[string]float64{"A": 10, "B": 30, "C": 10, "D": 30}
	if v, ok := coords[sectorID]; ok {
		return v
	}
	return 20
}
func sectorCoordY(sectorID string) float64 {
	coords := map[string]float64{"A": 10, "B": 10, "C": 30, "D": 30}
	if v, ok := coords[sectorID]; ok {
		return v
	}
	return 20
}
