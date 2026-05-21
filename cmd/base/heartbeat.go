package main

import (
	"encoding/json"
	"net"
	"strings"
	"time"

	"drone-system/internal/models"
	"drone-system/internal/network"
)

func startHBListener() {
	if err := network.StartUDPListener(cfg.hbAddr, handleHeartbeat); err != nil {
		alertMgr.Critical("Listener UDP heartbeat falhou: %v", err)
	}
}

func handleHeartbeat(msg models.Message, from net.Addr) {
	newTS := clk.Update(msg.LamportTS)

	var hb models.HeartbeatPayload
	if err := json.Unmarshal(msg.Payload, &hb); err != nil {
		return
	}

	switch strings.ToLower(hb.NodeType) {
	case "base":
		handleBaseHeartbeat(hb) // Removida a variável solta que causava erro
	case "drone":
		handleDroneHeartbeat(hb)
	case "sensor":
		state.touchSensor(hb.NodeID, newTS)
	case "broker":
		state.touchBroker(hb.NodeID, newTS)
	}
}

func handleBaseHeartbeat(hb models.HeartbeatPayload) {
	// Força maiúsculo para evitar bugs de visualização no Dashboard
	hb.NodeID = strings.ToUpper(hb.NodeID)

	base := models.Base{
		ID:         hb.NodeID,
		SectorID:   strings.ToUpper(hb.SectorID),
		Addr:       hb.Addr,
		GossipAddr: hb.GossipAddr,
		Status:     models.BaseOnline,
		LastSeen:   time.Now(),
	}

	needsSync := false
	existing := state.allPeers()
	found := false

	for _, p := range existing {
		if p.ID == hb.NodeID {
			found = true
			if p.Status == models.BaseOffline {
				needsSync = true
			}
			break
		}
	}

	// Se for uma nova base desconhecida, nós a adicionamos e sincronizamos
	if !found {
		needsSync = true
	}

	state.updatePeer(base)

	if needsSync && hb.Addr != "" {
		alertMgr.OK("Base %s detectada — sincronizando fila", hb.NodeID)
		go sendFullQueueSync(hb.Addr)
	}
}

func handleDroneHeartbeat(hb models.HeartbeatPayload) {
	state.touchDrone(hb.NodeID, hb.BaseID, hb.Status)
}

func broadcastMyHeartbeat() {
	payload := models.HeartbeatPayload{
		NodeType:   "base",
		NodeID:     cfg.baseID,
		SectorID:   cfg.sectorID,
		GossipAddr: cfg.gossipHost,
		Addr:       cfg.baseHost,
	}

	// CORREÇÃO: Removido o `payloadBytes` que o compilador do Go estava reclamando de "not used"
	ts := clk.Tick()
	network.BroadcastHeartbeat(cfg.peerHB, payload, cfg.baseID, ts)
}

func checkPeersHealth() {
	timeout := network.HeartbeatTimeout
	now := time.Now()

	for _, peer := range state.allPeers() {
		if peer.Status == models.BaseOffline {
			continue
		}
		if now.Sub(peer.LastSeen) > timeout {
			state.markPeerOffline(peer.ID)
			alertMgr.Critical("Base %s OFFLINE", peer.ID)
		}
	}
	checkDroneHealth()
	balanceDrones() // LIGADO DE NOVO!
}

func checkDroneHealth() {
	timeout := network.HeartbeatTimeout
	now := time.Now()

	for _, drone := range state.allDrones() {
		if drone.Status == models.DroneLost {
			// ==========================================
			// GARBAGE COLLECTOR (Lixeiro Automático)
			// Se o drone está morto há mais de 15 segundos,
			// apaga ele da memória para ele sumir da tela!
			// ==========================================
			if now.Sub(drone.LastSeen) > 15*time.Second {
				state.removeDrone(drone.ID)
			}
			continue
		}

		if now.Sub(drone.LastSeen) > timeout {
			alertMgr.Critical("Drone %s LOST", drone.ID)
			state.updateDroneStatus(drone.ID, models.DroneLost)
			recoverBusyDroneMission(drone)
		}
	}
}

func recoverBusyDroneMission(drone models.Drone) {
	all := pq.GetAll()
	for _, req := range all {
		// Se a missão tá grudada no drone morto, arranca ela de lá!
		if req.AssignedTo == drone.ID {

			req.Priority = -1
			req.LamportTS = 0

			desc := req.Description
			desc = strings.TrimPrefix(desc, "NORMAL - ")
			desc = strings.TrimPrefix(desc, "CRITICO - ")
			desc = strings.TrimPrefix(desc, "[FALHA] ")
			desc = strings.TrimPrefix(desc, "FALHA - ")

			req.Type = models.RequestType("FALHA")
			req.Description = "FALHA - " + desc

			// Destrava tudo para qualquer outro drone poder pegar
			req.Status = models.StatusPending
			req.AssignedTo = ""
			req.LockedBy = ""

			pq.Replace(req)
			state.releaseLock(req.ID)

			go broadcastLockRelease(req.ID)
			go gossipQueueUpdate(req)

			alertMgr.Warn("Missão [%s] resgatada do drone morto %s com sucesso!", req.ID, drone.ID)
		}
	}
}
