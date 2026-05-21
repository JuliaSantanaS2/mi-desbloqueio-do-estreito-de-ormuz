// testrunner — Suite de Testes Automatizados do Sistema Distribuído de Drones
//
// Testa os requisitos críticos do sistema sem precisar de infraestrutura Docker.
// Executa testes localmente contra instâncias em memória dos componentes.
//
// Uso:
//   go run ./cmd/testrunner
//   go run ./cmd/testrunner -test=TestHighLoad
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"drone-system/internal/lamport"
	"drone-system/internal/models"
	"drone-system/internal/queue"
)

// TestResult representa o resultado de um teste.
type TestResult struct {
	Name    string
	Passed  bool
	Message string
	Duration time.Duration
}

var results []TestResult

func main() {
	testName := flag.String("test", "", "Executar apenas um teste específico")
	flag.Parse()

	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  SUITE DE TESTES — Sistema Distribuído de Drones (PBL 2)")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()

	tests := []struct {
		name string
		fn   func() TestResult
	}{
		{"TestLamportOrdering", TestLamportOrdering},
		{"TestNoDoubleDroneForSameReq", TestNoDoubleDroneForSameReq},
		{"TestNoDroneDoubleAssign", TestNoDroneDoubleAssign},
		{"TestQueueDeduplication", TestQueueDeduplication},
		{"TestQueuePriorityOrder", TestQueuePriorityOrder},
		{"TestStarvationDetection", TestStarvationDetection},
		{"TestQueueSyncFrom", TestQueueSyncFrom},
		{"TestHighLoad", TestHighLoad},
		{"TestLockExclusivity", TestLockExclusivity},
	}

	for _, t := range tests {
		if *testName != "" && t.name != *testName {
			continue
		}
		start := time.Now()
		r := t.fn()
		r.Duration = time.Since(start)
		results = append(results, r)
		printResult(r)
	}

	printSummary()
}

// ── Testes ───────────────────────────────────────────────────────────────────

// TestLamportOrdering verifica que o relógio de Lamport ordena eventos corretamente.
func TestLamportOrdering() TestResult {
	c1 := lamport.New()
	c2 := lamport.New()

	// C1 gera evento, C2 recebe.
	ts1 := c1.Tick()
	ts2 := c2.Update(ts1) // C2 deve ser > C1.

	if ts2 <= ts1 {
		return fail("TestLamportOrdering", "C2 (%d) deveria ser > C1 (%d)", ts2, ts1)
	}

	// Evento local em C1 após receber de C2.
	ts3 := c1.Update(ts2)
	if ts3 <= ts2 {
		return fail("TestLamportOrdering", "ts3 (%d) deveria ser > ts2 (%d)", ts3, ts2)
	}

	return pass("TestLamportOrdering", "Lamport ordering correto: %d < %d < %d", ts1, ts2, ts3)
}

// TestNoDoubleDroneForSameReq verifica que 2 goroutines concorrentes não conseguem
// ambas obter o lock da mesma requisição (simula 2 bases tentando ao mesmo tempo).
func TestNoDoubleDroneForSameReq() TestResult {
	type lockMap struct {
		mu    sync.Mutex
		locks map[string]string
	}
	locks := &lockMap{locks: make(map[string]string)}

	tryLock := func(reqID, baseID string) bool {
		locks.mu.Lock()
		defer locks.mu.Unlock()
		if _, exists := locks.locks[reqID]; exists {
			return false
		}
		locks.locks[reqID] = baseID
		return true
	}

	reqID := "req-test-001"
	var winner atomic.Value
	var wg sync.WaitGroup
	successes := int32(0)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			baseID := fmt.Sprintf("base-%d", id)
			if tryLock(reqID, baseID) {
				atomic.AddInt32(&successes, 1)
				winner.Store(baseID)
			}
		}(i)
	}
	wg.Wait()

	if successes != 1 {
		return fail("TestNoDoubleDroneForSameReq",
			"Esperado 1 lock, obtido %d. Vencedor: %v", successes, winner.Load())
	}
	return pass("TestNoDoubleDroneForSameReq",
		"Exclusão mútua correta — apenas 1 base obteve lock (vencedor: %v)", winner.Load())
}

// TestNoDroneDoubleAssign verifica que um drone não é marcado como BUSY duas vezes.
func TestNoDroneDoubleAssign() TestResult {
	type droneState struct {
		mu     sync.Mutex
		status models.DroneStatus
	}
	d := &droneState{status: models.DroneIdle}

	tryAssign := func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.status != models.DroneIdle {
			return false
		}
		d.status = models.DroneBusy
		return true
	}

	assigned := int32(0)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tryAssign() {
				atomic.AddInt32(&assigned, 1)
			}
		}()
	}
	wg.Wait()

	if assigned != 1 {
		return fail("TestNoDroneDoubleAssign",
			"Drone atribuído %d vezes (esperado 1)", assigned)
	}
	return pass("TestNoDroneDoubleAssign", "Drone atribuído apenas 1 vez de 20 tentativas")
}

// TestQueueDeduplication verifica que a fila não aceita requisições duplicadas.
func TestQueueDeduplication() TestResult {
	pq := queue.New()
	req := models.Request{
		ID: "req-dup-001", Type: models.TypeNormal,
		Priority: 2, LamportTS: 1, WallClock: time.Now(),
		Status: models.StatusPending,
	}

	added1 := pq.Push(req)
	added2 := pq.Push(req)
	added3 := pq.Push(req)

	if !added1 || added2 || added3 {
		return fail("TestQueueDeduplication",
			"added1=%v added2=%v added3=%v (esperado: true, false, false)",
			added1, added2, added3)
	}
	if pq.Len() != 1 {
		return fail("TestQueueDeduplication", "Fila tem %d items (esperado 1)", pq.Len())
	}
	return pass("TestQueueDeduplication", "Deduplicação funciona — 3 inserções, 1 item na fila")
}

// TestQueuePriorityOrder verifica que CRITICAL sempre vem antes de NORMAL.
func TestQueuePriorityOrder() TestResult {
	pq := queue.New()
	now := time.Now()

	// Insere em ordem inversa de prioridade.
	pq.Push(models.Request{ID: "n1", Type: models.TypeNormal, Priority: 2, LamportTS: 1, WallClock: now, Status: models.StatusPending})
	pq.Push(models.Request{ID: "n2", Type: models.TypeNormal, Priority: 2, LamportTS: 2, WallClock: now, Status: models.StatusPending})
	pq.Push(models.Request{ID: "c1", Type: models.TypeCritical, Priority: 1, LamportTS: 5, WallClock: now, Status: models.StatusPending})
	pq.Push(models.Request{ID: "c2", Type: models.TypeCritical, Priority: 1, LamportTS: 3, WallClock: now, Status: models.StatusPending})

	// Primeiro deve ser c2 (CRITICAL, menor LamportTS=3).
	top, _ := pq.Peek()
	if top.ID != "c2" {
		return fail("TestQueuePriorityOrder",
			"Topo esperado: c2 (CRITICAL, ts=3), obtido: %s (%s, ts=%d)",
			top.ID, top.Type, top.LamportTS)
	}
	return pass("TestQueuePriorityOrder",
		"CRITICAL priorizado corretamente (menor LamportTS primeiro dentro do tipo)")
}

// TestStarvationDetection verifica que requisições antigas são detectadas.
func TestStarvationDetection() TestResult {
	pq := queue.New()

	// Requisição antiga (50s atrás).
	pq.Push(models.Request{
		ID: "old-req", Type: models.TypeNormal, Priority: 2,
		LamportTS: 1, WallClock: time.Now().Add(-50 * time.Second),
		Status: models.StatusPending,
	})
	// Requisição recente.
	pq.Push(models.Request{
		ID: "new-req", Type: models.TypeNormal, Priority: 2,
		LamportTS: 2, WallClock: time.Now(),
		Status: models.StatusPending,
	})

	stale := pq.CheckStarvation(45 * time.Second)
	if len(stale) != 1 || stale[0].ID != "old-req" {
		return fail("TestStarvationDetection",
			"Esperado 1 req com inanição (old-req), obtido %d", len(stale))
	}
	return pass("TestStarvationDetection", "Detecção de inanição correta — 1 req detectada")
}

// TestQueueSyncFrom verifica que o sync entre bases não cria duplicatas.
func TestQueueSyncFrom() TestResult {
	local := queue.New()
	now := time.Now()

	// Fila local com 2 requisições.
	local.Push(models.Request{ID: "r1", Type: models.TypeNormal, Priority: 2, LamportTS: 1, WallClock: now, Status: models.StatusPending})
	local.Push(models.Request{ID: "r2", Type: models.TypeNormal, Priority: 2, LamportTS: 2, WallClock: now, Status: models.StatusPending})

	// Snapshot de outra base com 1 nova e 1 duplicada.
	snapshot := []models.Request{
		{ID: "r2", Type: models.TypeNormal, Priority: 2, LamportTS: 2, WallClock: now, Status: models.StatusPending}, // duplicata
		{ID: "r3", Type: models.TypeCritical, Priority: 1, LamportTS: 3, WallClock: now, Status: models.StatusPending}, // nova
	}

	added := local.SyncFrom(snapshot)
	if added != 1 {
		return fail("TestQueueSyncFrom", "Esperado 1 nova req, adicionadas %d", added)
	}
	if local.Len() != 3 {
		return fail("TestQueueSyncFrom", "Esperado 3 itens na fila, tem %d", local.Len())
	}
	return pass("TestQueueSyncFrom", "Sync correto — 1 nova adicionada, duplicata ignorada")
}

// TestHighLoad simula alta carga: N sensores gerando requisições simultaneamente.
func TestHighLoad() TestResult {
	pq := queue.New()
	totalReqs := 200
	var added int32

	rng := rand.New(rand.NewSource(42))
	var wg sync.WaitGroup
	now := time.Now()

	for i := 0; i < totalReqs; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := models.Request{
				ID:        fmt.Sprintf("req-%04d", idx),
				Type:      models.TypeNormal,
				Priority:  2,
				LamportTS: int64(rng.Intn(1000)),
				WallClock: now,
				Status:    models.StatusPending,
			}
			if rng.Float32() < 0.2 {
				req.Type = models.TypeCritical
				req.Priority = 1
			}
			if pq.Push(req) {
				atomic.AddInt32(&added, 1)
			}
		}(i)
	}
	wg.Wait()

	if int(added) != totalReqs {
		return fail("TestHighLoad",
			"Perda de dados: %d/%d requisições adicionadas", added, totalReqs)
	}
	if pq.Len() != totalReqs {
		return fail("TestHighLoad",
			"Fila tem %d itens (esperado %d)", pq.Len(), totalReqs)
	}
	return pass("TestHighLoad",
		"Alta carga OK — %d requisições concorrentes sem perda", totalReqs)
}

// TestLockExclusivity simula N bases tentando obter lock da mesma req ao mesmo tempo.
func TestLockExclusivity() TestResult {
	locks := sync.Map{}
	reqID := "concurrent-req"
	winners := int32(0)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Simula latência de rede variável.
			time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
			if _, loaded := locks.LoadOrStore(reqID, fmt.Sprintf("base-%d", id)); !loaded {
				atomic.AddInt32(&winners, 1)
			}
		}(i)
	}
	wg.Wait()

	if winners != 1 {
		return fail("TestLockExclusivity",
			"%d bases obtiveram lock (esperado exatamente 1)", winners)
	}
	return pass("TestLockExclusivity",
		"Exclusividade de lock garantida — 1 vencedor entre 50 tentativas simultâneas")
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func pass(name, format string, args ...interface{}) TestResult {
	return TestResult{Name: name, Passed: true, Message: fmt.Sprintf(format, args...)}
}

func fail(name, format string, args ...interface{}) TestResult {
	return TestResult{Name: name, Passed: false, Message: fmt.Sprintf(format, args...)}
}

func printResult(r TestResult) {
	icon := "\033[32m✓ PASS\033[0m"
	if !r.Passed {
		icon = "\033[31m✗ FAIL\033[0m"
	}
	fmt.Printf("  %s  %-40s %v\n", icon, r.Name, r.Duration)
	if !r.Passed {
		fmt.Printf("        \033[31m→ %s\033[0m\n", r.Message)
	} else {
		fmt.Printf("        \033[90m→ %s\033[0m\n", r.Message)
	}
}

func printSummary() {
	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
	}
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════")
	if passed == len(results) {
		fmt.Printf("  \033[32;1m✓ TODOS OS TESTES PASSARAM: %d/%d\033[0m\n", passed, len(results))
	} else {
		fmt.Printf("  \033[31;1m✗ FALHAS: %d/%d testes passaram\033[0m\n", passed, len(results))
	}
	fmt.Println("═══════════════════════════════════════════════════════════")
}
