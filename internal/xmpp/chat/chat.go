package chat

import (
	"sync"
	"time"

	"mellium.im/xmpp/jid"
)

// Message represents a chat message
type Message struct {
	ID           string
	From         jid.JID
	To           jid.JID
	Body         string
	Type         string // chat, groupchat, headline, normal, error
	Timestamp    time.Time
	Encrypted    bool
	Received     bool // receipt received
	Displayed    bool // chat marker displayed
	Corrected    bool // message was corrected
	CorrectedID  string
	Thread       string
}

// ChatState represents the chat state (typing, etc.)
type ChatState string

const (
	StateActive    ChatState = "active"
	StateComposing ChatState = "composing"
	StatePaused    ChatState = "paused"
	StateInactive  ChatState = "inactive"
	StateGone      ChatState = "gone"
)

// Session represents a chat session with a contact
type Session struct {
	JID       jid.JID
	Thread    string
	State     ChatState
	Messages  []Message
	Unread    int
	LastRead  time.Time
	Encrypted bool
}

// Manager manages chat sessions
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewManager creates a new chat manager
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

// GetSession gets or creates a session for a JID
func (m *Manager) GetSession(j jid.JID) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := j.Bare().String()
	if session, ok := m.sessions[bare]; ok {
		return session
	}

	session := &Session{
		JID:      j.Bare(),
		State:    StateActive,
		Messages: []Message{},
	}
	m.sessions[bare] = session
	return session
}

// AddMessage adds a message to a session
func (m *Manager) AddMessage(msg Message) {
	session := m.GetSession(msg.From)

	m.mu.Lock()
	defer m.mu.Unlock()

	session.Messages = append(session.Messages, msg)
	session.Unread++
}

// MarkRead marks all messages as read
func (m *Manager) MarkRead(j jid.JID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := j.Bare().String()
	if session, ok := m.sessions[bare]; ok {
		session.Unread = 0
		session.LastRead = time.Now()
	}
}

// SetChatState sets the chat state for a session
func (m *Manager) SetChatState(j jid.JID, state ChatState) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := j.Bare().String()
	if session, ok := m.sessions[bare]; ok {
		session.State = state
	}
}

// GetHistory returns the message history for a JID
func (m *Manager) GetHistory(j jid.JID, limit int) []Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bare := j.Bare().String()
	session, ok := m.sessions[bare]
	if !ok {
		return nil
	}

	messages := session.Messages
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	return messages
}

// ClearHistory clears the message history for a JID
func (m *Manager) ClearHistory(j jid.JID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := j.Bare().String()
	if session, ok := m.sessions[bare]; ok {
		session.Messages = []Message{}
		session.Unread = 0
	}
}

// GetUnreadCount returns the total unread count
func (m *Manager) GetUnreadCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, session := range m.sessions {
		count += session.Unread
	}
	return count
}

// GetAllSessions returns all sessions
func (m *Manager) GetAllSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// DeleteSession deletes a session
func (m *Manager) DeleteSession(j jid.JID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := j.Bare().String()
	delete(m.sessions, bare)
}

// CorrectMessage corrects a previous message
func (m *Manager) CorrectMessage(j jid.JID, originalID, newBody string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := j.Bare().String()
	session, ok := m.sessions[bare]
	if !ok {
		return false
	}

	for i := len(session.Messages) - 1; i >= 0; i-- {
		if session.Messages[i].ID == originalID {
			session.Messages[i].Body = newBody
			session.Messages[i].Corrected = true
			session.Messages[i].CorrectedID = originalID
			return true
		}
	}
	return false
}

// MarkReceived marks a message as received (delivery receipt)
func (m *Manager) MarkReceived(j jid.JID, messageID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := j.Bare().String()
	session, ok := m.sessions[bare]
	if !ok {
		return false
	}

	for i := len(session.Messages) - 1; i >= 0; i-- {
		if session.Messages[i].ID == messageID {
			session.Messages[i].Received = true
			return true
		}
	}
	return false
}

// MarkDisplayed marks a message as displayed (chat marker)
func (m *Manager) MarkDisplayed(j jid.JID, messageID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := j.Bare().String()
	session, ok := m.sessions[bare]
	if !ok {
		return false
	}

	for i := len(session.Messages) - 1; i >= 0; i-- {
		if session.Messages[i].ID == messageID {
			session.Messages[i].Displayed = true
			return true
		}
	}
	return false
}
