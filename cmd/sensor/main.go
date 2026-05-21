package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"drone-system/internal/alerts"
	"drone-system/internal/lamport"
	"drone-system/internal/models"
	"drone-system/internal/network"
)

var (
	sectorID      string
	sensorID      string
	primaryBroker string
	peerBrokers   []string
	hbTargets     []string
	activeBroker  string
	brokerFailed  int32
	interval      time.Duration
	clk           = lamport.New()
	alertMgr      *alerts.Manager
	rng           = rand.New(rand.NewSource(time.Now().UnixNano()))
	eventCount    int64
)

func main() {
	sectorID = getEnv("SECTOR_ID", "A")
	sensorID = getEnv("SENSOR_ID", fmt.Sprintf("sensor-%s1", sectorID))
	primaryBroker = getEnv("BROKER_ADDR", "localhost:7001")
	activeBroker = primaryBroker

	intervalMsStr := getEnv("INTERVAL_MS", "500") // Manda dados a cada 500ms
	intervalMs, _ := strconv.Atoi(intervalMsStr)
	interval = time.Duration(intervalMs) * time.Millisecond

	if v := getEnv("PEER_BROKERS", ""); v != "" {
		peerBrokers = strings.Split(v, ",")
	}
	if v := getEnv("HB_TARGETS", ""); v != "" {
		hbTargets = strings.Split(v, ",")
	}

	alertMgr = alerts.New(sensorID)
	alertMgr.Info("=== Sensor %s (Setor %s) INICIADO ===", sensorID, sectorID)

	go watchPrimaryBroker()
	go runHeartbeat()

	sendEventsLoop()
}

func sendEventsLoop() {
	n := int64(0)
	for {
		n++
		sendEvent(n)
		time.Sleep(interval)
	}
}

func sendEvent(n int64) {
	req := generateEvent(n)
	count := atomic.AddInt64(&eventCount, 1)

	payloadBytes, _ := json.Marshal(req)
	msg := models.Message{
		Type:      models.MsgSensorEvent,
		SenderID:  sensorID,
		LamportTS: clk.Tick(),
		Payload:   payloadBytes,
	}

	if _, err := network.SendMessageWithReply(activeBroker, msg); err == nil {
		if req.Type != "TELEMETRY" {
			alertMgr.Info("[Evento #%d] Enviado e Confirmado → %s | [%s]", count, activeBroker, req.Type)
		}
		return
	}

	if tryFallbackBroker(msg, req, count) {
		return
	}
}

func tryFallbackBroker(msg models.Message, req models.Request, count int64) bool {
	for _, addr := range peerBrokers {
		if addr == activeBroker {
			continue
		}
		if _, err := network.SendMessageWithReply(addr, msg); err == nil {
			activeBroker = addr
			atomic.StoreInt32(&brokerFailed, 1)
			return true
		}
	}
	return false
}

func watchPrimaryBroker() {
	for {
		time.Sleep(10 * time.Second)
		if atomic.LoadInt32(&brokerFailed) == 0 {
			continue
		}
		if network.IsAlive(primaryBroker) {
			activeBroker = primaryBroker
			atomic.StoreInt32(&brokerFailed, 0)
		}
	}
}

func generateEvent(n int64) models.Request {
	reqType := models.RequestType("TELEMETRY") // 98.5% do tempo é apenas telemetria
	desc := "Dados contínuos descartados"

	roll := rng.Float32()

	// Como roda a cada 500ms, queremos que os eventos sejam raros.
	if roll > 0.900 { // 0.5% de chance de ser Crítico
		reqType = models.TypeCritical
		desc = "CRITICO - SENSOR " + sensorID + " AREA " + sectorID
	} else if roll > 0.100 { // 1.0% de chance de ser Normal
		reqType = models.TypeNormal
		desc = "NORMAL - SENSOR " + sensorID + " AREA " + sectorID
	}

	return models.Request{
		ID:          fmt.Sprintf("req-%s-%d", sensorID, n),
		SectorID:    sectorID,
		SensorID:    sensorID,
		Type:        reqType,
		Priority:    reqType.Priority(),
		Description: desc,
		Status:      models.StatusPending,
	}
}

func runHeartbeat() {
	payload := models.HeartbeatPayload{
		NodeType: "sensor",
		NodeID:   sensorID,
		SectorID: sectorID,
	}
	for {
		for _, target := range hbTargets {
			network.SendHeartbeat(target, payload, sensorID, clk.Tick())
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
