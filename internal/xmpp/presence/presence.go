package presence

import (
	"sync"

	"mellium.im/xmpp/jid"
)

// Show represents the presence show state
type Show string

const (
	ShowOnline Show = ""
	ShowAway   Show = "away"
	ShowChat   Show = "chat"
	ShowDND    Show = "dnd"
	ShowXA     Show = "xa"
)

// Status represents a presence status
type Status struct {
	JID      jid.JID
	Show     Show
	Status   string
	Priority int
	Caps     string
}

// Manager manages presence information
type Manager struct {
	mu       sync.RWMutex
	statuses map[string]map[string]*Status // bare JID -> resource -> status
	own      *Status
}

// NewManager creates a new presence manager
func NewManager() *Manager {
	return &Manager{
		statuses: make(map[string]map[string]*Status),
	}
}

// Set sets the presence for a JID
func (m *Manager) Set(status Status) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := status.JID.Bare().String()
	resource := status.JID.Resourcepart()

	if m.statuses[bare] == nil {
		m.statuses[bare] = make(map[string]*Status)
	}
	m.statuses[bare][resource] = &status
}

// Remove removes presence for a JID (all resources or specific resource)
func (m *Manager) Remove(j jid.JID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := j.Bare().String()
	resource := j.Resourcepart()

	if resource == "" {
		delete(m.statuses, bare)
	} else if m.statuses[bare] != nil {
		delete(m.statuses[bare], resource)
		if len(m.statuses[bare]) == 0 {
			delete(m.statuses, bare)
		}
	}
}

// Get returns the highest priority presence for a bare JID
func (m *Manager) Get(j jid.JID) *Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bare := j.Bare().String()
	resources := m.statuses[bare]
	if resources == nil {
		return nil
	}

	var best *Status
	for _, status := range resources {
		if best == nil || status.Priority > best.Priority {
			best = status
		}
	}
	return best
}

// GetResource returns presence for a specific full JID
func (m *Manager) GetResource(j jid.JID) *Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bare := j.Bare().String()
	resource := j.Resourcepart()

	if m.statuses[bare] == nil {
		return nil
	}
	return m.statuses[bare][resource]
}

// GetResources returns all resources for a bare JID
func (m *Manager) GetResources(j jid.JID) []*Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bare := j.Bare().String()
	resources := m.statuses[bare]
	if resources == nil {
		return nil
	}

	result := make([]*Status, 0, len(resources))
	for _, status := range resources {
		result = append(result, status)
	}
	return result
}

// IsOnline returns whether a JID has any online resources
func (m *Manager) IsOnline(j jid.JID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bare := j.Bare().String()
	return len(m.statuses[bare]) > 0
}

// SetOwn sets our own presence
func (m *Manager) SetOwn(status Status) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.own = &status
}

// GetOwn returns our own presence
func (m *Manager) GetOwn() *Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.own
}

// Clear clears all presence information
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statuses = make(map[string]map[string]*Status)
}

// ShowToString converts a Show value to a human-readable string
func ShowToString(show Show) string {
	switch show {
	case ShowOnline:
		return "online"
	case ShowAway:
		return "away"
	case ShowChat:
		return "chat"
	case ShowDND:
		return "dnd"
	case ShowXA:
		return "xa"
	default:
		return string(show)
	}
}

// StringToShow converts a string to a Show value
func StringToShow(s string) Show {
	switch s {
	case "online", "":
		return ShowOnline
	case "away":
		return ShowAway
	case "chat":
		return ShowChat
	case "dnd":
		return ShowDND
	case "xa":
		return ShowXA
	default:
		return Show(s)
	}
}
