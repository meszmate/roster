package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/meszmate/roster/internal/config"
	"github.com/meszmate/roster/internal/ui/components/chat"
	"github.com/meszmate/roster/internal/ui/components/dialogs"
	"github.com/meszmate/roster/internal/ui/components/roster"
	"github.com/meszmate/roster/internal/xmpp"
)

// EventType represents the type of event
type EventType int

const (
	EventRosterUpdate EventType = iota
	EventMessage
	EventPresence
	EventConnected
	EventDisconnected
	EventError
	EventMUCJoined
	EventMUCLeft
	EventMUCMessage
	EventTyping
	EventReceipt
)

// EventMsg represents an event from the app layer
type EventMsg struct {
	Type EventType
	Data interface{}
}

// ChatMessage represents a chat message
type ChatMessage struct {
	ID        string
	From      string
	To        string
	Body      string
	Timestamp time.Time
	Encrypted bool
	Type      string
	Outgoing  bool
}

// PresenceUpdate represents a presence update
type PresenceUpdate struct {
	JID       string
	Status    string
	StatusMsg string
}

// CommandAction represents a command that needs UI interaction
type CommandAction int

const (
	ActionShowHelp CommandAction = iota
	ActionShowAccountList
	ActionShowAccountAdd
	ActionShowAccountEdit
	ActionShowPassword
	ActionShowSettings
	ActionSwitchWindow
	ActionWindowNext
	ActionWindowPrev
	ActionSaveWindows
	ActionLoadWindows
)

// CommandActionMsg is sent when a command needs UI interaction
type CommandActionMsg struct {
	Action CommandAction
	Data   map[string]interface{}
}

// ConnectResultMsg is sent when a connection attempt completes
type ConnectResultMsg struct {
	Success bool
	JID     string
	Error   string
}

// ConnectingMsg is sent when connection is starting
type ConnectingMsg struct {
	JID string
}

// AddContactResultMsg is sent when add contact operation completes
type AddContactResultMsg struct {
	Success bool
	JID     string
	Name    string
	Error   string
}

// DisconnectResultMsg is sent when a disconnect operation completes
type DisconnectResultMsg struct {
	Success bool
	JID     string
	Error   string
}

// JoinRoomResultMsg is sent when join room operation completes
type JoinRoomResultMsg struct {
	Success  bool
	RoomJID  string
	Nick     string
	Error    string
}

// CreateRoomResultMsg is sent when create room operation completes
type CreateRoomResultMsg struct {
	Success bool
	RoomJID string
	Nick    string
	Error   string
}

// OperationTimeoutMsg is sent when an async operation times out
type OperationTimeoutMsg struct {
	Operation dialogs.OperationType
}

// App represents the main application
type App struct {
	cfg       *config.Config
	accounts  *config.AccountsConfig
	program   *tea.Program
	events    chan EventMsg
	ctx       context.Context
	cancel    context.CancelFunc

	// Multi-client XMPP support
	clients    map[string]*xmpp.Client // accountJID -> client
	xmppClient *xmpp.Client            // Current/default client for backward compat

	// State
	mu             sync.RWMutex
	connected      bool
	currentAccount string
	status         string
	statusMsg      string
	contacts       []roster.Contact
	chatHistory    map[string][]chat.Message

	// Multi-account state
	accountStatuses map[string]string // JID -> status (online, connecting, failed, offline)
	accountUnreads  map[string]int    // JID -> unread count

	// Per-account contact unread tracking: accountJID -> contactJID -> unread count
	contactUnreads map[string]map[string]int

	// Operation tracking for cancellation
	pendingOps   map[dialogs.OperationType]context.CancelFunc
	pendingOpsMu sync.Mutex
}

// New creates a new App instance
func New(cfg *config.Config) (*App, error) {
	accounts, err := config.LoadAccounts()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		cfg:             cfg,
		accounts:        accounts,
		events:          make(chan EventMsg, 100),
		ctx:             ctx,
		cancel:          cancel,
		status:          "offline",
		chatHistory:     make(map[string][]chat.Message),
		accountStatuses: make(map[string]string),
		accountUnreads:  make(map[string]int),
		clients:         make(map[string]*xmpp.Client),
		contactUnreads:  make(map[string]map[string]int),
		pendingOps:      make(map[dialogs.OperationType]context.CancelFunc),
	}

	return app, nil
}

// Config returns the configuration
func (a *App) Config() *config.Config {
	return a.cfg
}

// SetProgram sets the Bubble Tea program reference
func (a *App) SetProgram(p *tea.Program) {
	a.program = p
}

// Init returns an initialization command
func (a *App) Init() tea.Cmd {
	return tea.Batch(
		a.listenForEvents(),
		a.autoConnect(),
	)
}

// listenForEvents listens for events and sends them to the UI
func (a *App) listenForEvents() tea.Cmd {
	return func() tea.Msg {
		select {
		case event := <-a.events:
			return event
		case <-a.ctx.Done():
			return nil
		}
	}
}

// autoConnect auto-connects to accounts if configured
func (a *App) autoConnect() tea.Cmd {
	return func() tea.Msg {
		if !a.cfg.General.AutoConnect {
			return nil
		}

		for _, account := range a.accounts.Accounts {
			if account.AutoConnect && account.Password != "" {
				// Set current account and trigger connection
				a.currentAccount = account.JID
				a.mu.Lock()
				a.status = "connecting"
				a.accountStatuses[account.JID] = "connecting"
				a.mu.Unlock()
				// Return ConnectingMsg to trigger actual connection
				return ConnectingMsg{JID: account.JID}
			}
		}

		return nil
	}
}

// sendEvent sends an event to the UI
func (a *App) sendEvent(event EventMsg) {
	select {
	case a.events <- event:
	default:
		// Channel full, drop event
	}

	// Also send directly to program if available
	if a.program != nil {
		a.program.Send(event)
	}
}

// Close closes the app
func (a *App) Close() {
	a.cancel()
	close(a.events)
}

// Connected returns whether we're connected
func (a *App) Connected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connected
}

// CurrentAccount returns the current account JID
func (a *App) CurrentAccount() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentAccount
}

// Status returns the current status
func (a *App) Status() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status
}

// SetStatus sets the current status
func (a *App) SetStatus(status, statusMsg string) {
	a.mu.Lock()
	a.status = status
	a.statusMsg = statusMsg
	a.mu.Unlock()
}

// mapStatusToShow converts a status string to XMPP show value
func mapStatusToShow(status string) string {
	switch status {
	case "online":
		return "" // Empty show means online/available
	case "away":
		return "away"
	case "dnd":
		return "dnd"
	case "xa":
		return "xa"
	case "offline":
		return "" // Will send unavailable presence
	default:
		return ""
	}
}

// SetStatusAndSend sets the status and sends presence to the server
func (a *App) SetStatusAndSend(status, statusMsg string) error {
	a.SetStatus(status, statusMsg)

	a.mu.RLock()
	client := a.xmppClient
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return nil // Not connected, just set local status
	}

	show := mapStatusToShow(status)
	return client.SendPresence(show, statusMsg)
}

// GetContacts returns the roster contacts
func (a *App) GetContacts() []roster.Contact {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.contacts
}

// GetContactsWithStatusInfo returns contacts enriched with status sharing info
func (a *App) GetContactsWithStatusInfo() []roster.Contact {
	a.mu.RLock()
	contacts := make([]roster.Contact, len(a.contacts))
	copy(contacts, a.contacts)
	a.mu.RUnlock()

	// Enrich each contact with status sharing info
	for i := range contacts {
		contacts[i].StatusHidden = !a.IsStatusSharingEnabled(contacts[i].JID)
	}

	return contacts
}

// SetContacts sets the roster contacts
func (a *App) SetContacts(contacts []roster.Contact) {
	a.mu.Lock()
	a.contacts = contacts
	a.mu.Unlock()

	a.sendEvent(EventMsg{Type: EventRosterUpdate})
}

// GetChatHistory returns chat history for a JID
func (a *App) GetChatHistory(jid string) []chat.Message {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.chatHistory[jid]
}

// AddChatMessage adds a message to chat history
func (a *App) AddChatMessage(jid string, msg chat.Message) {
	a.mu.Lock()
	a.chatHistory[jid] = append(a.chatHistory[jid], msg)
	a.mu.Unlock()

	a.sendEvent(EventMsg{Type: EventMessage, Data: msg})
}

// ExecuteCommand executes a command
func (a *App) ExecuteCommand(cmd string, args []string) tea.Cmd {
	return func() tea.Msg {
		switch cmd {
		// General commands
		case "quit", "q":
			return tea.Quit()

		case "help", "h":
			return CommandActionMsg{Action: ActionShowHelp}

		// Account management
		case "account":
			if len(args) == 0 {
				return CommandActionMsg{Action: ActionShowAccountList}
			}
			switch args[0] {
			case "list":
				return CommandActionMsg{Action: ActionShowAccountList}
			case "add":
				return CommandActionMsg{Action: ActionShowAccountAdd}
			case "edit":
				if len(args) > 1 {
					return CommandActionMsg{
						Action: ActionShowAccountEdit,
						Data:   map[string]interface{}{"jid": args[1]},
					}
				}
				return CommandActionMsg{Action: ActionShowAccountList}
			case "remove":
				if len(args) > 1 {
					a.RemoveAccount(args[1])
				}
				return nil
			case "default":
				if len(args) > 1 {
					a.SetDefaultAccount(args[1])
				}
				return nil
			case "resource":
				// :account resource <jid> <resource_name>
				if len(args) >= 3 {
					a.SetAccountResource(args[1], args[2])
				}
				return nil
			}
			return nil

		case "connect":
			jidStr := ""

			if len(args) >= 2 {
				// Session-only connection: :connect user@server.com password [server] [port]
				jidStr = args[0]
				password := args[1]
				server := ""
				port := 5222
				if len(args) >= 3 {
					server = args[2]
				}
				if len(args) >= 4 {
					if p, err := strconv.Atoi(args[3]); err == nil {
						port = p
					}
				}

				// Create session account immediately so DoConnect can find it
				acc := config.Account{
					JID:      jidStr,
					Password: password,
					Server:   server,
					Port:     port,
					Resource: "roster",
					OMEMO:    true,
					Session:  true,
				}
				a.AddSessionAccount(acc)
			} else if len(args) == 1 {
				jidStr = args[0]
			} else if a.currentAccount != "" {
				jidStr = a.currentAccount
			} else if len(a.accounts.Accounts) > 0 {
				// Use first non-session account
				for _, acc := range a.accounts.Accounts {
					if !acc.Session {
						jidStr = acc.JID
						break
					}
				}
			}

			if jidStr == "" {
				return CommandActionMsg{Action: ActionShowAccountAdd}
			}

			// Check if we have an account with password
			acc := a.GetAccount(jidStr)
			if acc == nil {
				// Account doesn't exist, prompt for password (session connection)
				return CommandActionMsg{
					Action: ActionShowPassword,
					Data:   map[string]interface{}{"jid": jidStr, "session": true},
				}
			}

			if acc.Password == "" && !acc.UseKeyring {
				// Need password
				return CommandActionMsg{
					Action: ActionShowPassword,
					Data:   map[string]interface{}{"jid": jidStr, "session": acc.Session},
				}
			}

			// Store connection details and return connecting message
			// The UI will trigger the actual connection
			a.mu.Lock()
			a.status = "connecting"
			a.currentAccount = jidStr
			a.mu.Unlock()

			// Return connecting message first, then do actual connection
			return ConnectingMsg{JID: jidStr}

		case "disconnect":
			return a.Disconnect()()

		// Settings
		case "settings":
			return CommandActionMsg{Action: ActionShowSettings}

		case "set":
			if len(args) == 0 {
				return CommandActionMsg{Action: ActionShowSettings}
			}
			if len(args) >= 2 {
				a.SetSetting(args[0], args[1])
			}
			return nil

		// Status commands
		case "status", "away", "dnd", "xa", "online", "offline":
			status := cmd
			if cmd == "status" && len(args) > 0 {
				status = args[0]
			}
			msg := ""
			if len(args) > 1 {
				msg = args[1]
			} else if cmd != "status" && len(args) > 0 {
				msg = args[0]
			}
			if err := a.SetStatusAndSend(status, msg); err != nil {
				// Status set locally but failed to send
				_ = err
			}
			return nil

		// Messaging
		case "msg":
			if len(args) >= 2 {
				jid := args[0]
				body := args[1]
				msg := chat.Message{
					From:      a.currentAccount,
					To:        jid,
					Body:      body,
					Timestamp: time.Now(),
					Outgoing:  true,
				}
				a.AddChatMessage(jid, msg)
			}
			return nil

		// Window switching (like TTY: :1, :2, etc.)
		case "1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
			"11", "12", "13", "14", "15", "16", "17", "18", "19", "20":
			return CommandActionMsg{
				Action: ActionSwitchWindow,
				Data:   map[string]interface{}{"window": cmd},
			}

		case "window", "win", "w":
			if len(args) > 0 {
				return CommandActionMsg{
					Action: ActionSwitchWindow,
					Data:   map[string]interface{}{"window": args[0]},
				}
			}
			return nil

		case "wn", "wnext":
			return CommandActionMsg{Action: ActionWindowNext}

		case "wp", "wprev":
			return CommandActionMsg{Action: ActionWindowPrev}

		// Roster commands
		case "roster":
			// Toggle handled by UI
			return nil

		case "add":
			if len(args) >= 1 {
				// TODO: Add contact via XMPP
			}
			return nil

		case "remove":
			if len(args) >= 1 {
				// TODO: Remove contact via XMPP
			}
			return nil

		// Window management
		case "savew", "savewindows":
			return CommandActionMsg{Action: ActionSaveWindows}

		case "loadw", "loadwindows":
			return CommandActionMsg{Action: ActionLoadWindows}

		default:
			// Unknown command
			return nil
		}
	}
}

// RemoveAccount removes an account from the configuration
func (a *App) RemoveAccount(jid string) {
	for i, acc := range a.accounts.Accounts {
		if acc.JID == jid {
			isSession := acc.Session
			a.accounts.Accounts = append(a.accounts.Accounts[:i], a.accounts.Accounts[i+1:]...)
			// Only save if it wasn't a session account
			if !isSession {
				_ = config.SaveAccounts(a.accounts)
			}
			break
		}
	}
}

// SetDefaultAccount sets the default account
func (a *App) SetDefaultAccount(jid string) {
	for i := range a.accounts.Accounts {
		if !a.accounts.Accounts[i].Session {
			a.accounts.Accounts[i].AutoConnect = (a.accounts.Accounts[i].JID == jid)
		}
	}
	_ = config.SaveAccounts(a.accounts)
}

// SetAccountResource sets the resource (client identifier) for an account
func (a *App) SetAccountResource(jid, resource string) {
	for i := range a.accounts.Accounts {
		if a.accounts.Accounts[i].JID == jid {
			a.accounts.Accounts[i].Resource = resource
			// Only save if it's not a session account
			if !a.accounts.Accounts[i].Session {
				_ = config.SaveAccounts(a.accounts)
			}
			return
		}
	}
}

// AddAccount adds a new persistent account (saved to disk)
func (a *App) AddAccount(acc config.Account) {
	acc.Session = false // Ensure it's not a session account
	// Check if account already exists
	for i, existing := range a.accounts.Accounts {
		if existing.JID == acc.JID {
			a.accounts.Accounts[i] = acc
			_ = config.SaveAccounts(a.accounts)
			return
		}
	}
	a.accounts.Accounts = append(a.accounts.Accounts, acc)
	_ = config.SaveAccounts(a.accounts)
}

// AddSessionAccount adds a session-only account (not saved to disk)
func (a *App) AddSessionAccount(acc config.Account) {
	acc.Session = true
	// Check if account already exists
	for i, existing := range a.accounts.Accounts {
		if existing.JID == acc.JID {
			a.accounts.Accounts[i] = acc
			return // Don't save session accounts
		}
	}
	a.accounts.Accounts = append(a.accounts.Accounts, acc)
	// Don't save to disk
}

// SetSetting sets a configuration setting
func (a *App) SetSetting(key, value string) {
	switch key {
	case "theme":
		a.cfg.UI.Theme = value
	case "roster_width":
		if w, err := strconv.Atoi(value); err == nil {
			a.cfg.UI.RosterWidth = w
		}
	case "roster_position":
		a.cfg.UI.RosterPosition = value
	case "show_timestamps":
		a.cfg.UI.ShowTimestamps = (value == "true" || value == "on" || value == "1")
	case "time_format":
		a.cfg.UI.TimeFormat = value
	case "notifications":
		a.cfg.UI.Notifications = (value == "true" || value == "on" || value == "1")
	case "encryption", "default_encryption":
		a.cfg.Encryption.Default = value
	case "require_encryption":
		a.cfg.Encryption.RequireEncryption = (value == "true" || value == "on" || value == "1")
	}
	_ = config.Save(a.cfg)
}

// GetSettings returns current settings as a map
func (a *App) GetSettings() map[string]string {
	return map[string]string{
		"theme":              a.cfg.UI.Theme,
		"roster_width":       strconv.Itoa(a.cfg.UI.RosterWidth),
		"roster_position":    a.cfg.UI.RosterPosition,
		"show_timestamps":    strconv.FormatBool(a.cfg.UI.ShowTimestamps),
		"time_format":        a.cfg.UI.TimeFormat,
		"notifications":      strconv.FormatBool(a.cfg.UI.Notifications),
		"encryption":         a.cfg.Encryption.Default,
		"require_encryption": strconv.FormatBool(a.cfg.Encryption.RequireEncryption),
	}
}

// AccountInfo holds account info for display
type AccountInfo struct {
	JID     string
	Session bool
}

// ConnectedAccount represents a connected account with status info
type ConnectedAccount struct {
	JID       string
	Status    string // online, connecting, failed, offline
	Unread    int    // Total unread across all contacts for this account
	Connected bool
}

// GetAccountJIDs returns a list of account JIDs (for backward compatibility)
func (a *App) GetAccountJIDs() []string {
	jids := make([]string, len(a.accounts.Accounts))
	for i, acc := range a.accounts.Accounts {
		jids[i] = acc.JID
	}
	return jids
}

// GetAccountInfos returns account info for display
func (a *App) GetAccountInfos() []AccountInfo {
	infos := make([]AccountInfo, len(a.accounts.Accounts))
	for i, acc := range a.accounts.Accounts {
		infos[i] = AccountInfo{
			JID:     acc.JID,
			Session: acc.Session,
		}
	}
	return infos
}

// Accounts returns the account configurations
func (a *App) Accounts() []config.Account {
	return a.accounts.Accounts
}

// GetAccount returns an account by JID
func (a *App) GetAccount(jid string) *config.Account {
	for i := range a.accounts.Accounts {
		if a.accounts.Accounts[i].JID == jid {
			return &a.accounts.Accounts[i]
		}
	}
	return nil
}

// GetConnectedAccounts returns a list of all accounts with their connection status
func (a *App) GetConnectedAccounts() []ConnectedAccount {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var result []ConnectedAccount
	for _, acc := range a.accounts.Accounts {
		status := a.accountStatuses[acc.JID]
		if status == "" {
			status = "offline"
		}
		unread := a.accountUnreads[acc.JID]
		connected := status == "online"

		result = append(result, ConnectedAccount{
			JID:       acc.JID,
			Status:    status,
			Unread:    unread,
			Connected: connected,
		})
	}
	return result
}

// AccountDisplayInfo holds complete account info for UI display
type AccountDisplayInfo struct {
	JID         string
	Status      string // online, connecting, failed, offline
	UnreadMsgs  int    // Total unread messages
	UnreadChats int    // Number of contacts with unread messages
	Server      string
	Port        int
	Resource    string
	OMEMO       bool
	Session     bool
	AutoConnect bool
}

// GetAllAccountsDisplay returns ALL accounts with full display info
func (a *App) GetAllAccountsDisplay() []AccountDisplayInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var result []AccountDisplayInfo
	for _, acc := range a.accounts.Accounts {
		status := a.accountStatuses[acc.JID]
		if status == "" {
			status = "offline"
		}

		// Calculate unread messages and chats per account
		unreadMsgs := a.accountUnreads[acc.JID]
		unreadChats := 0

		// Count contacts with unread messages for this specific account
		if contactUnreads, exists := a.contactUnreads[acc.JID]; exists {
			for _, unread := range contactUnreads {
				if unread > 0 {
					unreadChats++
				}
			}
		}

		result = append(result, AccountDisplayInfo{
			JID:         acc.JID,
			Status:      status,
			UnreadMsgs:  unreadMsgs,
			UnreadChats: unreadChats,
			Server:      acc.Server,
			Port:        acc.Port,
			Resource:    acc.Resource,
			OMEMO:       acc.OMEMO,
			Session:     acc.Session,
			AutoConnect: acc.AutoConnect,
		})
	}
	return result
}

// GetAccountUnreadCount returns the unread count for a specific account
func (a *App) GetAccountUnreadCount(jid string) int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.accountUnreads[jid]
}

// SetAccountStatus sets the status for a specific account
func (a *App) SetAccountStatus(jid, status string) {
	a.mu.Lock()
	a.accountStatuses[jid] = status
	a.mu.Unlock()
}

// IncrementAccountUnread increments the unread count for an account
func (a *App) IncrementAccountUnread(jid string) {
	a.mu.Lock()
	a.accountUnreads[jid]++
	a.mu.Unlock()
}

// ClearAccountUnread clears the unread count for an account
func (a *App) ClearAccountUnread(jid string) {
	a.mu.Lock()
	a.accountUnreads[jid] = 0
	a.mu.Unlock()
}

// GetUnreadChatsForAccount returns the number of contacts with unread messages for a specific account
func (a *App) GetUnreadChatsForAccount(accountJID string) int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if contactUnreads, exists := a.contactUnreads[accountJID]; exists {
		count := 0
		for _, unread := range contactUnreads {
			if unread > 0 {
				count++
			}
		}
		return count
	}
	return 0
}

// IncrementContactUnread increments unread count for a specific contact under an account
func (a *App) IncrementContactUnread(accountJID, contactJID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.contactUnreads[accountJID] == nil {
		a.contactUnreads[accountJID] = make(map[string]int)
	}
	a.contactUnreads[accountJID][contactJID]++
	// Also increment total account unreads
	a.accountUnreads[accountJID]++
}

// ClearContactUnread clears unread count for a specific contact
func (a *App) ClearContactUnread(accountJID, contactJID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if contactUnreads, exists := a.contactUnreads[accountJID]; exists {
		if oldCount, ok := contactUnreads[contactJID]; ok && oldCount > 0 {
			// Decrease total account unreads by the amount we're clearing
			a.accountUnreads[accountJID] -= oldCount
			if a.accountUnreads[accountJID] < 0 {
				a.accountUnreads[accountJID] = 0
			}
		}
		delete(contactUnreads, contactJID)
	}
}

// SwitchActiveAccount switches to a different account
func (a *App) SwitchActiveAccount(jid string) {
	a.mu.Lock()
	a.currentAccount = jid
	a.mu.Unlock()
}

// WindowState represents a saved window
type WindowState struct {
	Type   string `json:"type"`   // "console", "chat", "muc"
	JID    string `json:"jid"`    // JID for chat/muc windows
	Title  string `json:"title"`  // Window title
	Active bool   `json:"active"` // Whether this was the active window
}

// SaveWindowState saves the current window state to a file
func (a *App) SaveWindowState(windows []WindowState) error {
	paths, err := config.GetPaths()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(windows, "", "  ")
	if err != nil {
		return err
	}

	windowsFile := filepath.Join(paths.DataDir, "windows.json")
	return os.WriteFile(windowsFile, data, 0600)
}

// LoadWindowState loads the saved window state from a file
func (a *App) LoadWindowState() ([]WindowState, error) {
	paths, err := config.GetPaths()
	if err != nil {
		return nil, err
	}

	windowsFile := filepath.Join(paths.DataDir, "windows.json")
	data, err := os.ReadFile(windowsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No saved state
		}
		return nil, err
	}

	var windows []WindowState
	if err := json.Unmarshal(data, &windows); err != nil {
		return nil, err
	}

	return windows, nil
}

// doConnect performs the actual XMPP connection
func (a *App) doConnect(jidStr, password, server string, port int, isSession bool) tea.Cmd {
	return func() tea.Msg {
		// Check if already connected
		a.mu.RLock()
		if client, exists := a.clients[jidStr]; exists && client.IsConnected() {
			a.mu.RUnlock()
			return ConnectResultMsg{
				Success: true,
				JID:     jidStr,
			}
		}
		a.mu.RUnlock()

		// Set connecting status
		a.mu.Lock()
		a.status = "connecting"
		a.currentAccount = jidStr
		a.accountStatuses[jidStr] = "connecting" // Update account-specific status
		// NOTE: We no longer disconnect existing clients - true multi-account support
		// Each account maintains its own connection in the clients map
		a.mu.Unlock()

		// Notify UI of status change
		a.sendEvent(EventMsg{Type: EventPresence})

		// Create new client
		client, err := xmpp.NewClient(xmpp.ClientConfig{
			JID:      jidStr,
			Password: password,
			Server:   server,
			Port:     port,
			Resource: "roster",
		})
		if err != nil {
			a.mu.Lock()
			a.connected = false
			a.status = "failed"
			a.accountStatuses[jidStr] = "failed"
			a.mu.Unlock()
			return ConnectResultMsg{
				Success: false,
				JID:     jidStr,
				Error:   "Invalid JID: " + err.Error(),
			}
		}

		// Set up handlers
		client.SetConnectHandler(func() {
			a.sendEvent(EventMsg{Type: EventConnected})
		})

		client.SetDisconnectHandler(func(err error) {
			a.mu.Lock()
			a.connected = false
			a.status = "offline"
			a.accountStatuses[jidStr] = "offline"
			delete(a.clients, jidStr) // Remove from multi-client map
			a.mu.Unlock()
			a.sendEvent(EventMsg{Type: EventDisconnected, Data: err})
		})

		client.SetErrorHandler(func(err error) {
			a.sendEvent(EventMsg{Type: EventError, Data: err})
		})

		client.SetMessageHandler(func(msg xmpp.Message) {
			chatMsg := chat.Message{
				From:      msg.From.String(),
				To:        msg.To.String(),
				Body:      msg.Body,
				Timestamp: msg.Timestamp,
				Encrypted: msg.Encrypted,
				Outgoing:  false,
			}
			contactJID := msg.From.Bare().String()
			a.AddChatMessage(contactJID, chatMsg)
			// Track per-account unreads for multi-account support
			a.IncrementContactUnread(jidStr, contactJID)
		})

		// Attempt to connect
		if err := client.Connect(); err != nil {
			a.mu.Lock()
			a.connected = false
			a.status = "failed"
			a.accountStatuses[jidStr] = "failed"
			a.mu.Unlock()
			return ConnectResultMsg{
				Success: false,
				JID:     jidStr,
				Error:   err.Error(),
			}
		}

		// Connection successful
		a.mu.Lock()
		a.xmppClient = client
		a.clients[jidStr] = client // Store in multi-client map
		a.connected = true
		a.currentAccount = jidStr
		a.status = "online"
		a.accountStatuses[jidStr] = "online"
		a.mu.Unlock()

		// Add session account if needed
		if isSession {
			acc := config.Account{
				JID:      jidStr,
				Password: password,
				Server:   server,
				Port:     port,
				Resource: "roster",
				OMEMO:    true,
				Session:  true,
			}
			a.AddSessionAccount(acc)
		}

		return ConnectResultMsg{
			Success: true,
			JID:     jidStr,
		}
	}
}

// DoConnect starts the actual connection process
// Call this after showing "Connecting..." status to the user
func (a *App) DoConnect(jidStr string) tea.Cmd {
	// Find account details
	acc := a.GetAccount(jidStr)
	if acc == nil {
		a.mu.Lock()
		a.connected = false
		a.status = "failed"
		a.mu.Unlock()
		return func() tea.Msg {
			return ConnectResultMsg{
				Success: false,
				JID:     jidStr,
				Error:   "Account not found",
			}
		}
	}

	return a.doConnect(acc.JID, acc.Password, acc.Server, acc.Port, acc.Session)
}

// Disconnect disconnects from the XMPP server (legacy - disconnects current account)
func (a *App) Disconnect() tea.Cmd {
	a.mu.RLock()
	currentAcc := a.currentAccount
	a.mu.RUnlock()
	return a.DoDisconnect(currentAcc)
}

// DoDisconnect disconnects a specific account by JID
func (a *App) DoDisconnect(jidStr string) tea.Cmd {
	return func() tea.Msg {
		if jidStr == "" {
			return DisconnectResultMsg{
				Success: false,
				JID:     jidStr,
				Error:   "No account specified",
			}
		}

		// Get client reference while holding lock
		a.mu.Lock()
		client, exists := a.clients[jidStr]
		if exists {
			delete(a.clients, jidStr)
		}
		// Update status
		a.accountStatuses[jidStr] = "offline"
		// If this was the current account, update global status
		if a.currentAccount == jidStr {
			a.xmppClient = nil
			a.connected = false
			a.status = "offline"
		}
		a.mu.Unlock()

		// Call Disconnect without holding the lock
		if client != nil {
			_ = client.Disconnect()
		}

		a.sendEvent(EventMsg{Type: EventDisconnected})
		return DisconnectResultMsg{
			Success: true,
			JID:     jidStr,
		}
	}
}

// AddContact adds a contact to the roster and sends a subscription request (sync version)
func (a *App) AddContact(contactJID, name, group string) error {
	a.mu.RLock()
	client := a.xmppClient
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}

	// Build groups slice
	var groups []string
	if group != "" {
		groups = []string{group}
	}

	// Add to roster
	if err := client.AddContact(contactJID, name, groups); err != nil {
		return fmt.Errorf("failed to add contact: %w", err)
	}

	// Send subscription request
	if err := client.Subscribe(contactJID); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	return nil
}

// DoAddContact adds a contact asynchronously and returns a tea.Cmd
func (a *App) DoAddContact(contactJID, name, group string) tea.Cmd {
	// Register the operation
	a.RegisterOperation(dialogs.OpAddContact, func() {})

	return func() tea.Msg {
		// Mark operation as complete when done (before returning)
		defer a.CompleteOperation(dialogs.OpAddContact)

		err := a.AddContact(contactJID, name, group)
		if err != nil {
			return AddContactResultMsg{
				Success: false,
				JID:     contactJID,
				Name:    name,
				Error:   err.Error(),
			}
		}
		return AddContactResultMsg{
			Success: true,
			JID:     contactJID,
			Name:    name,
		}
	}
}

// RequestRosterRefresh requests a fresh roster from the XMPP server
func (a *App) RequestRosterRefresh() tea.Cmd {
	return func() tea.Msg {
		a.mu.RLock()
		client := a.xmppClient
		a.mu.RUnlock()

		if client == nil || !client.IsConnected() {
			return nil
		}

		// Request roster from server (this will trigger EventRosterUpdate when complete)
		if err := client.RequestRoster(); err != nil {
			return nil
		}

		return nil
	}
}

// CancelOperation cancels a pending async operation
func (a *App) CancelOperation(op dialogs.OperationType) {
	a.pendingOpsMu.Lock()
	defer a.pendingOpsMu.Unlock()

	if cancel, exists := a.pendingOps[op]; exists {
		cancel()
		delete(a.pendingOps, op)
	}
}

// OperationTimeout returns a command that sends a timeout message after the specified duration
func (a *App) OperationTimeout(op dialogs.OperationType, seconds int) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(time.Duration(seconds) * time.Second)

		// Check if the operation is still pending
		a.pendingOpsMu.Lock()
		_, stillPending := a.pendingOps[op]
		a.pendingOpsMu.Unlock()

		if stillPending {
			return OperationTimeoutMsg{Operation: op}
		}
		return nil
	}
}

// RegisterOperation registers a pending operation with a cancel function
func (a *App) RegisterOperation(op dialogs.OperationType, cancel context.CancelFunc) {
	a.pendingOpsMu.Lock()
	defer a.pendingOpsMu.Unlock()
	a.pendingOps[op] = cancel
}

// CompleteOperation marks an operation as complete (removes from pending)
func (a *App) CompleteOperation(op dialogs.OperationType) {
	a.pendingOpsMu.Lock()
	defer a.pendingOpsMu.Unlock()
	delete(a.pendingOps, op)
}

// DoJoinRoom joins a room asynchronously and returns a tea.Cmd
func (a *App) DoJoinRoom(roomJID, nick, password string) tea.Cmd {
	return func() tea.Msg {
		err := a.JoinRoom(roomJID, nick, password)
		if err != nil {
			return JoinRoomResultMsg{
				Success: false,
				RoomJID: roomJID,
				Nick:    nick,
				Error:   err.Error(),
			}
		}
		return JoinRoomResultMsg{
			Success: true,
			RoomJID: roomJID,
			Nick:    nick,
		}
	}
}

// DoCreateRoom creates a room asynchronously and returns a tea.Cmd
func (a *App) DoCreateRoom(roomJID, nick, password string, useDefaults, membersOnly, persistent bool) tea.Cmd {
	return func() tea.Msg {
		err := a.CreateRoom(roomJID, nick, password, useDefaults, membersOnly, persistent)
		if err != nil {
			return CreateRoomResultMsg{
				Success: false,
				RoomJID: roomJID,
				Nick:    nick,
				Error:   err.Error(),
			}
		}
		return CreateRoomResultMsg{
			Success: true,
			RoomJID: roomJID,
			Nick:    nick,
		}
	}
}

// Per-contact presence management

// ContactPresence holds custom presence settings for a contact
type ContactPresence struct {
	Show      string // away, dnd, xa, etc.
	StatusMsg string
}

// SetPresenceForContact sets your custom presence for a specific contact
// If show is empty, the custom presence is removed and default is used
func (a *App) SetPresenceForContact(contactJID, show, statusMsg string) error {
	// TODO: Save to storage when storage is integrated
	// For now, this is a placeholder that can be implemented with storage
	_ = contactJID
	_ = show
	_ = statusMsg
	return nil
}

// GetPresenceForContact gets your custom presence for a specific contact
// Returns empty strings if no custom presence is set (use default)
func (a *App) GetPresenceForContact(contactJID string) (show, statusMsg string) {
	// TODO: Load from storage when storage is integrated
	// For now, returns empty (no custom presence)
	_ = contactJID
	return "", ""
}

// SaveContactLastPresence saves the contact's last known presence when they go offline
func (a *App) SaveContactLastPresence(contactJID, show, statusMsg string) error {
	// TODO: Save to storage when storage is integrated
	_ = contactJID
	_ = show
	_ = statusMsg
	return nil
}

// GetContactLastPresence gets the contact's last known presence
func (a *App) GetContactLastPresence(contactJID string) (show, statusMsg string, lastSeen time.Time) {
	// TODO: Load from storage when storage is integrated
	_ = contactJID
	return "", "", time.Time{}
}

// ToggleAccountAutoConnect toggles the auto-connect setting for an account
func (a *App) ToggleAccountAutoConnect(jid string) bool {
	for i := range a.accounts.Accounts {
		if a.accounts.Accounts[i].JID == jid && !a.accounts.Accounts[i].Session {
			a.accounts.Accounts[i].AutoConnect = !a.accounts.Accounts[i].AutoConnect
			_ = config.SaveAccounts(a.accounts)
			return a.accounts.Accounts[i].AutoConnect
		}
	}
	return false
}

// GetClientForAccount returns the XMPP client for a specific account
func (a *App) GetClientForAccount(accountJID string) *xmpp.Client {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.clients[accountJID]
}

// GetConnectedClient returns a connected client for the account (or nil)
func (a *App) getConnectedClient(accountJID string) *xmpp.Client {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if client, ok := a.clients[accountJID]; ok && client.IsConnected() {
		return client
	}
	return nil
}

// IsAccountConnected checks if a specific account is connected
func (a *App) IsAccountConnected(accountJID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if client, ok := a.clients[accountJID]; ok {
		return client.IsConnected()
	}
	return false
}

// GetContactsForAccount returns contacts filtered by account
func (a *App) GetContactsForAccount(accountJID string) []roster.Contact {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if accountJID == "" {
		return a.contacts // Return all if no account specified
	}

	var filtered []roster.Contact
	for _, c := range a.contacts {
		if c.AccountJID == accountJID {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// SendPresenceForAccount sends presence for a specific account
func (a *App) SendPresenceForAccount(accountJID, show, status string) error {
	client := a.getConnectedClient(accountJID)
	if client == nil {
		return fmt.Errorf("account not connected")
	}
	return client.SendPresence(show, status)
}

// JoinRoom joins an existing MUC room
func (a *App) JoinRoom(roomJID, nick, password string) error {
	a.mu.RLock()
	client := a.xmppClient
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}

	return client.JoinRoom(roomJID, nick, password)
}

// CreateRoom creates a new MUC room
func (a *App) CreateRoom(roomJID, nick, password string, useDefaults, membersOnly, persistent bool) error {
	a.mu.RLock()
	client := a.xmppClient
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}

	config := &xmpp.RoomConfig{
		UseDefaults: useDefaults,
		Password:    password,
		MembersOnly: membersOnly,
		Persistent:  persistent,
	}

	return client.CreateRoom(roomJID, nick, config)
}

// LeaveRoom leaves a MUC room
func (a *App) LeaveRoom(roomJID, nick string) error {
	a.mu.RLock()
	client := a.xmppClient
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}

	return client.LeaveRoom(roomJID, nick)
}

// ToggleStatusSharing toggles status sharing for a contact
// Returns the new state (true = sharing enabled)
func (a *App) ToggleStatusSharing(contactJID string) (bool, error) {
	a.mu.RLock()
	currentAccount := a.currentAccount
	client := a.xmppClient
	a.mu.RUnlock()

	if currentAccount == "" {
		return false, fmt.Errorf("no account selected")
	}

	// For now, we'll store this in memory until storage is integrated
	// TODO: Use storage.SetStatusSharing when DB is integrated into App
	// For now, toggle and send appropriate presence

	// If connected, send directed presence or hide
	if client != nil && client.IsConnected() {
		// Get current status
		status := a.Status()
		show := mapStatusToShow(status)

		// Since we don't have persistent storage here yet, we'll just
		// demonstrate by sending presence
		// In production, you'd check/toggle the DB setting
		// For now, return true to indicate "enabled" (we're sending presence)
		if err := client.SendDirectedPresence(contactJID, show, a.statusMsg); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

// GetContactFingerprints returns OMEMO fingerprints for a contact
func (a *App) GetContactFingerprints(contactJID string) []string {
	// TODO: Integrate with OMEMO manager when available
	// For now, return placeholder fingerprints
	// In production, this would query the OMEMO storage
	return []string{
		"No OMEMO fingerprints available",
		"(OMEMO integration pending)",
	}
}

// GetOwnFingerprint returns the own OMEMO fingerprint for an account
func (a *App) GetOwnFingerprint(accountJID string) (string, uint32) {
	// Check if account has OMEMO enabled
	acc := a.GetAccount(accountJID)
	if acc == nil || !acc.OMEMO {
		return "", 0
	}

	// Check if account is connected
	a.mu.RLock()
	client, exists := a.clients[accountJID]
	a.mu.RUnlock()

	if !exists || client == nil || !client.IsConnected() {
		return "", 0
	}

	// TODO: Integrate with actual OMEMO manager when available
	// For now, return empty - will be populated when OMEMO is fully integrated
	// The actual fingerprint would come from the OMEMO key store
	return "", 0
}

// IsStatusSharingEnabled checks if status sharing is enabled for a contact
func (a *App) IsStatusSharingEnabled(contactJID string) bool {
	// TODO: Query storage when integrated
	return false
}
