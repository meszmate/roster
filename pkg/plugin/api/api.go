package api

import (
	"sync"
	"time"

	"github.com/meszmate/roster/pkg/plugin"
)

// PluginAPI implements the plugin.API interface
type PluginAPI struct {
	mu sync.RWMutex

	// Callbacks to the main application
	sendMessage      func(to, body string) error
	getContacts      func() []plugin.Contact
	getContact       func(jid string) *plugin.Contact
	addContact       func(jid, name string, groups []string) error
	removeContact    func(jid string) error
	getPresence      func(jid string) string
	getHistory       func(jid string, limit int) []plugin.Message
	getUnreadCount   func(jid string) int
	showNotification func(title, body string) error
	showDialog       func(title, message string, buttons []string) (int, error)

	// Event handlers
	messageHandlers    []func(msg plugin.Message)
	presenceHandlers   []func(jid, status string)
	connectHandlers    []func()
	disconnectHandlers []func()

	// Commands
	commands map[string]registeredCommand

	// Status bar items
	statusBarItems map[string]string
}

type registeredCommand struct {
	description string
	handler     plugin.CommandHandler
}

// NewPluginAPI creates a new plugin API
func NewPluginAPI() *PluginAPI {
	return &PluginAPI{
		commands:       make(map[string]registeredCommand),
		statusBarItems: make(map[string]string),
	}
}

// SetSendMessage sets the send message callback
func (a *PluginAPI) SetSendMessage(f func(to, body string) error) {
	a.sendMessage = f
}

// SetGetContacts sets the get contacts callback
func (a *PluginAPI) SetGetContacts(f func() []plugin.Contact) {
	a.getContacts = f
}

// SetGetContact sets the get contact callback
func (a *PluginAPI) SetGetContact(f func(jid string) *plugin.Contact) {
	a.getContact = f
}

// SetAddContact sets the add contact callback
func (a *PluginAPI) SetAddContact(f func(jid, name string, groups []string) error) {
	a.addContact = f
}

// SetRemoveContact sets the remove contact callback
func (a *PluginAPI) SetRemoveContact(f func(jid string) error) {
	a.removeContact = f
}

// SetGetPresence sets the get presence callback
func (a *PluginAPI) SetGetPresence(f func(jid string) string) {
	a.getPresence = f
}

// SetGetHistory sets the get history callback
func (a *PluginAPI) SetGetHistory(f func(jid string, limit int) []plugin.Message) {
	a.getHistory = f
}

// SetGetUnreadCount sets the get unread count callback
func (a *PluginAPI) SetGetUnreadCount(f func(jid string) int) {
	a.getUnreadCount = f
}

// SetShowNotification sets the show notification callback
func (a *PluginAPI) SetShowNotification(f func(title, body string) error) {
	a.showNotification = f
}

// SetShowDialog sets the show dialog callback
func (a *PluginAPI) SetShowDialog(f func(title, message string, buttons []string) (int, error)) {
	a.showDialog = f
}

// RosterAPI implementation

// GetContacts returns all contacts
func (a *PluginAPI) GetContacts() []plugin.Contact {
	if a.getContacts != nil {
		return a.getContacts()
	}
	return nil
}

// GetContact returns a specific contact
func (a *PluginAPI) GetContact(jid string) *plugin.Contact {
	if a.getContact != nil {
		return a.getContact(jid)
	}
	return nil
}

// AddContact adds a contact
func (a *PluginAPI) AddContact(jid, name string, groups []string) error {
	if a.addContact != nil {
		return a.addContact(jid, name, groups)
	}
	return nil
}

// RemoveContact removes a contact
func (a *PluginAPI) RemoveContact(jid string) error {
	if a.removeContact != nil {
		return a.removeContact(jid)
	}
	return nil
}

// GetPresence returns presence for a JID
func (a *PluginAPI) GetPresence(jid string) string {
	if a.getPresence != nil {
		return a.getPresence(jid)
	}
	return ""
}

// ChatAPI implementation

// SendMessage sends a message
func (a *PluginAPI) SendMessage(to, body string) error {
	if a.sendMessage != nil {
		return a.sendMessage(to, body)
	}
	return nil
}

// GetHistory returns chat history
func (a *PluginAPI) GetHistory(jid string, limit int) []plugin.Message {
	if a.getHistory != nil {
		return a.getHistory(jid, limit)
	}
	return nil
}

// GetUnreadCount returns unread message count
func (a *PluginAPI) GetUnreadCount(jid string) int {
	if a.getUnreadCount != nil {
		return a.getUnreadCount(jid)
	}
	return 0
}

// UIAPI implementation

// ShowNotification shows a desktop notification
func (a *PluginAPI) ShowNotification(title, body string) error {
	if a.showNotification != nil {
		return a.showNotification(title, body)
	}
	return nil
}

// AddStatusBarItem adds an item to the status bar
func (a *PluginAPI) AddStatusBarItem(id, text string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.statusBarItems[id] = text
	return nil
}

// RemoveStatusBarItem removes a status bar item
func (a *PluginAPI) RemoveStatusBarItem(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.statusBarItems, id)
	return nil
}

// ShowDialog shows a dialog
func (a *PluginAPI) ShowDialog(title, message string, buttons []string) (int, error) {
	if a.showDialog != nil {
		return a.showDialog(title, message, buttons)
	}
	return -1, nil
}

// GetStatusBarItems returns all status bar items
func (a *PluginAPI) GetStatusBarItems() map[string]string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make(map[string]string)
	for k, v := range a.statusBarItems {
		result[k] = v
	}
	return result
}

// EventsAPI implementation

// OnMessage registers a message handler
func (a *PluginAPI) OnMessage(handler func(msg plugin.Message)) func() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.messageHandlers = append(a.messageHandlers, handler)

	// Return unsubscribe function
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		// Remove handler (simplified - in practice would track by ID)
	}
}

// OnPresence registers a presence handler
func (a *PluginAPI) OnPresence(handler func(jid, status string)) func() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.presenceHandlers = append(a.presenceHandlers, handler)

	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
	}
}

// OnConnect registers a connect handler
func (a *PluginAPI) OnConnect(handler func()) func() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.connectHandlers = append(a.connectHandlers, handler)

	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
	}
}

// OnDisconnect registers a disconnect handler
func (a *PluginAPI) OnDisconnect(handler func()) func() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.disconnectHandlers = append(a.disconnectHandlers, handler)

	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
	}
}

// EmitMessage emits a message event to all handlers
func (a *PluginAPI) EmitMessage(msg plugin.Message) {
	a.mu.RLock()
	handlers := make([]func(plugin.Message), len(a.messageHandlers))
	copy(handlers, a.messageHandlers)
	a.mu.RUnlock()

	for _, handler := range handlers {
		go handler(msg)
	}
}

// EmitPresence emits a presence event to all handlers
func (a *PluginAPI) EmitPresence(jid, status string) {
	a.mu.RLock()
	handlers := make([]func(string, string), len(a.presenceHandlers))
	copy(handlers, a.presenceHandlers)
	a.mu.RUnlock()

	for _, handler := range handlers {
		go handler(jid, status)
	}
}

// EmitConnect emits a connect event to all handlers
func (a *PluginAPI) EmitConnect() {
	a.mu.RLock()
	handlers := make([]func(), len(a.connectHandlers))
	copy(handlers, a.connectHandlers)
	a.mu.RUnlock()

	for _, handler := range handlers {
		go handler()
	}
}

// EmitDisconnect emits a disconnect event to all handlers
func (a *PluginAPI) EmitDisconnect() {
	a.mu.RLock()
	handlers := make([]func(), len(a.disconnectHandlers))
	copy(handlers, a.disconnectHandlers)
	a.mu.RUnlock()

	for _, handler := range handlers {
		go handler()
	}
}

// CommandsAPI implementation

// RegisterCommand registers a custom command
func (a *PluginAPI) RegisterCommand(name, description string, handler plugin.CommandHandler) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.commands[name] = registeredCommand{
		description: description,
		handler:     handler,
	}
	return nil
}

// UnregisterCommand removes a custom command
func (a *PluginAPI) UnregisterCommand(name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.commands, name)
	return nil
}

// GetCommands returns all registered commands
func (a *PluginAPI) GetCommands() map[string]string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make(map[string]string)
	for name, cmd := range a.commands {
		result[name] = cmd.description
	}
	return result
}

// ExecuteCommand executes a plugin command
func (a *PluginAPI) ExecuteCommand(name string, args []string) error {
	a.mu.RLock()
	cmd, ok := a.commands[name]
	a.mu.RUnlock()

	if !ok {
		return nil
	}

	return cmd.handler(args)
}

// CreateMessage creates a plugin message from app data
func CreateMessage(id, from, to, body string, ts time.Time, encrypted, outgoing bool) plugin.Message {
	return plugin.Message{
		ID:        id,
		From:      from,
		To:        to,
		Body:      body,
		Timestamp: ts,
		Encrypted: encrypted,
		Outgoing:  outgoing,
	}
}
