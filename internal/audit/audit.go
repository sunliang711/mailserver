package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sunliang711/mailserver/internal/db"
)

// Logger writes structured audit records to the SQLite audit_logs table.
type Logger struct {
	store *db.Store
}

func NewLogger(store *db.Store) *Logger {
	return &Logger{store: store}
}

type Event struct {
	ActorType    string // admin | client | system
	ActorID      string
	ActorName    string
	Action       string
	ResourceType string
	ResourceID   string
	Status       string // success | failure
	IPAddress    string
	UserAgent    string
	RequestID    string
	Detail       any
}

func (l *Logger) Record(ctx context.Context, ev Event) {
	if l == nil || l.store == nil {
		return
	}
	detailJSON := "{}"
	if ev.Detail != nil {
		if b, err := json.Marshal(ev.Detail); err == nil {
			detailJSON = string(b)
		}
	}
	if ev.Status == "" {
		ev.Status = "success"
	}
	_ = l.store.InsertAuditLog(ctx, &db.AuditLog{
		ActorType:    ev.ActorType,
		ActorID:      ev.ActorID,
		ActorName:    ev.ActorName,
		Action:       ev.Action,
		ResourceType: ev.ResourceType,
		ResourceID:   ev.ResourceID,
		Status:       ev.Status,
		IPAddress:    ev.IPAddress,
		UserAgent:    ev.UserAgent,
		RequestID:    ev.RequestID,
		DetailJSON:   detailJSON,
		CreatedAt:    time.Now().UTC(),
	})
}
