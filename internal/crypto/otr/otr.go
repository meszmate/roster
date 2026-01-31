package otr

import (
	"errors"
	"sync"
)

// State represents the OTR conversation state
type State int

const (
	StatePlaintext State = iota
	StateEncrypted
	StateFinished
)

// Policy represents OTR policy options
type Policy int

const (
	PolicyNever Policy = iota
	PolicyManual
	PolicyOpportunistic
	PolicyAlways
)

// Session represents an OTR session
type Session struct {
	JID       string
	State     State
	Verified  bool
	Fingerprint string
}

// Manager manages OTR sessions
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	policy   Policy
}

// NewManager creates a new OTR manager
func NewManager(policy Policy) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		policy:   policy,
	}
}

// StartSession initiates OTR with a JID
func (m *Manager) StartSession(jid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[jid] = &Session{
		JID:   jid,
		State: StatePlaintext,
	}

	// OTR query message would be sent here
	return nil
}

// EndSession ends an OTR session
func (m *Manager) EndSession(jid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session := m.sessions[jid]
	if session == nil {
		return nil
	}

	session.State = StateFinished
	delete(m.sessions, jid)
	return nil
}

// GetSession returns a session
func (m *Manager) GetSession(jid string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[jid]
}

// IsEncrypted returns whether a session is encrypted
func (m *Manager) IsEncrypted(jid string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session := m.sessions[jid]
	return session != nil && session.State == StateEncrypted
}

// Encrypt encrypts a message using OTR
func (m *Manager) Encrypt(jid, plaintext string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session := m.sessions[jid]
	if session == nil || session.State != StateEncrypted {
		return "", errors.New("no encrypted session")
	}

	// OTR encryption would happen here
	return plaintext, nil
}

// Decrypt decrypts an OTR message
func (m *Manager) Decrypt(jid, ciphertext string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session := m.sessions[jid]
	if session == nil {
		return "", errors.New("no session")
	}

	// OTR decryption would happen here
	return ciphertext, nil
}

// ProcessMessage processes an incoming message for OTR content
func (m *Manager) ProcessMessage(jid, message string) (string, bool, error) {
	// Check for OTR markers
	// This is a placeholder - real implementation would parse OTR messages
	return message, false, nil
}

// GetFingerprint returns our OTR fingerprint
func (m *Manager) GetFingerprint() string {
	// Would return the fingerprint of our OTR key
	return ""
}

// GetPeerFingerprint returns the fingerprint of a peer
func (m *Manager) GetPeerFingerprint(jid string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session := m.sessions[jid]
	if session == nil {
		return ""
	}
	return session.Fingerprint
}

// VerifyFingerprint marks a fingerprint as verified
func (m *Manager) VerifyFingerprint(jid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session := m.sessions[jid]
	if session == nil {
		return errors.New("no session")
	}

	session.Verified = true
	return nil
}

// SetPolicy sets the OTR policy
func (m *Manager) SetPolicy(policy Policy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policy = policy
}

// GetPolicy returns the current OTR policy
func (m *Manager) GetPolicy() Policy {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.policy
}
