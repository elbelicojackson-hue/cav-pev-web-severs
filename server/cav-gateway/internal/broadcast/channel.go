// Package broadcast implements the one-to-all knowledge propagation channel.
//
// When Agent A learns something (publishes an EXP, discovers a pattern,
// survives a challenge), it broadcasts to ALL connected agents in real-time.
// This is the "stigmergic" channel — like ant pheromones, one agent's
// discovery immediately becomes available to the entire colony.
//
// Message types:
//   - learning:   Agent learned something new (EXP published)
//   - challenge:  Agent is challenging a claim
//   - consensus:  A consensus verdict was reached
//   - heartbeat:  Agent is alive (periodic)
//
// All messages are signed by the sender's Ed25519 key for authenticity.
package broadcast

import (
	"encoding/json"
	"time"
)

// MessageType defines the kind of broadcast.
type MessageType string

const (
	TypeLearning  MessageType = "learning"
	TypeChallenge MessageType = "challenge"
	TypeConsensus MessageType = "consensus"
	TypeHeartbeat MessageType = "heartbeat"
)

// Message is a broadcast payload from one agent to all others.
type Message struct {
	// Header
	ID        string      `json:"id"`
	Type      MessageType `json:"type"`
	From      string      `json:"from"`       // sender fingerprint
	FromDID   string      `json:"from_did"`   // sender DID (for verification)
	Timestamp string      `json:"timestamp"`

	// Payload (varies by type)
	Subject   string `json:"subject,omitempty"`   // what was learned/challenged
	Content   string `json:"content,omitempty"`   // human-readable summary
	PraxonID  string `json:"praxon_id,omitempty"` // related praxon (if any)
	Signature string `json:"signature,omitempty"` // Ed25519 signature of payload

	// Metadata
	Confidence float64  `json:"confidence,omitempty"` // sender's confidence [0,1]
	Tags       []string `json:"tags,omitempty"`       // hypothesis kinds / categories
}

// NewLearningMessage creates a broadcast for when an agent learns something.
func NewLearningMessage(fingerprint, did, subject, content, praxonID string, confidence float64, tags []string) *Message {
	return &Message{
		ID:         generateID(),
		Type:       TypeLearning,
		From:       fingerprint,
		FromDID:    did,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Subject:    subject,
		Content:    content,
		PraxonID:   praxonID,
		Confidence: confidence,
		Tags:       tags,
	}
}

// NewChallengeMessage creates a broadcast for when an agent challenges a claim.
func NewChallengeMessage(fingerprint, did, praxonID, reason string) *Message {
	return &Message{
		ID:        generateID(),
		Type:      TypeChallenge,
		From:      fingerprint,
		FromDID:   did,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Subject:   "Challenge: " + praxonID,
		Content:   reason,
		PraxonID:  praxonID,
	}
}

// ToJSON serializes the message.
func (m *Message) ToJSON() []byte {
	data, _ := json.Marshal(m)
	return data
}

func generateID() string {
	// Simple timestamp-based ID for now
	return "msg_" + time.Now().Format("20060102150405") + "_" + randomHex(4)
}

func randomHex(n int) string {
	b := make([]byte, n)
	// Use time-based pseudo-random for simplicity (crypto/rand in production)
	t := time.Now().UnixNano()
	for i := range b {
		b[i] = byte(t >> (i * 8))
	}
	const hex = "0123456789abcdef"
	result := make([]byte, n*2)
	for i, v := range b {
		result[i*2] = hex[v>>4]
		result[i*2+1] = hex[v&0x0f]
	}
	return string(result)
}
