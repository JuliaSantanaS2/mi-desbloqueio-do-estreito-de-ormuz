// Package network fornece helpers para comunicação TCP no sistema distribuído.
// Todas as mensagens são serializadas como JSON com um newline (\n) como delimitador.
package network

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"drone-system/internal/models"
)

const (
	// DialTimeout é o tempo máximo para estabelecer uma conexão TCP.
	DialTimeout = 3 * time.Second
	// WriteTimeout é o tempo máximo para enviar uma mensagem.
	WriteTimeout = 5 * time.Second
	// ReadTimeout é o tempo máximo para ler uma mensagem em conexões persistentes.
	ReadTimeout = 30 * time.Second
)

// SendMessage serializa e envia uma mensagem TCP para um endereço remoto.
// A conexão é aberta e fechada a cada chamada (stateless send).
// Retorna erro se a conexão ou envio falhar.
func SendMessage(addr string, msg models.Message) error {
	conn, err := net.DialTimeout("tcp", addr, DialTimeout)
	if err != nil {
		return fmt.Errorf("tcp connect to %s: %w", addr, err)
	}
	defer conn.Close()

	// Define timeout de escrita para evitar bloqueio indefinido.
	conn.SetWriteDeadline(time.Now().Add(WriteTimeout))

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	// Envia com newline como delimitador de mensagem.
	data = append(data, '\n')
	_, err = conn.Write(data)
	return err
}

// SendMessageWithReply envia uma mensagem TCP e aguarda uma resposta.
// Usado no protocolo DISPATCH_LOCK onde cada base responde ao pedido.
func SendMessageWithReply(addr string, msg models.Message) (*models.Message, error) {
	conn, err := net.DialTimeout("tcp", addr, DialTimeout)
	if err != nil {
		return nil, fmt.Errorf("tcp connect to %s: %w", addr, err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(WriteTimeout))

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}
	data = append(data, '\n')
	if _, err = conn.Write(data); err != nil {
		return nil, fmt.Errorf("tcp write: %w", err)
	}

	// Aguarda resposta com timeout estendido.
	conn.SetDeadline(time.Now().Add(WriteTimeout))
	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		var reply models.Message
		if err := json.Unmarshal(scanner.Bytes(), &reply); err != nil {
			return nil, fmt.Errorf("json unmarshal reply: %w", err)
		}
		return &reply, nil
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("tcp read reply: %w", err)
	}
	return nil, fmt.Errorf("conexao fechada sem resposta de %s", addr)
}

// StartTCPServer inicia um servidor TCP na porta especificada.
// Para cada conexão aceita, chama handler em uma goroutine separada.
// Bloqueante — deve ser chamado em goroutine.
func StartTCPServer(addr string, handler func(conn net.Conn)) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	fmt.Printf("[TCP] Ouvindo em %s\n", addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			// Continua tentando aceitar mesmo com erros pontuais.
			fmt.Printf("[TCP] Erro ao aceitar conexao: %v\n", err)
			continue
		}
		go handler(conn)
	}
}

// ReadMessage lê uma única mensagem JSON de uma conexão TCP (delimitada por \n).
func ReadMessage(conn net.Conn) (*models.Message, error) {
	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		var msg models.Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			return nil, fmt.Errorf("json unmarshal: %w", err)
		}
		return &msg, nil
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("conexao fechada")
}

// WriteMessage serializa e escreve uma mensagem em uma conexão TCP existente.
func WriteMessage(conn net.Conn, msg models.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
	_, err = conn.Write(data)
	return err
}

// BroadcastMessage envia uma mensagem para múltiplos endereços em paralelo.
// Retorna a lista de endereços que falharam.
func BroadcastMessage(addrs []string, msg models.Message) []string {
	type result struct {
		addr string
		err  error
	}
	ch := make(chan result, len(addrs))

	for _, addr := range addrs {
		go func(a string) {
			err := SendMessage(a, msg)
			ch <- result{addr: a, err: err}
		}(addr)
	}

	var failed []string
	for range addrs {
		r := <-ch
		if r.err != nil {
			failed = append(failed, r.addr)
		}
	}
	return failed
}

// IsAlive verifica se um endereço TCP está respondendo (health check simples).
func IsAlive(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, DialTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
