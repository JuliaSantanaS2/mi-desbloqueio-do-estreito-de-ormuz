// main.go — Web Dashboard para o Sistema de Drones
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"drone-system/internal/models"
	"drone-system/internal/network"
)

func main() {
	httpAddr := getEnv("HTTP_ADDR", ":8180") // Atualizado para a porta que você está usando (8180)
	baseAddrsRaw := getEnv("BASE_ADDRS",
		"A=localhost:3001,B=localhost:3011,C=localhost:3021,D=localhost:3031")
	baseTCPAddrsRaw := getEnv("BASE_TCP_ADDRS",
		"A=localhost:8001,B=localhost:8011,C=localhost:8021,D=localhost:8031")

	type sectorDash struct{ id, addr string }
	var sectors []sectorDash
	for _, part := range strings.Split(baseAddrsRaw, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 {
			sectors = append(sectors, sectorDash{id: kv[0], addr: kv[1]})
		}
	}

	tcpSectors := make([]sectorDash, 0, len(sectors))
	ids := make([]string, 0, len(sectors))
	for _, s := range sectors {
		ids = append(ids, s.id)
	}
	for _, part := range strings.Split(baseTCPAddrsRaw, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 {
			tcpSectors = append(tcpSectors, sectorDash{id: kv[0], addr: kv[1]})
		}
	}

	agg := newAggregator(ids)
	hub := newHub()

	for _, s := range sectors {
		go connectSector(agg, s.id, s.addr)
	}
	baseTCPAddrs := tcpSectors

	go startBroadcastLoop(agg, hub)

	mux := http.NewServeMux()

	// Arquivos estáticos (app.js e css) via embed.FS
	staticRoot, _ := fs.Sub(staticFS, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticRoot))))

	// Página principal (index.html)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, err := staticFS.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "Arquivo não encontrado", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	// Rota para o WebSocket
	mux.HandleFunc("/ws", hub.ServeWS)

	// API REST fallback e ações de controle
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		snap := agg.GlobalSnapshot()
		if err := json.NewEncoder(w).Encode(snap); err != nil {
			http.Error(w, err.Error(), 500)
		}
	})

	mux.HandleFunc("/api/clear-requests", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		ok := false
		for _, s := range baseTCPAddrs {
			msg := models.Message{Type: models.MsgQueueClear, SenderID: "webdash"}
			if err := network.SendMessage(s.addr, msg); err == nil {
				ok = true
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]bool{"ok": false})
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	fmt.Printf("\n==============================================\n")
	fmt.Printf("  DRONE SYSTEM - Web Dashboard\n")
	fmt.Printf("  Acesse no navegador: http://127.0.0.1%s\n", httpAddr)
	fmt.Printf("==============================================\n\n")

	log.Printf("[HTTP] Servidor escutando na porta %s", httpAddr)
	if err := http.ListenAndServe(httpAddr, mux); err != nil {
		log.Fatalf("[HTTP] Erro fatal: %v", err)
	}
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
