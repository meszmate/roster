package roster

import (
	"sync"

	"mellium.im/xmpp/jid"
)

// Subscription represents the subscription state
type Subscription string

const (
	SubscriptionNone   Subscription = "none"
	SubscriptionTo     Subscription = "to"
	SubscriptionFrom   Subscription = "from"
	SubscriptionBoth   Subscription = "both"
	SubscriptionRemove Subscription = "remove"
)

// Item represents a roster item
type Item struct {
	JID          jid.JID
	Name         string
	Subscription Subscription
	Groups       []string
	Approved     bool
	Ask          string
}

// Manager manages the roster
type Manager struct {
	mu    sync.RWMutex
	items map[string]*Item
}

// NewManager creates a new roster manager
func NewManager() *Manager {
	return &Manager{
		items: make(map[string]*Item),
	}
}

// Set sets or updates a roster item
func (m *Manager) Set(item Item) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[item.JID.Bare().String()] = &item
}

// Get returns a roster item by JID
func (m *Manager) Get(j jid.JID) *Item {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.items[j.Bare().String()]
}

// Remove removes a roster item
func (m *Manager) Remove(j jid.JID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, j.Bare().String())
}

// All returns all roster items
func (m *Manager) All() []*Item {
	m.mu.RLock()
	defer m.mu.RUnlock()

	items := make([]*Item, 0, len(m.items))
	for _, item := range m.items {
		items = append(items, item)
	}
	return items
}

// Clear removes all roster items
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = make(map[string]*Item)
}

// Count returns the number of roster items
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}

// Groups returns all unique groups
func (m *Manager) Groups() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	groupSet := make(map[string]bool)
	for _, item := range m.items {
		for _, group := range item.Groups {
			groupSet[group] = true
		}
	}

	groups := make([]string, 0, len(groupSet))
	for group := range groupSet {
		groups = append(groups, group)
	}
	return groups
}

// ByGroup returns items in a specific group
func (m *Manager) ByGroup(group string) []*Item {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var items []*Item
	for _, item := range m.items {
		for _, g := range item.Groups {
			if g == group {
				items = append(items, item)
				break
			}
		}
	}
	return items
}

// Ungrouped returns items not in any group
func (m *Manager) Ungrouped() []*Item {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var items []*Item
	for _, item := range m.items {
		if len(item.Groups) == 0 {
			items = append(items, item)
		}
	}
	return items
}
