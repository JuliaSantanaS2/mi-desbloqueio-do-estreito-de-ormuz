// state.go — Estado compartilhado da base (drones, peers, locks)
package main

import (
	"sync"
	"time"

	"drone-system/internal/models"
)

type BaseState struct {
	mu sync.RWMutex

	drones map[string]*models.Drone
	peers  map[string]*models.Base
	locks  map[string]string

	dashClients map[string]chan []byte
	sensors     map[string]int64
	brokers     map[string]int64
}

func newBaseState() *BaseState {
	return &BaseState{
		drones:      make(map[string]*models.Drone),
		peers:       make(map[string]*models.Base),
		locks:       make(map[string]string),
		dashClients: make(map[string]chan []byte),
		sensors:     make(map[string]int64),
		brokers:     make(map[string]int64),
	}
}

func (s *BaseState) touchSensor(id string, ts int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.sensors[id]; !ok || ts > existing {
		s.sensors[id] = ts
	}
}

func (s *BaseState) getActiveSensors() map[string]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copy := make(map[string]int64)
	for k, v := range s.sensors {
		copy[k] = v
	}
	return copy
}

func (s *BaseState) touchBroker(id string, ts int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.brokers[id]; !ok || ts > existing {
		s.brokers[id] = ts
	}
}

func (s *BaseState) getActiveBrokers() map[string]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copy := make(map[string]int64)
	for k, v := range s.brokers {
		copy[k] = v
	}
	return copy
}

func (s *BaseState) registerDrone(d models.Drone) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.drones[d.ID]; ok {
		if existing.LastSeen.After(d.LastSeen) {
			return
		}
	}
	s.drones[d.ID] = &d
}

func (s *BaseState) updateDroneStatus(id string, status models.DroneStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.drones[id]; ok {
		d.Status = status
		d.LastSeen = time.Now()
	}
}

func (s *BaseState) touchDrone(id string, baseID string, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.drones[id]; ok {
		d.LastSeen = time.Now()
		if baseID != "" && d.BaseID != baseID {
			d.BaseID = baseID
		}
		if status != "" {
			d.Status = models.DroneStatus(status)
		} else if d.Status == models.DroneLost {
			d.Status = models.DroneIdle
		}
	}
}

func (s *BaseState) updateDroneMissionDesc(id, desc string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.drones[id]; ok {
		d.MissionDesc = desc
		d.LastSeen = time.Now()
	}
}

func (s *BaseState) getDrone(id string) (models.Drone, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if d, ok := s.drones[id]; ok {
		return *d, true
	}
	return models.Drone{}, false
}

func (s *BaseState) idleDrone(localBaseID string) (models.Drone, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.drones {
		if d.BaseID == localBaseID && d.Status == models.DroneIdle {
			return *d, true
		}
	}
	return models.Drone{}, false
}

func (s *BaseState) allDrones() []models.Drone {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]models.Drone, 0, len(s.drones))
	for _, d := range s.drones {
		list = append(list, *d)
	}
	return list
}

func (s *BaseState) droneCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.drones)
}

func (s *BaseState) removeDrone(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.drones, id)
}

// CORREÇÃO: Função para limpar os drones mortos da memória da Base
func (s *BaseState) PurgeLostDrones() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, d := range s.drones {
		if d.Status == models.DroneLost {
			delete(s.drones, id)
		}
	}
}

func (s *BaseState) updatePeer(b models.Base) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.peers[b.ID]; ok {
		existing.LastSeen = b.LastSeen
		existing.Status = b.Status
		existing.Addr = b.Addr
		existing.GossipAddr = b.GossipAddr
	} else {
		s.peers[b.ID] = &b
	}
}

func (s *BaseState) markPeerOffline(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.peers[id]; ok {
		p.Status = models.BaseOffline
	}
}

func (s *BaseState) onlinePeers() []models.Base {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []models.Base
	for _, p := range s.peers {
		if p.Status == models.BaseOnline {
			list = append(list, *p)
		}
	}
	return list
}

func (s *BaseState) allPeers() []models.Base {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]models.Base, 0, len(s.peers))
	for _, p := range s.peers {
		list = append(list, *p)
	}
	return list
}

func (s *BaseState) tryLock(reqID, baseID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.locks[reqID]; exists {
		return false
	}
	s.locks[reqID] = baseID
	return true
}

func (s *BaseState) releaseLock(reqID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.locks, reqID)
}

func (s *BaseState) clearLocks() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.locks = make(map[string]string)
}

func (s *BaseState) isLocked(reqID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.locks[reqID]
	return ok
}
