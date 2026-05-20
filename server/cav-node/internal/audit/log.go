// Package audit provides append-only NDJSON audit logging for the CAV node.
package audit

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Entry is a single audit log record.
type Entry struct {
	Timestamp string `json:"timestamp"`
	Event     string `json:"event"`
	PraxonID  string `json:"praxon_id"`
	Issuer    string `json:"issuer,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// Log is an append-only NDJSON audit log.
type Log struct {
	path string
	mu   sync.Mutex
}

// NewNDJSONLog creates a new audit log at the given path.
func NewNDJSONLog(path string) *Log {
	return &Log{path: path}
}

// Append writes a single audit entry to the log.
func (l *Log) Append(event, praxonID, issuer, reason string) {
	entry := Entry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Event:     event,
		PraxonID:  praxonID,
		Issuer:    issuer,
		Reason:    reason,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return // silently drop — audit should never crash the server
	}
	data = append(data, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(data)
}
