package main

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"drone-system/internal/models"
)

type SectorState struct {
	Payload   models.StatusUpdatePayload
	UpdatedAt time.Time
	Online    bool
}

type Aggregator struct {
	mu      sync.RWMutex
	sectors map[string]*SectorState
}

func newAggregator(sectorIDs []string) *Aggregator {
	a := &Aggregator{
		sectors: make(map[string]*SectorState),
	}
	for _, id := range sectorIDs {
		idMaiusculo := strings.ToUpper(id)
		a.sectors[idMaiusculo] = &SectorState{
			Payload: models.StatusUpdatePayload{SectorID: idMaiusculo},
			Online:  false,
		}
	}
	return a
}

func (a *Aggregator) update(payload models.StatusUpdatePayload) {
	a.mu.Lock()
	defer a.mu.Unlock()

	idSeguro := strings.ToUpper(payload.SectorID)
	if s, ok := a.sectors[idSeguro]; ok {
		s.Payload = payload
		s.UpdatedAt = time.Now()
		s.Online = true
	} else {
		// Se o setor for novo e não estava no config, adiciona
		a.sectors[idSeguro] = &SectorState{
			Payload:   payload,
			UpdatedAt: time.Now(),
			Online:    true,
		}
	}
}

func (a *Aggregator) markOffline(sectorID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	idSeguro := strings.ToUpper(sectorID)
	if s, ok := a.sectors[idSeguro]; ok {
		s.Online = false
	}
}

func (a *Aggregator) GlobalSnapshot() GlobalState {
	a.mu.RLock()
	defer a.mu.RUnlock()

	gs := GlobalState{
		Timestamp: time.Now(),
		Sectors:   make([]SectorSnapshot, 0, len(a.sectors)),
	}

	globalDrones := map[string]models.Drone{}
	for _, s := range a.sectors {
		for _, d := range s.Payload.Drones {
			if existing, ok := globalDrones[d.ID]; !ok || d.LastSeen.After(existing.LastSeen) {
				globalDrones[d.ID] = d
			}
		}
	}

	globalReqs := map[string]models.Request{}
	for _, s := range a.sectors {
		for _, r := range s.Payload.Queue {
			if _, ok := globalReqs[r.ID]; !ok {
				globalReqs[r.ID] = r
			}
		}
	}

	droneTotal, droneBusy, droneIdle, droneLost := 0, 0, 0, 0
	pendingTotal := 0
	for _, d := range globalDrones {
		droneTotal++
		switch d.Status {
		case models.DroneBusy:
			droneBusy++
		case models.DroneIdle:
			droneIdle++
		case models.DroneLost:
			droneLost++
		}
	}
	for _, r := range globalReqs {
		if r.Status == models.StatusPending || r.Status == models.StatusLocked {
			pendingTotal++
		}
	}

	gs.Stats = GlobalStats{
		DroneTotal:   droneTotal,
		DroneBusy:    droneBusy,
		DroneIdle:    droneIdle,
		DroneLost:    droneLost,
		PendingQueue: pendingTotal,
	}

	for _, s := range a.sectors {
		// =========================================================
		// CORREÇÃO DOS FANTASMAS: Se a base caiu, limpamos os
		// sensores, brokers e drones atrelados a ela da tela!
		// =========================================================
		if !s.Online {
			s.Payload.ActiveSensors = make(map[string]int64)
			s.Payload.ActiveBrokers = make(map[string]int64)
			s.Payload.Drones = []models.Drone{} // Apaga drones fantasmas
		}

		for i := range s.Payload.Bases {
			if strings.EqualFold(s.Payload.Bases[i].ID, "base-"+s.Payload.SectorID) {
				if s.Online {
					s.Payload.Bases[i].Status = models.BaseOnline
				} else {
					s.Payload.Bases[i].Status = models.BaseOffline
				}
			}
		}

		gs.Sectors = append(gs.Sectors, SectorSnapshot{
			SectorID:      s.Payload.SectorID,
			Online:        s.Online,
			UpdatedAt:     s.UpdatedAt,
			Bases:         s.Payload.Bases,
			Drones:        s.Payload.Drones,
			Queue:         s.Payload.Queue,
			Alerts:        s.Payload.Alerts,
			ActiveSensors: s.Payload.ActiveSensors,
			ActiveBrokers: s.Payload.ActiveBrokers,
		})
	}

	return gs
}

type GlobalState struct {
	Timestamp time.Time        `json:"timestamp"`
	Sectors   []SectorSnapshot `json:"sectors"`
	Stats     GlobalStats      `json:"stats"`
}

type SectorSnapshot struct {
	SectorID      string           `json:"sector_id"`
	Online        bool             `json:"online"`
	UpdatedAt     time.Time        `json:"updated_at"`
	Bases         []models.Base    `json:"bases"`
	Drones        []models.Drone   `json:"drones"`
	Queue         []models.Request `json:"queue"`
	Alerts        []models.Alert   `json:"alerts"`
	ActiveSensors map[string]int64 `json:"active_sensors"`
	ActiveBrokers map[string]int64 `json:"active_brokers"`
}

type GlobalStats struct {
	DroneTotal   int `json:"drone_total"`
	DroneBusy    int `json:"drone_busy"`
	DroneIdle    int `json:"drone_idle"`
	DroneLost    int `json:"drone_lost"`
	PendingQueue int `json:"pending_queue"`
}

func connectSector(agg *Aggregator, sectorID, dashAddr string) {
	for {
		err := readSectorLoop(agg, sectorID, dashAddr)
		agg.markOffline(sectorID)
		fmt.Printf("[WEBDASH] Setor %s offline (%v). Reconectando...\n", sectorID, err)
		time.Sleep(5 * time.Second)
	}
}

func readSectorLoop(agg *Aggregator, sectorID, dashAddr string) error {
	conn, err := net.DialTimeout("tcp", dashAddr, 3*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	decoder := json.NewDecoder(conn)
	for {
		var payload models.StatusUpdatePayload
		if err := decoder.Decode(&payload); err != nil {
			return err
		}
		if payload.SectorID == "" {
			payload.SectorID = sectorID
		}
		agg.update(payload)
	}
}
