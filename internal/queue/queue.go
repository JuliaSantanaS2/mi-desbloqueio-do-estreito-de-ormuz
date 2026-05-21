// Package queue implementa uma fila de prioridade thread-safe para requisições de drones.
package queue

import (
	"container/heap"
	"sort"
	"sync"
	"time"

	"drone-system/internal/models"
)

type requestHeap []models.Request

func (h requestHeap) Len() int { return len(h) }

func (h requestHeap) Less(i, j int) bool {
	if h[i].Priority != h[j].Priority {
		return h[i].Priority < h[j].Priority
	}
	if h[i].LamportTS != h[j].LamportTS {
		return h[i].LamportTS < h[j].LamportTS
	}
	return h[i].ID < h[j].ID
}

func (h requestHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *requestHeap) Push(x interface{}) { *h = append(*h, x.(models.Request)) }
func (h *requestHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

type PriorityQueue struct {
	mu   sync.RWMutex
	heap requestHeap
	seen map[string]bool
}

func New() *PriorityQueue {
	h := make(requestHeap, 0)
	heap.Init(&h)
	return &PriorityQueue{
		heap: h,
		seen: make(map[string]bool),
	}
}

func (pq *PriorityQueue) Push(req models.Request) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if pq.seen[req.ID] {
		return false
	}
	pq.seen[req.ID] = true
	heap.Push(&pq.heap, req)
	return true
}

func (pq *PriorityQueue) Replace(req models.Request) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if !pq.seen[req.ID] {
		pq.seen[req.ID] = true
		heap.Push(&pq.heap, req)
		return
	}

	for i := range pq.heap {
		if pq.heap[i].ID == req.ID {
			pq.heap[i] = req
			heap.Init(&pq.heap)
			return
		}
	}
}

func (pq *PriorityQueue) Peek() (models.Request, bool) {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	var bestReq models.Request
	found := false

	for _, req := range pq.heap {
		if req.Status == models.StatusPending {
			if !found ||
				req.Priority < bestReq.Priority ||
				(req.Priority == bestReq.Priority && req.LamportTS < bestReq.LamportTS) ||
				(req.Priority == bestReq.Priority && req.LamportTS == bestReq.LamportTS && req.ID < bestReq.ID) {
				bestReq = req
				found = true
			}
		}
	}
	return bestReq, found
}

func (pq *PriorityQueue) UpdateStatus(reqID string, status models.RequestStatus, lockedBy, assignedTo string) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	for i := range pq.heap {
		if pq.heap[i].ID == reqID {
			pq.heap[i].Status = status
			if status == models.StatusPending && lockedBy == "" && assignedTo == "" {
				pq.heap[i].LockedBy = ""
				pq.heap[i].AssignedTo = ""
			} else {
				if lockedBy != "" {
					pq.heap[i].LockedBy = lockedBy
				}
				if assignedTo != "" {
					pq.heap[i].AssignedTo = assignedTo
				}
			}

			// O SEGREDO AQUI: Quando a missão é pega pelo drone, resetamos
			// o relógio dela para que ela caia exatamente no final da fila "Atendendo"!
			if status == models.StatusAssigned {
				pq.heap[i].WallClock = time.Now()
			}
			return true
		}
	}
	return false
}

func (pq *PriorityQueue) ClearAssignment(reqID string) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	for i := range pq.heap {
		if pq.heap[i].ID == reqID {
			pq.heap[i].LockedBy = ""
			pq.heap[i].AssignedTo = ""
			return
		}
	}
}

func (pq *PriorityQueue) Remove(reqID string) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	for i, req := range pq.heap {
		if req.ID == reqID {
			pq.heap = append(pq.heap[:i], pq.heap[i+1:]...)
			heap.Init(&pq.heap)

			// AQUI ESTÁ A CORREÇÃO DO FANTASMA!
			// NÃO apagamos mais o pq.seen[reqID].
			// A missão sai da fila, mas a base lembra que ela já foi concluída.
			// Nenhuma base atrasada vai conseguir injetar ela de volta!
			return true
		}
	}
	return false
}

func (pq *PriorityQueue) ClearAll() {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	pq.heap = make(requestHeap, 0)
	pq.seen = make(map[string]bool)
}

func (pq *PriorityQueue) GetAll() []models.Request {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	result := make([]models.Request, len(pq.heap))
	copy(result, pq.heap)

	// ORDENAÇÃO FIXA PARA O DASHBOARD
	sort.SliceStable(result, func(i, j int) bool {

		// REGRA 1: Se as duas requisições estão na fila de PENDENTES
		if result[i].Status == models.StatusPending && result[j].Status == models.StatusPending {

			// 1º REGRA DE OURO: Prioridade!
			// CRÍTICO = 1, NORMAL = 2. O Crítico sobe para o topo imediatamente!
			if result[i].Priority != result[j].Priority {
				return result[i].Priority < result[j].Priority
			}

			// 2º REGRA DE DESEMPATE: Se empatou a prioridade, quem foi gerado primeiro?
			// Usa o relógio lógico de Lamport para o desempate
			if result[i].LamportTS != result[j].LamportTS {
				return result[i].LamportTS < result[j].LamportTS
			}

			// 3º REGRA: Desempate alfabético por ID para garantir estabilidade visual
			return result[i].ID < result[j].ID
		}

		// REGRA 2: Se as duas requisições já estão sendo ATENDIDAS por drones (Assigned)
		if result[i].Status == models.StatusAssigned && result[j].Status == models.StatusAssigned {
			// A mais antiga (quem começou a ser atendida primeiro) fica no topo.
			// A nova missão que o drone acabou de pegar entra no final (embaixo).
			return result[i].WallClock.Before(result[j].WallClock)
		}

		// REGRA 3: Se as requisições têm status diferentes, agrupamos por status.
		// Evita qualquer bug visual no painel misturando as listas.
		return result[i].Status < result[j].Status
	})

	return result
}

func (pq *PriorityQueue) ClearDroneAssignments(droneID string) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	changed := false
	for i := range pq.heap {
		if pq.heap[i].Status == models.StatusAssigned && pq.heap[i].AssignedTo == droneID {
			pq.heap[i].Status = models.StatusPending
			pq.heap[i].AssignedTo = ""
			pq.heap[i].LockedBy = ""
			changed = true
		}
	}
	if changed {
		heap.Init(&pq.heap)
	}
}

func (pq *PriorityQueue) Len() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return len(pq.heap)
}

func (pq *PriorityQueue) PendingLen() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	count := 0
	for _, req := range pq.heap {
		if req.Status == models.StatusPending {
			count++
		}
	}
	return count
}

func (pq *PriorityQueue) CheckStarvation(maxWait time.Duration) []models.Request {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	var stale []models.Request
	cutoff := time.Now().Add(-maxWait)
	for _, req := range pq.heap {
		if req.Status == models.StatusPending && req.WallClock.Before(cutoff) {
			stale = append(stale, req)
		}
	}
	return stale
}

func (pq *PriorityQueue) Has(reqID string) bool {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return pq.seen[reqID]
}

func (pq *PriorityQueue) SyncFrom(snapshot []models.Request) (added int) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	for _, req := range snapshot {
		if !pq.seen[req.ID] {
			pq.seen[req.ID] = true
			heap.Push(&pq.heap, req)
			added++
		}
	}
	return added
}

// BAREMA: Blindagem máxima - Verifica na fila se o drone já está ocupado com algo
func (pq *PriorityQueue) IsDroneAssigned(droneID string) bool {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	for _, req := range pq.heap {
		if req.Status == models.StatusAssigned && req.AssignedTo == droneID {
			return true
		}
	}
	return false
}
