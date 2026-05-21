// Package models define as estruturas de dados compartilhadas
package models

import (
	"encoding/json"
	"time"
)

type RequestType string

const (
	TypeCritical RequestType = "CRITICAL" 
	TypeNormal   RequestType = "NORMAL"   
)

func (rt RequestType) Priority() int {
	switch rt {
	case TypeCritical:
		return 1
	case TypeNormal:
		return 2
	default:
		return 99
	}
}

type RequestStatus string

const (
	StatusPending  RequestStatus = "PENDING"  
	StatusLocked   RequestStatus = "LOCKED"   
	StatusAssigned RequestStatus = "ASSIGNED" 
	StatusDone     RequestStatus = "DONE"     
)

type Request struct {
	ID          string        `json:"id"`
	SectorID    string        `json:"sector_id"`
	SensorID    string        `json:"sensor_id"`
	Type        RequestType   `json:"type"`
	Priority    int           `json:"priority"`
	LamportTS   int64         `json:"lamport_ts"`
	WallClock   time.Time     `json:"wall_clock"`
	Status      RequestStatus `json:"status"`
	LockedBy    string        `json:"locked_by"`
	AssignedTo  string        `json:"assigned_to"`
	Description string        `json:"description"`
}

type DroneStatus string

const (
	DroneIdle DroneStatus = "IDLE" 
	DroneBusy DroneStatus = "BUSY" 
	DroneLost DroneStatus = "LOST" 
)

type Drone struct {
	ID          string      `json:"id"`
	BaseID      string      `json:"base_id"`
	SectorID    string      `json:"sector_id"`
	Status      DroneStatus `json:"status"`
	MissionDesc string      `json:"mission_desc"`
	X           float64     `json:"x"`
	Y           float64     `json:"y"`
	LastSeen    time.Time   `json:"last_seen"`
	Addr        string      `json:"addr"`
}

type BaseStatus string

const (
	BaseOnline  BaseStatus = "ONLINE"
	BaseOffline BaseStatus = "OFFLINE"
)

type Base struct {
	ID         string     `json:"id"`
	SectorID   string     `json:"sector_id"`
	Addr       string     `json:"addr"`
	GossipAddr string     `json:"gossip_addr"`
	Status     BaseStatus `json:"status"`
	LastSeen   time.Time  `json:"last_seen"`
	X          float64    `json:"x"`
	Y          float64    `json:"y"`
}

type AlertLevel string

const (
	AlertInfo     AlertLevel = "INFO"
	AlertWarn     AlertLevel = "WARN"
	AlertCritical AlertLevel = "CRITICAL"
	AlertOK       AlertLevel = "OK"
)

type Alert struct {
	Level     AlertLevel `json:"level"`
	Source    string     `json:"source"` 
	Message   string     `json:"message"`
	Timestamp time.Time  `json:"timestamp"`
}

type MessageType string

const (
	MsgSensorEvent       MessageType = "SENSOR_EVENT"
	MsgNewRequest        MessageType = "NEW_REQUEST"
	MsgQueueSync         MessageType = "QUEUE_SYNC"
	MsgQueueAdd          MessageType = "QUEUE_ADD"
	MsgQueueClear        MessageType = "QUEUE_CLEAR"
	MsgQueueUpdate       MessageType = "QUEUE_UPDATE"
	MsgLockRequest       MessageType = "LOCK_REQUEST"
	MsgLockReply         MessageType = "LOCK_REPLY"
	MsgLockRelease       MessageType = "LOCK_RELEASE"
	MsgDroneInfo         MessageType = "DRONE_INFO"
	MsgDroneAdopt        MessageType = "DRONE_ADOPT"
	MsgDispatch          MessageType = "DISPATCH"
	MsgReassign          MessageType = "REASSIGN"
	MsgMissionDone       MessageType = "MISSION_DONE"
	MsgDroneStatusUpdate MessageType = "DRONE_STATUS_UPDATE"
	MsgRegister          MessageType = "REGISTER"
	MsgAlert             MessageType = "ALERT"
	MsgHeartbeat         MessageType = "HEARTBEAT"
	MsgStatusUpdate      MessageType = "STATUS_UPDATE"
)

type Message struct {
	Type      MessageType     `json:"type"`
	SenderID  string          `json:"sender_id"`
	LamportTS int64           `json:"lamport_ts"`
	Payload   json.RawMessage `json:"payload"`
}

type HeartbeatPayload struct {
	NodeType   string `json:"node_type"` 
	NodeID     string `json:"node_id"`
	BaseID     string `json:"base_id,omitempty"` 
	SectorID   string `json:"sector_id,omitempty"`
	GossipAddr string `json:"gossip_addr,omitempty"`
	Addr       string `json:"addr"` 
	Status     string `json:"status,omitempty"` // ADICIONADO: Drone envia seu status verdadeiro
}

type LockRequestPayload struct {
	RequestID string `json:"request_id"`
	BaseID    string `json:"base_id"`
	LamportTS int64  `json:"lamport_ts"`
}

type LockReplyPayload struct {
	RequestID string `json:"request_id"`
	Granted   bool   `json:"granted"` 
	BaseID    string `json:"base_id"` 
}

type DispatchPayload struct {
	RequestID   string  `json:"request_id"`
	SectorID    string  `json:"sector_id"`
	TargetX     float64 `json:"target_x"`
	TargetY     float64 `json:"target_y"`
	Description string  `json:"description"`
	Priority    int     `json:"priority"`
}

type MissionDonePayload struct {
	DroneID   string `json:"drone_id"`
	RequestID string `json:"request_id"`
	Success   bool   `json:"success"`
}

type DroneStatusPayload struct {
	DroneID     string `json:"drone_id"`
	MissionDesc string `json:"mission_desc"`
}

type RegisterPayload struct {
	DroneID string  `json:"drone_id"`
	Addr    string  `json:"addr"` 
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
}

type ReassignPayload struct {
	NewBaseID   string `json:"new_base_id"`
	NewBaseAddr string `json:"new_base_addr"`
}

type StatusUpdatePayload struct {
	SectorID      string           `json:"sector_id"`
	Bases         []Base           `json:"bases"`
	Drones        []Drone          `json:"drones"`
	Queue         []Request        `json:"queue"`
	Alerts        []Alert          `json:"alerts"`
	ActiveSensors map[string]int64 `json:"active_sensors"`
	ActiveBrokers map[string]int64 `json:"active_brokers"` 
}