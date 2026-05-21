// Package alerts implementa o sistema de alertas do sistema distribuído de drones.
// Alertas são gerados localmente e propagados via TCP para todas as bases e o dashboard.
package alerts

import (
	"fmt"
	"sync"
	"time"

	"drone-system/internal/models"
)

const (
	// MaxAlerts é o número máximo de alertas mantidos em memória.
	MaxAlerts = 100
)

// Manager gerencia o histórico de alertas e notifica listeners registrados.
type Manager struct {
	mu       sync.RWMutex
	alerts   []models.Alert
	source   string                      // Identificador do componente (ex: "base-A").
	handlers []func(alert models.Alert)  // Callbacks chamados a cada novo alerta.
}

// New cria um novo gerenciador de alertas para um componente.
func New(source string) *Manager {
	return &Manager{
		source:  source,
		alerts:  make([]models.Alert, 0, MaxAlerts),
		handlers: make([]func(models.Alert), 0),
	}
}

// OnAlert registra um callback chamado sempre que um novo alerta é gerado.
// Usado para propagar alertas ao dashboard ou para outras bases via TCP.
func (m *Manager) OnAlert(handler func(alert models.Alert)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = append(m.handlers, handler)
}

// emit é o método interno que adiciona o alerta e notifica handlers.
func (m *Manager) emit(level models.AlertLevel, msg string) models.Alert {
	alert := models.Alert{
		Level:     level,
		Source:    m.source,
		Message:   msg,
		Timestamp: time.Now(),
	}

	m.mu.Lock()
	// Mantém apenas os últimos MaxAlerts alertas (FIFO).
	if len(m.alerts) >= MaxAlerts {
		m.alerts = m.alerts[1:]
	}
	m.alerts = append(m.alerts, alert)
	handlers := make([]func(models.Alert), len(m.handlers))
	copy(handlers, m.handlers)
	m.mu.Unlock()

	// Imprime no terminal com cor ANSI.
	printAlert(alert)

	// Notifica todos os handlers registrados de forma assíncrona.
	for _, h := range handlers {
		go h(alert)
	}

	return alert
}

// Info emite um alerta informativo (operação normal).
func (m *Manager) Info(format string, args ...interface{}) {
	m.emit(models.AlertInfo, fmt.Sprintf(format, args...))
}

// Warn emite um aviso — situação que requer atenção.
func (m *Manager) Warn(format string, args ...interface{}) {
	m.emit(models.AlertWarn, fmt.Sprintf(format, args...))
}

// Critical emite um alerta crítico — ação imediata necessária.
func (m *Manager) Critical(format string, args ...interface{}) {
	m.emit(models.AlertCritical, fmt.Sprintf(format, args...))
}

// OK emite uma confirmação de normalização após um problema.
func (m *Manager) OK(format string, args ...interface{}) {
	m.emit(models.AlertOK, fmt.Sprintf(format, args...))
}

// GetAll retorna uma cópia do histórico de alertas.
func (m *Manager) GetAll() []models.Alert {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]models.Alert, len(m.alerts))
	copy(result, m.alerts)
	return result
}

// GetLast retorna os últimos N alertas.
func (m *Manager) GetLast(n int) []models.Alert {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if n >= len(m.alerts) {
		result := make([]models.Alert, len(m.alerts))
		copy(result, m.alerts)
		return result
	}
	result := make([]models.Alert, n)
	copy(result, m.alerts[len(m.alerts)-n:])
	return result
}

// Add adiciona externamente um alerta recebido de outro nó (ex: propagação entre bases).
func (m *Manager) Add(alert models.Alert) {
	m.mu.Lock()
	if len(m.alerts) >= MaxAlerts {
		m.alerts = m.alerts[1:]
	}
	m.alerts = append(m.alerts, alert)
	m.mu.Unlock()
	printAlert(alert)
}

// ============================================================
// Formatação ANSI para o terminal
// ============================================================

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorGreen  = "\033[32m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// printAlert imprime o alerta no terminal com cores ANSI.
func printAlert(a models.Alert) {
	ts := a.Timestamp.Format("15:04:05")
	var color, icon string
	switch a.Level {
	case models.AlertCritical:
		color, icon = colorRed+colorBold, "[CRITICAL]"
	case models.AlertWarn:
		color, icon = colorYellow, "[WARN    ]"
	case models.AlertOK:
		color, icon = colorGreen, "[OK      ]"
	default:
		color, icon = colorCyan, "[INFO    ]"
	}
	fmt.Printf("%s%s %s [%s] %s%s\n", color, icon, ts, a.Source, a.Message, colorReset)
}
