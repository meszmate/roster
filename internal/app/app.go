package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/meszmate/roster/internal/config"
	"github.com/meszmate/roster/internal/storage/sqlite"
	"github.com/meszmate/roster/internal/ui/components/chat"
	"github.com/meszmate/roster/internal/ui/components/dialogs"
	"github.com/meszmate/roster/internal/ui/components/roster"
	"github.com/meszmate/roster/internal/xmpp"
	"github.com/meszmate/roster/internal/xmpp/register"
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

// MessageStatus represents the delivery status of a message
type MessageStatus int

const (
	StatusNone      MessageStatus = iota // No status (incoming messages)
	StatusSending                        // Being sent
	StatusSent                           // Server received
	StatusDelivered                      // Recipient received (XEP-0184)
	StatusRead                           // Recipient read (XEP-0333)
	StatusFailed                         // Send failed
)

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
	Status    MessageStatus
}

// MessageStatusUpdateMsg is sent when a message status changes
type MessageStatusUpdateMsg struct {
	MessageID string
	Status    MessageStatus
}

// SendMessageResultMsg is sent after attempting to send a message
type SendMessageResultMsg struct {
	Success   bool
	MessageID string
	To        string
	Error     string
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
	ActionShowRegister
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

// RegisterFormMsg is sent when a registration form is received
type RegisterFormMsg struct {
	Server          string
	Port            int
	Fields          []register.RegistrationField
	Instructions    string
	IsDataForm      bool
	FormType        string
	RequiresCaptcha bool
	Captcha         *register.CaptchaData
	Error           string
}

// RegisterResultMsg is sent when a registration attempt completes
type RegisterResultMsg struct {
	Success  bool
	JID      string
	Password string
	Server   string
	Port     int
	Error    string
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
	rosters        []roster.Roster
	chatHistory    map[string][]chat.Message

	// Multi-account state
	accountStatuses map[string]string // JID -> status (online, connecting, failed, offline)
	accountUnreads  map[string]int    // JID -> unread count

	// Per-account contact unread tracking: accountJID -> contactJID -> unread count
	contactUnreads map[string]map[string]int

	// Status sharing state: contactJID -> enabled (true = sharing status with contact)
	statusSharing map[string]bool

	// Operation tracking for cancellation
	pendingOps   map[dialogs.OperationType]context.CancelFunc
	pendingOpsMu sync.Mutex

	// SQLite storage for roster persistence
	storage *sqlite.DB
}

// New creates a new App instance
func New(cfg *config.Config) (*App, error) {
	accounts, err := config.LoadAccounts()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Get data directory from config or use default
	dataDir := cfg.General.DataDir
	if dataDir == "" {
		paths, _ := config.GetPaths()
		if paths != nil {
			dataDir = paths.DataDir
		}
	}

	// Initialize SQLite storage
	var storage *sqlite.DB
	if dataDir != "" {
		storage, err = sqlite.New(dataDir)
		if err != nil {
			// Log error but don't fail - roster persistence is optional
			fmt.Fprintf(os.Stderr, "Warning: failed to initialize storage: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "[DEBUG] SQLite storage initialized at %s\n", dataDir)
		}
	} else {
		fmt.Fprintf(os.Stderr, "[WARN] dataDir is empty, storage not initialized\n")
	}

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
		statusSharing:   make(map[string]bool),
		pendingOps:      make(map[dialogs.OperationType]context.CancelFunc),
		storage:         storage,
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
	// Collect all accounts that need auto-connect (per-account setting)
	var cmds []tea.Cmd
	var firstAccount string

	for _, account := range a.accounts.Accounts {
		if account.AutoConnect && account.Password != "" {
			jid := account.JID
			if firstAccount == "" {
				firstAccount = jid
			}

			// Create a command for each account
			cmds = append(cmds, func() tea.Msg {
				a.mu.Lock()
				a.accountStatuses[jid] = "connecting"
				a.mu.Unlock()
				return ConnectingMsg{JID: jid}
			})
		}
	}

	// Set the first auto-connect account as current
	if firstAccount != "" {
		a.currentAccount = firstAccount
		a.mu.Lock()
		a.status = "connecting"
		a.mu.Unlock()
	}

	if len(cmds) == 0 {
		return nil
	}

	return tea.Batch(cmds...)
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
	if a.storage != nil {
		a.storage.Close()
	}
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

// GetContacts returns the roster entries (alias for GetRosters for compatibility)
func (a *App) GetContacts() []roster.Roster {
	return a.GetRosters()
}

// GetRosters returns the roster entries
func (a *App) GetRosters() []roster.Roster {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.rosters
}

// GetContactsWithStatusInfo returns roster entries enriched with status sharing info
func (a *App) GetContactsWithStatusInfo() []roster.Roster {
	a.mu.RLock()
	rosters := make([]roster.Roster, len(a.rosters))
	copy(rosters, a.rosters)
	a.mu.RUnlock()

	// Enrich each roster entry with status sharing info
	for i := range rosters {
		rosters[i].StatusHidden = !a.IsStatusSharingEnabled(rosters[i].JID)
	}

	return rosters
}

// SetContacts sets the roster entries (alias for SetRosters for compatibility)
func (a *App) SetContacts(rosters []roster.Roster) {
	a.SetRosters(rosters)
}

// SetRosters sets the roster entries
func (a *App) SetRosters(rosters []roster.Roster) {
	a.mu.Lock()
	a.rosters = rosters
	a.mu.Unlock()

	a.sendEvent(EventMsg{Type: EventRosterUpdate})
}

// GetChatHistory returns chat history for a JID
func (a *App) GetChatHistory(jid string) []chat.Message {
	a.mu.RLock()
	history := a.chatHistory[jid]
	currentAccount := a.currentAccount
	a.mu.RUnlock()

	// If we have messages in memory, return them
	if len(history) > 0 {
		return history
	}

	// Try to load from database
	if a.storage != nil && currentAccount != "" {
		dbMessages, err := a.storage.GetMessages(currentAccount, jid, 100, 0)
		if err == nil && len(dbMessages) > 0 {
			// Convert storage messages to chat messages
			messages := make([]chat.Message, len(dbMessages))
			for i, dbMsg := range dbMessages {
				status := chat.StatusNone
				if dbMsg.Displayed {
					status = chat.StatusRead
				} else if dbMsg.Received {
					status = chat.StatusDelivered
				} else if dbMsg.Outgoing {
					status = chat.StatusSent
				}

				messages[i] = chat.Message{
					ID:        dbMsg.ID,
					From:      currentAccount,
					To:        jid,
					Body:      dbMsg.Body,
					Timestamp: dbMsg.Timestamp,
					Encrypted: dbMsg.Encrypted,
					Outgoing:  dbMsg.Outgoing,
					Type:      dbMsg.Type,
					Status:    status,
				}
				if !dbMsg.Outgoing {
					messages[i].From = jid
					messages[i].To = currentAccount
				}
			}

			// Cache in memory
			a.mu.Lock()
			a.chatHistory[jid] = messages
			a.mu.Unlock()

			return messages
		}
	}

	return history
}

// AddChatMessage adds a message to chat history
func (a *App) AddChatMessage(jid string, msg chat.Message) {
	a.mu.Lock()
	a.chatHistory[jid] = append(a.chatHistory[jid], msg)
	currentAccount := a.currentAccount
	a.mu.Unlock()

	// Persist to database if enabled
	if a.storage != nil && currentAccount != "" && a.cfg.Storage.SaveMessages {
		msgType := msg.Type
		if msgType == "" {
			msgType = "chat"
		}
		_ = a.storage.SaveMessage(
			currentAccount,
			jid,
			msg.ID,
			msg.Body,
			msgType,
			msg.Timestamp,
			msg.Outgoing,
			msg.Encrypted,
		)
	}

	a.sendEvent(EventMsg{Type: EventMessage, Data: msg})
}

// SendChatMessage sends a message and returns a command to handle the result
func (a *App) SendChatMessage(to, body string) tea.Cmd {
	return func() tea.Msg {
		a.mu.RLock()
		client := a.clients[a.currentAccount]
		currentAccount := a.currentAccount
		a.mu.RUnlock()

		if client == nil || !client.IsConnected() {
			return SendMessageResultMsg{
				Success: false,
				To:      to,
				Error:   "not connected",
			}
		}

		// Send the message
		msgID, err := client.SendMessage(to, body)
		if err != nil {
			return SendMessageResultMsg{
				Success:   false,
				MessageID: msgID,
				To:        to,
				Error:     err.Error(),
			}
		}

		// Create local echo message with Sending status
		timestamp := time.Now()
		localMsg := chat.Message{
			ID:        msgID,
			From:      currentAccount,
			To:        to,
			Body:      body,
			Timestamp: timestamp,
			Outgoing:  true,
			Status:    chat.MessageStatus(StatusSending),
		}

		// Add to chat history and notify UI
		a.mu.Lock()
		a.chatHistory[to] = append(a.chatHistory[to], localMsg)
		a.mu.Unlock()

		// Send event to update UI immediately with the local echo
		a.sendEvent(EventMsg{Type: EventMessage, Data: localMsg})

		// Persist to database if enabled
		if a.storage != nil && a.cfg.Storage.SaveMessages {
			_ = a.storage.SaveMessage(currentAccount, to, msgID, body, "chat", timestamp, true, false)
		}

		// After successful send, update status to Sent
		// The message was accepted by the XMPP library
		a.UpdateMessageStatus(to, msgID, StatusSent)

		return SendMessageResultMsg{
			Success:   true,
			MessageID: msgID,
			To:        to,
		}
	}
}

// UpdateMessageStatus updates the status of a message by ID
func (a *App) UpdateMessageStatus(contactJID, msgID string, status MessageStatus) {
	a.mu.Lock()
	if messages, ok := a.chatHistory[contactJID]; ok {
		for i, msg := range messages {
			if msg.ID == msgID {
				// Convert to chat.MessageStatus
				a.chatHistory[contactJID][i].Status = chat.MessageStatus(status)
				break
			}
		}
	}
	a.mu.Unlock()

	// Persist status to database
	if a.storage != nil {
		switch status {
		case StatusDelivered:
			_ = a.storage.MarkMessageReceived(msgID)
		case StatusRead:
			_ = a.storage.MarkMessageDisplayed(msgID)
		}
	}

	// Notify UI of the status update
	a.sendEvent(EventMsg{
		Type: EventReceipt,
		Data: MessageStatusUpdateMsg{
			MessageID: msgID,
			Status:    status,
		},
	})
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

		case "register":
			if len(args) >= 1 {
				server := args[0]
				port := 5222
				if len(args) >= 2 {
					if p, err := strconv.Atoi(args[1]); err == nil {
						port = p
					}
				}
				return a.FetchRegistrationForm(server, port)()
			}
			return CommandActionMsg{Action: ActionShowRegister}

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
	previousAccount := a.currentAccount
	a.currentAccount = jid
	a.mu.Unlock()

	// Load roster from database when switching to a different non-empty account
	if jid != previousAccount && jid != "" {
		a.loadRosterFromDB(jid)
	}
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

// loadRosterFromDB loads cached roster from database for an account
func (a *App) loadRosterFromDB(accountJID string) {
	if a.storage == nil {
		return
	}

	items, err := a.storage.GetRoster(accountJID)
	if err != nil {
		return
	}

	a.mu.Lock()
	for _, item := range items {
		var groups []string
		if item.Groups != "" {
			groups = strings.Split(item.Groups, ",")
		}

		newEntry := roster.Roster{
			JID:          item.JID,
			Name:         item.Name,
			Groups:       groups,
			Status:       "offline",
			AccountJID:   accountJID,
			Subscription: item.Subscription,
		}

		// Add if not exists
		found := false
		for i, r := range a.rosters {
			if r.AccountJID == accountJID && r.JID == item.JID {
				a.rosters[i] = newEntry
				found = true
				break
			}
		}
		if !found {
			a.rosters = append(a.rosters, newEntry)
		}
	}
	a.mu.Unlock()
	// Note: We don't send EventRosterUpdate here because:
	// - In doConnect, the server roster response will trigger the update
	// - In SwitchActiveAccount (called from UI), the UI refreshes the roster directly
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

		// Load cached roster from database while connecting
		a.loadRosterFromDB(jidStr)

		// Notify UI of roster loaded from cache and status change
		a.sendEvent(EventMsg{Type: EventRosterUpdate})
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
				ID:        msg.ID,
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

			// Send delivery receipt if the message has an ID and requests one
			if msg.ID != "" {
				go func() {
					_ = client.SendReceipt(msg.From.String(), msg.ID)
				}()
			}
		})

		// Set up receipt handler for delivery/read notifications
		client.SetReceiptHandler(func(messageID string, status string) {
			// Find which contact this message belongs to and update status
			a.mu.Lock()
			var contactJID string
			for jid, messages := range a.chatHistory {
				for _, msg := range messages {
					if msg.ID == messageID {
						contactJID = jid
						break
					}
				}
				if contactJID != "" {
					break
				}
			}
			a.mu.Unlock()

			if contactJID != "" {
				var newStatus MessageStatus
				switch status {
				case "delivered":
					newStatus = StatusDelivered
				case "read":
					newStatus = StatusRead
				default:
					return
				}
				a.UpdateMessageStatus(contactJID, messageID, newStatus)
			}
		})

		// Set up roster handler
		accountJID := jidStr // Capture for closure
		client.SetRosterHandler(func(items []xmpp.RosterItem) {
			fmt.Fprintf(os.Stderr, "[DEBUG] SetRosterHandler called with %d items for account %s, storage=%v\n", len(items), accountJID, a.storage != nil)

			a.mu.Lock()

			// Build map of existing rosters for this account: JID -> index
			existingByJID := make(map[string]int)
			for i, r := range a.rosters {
				if r.AccountJID == accountJID {
					existingByJID[r.JID] = i
				}
			}

			for _, item := range items {
				itemJID := item.JID.Bare().String()

				if item.Subscription == "remove" {
					// Remove from roster
					if idx, exists := existingByJID[itemJID]; exists {
						a.rosters = append(a.rosters[:idx], a.rosters[idx+1:]...)
						// Update indices for entries after the removed one
						for jid, i := range existingByJID {
							if i > idx {
								existingByJID[jid] = i - 1
							}
						}
						delete(existingByJID, itemJID)
					}
					// Delete from database
					if a.storage != nil {
						_ = a.storage.DeleteRosterItem(accountJID, itemJID)
					}
					continue
				}

				newEntry := roster.Roster{
					JID:          itemJID,
					Name:         item.Name,
					Groups:       item.Groups,
					Status:       "offline", // Will be updated by presence
					AccountJID:   accountJID,
					Subscription: item.Subscription,
				}

				if idx, exists := existingByJID[itemJID]; exists {
					// Update existing entry - preserve Status, StatusMsg, and Unread
					newEntry.Status = a.rosters[idx].Status
					newEntry.StatusMsg = a.rosters[idx].StatusMsg
					newEntry.Unread = a.rosters[idx].Unread
					a.rosters[idx] = newEntry
				} else {
					// Add new entry
					a.rosters = append(a.rosters, newEntry)
					existingByJID[itemJID] = len(a.rosters) - 1
				}

				// Save to database
				if a.storage != nil {
					groupsStr := strings.Join(item.Groups, ",")
					if err := a.storage.SaveRosterItem(accountJID, itemJID, item.Name, item.Subscription, groupsStr); err != nil {
						fmt.Fprintf(os.Stderr, "[ERROR] Failed to save roster item %s: %v\n", itemJID, err)
					}
				} else {
					fmt.Fprintf(os.Stderr, "[WARN] Storage is nil, cannot save roster item %s\n", itemJID)
				}
			}

			// Unlock before sending event to avoid holding lock during event dispatch
			a.mu.Unlock()
			a.sendEvent(EventMsg{Type: EventRosterUpdate})
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

		// Request roster from server (async - response handled by roster handler)
		go func() {
			if err := client.RequestRoster(); err != nil {
				a.sendEvent(EventMsg{Type: EventError, Data: "Failed to request roster: " + err.Error()})
			}
		}()

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

		// Add to local roster immediately for optimistic UI update
		a.addContactToLocalRoster(contactJID, name, group)

		return AddContactResultMsg{
			Success: true,
			JID:     contactJID,
			Name:    name,
		}
	}
}

// addContactToLocalRoster adds a contact to local roster immediately (optimistic update)
func (a *App) addContactToLocalRoster(contactJID, name, group string) {
	a.mu.Lock()

	// Check if already exists
	for _, r := range a.rosters {
		if r.JID == contactJID {
			a.mu.Unlock()
			return
		}
	}

	var groups []string
	if group != "" {
		groups = []string{group}
	}

	currentAccount := a.currentAccount

	newContact := roster.Roster{
		JID:          contactJID,
		Name:         name,
		Groups:       groups,
		Status:       "offline",
		AccountJID:   currentAccount,
		Subscription: "none",
	}

	a.rosters = append(a.rosters, newContact)

	// Enable status sharing by default for new contacts
	a.statusSharing[contactJID] = true

	a.mu.Unlock()

	// Save to database immediately (don't wait for server push)
	if a.storage != nil && currentAccount != "" {
		groupsStr := ""
		if group != "" {
			groupsStr = group
		}
		if err := a.storage.SaveRosterItem(currentAccount, contactJID, name, "none", groupsStr); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Failed to save roster item %s: %v\n", contactJID, err)
		} else {
			fmt.Fprintf(os.Stderr, "[DEBUG] Saved roster item %s to database\n", contactJID)
		}
	}

	// Notify UI
	a.sendEvent(EventMsg{Type: EventRosterUpdate})
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

// GetContactsForAccount returns roster entries filtered by account
func (a *App) GetContactsForAccount(accountJID string) []roster.Roster {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if accountJID == "" {
		return a.rosters // Return all if no account specified
	}

	var filtered []roster.Roster
	for _, r := range a.rosters {
		if r.AccountJID == accountJID {
			filtered = append(filtered, r)
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
	a.mu.Lock()
	currentAccount := a.currentAccount
	client := a.xmppClient

	if currentAccount == "" {
		a.mu.Unlock()
		return false, fmt.Errorf("no account selected")
	}

	// Toggle the status sharing state
	currentState := a.statusSharing[contactJID]
	newState := !currentState
	a.statusSharing[contactJID] = newState
	a.mu.Unlock()

	// If connected, send directed presence or unavailable based on new state
	if client != nil && client.IsConnected() {
		if newState {
			// Sharing enabled - send current presence to contact
			status := a.Status()
			show := mapStatusToShow(status)
			if err := client.SendDirectedPresence(contactJID, show, a.statusMsg); err != nil {
				// Revert state on error
				a.mu.Lock()
				a.statusSharing[contactJID] = currentState
				a.mu.Unlock()
				return currentState, err
			}
		} else {
			// Sharing disabled - hide status from contact
			if err := client.HideStatusFrom(contactJID); err != nil {
				// Revert state on error
				a.mu.Lock()
				a.statusSharing[contactJID] = currentState
				a.mu.Unlock()
				return currentState, err
			}
		}
	}

	return newState, nil
}

// GetContactFingerprints returns OMEMO fingerprints for a contact
func (a *App) GetContactFingerprints(contactJID string) []string {
	if a.storage == nil {
		return []string{"Storage not available"}
	}

	a.mu.RLock()
	currentAccount := a.currentAccount
	a.mu.RUnlock()

	if currentAccount == "" {
		return []string{"No account selected"}
	}

	identities, err := a.storage.GetOMEMOIdentities(currentAccount, contactJID)
	if err != nil {
		return []string{"Error loading fingerprints: " + err.Error()}
	}

	if len(identities) == 0 {
		return []string{"No OMEMO fingerprints found"}
	}

	var fingerprints []string
	for _, id := range identities {
		fp := formatFingerprint(id.IdentityKey)
		trust := trustLevelString(id.TrustLevel)
		fingerprints = append(fingerprints,
			fmt.Sprintf("Device %d: %s [%s]", id.DeviceID, fp, trust))
	}
	return fingerprints
}

// formatFingerprint formats a fingerprint for display in groups of 8 hex chars
func formatFingerprint(key []byte) string {
	hex := fmt.Sprintf("%X", key)
	// Format in groups of 8 for readability
	var parts []string
	for i := 0; i < len(hex); i += 8 {
		end := i + 8
		if end > len(hex) {
			end = len(hex)
		}
		parts = append(parts, hex[i:end])
	}
	return strings.Join(parts, " ")
}

// trustLevelString converts a trust level integer to a string
func trustLevelString(level int) string {
	switch level {
	case 0:
		return "undecided"
	case 1:
		return "trusted"
	case 2:
		return "verified"
	case -1:
		return "untrusted"
	default:
		return "unknown"
	}
}

// GetOwnFingerprint returns the own OMEMO fingerprint for an account
func (a *App) GetOwnFingerprint(accountJID string) (string, uint32) {
	// Check if account has OMEMO enabled
	acc := a.GetAccount(accountJID)
	if acc == nil || !acc.OMEMO {
		return "", 0
	}

	if a.storage == nil {
		return "", 0
	}

	// Get own identity from storage (using account JID as both account and contact)
	identities, err := a.storage.GetOMEMOIdentities(accountJID, accountJID)
	if err != nil || len(identities) == 0 {
		return "", 0
	}

	// Return the first (own) identity
	identity := identities[0]
	return formatFingerprint(identity.IdentityKey), uint32(identity.DeviceID)
}

// IsStatusSharingEnabled checks if status sharing is enabled for a contact
func (a *App) IsStatusSharingEnabled(contactJID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.statusSharing[contactJID]
}

// FetchRegistrationForm fetches the registration form from a server
func (a *App) FetchRegistrationForm(server string, port int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
		defer cancel()

		form, err := register.FetchRegistrationForm(ctx, server, port)
		if err != nil {
			return RegisterFormMsg{
				Server: server,
				Port:   port,
				Error:  err.Error(),
			}
		}

		return RegisterFormMsg{
			Server:          form.Server,
			Port:            form.Port,
			Fields:          form.Fields,
			Instructions:    form.Instructions,
			IsDataForm:      form.IsDataForm,
			FormType:        form.FormType,
			RequiresCaptcha: form.RequiresCaptcha,
			Captcha:         form.Captcha,
		}
	}
}

// SubmitRegistration submits a registration form to the server
func (a *App) SubmitRegistration(server string, port int, fields map[string]string, isDataForm bool, formType string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
		defer cancel()

		result, err := register.SubmitRegistration(ctx, server, port, fields, isDataForm, formType)
		if err != nil {
			return RegisterResultMsg{
				Success: false,
				Server:  server,
				Port:    port,
				Error:   err.Error(),
			}
		}

		if !result.Success {
			return RegisterResultMsg{
				Success: false,
				Server:  server,
				Port:    port,
				Error:   result.Error,
			}
		}

		return RegisterResultMsg{
			Success:  true,
			JID:      result.JID,
			Password: fields["password"],
			Server:   server,
			Port:     port,
		}
	}
}
