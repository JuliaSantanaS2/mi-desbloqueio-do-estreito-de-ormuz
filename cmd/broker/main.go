// Broker — Setor de Monitoramento Marítimo
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"drone-system/internal/alerts"
	"drone-system/internal/lamport"
	"drone-system/internal/models"
	"drone-system/internal/network"
)

var (
	sectorID   string
	brokerID   string
	peerBases  []string
	hbTargets  []string // NOVO: Destinos de heartbeat
	clock      = lamport.New()
	alertMgr   *alerts.Manager
	reqCounter int64 
)

func main() {
	sectorID = getEnv("SECTOR_ID", "A")
	brokerAddr := getEnv("BROKER_ADDR", "0.0.0.0:7001")
	basesEnv := getEnv("PEER_BASES", "")
	baseAddr := getEnv("BASE_ADDR", "127.0.0.1:8101") 
	hbEnv := getEnv("HB_TARGETS", "") // Puxa os endereços de UDP

	brokerID = fmt.Sprintf("broker-%s", sectorID)
	alertMgr = alerts.New(brokerID)

	if basesEnv != "" {
		peerBases = strings.Split(basesEnv, ",")
	}
	if baseAddr != "" {
		peerBases = append(peerBases, baseAddr)
	}
	if hbEnv != "" {
		hbTargets = strings.Split(hbEnv, ",")
	}

	alertMgr.Info("Broker do Setor %s iniciando em %s", sectorID, brokerAddr)

	// NOVO: Inicia a emissão de batimentos cardíacos UDP para as bases
	go runHeartbeat()

	if err := network.StartTCPServer(brokerAddr, handleSensorConn); err != nil {
		alertMgr.Critical("Falha ao iniciar servidor TCP: %v", err)
		os.Exit(1)
	}
}

func handleSensorConn(conn net.Conn) {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	msg, err := network.ReadMessage(conn)
	if err != nil {
		return 
	}

	if msg.Type != models.MsgSensorEvent {
		return
	}

	ts := clock.Update(msg.LamportTS)

	var req models.Request
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		alertMgr.Warn("Payload inválido do sensor %s: %v", msg.SenderID, err)
		return
	}

	if req.Type == "TELEMETRY" {
		network.WriteMessage(conn, models.Message{Type: models.MsgNewRequest, SenderID: brokerID})
		return 
	}

	reqID := fmt.Sprintf("req-%s-%d-%d",
		req.SectorID, 
		time.Now().UnixNano(),
		atomic.AddInt64(&reqCounter, 1),
	)
	req.ID = reqID
	req.LamportTS = ts
	req.WallClock = time.Now()
	req.Status = models.StatusPending
	req.Priority = req.Type.Priority()

	alertMgr.Info("Req recebida do sensor %s: [%s] %s (Lamport=%d)",
		req.SensorID, req.Type, req.Description, req.LamportTS)

	network.WriteMessage(conn, models.Message{
		Type:     models.MsgNewRequest,
		SenderID: brokerID,
	})

	propagateRequest(req)
}

func propagateRequest(req models.Request) {
	payloadBytes, err := json.Marshal(req)
	if err != nil {
		alertMgr.Critical("Erro ao serializar requisição %s: %v", req.ID, err)
		return
	}

	msg := models.Message{
		Type:      models.MsgNewRequest,
		SenderID:  brokerID,
		LamportTS: clock.Tick(),
		Payload:   payloadBytes,
	}

	if len(peerBases) == 0 {
		alertMgr.Warn("Nenhuma base configurada — requisição %s perdida!", req.ID)
		return
	}

	failed := network.BroadcastMessage(peerBases, msg)

	if len(failed) > 0 {
		alertMgr.Warn("Falha ao propagar para %d bases: %v", len(failed), failed)
	}

	success := len(peerBases) - len(failed)
	if success > 0 {
		alertMgr.Info("Req %s propagada para %d/%d bases", req.ID, success, len(peerBases))
	} else {
		alertMgr.Critical("Req %s NÃO chegou a nenhuma base! Sistema sem drones disponíveis?", req.ID)
	}
}

// NOVO: Função para manter as bases cientes de que este Broker está vivo
func runHeartbeat() {
	payload := models.HeartbeatPayload{
		NodeType: "broker",
		NodeID:   brokerID,
		SectorID: sectorID,
	}
	for {
		for _, target := range hbTargets {
			network.SendHeartbeat(target, payload, brokerID, clock.Tick())
		}
		time.Sleep(network.HeartbeatInterval)
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}