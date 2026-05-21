package main

import (
	"drone-system/internal/models"
	"drone-system/internal/network"
	"encoding/json"
	"net"
	"strings" // <-- ADICIONE ISSO
	"time"
)

func handleDroneRegister(conn net.Conn, msg models.Message) {
	var reg models.RegisterPayload
	if err := json.Unmarshal(msg.Payload, &reg); err != nil {
		return
	}

	drone := models.Drone{
		ID:       reg.DroneID,
		BaseID:   cfg.baseID,
		SectorID: cfg.sectorID,
		Status:   models.DroneIdle,
		X:        reg.X,
		Y:        reg.Y,
		LastSeen: time.Now(),
		Addr:     reg.Addr,
	}

	state.registerDrone(drone)

	// PROTEÇÃO MÁXIMA: Se o drone acabou de pular pra cá,
	// limpamos qualquer missão fantasma presa no nome dele!
	pq.ClearDroneAssignments(drone.ID)

	network.WriteMessage(conn, models.Message{Type: models.MsgRegister, SenderID: cfg.baseID, LamportTS: clk.Tick()})
	go propagateDroneInfo(drone)
}

func handleMissionDone(msg models.Message) {
	var done models.MissionDonePayload
	if err := json.Unmarshal(msg.Payload, &done); err != nil {
		return
	}

	state.updateDroneStatus(done.DroneID, models.DroneIdle)
	state.updateDroneMissionDesc(done.DroneID, "")

	if d, ok := state.getDrone(done.DroneID); ok {
		go propagateDroneInfo(d)
	}

	pq.UpdateStatus(done.RequestID, models.StatusDone, cfg.baseID, done.DroneID)
	pq.Remove(done.RequestID)
	state.releaseLock(done.RequestID)
	go broadcastLockRelease(done.RequestID)

	req := models.Request{ID: done.RequestID, Status: models.StatusDone, AssignedTo: done.DroneID, LockedBy: cfg.baseID}
	go gossipQueueUpdate(req)
}

func handleDroneStatusUpdate(msg models.Message) {
	var update models.DroneStatusPayload
	if err := json.Unmarshal(msg.Payload, &update); err != nil {
		return
	}

	state.updateDroneMissionDesc(update.DroneID, update.MissionDesc)
	if d, ok := state.getDrone(update.DroneID); ok {
		go propagateDroneInfo(d)
	}
}

func propagateDroneInfo(drone models.Drone) {
	payload, _ := json.Marshal(drone)
	msg := models.Message{Type: models.MsgDroneInfo, SenderID: cfg.baseID, LamportTS: clk.Tick(), Payload: payload}
	network.BroadcastMessage(cfg.peerBases, msg)
}

func balanceDrones() {
	myCount := 0
	var myIdleDrones []models.Drone

	// CONTAGEM LOCAL: Ignora diferenças entre "base-a" e "BASE-A"
	for _, d := range state.allDrones() {
		if strings.EqualFold(d.BaseID, cfg.baseID) && d.Status != models.DroneLost {
			myCount++
			if d.Status == models.DroneIdle {
				myIdleDrones = append(myIdleDrones, d)
			}
		}
	}
	if len(myIdleDrones) == 0 {
		return
	}

	var targetPeer models.Base
	found := false
	minDrones := myCount

	peers := state.onlinePeers()
	myID := strings.ToLower(cfg.baseID)

	for _, peer := range peers {
		count := 0
		peerID := strings.ToLower(peer.ID)

		// CONTAGEM DO PEER: Ignora diferenças de letras
		for _, d := range state.allDrones() {
			if strings.EqualFold(d.BaseID, peer.ID) && d.Status != models.DroneLost {
				count++
			}
		}

		diff := myCount - count

		// A MATEMÁTICA PERFEITA: A > B > C > D
		// Só doa se tiver 2+ de diferença, ou se tiver 1 de diferença MAS a outra base tiver maior prioridade
		if diff > 1 || (diff == 1 && myID > peerID) {
			if count < minDrones {
				minDrones = count
				targetPeer = peer
				found = true
			} else if found && count == minDrones {
				// Desempate: Se duas bases têm a mesma quantidade, a prioridade (A, B, C) ganha
				if peerID < strings.ToLower(targetPeer.ID) {
					targetPeer = peer
				}
			}
		}
	}

	if found {
		droneToMove := myIdleDrones[0]
		state.updateDroneStatus(droneToMove.ID, models.DroneLost)
		payload, _ := json.Marshal(models.ReassignPayload{NewBaseID: targetPeer.ID, NewBaseAddr: targetPeer.Addr})
		msg := models.Message{Type: models.MsgReassign, SenderID: cfg.baseID, LamportTS: clk.Tick(), Payload: payload}
		network.SendMessage(droneToMove.Addr, msg)
		go balanceDrones()
	}
}

func init() {
	_ = handleDroneRegister
	_ = handleMissionDone
}
