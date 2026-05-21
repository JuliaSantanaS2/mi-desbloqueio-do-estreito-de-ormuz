// Base — Núcleo do Sistema Distribuído de Drones
package main

import (
	"net"
	"os"
	"strings"
	"time"

	"drone-system/internal/alerts"
	"drone-system/internal/lamport"
	"drone-system/internal/queue"
)

var (
	cfg = struct {
		sectorID     string
		baseID       string
		baseAddr     string   
		baseHost     string   
		gossipAddr   string   
		gossipHost   string   
		hbAddr       string   
		dashAddr     string   
		peerBases    []string 
		peerGossip   []string 
		peerHB       []string 
		baseX, baseY float64
	}{}

	clk      = lamport.New()
	alertMgr *alerts.Manager
	pq       = queue.New()
	state    *BaseState
)

func main() {
	loadConfig()
	alertMgr = alerts.New(cfg.baseID)
	state = newBaseState()

	alertMgr.Info("Base %s (Setor %s) iniciando", cfg.baseID, cfg.sectorID)
	alertMgr.Info("Peers: %v", cfg.peerBases)

	go startMainServer()      
	go startGossipServer()    
	go startHBListener()      
	go startHBBroadcast()     
	go startDashPush()        
	go startDispatcher()      
	go startHealthChecker()   
	go startStarvationCheck() 
	
	// SOLUÇÃO DOS DRONES "DOIDOS": 
	// O AutoBalancer foi removido. Os drones pararam de jogar ping-pong!
	// A redistribuição agora só ocorre em caso de FALHA de uma base (no heartbeat.go)

	alertMgr.OK("Base %s pronta para operação", cfg.baseID)
	select {}
}

func loadConfig() {
	cfg.sectorID = getEnv("SECTOR_ID", "A")
	cfg.baseID = "base-" + cfg.sectorID
	cfg.baseAddr = getEnv("BASE_ADDR", "0.0.0.0:8001")
	cfg.baseHost = getEnv("BASE_HOST", defaultHost(cfg.baseAddr))
	cfg.gossipAddr = getEnv("GOSSIP_ADDR", "0.0.0.0:8002")
	cfg.gossipHost = getEnv("GOSSIP_HOST", defaultHost(cfg.gossipAddr))
	cfg.hbAddr = getEnv("HB_ADDR", "0.0.0.0:8500")
	cfg.dashAddr = getEnv("DASH_ADDR", "0.0.0.0:3001")

	if v := getEnv("PEER_BASES", ""); v != "" {
		cfg.peerBases = strings.Split(v, ",")
	}
	if v := getEnv("PEER_GOSSIP", ""); v != "" {
		cfg.peerGossip = strings.Split(v, ",")
	}
	if v := getEnv("PEER_HB", ""); v != "" {
		cfg.peerHB = strings.Split(v, ",")
	}
}

func startHBBroadcast() {
	for {
		broadcastMyHeartbeat()
		time.Sleep(3 * time.Second)
	}
}

func startHealthChecker() {
	for {
		time.Sleep(5 * time.Second)
		checkPeersHealth()
	}
}

func startStarvationCheck() {
	for {
		time.Sleep(15 * time.Second)
		stale := pq.CheckStarvation(45 * time.Second)
		for _, req := range stale {
			alertMgr.Warn("Req %s aguardando há mais de 45s (inanição) [%s]", req.ID, req.Type)
		}
	}
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func defaultHost(bindAddr string) string {
	host, port, err := net.SplitHostPort(bindAddr)
	if err != nil {
		return bindAddr
	}
	if host == "" || host == "0.0.0.0" || host == "127.0.0.1" || host == "localhost" {
		return net.JoinHostPort("localhost", port)
	}
	return net.JoinHostPort(host, port)
}