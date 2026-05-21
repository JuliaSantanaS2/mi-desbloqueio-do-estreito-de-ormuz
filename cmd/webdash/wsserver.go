// wsserver.go — Hub WebSocket: distribui o estado global a todos os browsers conectados.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 64 * 1024,
	CheckOrigin:     func(r *http.Request) bool { return true }, // Permite acesso de qualquer origem local
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
}

func newHub() *Hub {
	return &Hub{clients: make(map[*websocket.Conn]struct{})}
}

func (h *Hub) add(c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
}

func (h *Hub) remove(c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
	c.Close()
}

func (h *Hub) broadcast(data []byte) {
	h.mu.RLock()
	conns := make([]*websocket.Conn, 0, len(h.clients))
	for c := range h.clients {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, c := range conns {
		c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if err := c.WriteMessage(websocket.TextMessage, data); err != nil {
			h.remove(c)
		}
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Erro no upgrade do WebSocket: %v", err)
		return
	}
	h.add(conn)
	log.Printf("[WS] Navegador conectado! IP: %s (Total: %d)", conn.RemoteAddr(), h.count())

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
	h.remove(conn)
	log.Printf("[WS] Navegador desconectado: %s", conn.RemoteAddr())
}

func (h *Hub) count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func startBroadcastLoop(agg *Aggregator, hub *Hub) {
	ticker := time.NewTicker(1 * time.Second)
	go func() {
		for range ticker.C {
			if hub.count() == 0 {
				continue // Pula serialização se ninguém estiver assistindo a página
			}
			snapshot := agg.GlobalSnapshot()
			data, err := json.Marshal(snapshot)
			if err != nil {
				log.Printf("[WS] Erro ao converter estado para JSON: %v", err)
				continue
			}
			hub.broadcast(data)
		}
	}()
}
