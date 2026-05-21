// Package network — UDP helpers para heartbeat broadcast no sistema de drones.
// UDP é usado para heartbeats (fire-and-forget) porque:
//   - É leve e não bloqueia em caso de falha
//   - A ausência de pacotes sinaliza a queda de um nó
//   - Não precisa de garantia de entrega (periodicidade compensa perdas)
package network

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"drone-system/internal/models"
)

const (
	// UDPBufferSize é o tamanho do buffer de leitura UDP.
	UDPBufferSize = 4096
	// HeartbeatInterval é a frequência de envio de heartbeats.
	HeartbeatInterval = 1 * time.Second
	// HeartbeatTimeout é o tempo sem heartbeat para considerar um nó morto.
	HeartbeatTimeout = 3 * time.Second
)

// SendHeartbeat envia um heartbeat UDP para um endereço específico.
// Fire-and-forget: erros são ignorados pois a periodicidade compensa perdas.
func SendHeartbeat(addr string, payload models.HeartbeatPayload, senderID string, lamportTS int64) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return
	}
	msg := models.Message{
		Type:      models.MsgHeartbeat,
		SenderID:  senderID,
		LamportTS: lamportTS,
		Payload:   payloadBytes,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	// Resolução e envio UDP — ignora erros propositalmente.
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
	conn.Write(data)
}

// BroadcastHeartbeat envia heartbeat UDP para múltiplos endereços em paralelo.
func BroadcastHeartbeat(addrs []string, payload models.HeartbeatPayload, senderID string, lamportTS int64) {
	for _, addr := range addrs {
		go SendHeartbeat(addr, payload, senderID, lamportTS)
	}
}

// StartUDPListener inicia um listener UDP e chama handler para cada mensagem recebida.
// Bloqueante — deve ser chamado em goroutine.
func StartUDPListener(addr string, handler func(msg models.Message, from net.Addr)) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("resolve udp addr %s: %w", addr, err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("listen udp %s: %w", addr, err)
	}
	defer conn.Close()

	fmt.Printf("[UDP] Ouvindo heartbeats em %s\n", addr)
	buf := make([]byte, UDPBufferSize)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue // Ignora erros pontuais de leitura UDP.
		}
		var msg models.Message
		if err := json.Unmarshal(buf[:n], &msg); err != nil {
			continue // Ignora mensagens malformadas.
		}
		go handler(msg, from)
	}
}
