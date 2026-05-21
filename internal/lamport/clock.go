// Package lamport implementa o Relógio Lógico de Lamport para ordenação causal
// de eventos em sistemas distribuídos, sem necessidade de sincronização de relógio físico.
//
// Regras do relógio de Lamport:
//   - Antes de enviar uma mensagem: incrementa o relógio local e usa o valor como timestamp.
//   - Ao receber uma mensagem com timestamp T: novo_valor = max(local, T) + 1.
//   - Eventos locais: apenas incrementa.
package lamport

import "sync"

// Clock é um relógio lógico de Lamport thread-safe.
type Clock struct {
	mu    sync.Mutex
	value int64
}

// New cria um novo relógio de Lamport iniciado em 0.
func New() *Clock {
	return &Clock{value: 0}
}

// Tick incrementa o relógio para um evento local ou antes de enviar uma mensagem.
// Retorna o novo valor a ser usado como timestamp.
func (c *Clock) Tick() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value++
	return c.value
}

// Update atualiza o relógio ao receber uma mensagem com timestamp externo.
// Aplica a regra de Lamport: max(local, received) + 1.
// Retorna o novo valor do relógio.
func (c *Clock) Update(received int64) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if received > c.value {
		c.value = received
	}
	c.value++
	return c.value
}

// Get retorna o valor atual do relógio sem modificá-lo.
func (c *Clock) Get() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

// Set força um valor mínimo no relógio (útil ao restaurar estado).
func (c *Clock) Set(v int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v > c.value {
		c.value = v
	}
}
