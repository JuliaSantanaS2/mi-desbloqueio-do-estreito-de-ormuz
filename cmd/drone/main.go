package main

import (
	"encoding/json"
	"math"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"drone-system/internal/alerts"
	"drone-system/internal/lamport"
	"drone-system/internal/models"
	"drone-system/internal/network"
)

var (
	droneID    string
	sectorID   string
	droneAddr  string
	droneHost  string
	baseAddr   string
	baseID     string
	peerBases  []string
	hbTargets  []string
	posX, posY float64

	clk      = lamport.New()
	alertMgr *alerts.Manager
	rng      = rand.New(rand.NewSource(time.Now().UnixNano()))

	droneBusy bool
	droneMu   sync.Mutex
)

func main() {
	droneID = getEnv("DRONE_ID", "drone-A1")
	sectorID = getEnv("SECTOR_ID", "A")
	droneAddr = getEnv("DRONE_ADDR", "0.0.0.0:9001")
	host, port, _ := net.SplitHostPort(droneAddr)
	defaultDroneHost := droneAddr
	if host == "" || host == "0.0.0.0" || host == "127.0.0.1" || host == "localhost" {
		defaultDroneHost = net.JoinHostPort("localhost", port)
	}
	droneHost = getEnv("DRONE_HOST", defaultDroneHost)
	baseAddr = getEnv("BASE_ADDR", "localhost:8001")
	baseID = "base-" + sectorID

	if v := getEnv("PEER_BASES", ""); v != "" {
		peerBases = strings.Split(v, ",")
	}
	if v := getEnv("HB_TARGETS", ""); v != "" {
		hbTargets = strings.Split(v, ",")
	}

	alertMgr = alerts.New(droneID)

	if err := registerWithBase(baseAddr); err != nil {
		alertMgr.Critical("Falha ao registrar na base home %s", baseAddr)
		tryFallbackBases()
	}

	go runHeartbeat()
	go watchBaseConnection()

	alertMgr.Info("Drone %s ouvindo comandos em %s", droneID, droneAddr)
	network.StartTCPServer(droneAddr, handleBaseCommand)
}

func watchBaseConnection() {
	fails := 0
	for {
		time.Sleep(3 * time.Second)
		if baseAddr == "" {
			continue
		}

		// Trava de segurança: Se o drone estiver ocupado, ele JAMAIS abandona a base.
		droneMu.Lock()
		isBusy := droneBusy
		droneMu.Unlock()

		if isBusy {
			fails = 0
			continue
		}

		// Tenta pingar a base via TCP
		conn, err := net.DialTimeout("tcp", baseAddr, 2*time.Second)
		if err != nil {
			fails++
			// Só declara a base como morta se falhar 3 vezes seguidas (15 segundos)
			if fails >= 3 {
				if tryFallbackBases() {
					alertMgr.OK("Drone redistribuído para %s", baseAddr)
					fails = 0
				}
			}
		} else {
			fails = 0
			conn.Close()
		}
	}
}

func registerWithBase(addr string) error {
	payload, _ := json.Marshal(models.RegisterPayload{DroneID: droneID, Addr: droneHost, X: posX, Y: posY})
	msg := models.Message{Type: models.MsgRegister, SenderID: droneID, LamportTS: clk.Tick(), Payload: payload}

	// CORREÇÃO: O Drone agora ESPERA a resposta da base
	reply, err := network.SendMessageWithReply(addr, msg)
	if err != nil {
		return err
	}

	baseAddr = addr
	// O SEGREDO: O drone lê a resposta e adota o ID da base que o aceitou!
	baseID = reply.SenderID

	return nil
}
func tryFallbackBases() bool {
	for _, addr := range peerBases {
		if addr == baseAddr {
			continue
		}
		if err := registerWithBase(addr); err == nil {
			return true
		}
	}
	return false
}

func handleBaseCommand(conn net.Conn) {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	msg, err := network.ReadMessage(conn)
	if err != nil {
		return
	}
	clk.Update(msg.LamportTS)

	switch msg.Type {
	case models.MsgDispatch:
		droneMu.Lock()
		if droneBusy {
			droneMu.Unlock()
			return
		}
		droneBusy = true
		droneMu.Unlock()

		network.WriteMessage(conn, models.Message{Type: models.MsgStatusUpdate, SenderID: droneID, LamportTS: clk.Tick()})
		go handleDispatch(*msg)

	case models.MsgReassign:
		go handleReassign(*msg)
	}
}

func sendStatusUpdate(desc string) {
	if baseAddr == "" {
		return
	}
	payload, _ := json.Marshal(models.DroneStatusPayload{DroneID: droneID, MissionDesc: desc})
	msg := models.Message{Type: models.MsgDroneStatusUpdate, SenderID: droneID, LamportTS: clk.Tick(), Payload: payload}
	network.SendMessage(baseAddr, msg)
}

func handleDispatch(msg models.Message) {
	var dp models.DispatchPayload
	json.Unmarshal(msg.Payload, &dp)

	alertMgr.Info("Missão iniciada: req %s", dp.RequestID)
	// ENVIANDO O TEXTO CORRETO PARA A BASE
	sendStatusUpdate("Atendendo...")

	dist := math.Sqrt(math.Pow(dp.TargetX-posX, 2) + math.Pow(dp.TargetY-posY, 2))
	travelTime := time.Duration(dist*800) * time.Millisecond
	if travelTime < 10*time.Second {
		travelTime = 10 * time.Second
	}
	if travelTime > 20*time.Second {
		travelTime = 20 * time.Second
	}

	time.Sleep(travelTime)
	posX, posY = dp.TargetX, dp.TargetY
	sendStatusUpdate("Retornando para a base")
	time.Sleep(travelTime)

	reportMissionDone(dp.RequestID)

	droneMu.Lock()
	droneBusy = false
	droneMu.Unlock()
}

func reportMissionDone(reqID string) {
	payload, _ := json.Marshal(models.MissionDonePayload{DroneID: droneID, RequestID: reqID, Success: true})
	msg := models.Message{Type: models.MsgMissionDone, SenderID: droneID, LamportTS: clk.Tick(), Payload: payload}

	for i := 0; i < 3; i++ {
		if baseAddr != "" {
			_, err := network.SendMessageWithReply(baseAddr, msg)
			if err == nil {
				return // Sucesso!
			}
		}
		time.Sleep(1 * time.Second)
	}

	// Se falhou 3 vezes, a base morreu bem na hora que o drone acabou a missão!
	// O drone salva o sistema conectando-se a uma nova base e entregando
	// o aviso de conclusão para ela.
	if tryFallbackBases() {
		network.SendMessageWithReply(baseAddr, msg)
	}
}
func handleReassign(msg models.Message) {
	var rp models.ReassignPayload
	json.Unmarshal(msg.Payload, &rp)
	baseAddr = rp.NewBaseAddr
	baseID = rp.NewBaseID
	registerWithBase(baseAddr)
}

func runHeartbeat() {
	payload := models.HeartbeatPayload{NodeType: "drone", NodeID: droneID, BaseID: baseID, Addr: droneHost}
	for {
		payload.BaseID = baseID
		droneMu.Lock()
		if droneBusy {
			payload.Status = string(models.DroneBusy)
		} else {
			payload.Status = string(models.DroneIdle)
		}
		droneMu.Unlock()

		for _, target := range hbTargets {
			// A MÁGICA AQUI: O "go" cria uma thread. Se a base alvo estiver desligada,
			// a rede trava essa thread, mas o drone continua mandando pros vivos imediatamente!
			go network.SendHeartbeat(target, payload, droneID, clk.Tick())
		}
		time.Sleep(network.HeartbeatInterval)
	}
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
