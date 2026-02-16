package plugin

import (
	"context"
	"time"
)

// Plugin is the interface that all plugins must implement
type Plugin interface {
	// Name returns the plugin name
	Name() string

	// Version returns the plugin version
	Version() string

	// Description returns a short description
	Description() string

	// Init initializes the plugin with the API
	Init(ctx context.Context, api API) error

	// Start starts the plugin
	Start() error

	// Stop stops the plugin
	Stop() error
}

// API is the interface exposed to plugins
type API interface {
	RosterAPI
	ChatAPI
	UIAPI
	EventsAPI
	CommandsAPI
}

// RosterAPI provides access to roster operations
type RosterAPI interface {
	// GetContacts returns all contacts
	GetContacts() []Contact

	// GetContact returns a specific contact
	GetContact(jid string) *Contact

	// AddContact adds a contact
	AddContact(jid, name string, groups []string) error

	// RemoveContact removes a contact
	RemoveContact(jid string) error

	// GetPresence returns presence for a JID
	GetPresence(jid string) string
}

// ChatAPI provides access to chat operations
type ChatAPI interface {
	// SendMessage sends a message
	SendMessage(to, body string) error

	// GetHistory returns chat history
	GetHistory(jid string, limit int) []Message

	// GetUnreadCount returns unread message count
	GetUnreadCount(jid string) int
}

// UIAPI provides access to UI operations
type UIAPI interface {
	// ShowNotification shows a desktop notification
	ShowNotification(title, body string) error

	// AddStatusBarItem adds an item to the status bar
	AddStatusBarItem(id, text string) error

	// RemoveStatusBarItem removes a status bar item
	RemoveStatusBarItem(id string) error

	// ShowDialog shows a dialog
	ShowDialog(title, message string, buttons []string) (int, error)
}

// EventsAPI provides access to event subscriptions
type EventsAPI interface {
	// OnMessage registers a message handler
	OnMessage(handler func(msg Message)) func()

	// OnPresence registers a presence handler
	OnPresence(handler func(jid, status string)) func()

	// OnConnect registers a connect handler
	OnConnect(handler func()) func()

	// OnDisconnect registers a disconnect handler
	OnDisconnect(handler func()) func()
}

// CommandsAPI provides access to command registration
type CommandsAPI interface {
	// RegisterCommand registers a custom command
	RegisterCommand(name, description string, handler CommandHandler) error

	// UnregisterCommand removes a custom command
	UnregisterCommand(name string) error
}

// Contact represents a roster contact
type Contact struct {
	JID       string
	Name      string
	Groups    []string
	Status    string
	StatusMsg string
}

// Message represents a chat message
type Message struct {
	ID        string
	From      string
	To        string
	Body      string
	Timestamp time.Time
	Encrypted bool
	Outgoing  bool
}

// CommandHandler handles a plugin command
type CommandHandler func(args []string) error

// Metadata contains plugin metadata
type Metadata struct {
	Name        string
	Version     string
	Description string
	Author      string
	Homepage    string
	License     string
	MinVersion  string // Minimum roster version required
}

// Config contains plugin configuration
type Config struct {
	Enabled bool
	Options map[string]interface{}
}
