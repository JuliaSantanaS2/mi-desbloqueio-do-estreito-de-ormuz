// Dashboard — Monitor Visual TUI (Terminal User Interface)
//
// Conecta-se à base local via TCP e recebe atualizações de estado a cada 1s.
// Renderiza um painel completo no terminal usando ANSI escape codes.
// Sem HTTP, sem dependências externas — apenas TCP e Go padrão.
//
// Variáveis de ambiente:
//
//	BASE_DASH_ADDR  Endereço TCP da base para o push de estado (padrão modificado para localhost:3101)
//	SECTOR_ID       Setor local (para exibição)
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"drone-system/internal/models"
	"drone-system/internal/network"
)

var (
	sectorID     string
	baseDashAddr string
)

func main() {
	sectorID = getEnv("SECTOR_ID", "A")
	// CORREÇÃO: Padrão alterado de 3001 para 3101 para alinhar com o docker-compose
	baseDashAddr = getEnv("BASE_DASH_ADDR", "localhost:3101")

	fmt.Printf("Dashboard conectando à base em %s...\n", baseDashAddr)

	// Tenta conectar com retry.
	for {
		if err := connect(); err != nil {
			fmt.Printf("[DASH] Base indisponível, tentando novamente em 3s: %v\n", err)
			time.Sleep(3 * time.Second)
			continue
		}
	}
}

// connect estabelece conexão TCP com a base e processa o stream de status.
func connect() error {
	conn, err := net.DialTimeout("tcp", baseDashAddr, network.DialTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Println("[DASH] Conectado à base — iniciando monitor...")
	scanner := bufio.NewScanner(conn)

	for scanner.Scan() {
		var payload models.StatusUpdatePayload
		if err := json.Unmarshal(scanner.Bytes(), &payload); err != nil {
			continue
		}
		render(payload)
	}

	return fmt.Errorf("conexão encerrada")
}

// render limpa o terminal e redesenha o painel completo.
func render(p models.StatusUpdatePayload) {
	// Limpa tela com ANSI.
	fmt.Print("\033[H\033[2J")

	width := 70
	now := time.Now().Format("15:04:05")

	// ── Cabeçalho ──────────────────────────────────────────────────────────
	printLine(width)
	fmt.Printf("  \033[1;36m🌊 ESTREITO DE ORMUZ — SISTEMA DRONES\033[0m"+
		"   [Setor %s]  %s\n", p.SectorID, now)
	printLine(width)

	// ── Seção superior: Bases | Fila ───────────────────────────────────────
	fmt.Println()
	leftHeader := "  BASES"
	rightHeader := "FILA DE REQUISIÇÕES (global)"
	fmt.Printf("%-35s  %s\n", leftHeader, rightHeader)
	printDivider(width)

	// Coleta drones por base para contagem.
	dronesByBase := map[string][]models.Drone{}
	for _, d := range p.Drones {
		dronesByBase[d.BaseID] = append(dronesByBase[d.BaseID], d)
	}

	// Filtra queue para mostrar apenas PENDING/LOCKED.
	var pending []models.Request
	for _, r := range p.Queue {
		if r.Status == models.StatusPending || r.Status == models.StatusLocked {
			pending = append(pending, r)
		}
		if len(pending) >= 8 {
			break
		}
	}

	maxRows := len(p.Bases)
	if len(pending) > maxRows {
		maxRows = len(pending)
	}

	for i := 0; i < maxRows; i++ {
		left := "                                "
		right := ""

		if i < len(p.Bases) {
			b := p.Bases[i]
			icon, color := "●", "\033[32m" // verde = online
			if b.Status == models.BaseOffline {
				icon, color = "✕", "\033[31m" // vermelho = offline
			}
			drones := dronesByBase[b.ID]
			left = fmt.Sprintf("  %s[%s]\033[0m %-10s  Drones:%-2d",
				color, icon+b.ID, b.Status, len(drones))
		}

		if i < len(pending) {
			r := pending[i]
			typeColor := "\033[33m" // amarelo = normal
			if r.Type == models.TypeCritical {
				typeColor = "\033[31;1m" // vermelho = critical
			}
			right = fmt.Sprintf("%s[%-8s]\033[0m %s ts:%-5d",
				typeColor, r.Type, r.SectorID, r.LamportTS)
		}

		fmt.Printf("%-35s  %s\n", left, right)
	}

	// ── Seção inferior: Drones | Alertas ───────────────────────────────────
	fmt.Println()
	fmt.Printf("%-35s  %s\n", "  DRONES", "ALERTAS RECENTES")
	printDivider(width)

	alerts := p.Alerts
	if len(alerts) > 8 {
		alerts = alerts[len(alerts)-8:]
	}

	maxRows2 := len(p.Drones)
	if len(alerts) > maxRows2 {
		maxRows2 = len(alerts)
	}

	for i := 0; i < maxRows2; i++ {
		left := "                                "
		right := ""

		if i < len(p.Drones) {
			d := p.Drones[i]
			color := "\033[32m"
			switch d.Status {
			case models.DroneBusy:
				color = "\033[33m"
			case models.DroneLost:
				color = "\033[31m"
			}
			left = fmt.Sprintf("  %s%-12s %-5s\033[0m %s",
				color, d.ID, d.Status, d.BaseID)
		}

		if i < len(alerts) {
			a := alerts[i]
			color := "\033[36m"
			switch a.Level {
			case models.AlertCritical:
				color = "\033[31;1m"
			case models.AlertWarn:
				color = "\033[33m"
			case models.AlertOK:
				color = "\033[32m"
			}
			ts := a.Timestamp.Format("15:04:05")
			msg := a.Message
			if len(msg) > 38 {
				msg = msg[:35] + "..."
			}
			right = fmt.Sprintf("%s[%-8s]\033[0m %s %s",
				color, a.Level, ts, msg)
		}

		fmt.Printf("%-35s  %s\n", left, right)
	}

	// ── Resumo ──────────────────────────────────────────────────────────────
	printLine(width)
	pendingCount := 0
	busyCount := 0
	for _, r := range p.Queue {
		if r.Status == models.StatusPending {
			pendingCount++
		}
	}
	for _, d := range p.Drones {
		if d.Status == models.DroneBusy {
			busyCount++
		}
	}
	fmt.Printf("  Drones em missão: %d  |  Req pendentes: %d  |  Total na fila: %d\n",
		busyCount, pendingCount, len(p.Queue))
	printLine(width)
}

func printLine(width int)    { fmt.Println(strings.Repeat("─", width)) }
func printDivider(width int) { fmt.Println(strings.Repeat("·", width)) }

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
