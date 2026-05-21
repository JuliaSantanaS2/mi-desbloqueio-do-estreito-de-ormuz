// servers.go — Servidores TCP da base (principal + gossip + dashboard)
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"drone-system/internal/models"
	"drone-system/internal/network"
)

// ── Servidor Principal :8001 ─────────────────────────────────────────────────

func startPeriodicChecks() {
	go func() {
		for {
			time.Sleep(1 * time.Second)
			checkPeersHealth()
		}
	}()
}

func startMainServer() {
	if err := network.StartTCPServer(cfg.baseAddr, handleMainConn); err != nil {
		alertMgr.Critical("Servidor principal falhou: %v", err)
	}
}

func handleMainConn(conn net.Conn) {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	msg, err := network.ReadMessage(conn)
	if err != nil {
		return
	}
	clk.Update(msg.LamportTS)

	switch msg.Type {
	case models.MsgNewRequest:
		handleNewRequest(*msg)
	case models.MsgQueueSync:
		handleQueueSync(*msg)
	case models.MsgQueueAdd:
		handleQueueAdd(*msg)
	case models.MsgQueueUpdate:
		handleQueueUpdate(*msg)
	case models.MsgQueueClear:
		handleQueueClear(*msg)
	case models.MsgDroneInfo:
		handleDroneInfo(*msg)
	case models.MsgRegister:
		handleDroneRegister(conn, *msg)
	case models.MsgMissionDone:
		// BAREMA: ACK! A Base avisa o Drone que recebeu a conclusão.
		network.WriteMessage(conn, models.Message{
			Type:      models.MsgStatusUpdate,
			SenderID:  cfg.baseID,
			LamportTS: clk.Tick(),
		})
		handleMissionDone(*msg)
	case models.MsgDroneStatusUpdate:
		handleDroneStatusUpdate(*msg)
	case models.MsgAlert:
		handleIncomingAlert(*msg)
	}
}

func handleNewRequest(msg models.Message) {
	var req models.Request
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return
	}
	added := pq.Push(req)
	if added {
		alertMgr.Info("Nova req %s [%s] adicionada à fila", req.ID, req.Type)
		go gossipQueueAdd(req)
	}
}

func handleQueueAdd(msg models.Message) {
	var req models.Request
	if err := json.Unmarshal(msg.Payload, &req); err == nil {
		pq.Push(req)
	}
}

func handleQueueSync(msg models.Message) {
	var snapshot []models.Request
	if err := json.Unmarshal(msg.Payload, &snapshot); err == nil {
		pq.SyncFrom(snapshot)
	}
}

func handleQueueUpdate(msg models.Message) {
	var req models.Request
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return
	}

	pq.Replace(req)

	if req.Status == models.StatusDone {
		pq.Remove(req.ID)
		state.releaseLock(req.ID)
	}
}

func handleQueueClear(msg models.Message) {
	pq.ClearAll()
	state.clearLocks()
	state.PurgeLostDrones() // <- CORREÇÃO: Limpa drones mortos permanentemente!
}

func handleDroneInfo(msg models.Message) {
	var d models.Drone
	if err := json.Unmarshal(msg.Payload, &d); err == nil {
		state.registerDrone(d)
	}
}

func handleIncomingAlert(msg models.Message) {
	var alert models.Alert
	if err := json.Unmarshal(msg.Payload, &alert); err == nil {
		alertMgr.Add(alert)
	}
}

// ── Servidor Gossip :8002 ────────────────────────────────────────────────────

func startGossipServer() {
	if err := network.StartTCPServer(cfg.gossipAddr, handleGossipConn); err != nil {
		alertMgr.Critical("Servidor gossip falhou: %v", err)
	}
}

func handleGossipConn(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	msg, err := network.ReadMessage(conn)
	if err != nil {
		return
	}
	clk.Update(msg.LamportTS)

	switch msg.Type {
	case models.MsgLockRequest:
		handleLockRequest(conn, *msg)
	case models.MsgLockRelease:
		handleLockRelease(*msg)
	}
}

func handleLockRequest(conn net.Conn, msg models.Message) {
	var payload models.LockRequestPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return
	}

	granted := state.tryLock(payload.RequestID, payload.BaseID)
	replyPayload, _ := json.Marshal(models.LockReplyPayload{
		RequestID: payload.RequestID,
		Granted:   granted,
		BaseID:    cfg.baseID,
	})

	network.WriteMessage(conn, models.Message{
		Type:      models.MsgLockReply,
		SenderID:  cfg.baseID,
		LamportTS: clk.Tick(),
		Payload:   replyPayload,
	})
}

func handleLockRelease(msg models.Message) {
	var payload models.LockRequestPayload
	if err := json.Unmarshal(msg.Payload, &payload); err == nil {
		state.releaseLock(payload.RequestID)
	}
}

// ── Servidor Dashboard :3001 ─────────────────────────────────────────────────

func startDashPush() {
	if err := network.StartTCPServer(cfg.dashAddr, handleDashConn); err != nil {
		alertMgr.Warn("Dashboard TCP falhou: %v", err)
	}
}

func handleDashConn(conn net.Conn) {
	defer conn.Close()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		update := buildStatusUpdate()
		data, err := json.Marshal(update)
		if err != nil {
			break
		}
		data = append(data, '\n')
		conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if _, err := conn.Write(data); err != nil {
			break
		}
	}
}

func buildStatusUpdate() models.StatusUpdatePayload {
	// Pega a lista de outras bases conhecidas (peers)
	todasAsBases := state.allPeers()

	// CORREÇÃO: Adiciona a própria base na lista para o Dashboard saber que ela existe!
	minhaBase := models.Base{
		ID:       cfg.baseID,
		SectorID: cfg.sectorID,
		Addr:     cfg.baseHost,
		Status:   models.BaseOnline,
	}
	todasAsBases = append(todasAsBases, minhaBase)

	return models.StatusUpdatePayload{
		SectorID:      cfg.sectorID,
		Bases:         todasAsBases,
		Drones:        state.allDrones(),
		Queue:         pq.GetAll(),
		Alerts:        alertMgr.GetLast(20),
		ActiveSensors: state.getActiveSensors(),
		ActiveBrokers: state.getActiveBrokers(),
	}
}

func gossipQueueAdd(req models.Request) {
	payload, _ := json.Marshal(req)
	msg := models.Message{
		Type:      models.MsgQueueAdd,
		SenderID:  cfg.baseID,
		LamportTS: clk.Tick(),
		Payload:   payload,
	}
	network.BroadcastMessage(cfg.peerBases, msg)
}

func gossipQueueUpdate(req models.Request) {
	payload, _ := json.Marshal(req)
	msg := models.Message{
		Type:      models.MsgQueueUpdate,
		SenderID:  cfg.baseID,
		LamportTS: clk.Tick(),
		Payload:   payload,
	}
	network.BroadcastMessage(cfg.peerBases, msg)
}

func sendFullQueueSync(peerAddr string) {
	snapshot := pq.GetAll()
	payload, _ := json.Marshal(snapshot)
	msg := models.Message{
		Type:      models.MsgQueueSync,
		SenderID:  cfg.baseID,
		LamportTS: clk.Tick(),
		Payload:   payload,
	}
	if err := network.SendMessage(peerAddr, msg); err == nil {
		fmt.Printf("[GOSSIP] Fila sincronizada com %s\n", peerAddr)
	}
}
