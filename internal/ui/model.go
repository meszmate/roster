package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/meszmate/roster/internal/app"
	"github.com/meszmate/roster/internal/config"
	"github.com/meszmate/roster/internal/ui/components/chat"
	"github.com/meszmate/roster/internal/ui/components/commandline"
	"github.com/meszmate/roster/internal/ui/components/dialogs"
	"github.com/meszmate/roster/internal/ui/components/roster"
	"github.com/meszmate/roster/internal/ui/components/settings"
	"github.com/meszmate/roster/internal/ui/components/statusbar"
	"github.com/meszmate/roster/internal/ui/components/windows"
	"github.com/meszmate/roster/internal/ui/keybindings"
	"github.com/meszmate/roster/internal/ui/theme"
)

// Focus represents which component is focused
type Focus int

const (
	FocusRoster Focus = iota
	FocusChat
	FocusCommandLine
	FocusDialog
	FocusSettings
	FocusAccounts
)

// restoreViewerMsg is sent to restore the CAPTCHA viewer after showing a status
type restoreViewerMsg struct{}

// Model is the root Bubble Tea model
type Model struct {
	app          *app.App
	width        int
	height       int
	focus        Focus
	ready        bool

	// Components
	roster      roster.Model
	chat        chat.Model
	statusbar   statusbar.Model
	commandline commandline.Model
	windows     windows.Model
	dialog      dialogs.Model
	settings    settings.Model

	// Managers
	keys        *keybindings.Manager
	themes      *theme.Manager

	// State
	showRoster   bool
	showHelp     bool
	showSettings bool
	quitting     bool

	// CAPTCHA state for registration
	captchaData []byte
	captchaMime string
	captchaURL  string
}

// NewModel creates a new root model
func NewModel(application *app.App) Model {
	cfg := application.Config()
	themeManager := theme.NewManager("themes", cfg.General.DataDir+"/themes")
	if err := themeManager.SetTheme(cfg.UI.Theme); err != nil {
		// Fall back to default theme
		_ = themeManager.SetTheme("rainbow")
	}

	keysManager := keybindings.NewManager()

	return Model{
		app:         application,
		focus:       FocusRoster,
		showRoster:  true,
		keys:        keysManager,
		themes:      themeManager,
		roster:      roster.New(themeManager.Styles()),
		chat:        chat.New(themeManager.Styles()),
		statusbar:   statusbar.New(themeManager.Styles()),
		commandline: commandline.New(themeManager.Styles()),
		windows:     windows.New(themeManager.Styles()),
		dialog:      dialogs.New(themeManager.Styles()),
		settings:    settings.New(cfg, themeManager.Styles(), themeManager.AvailableThemes()),
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		m.app.Init(),
	)
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.updateComponentSizes()

	case tea.KeyMsg:
		// Handle quitting
		if msg.Type == tea.KeyCtrlC {
			m.quitting = true
			return m, tea.Quit
		}

		// Handle settings menu if active
		if m.showSettings {
			var cmd tea.Cmd
			if msg.String() == "esc" || msg.String() == "q" {
				m.showSettings = false
				m.focus = FocusRoster
				return m, nil
			}
			m.settings, cmd = m.settings.Update(msg)
			return m, cmd
		}

		// Handle dialog first if active
		if m.dialog.Active() {
			var cmd tea.Cmd
			m.dialog, cmd = m.dialog.Update(msg)
			return m, cmd
		}

		// Process key through keybinding manager
		action := m.keys.HandleKey(msg)
		cmd := m.handleAction(action, msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

		// Don't pass the key to component if it was a mode-switching action
		// (the key that triggered the mode switch shouldn't be typed)
		isModeSwitch := action == keybindings.ActionEnterCommand ||
			action == keybindings.ActionEnterSearch ||
			action == keybindings.ActionEnterInsert ||
			action == keybindings.ActionEnterInsertAfter ||
			action == keybindings.ActionEnterInsertLineStart ||
			action == keybindings.ActionEnterInsertLineEnd ||
			action == keybindings.ActionExitMode ||
			action == keybindings.ActionCancelCommand

		// Pass to focused component if in insert, command, or search mode
		if !isModeSwitch && (m.keys.Mode() == keybindings.ModeInsert || m.keys.Mode() == keybindings.ModeCommand || m.keys.Mode() == keybindings.ModeSearch) {
			cmds = append(cmds, m.updateFocusedComponent(msg)...)
		}

	case app.EventMsg:
		// Handle application events
		cmds = append(cmds, m.handleAppEvent(msg))

	case roster.SelectMsg:
		// Contact selected in roster
		m.openChat(msg.JID)
		m.focus = FocusChat
		m.keys.SetMode(keybindings.ModeInsert)

	case commandline.CommandMsg:
		// Command executed
		cmds = append(cmds, m.executeCommand(msg.Command, msg.Args))
		m.keys.SetMode(keybindings.ModeNormal)
		m.focus = FocusRoster

	case commandline.CancelMsg:
		// Exit command mode (backspace on empty input)
		m.keys.SetMode(keybindings.ModeNormal)
		m.commandline = m.commandline.Clear()
		m.focus = FocusRoster

	case app.CommandActionMsg:
		// Handle command actions that need UI
		m.handleCommandAction(msg)

	case dialogs.DialogResult:
		// Handle dialog results
		cmds = append(cmds, m.handleDialogResult(msg))

	case app.ConnectingMsg:
		// Show connecting status and start actual connection
		m.chat = m.chat.SetStatusMsg("Connecting to " + msg.JID + "...")
		cmds = append(cmds, m.app.DoConnect(msg.JID))

	case app.ConnectResultMsg:
		// Handle connection result
		if msg.Success {
			m.chat = m.chat.ClearStatusMsg()
		} else {
			m.chat = m.chat.SetStatusMsg("Connection failed: " + msg.Error)
			m.dialog = m.dialog.ShowError("Connection failed: " + msg.Error)
			m.focus = FocusDialog
		}

	case settings.SaveMsg:
		// Settings saved, apply theme change if needed
		if err := m.themes.SetTheme(m.app.Config().UI.Theme); err == nil {
			// Update all component styles
			styles := m.themes.Styles()
			m.roster = roster.New(styles).SetContacts(m.app.GetContacts())
			m.chat = chat.New(styles)
			m.statusbar = statusbar.New(styles)
			m.commandline = commandline.New(styles)
			m.dialog = dialogs.New(styles)
			m.updateComponentSizes()
		}

	case app.RegisterFormMsg:
		// Handle registration form received
		if msg.Error != "" {
			m.dialog = m.dialog.ShowError("Registration error: " + msg.Error)
		} else {
			fields := make([]dialogs.RegistrationField, len(msg.Fields))
			for i, f := range msg.Fields {
				fields[i] = dialogs.RegistrationField{
					Name:     f.Name,
					Label:    f.Label,
					Required: f.Required,
					Password: f.Password,
					Type:     f.Type,
					Value:    f.Value,
				}
			}
			// Convert CAPTCHA info if present
			var captchaInfo *dialogs.CaptchaInfo
			if msg.Captcha != nil {
				captchaInfo = &dialogs.CaptchaInfo{
					Type:      msg.Captcha.Type,
					Challenge: msg.Captcha.Challenge,
					MimeType:  msg.Captcha.MimeType,
					Data:      msg.Captcha.Data,
					URLs:      msg.Captcha.URLs,
					URL:       msg.Captcha.URL,
					Question:  msg.Captcha.Question,
					FieldVar:  msg.Captcha.FieldVar,
				}
			}
			m.dialog = m.dialog.ShowRegisterForm(msg.Server, msg.Port, fields, msg.Instructions, msg.IsDataForm, msg.FormType, captchaInfo)
			// Store CAPTCHA data for later viewing
			if msg.Captcha != nil {
				if len(msg.Captcha.Data) > 0 {
					m.captchaData = msg.Captcha.Data
					m.captchaMime = msg.Captcha.MimeType
				}
				// Only store URL if it's a real HTTP URL, not a cid: reference
				if msg.Captcha.URL != "" && !strings.HasPrefix(msg.Captcha.URL, "cid:") {
					m.captchaURL = msg.Captcha.URL
				} else if len(msg.Captcha.URLs) > 0 {
					// Try to find an HTTP URL from alternatives
					for _, u := range msg.Captcha.URLs {
						if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
							m.captchaURL = u
							break
						}
					}
				}
			}
		}
		m.focus = FocusDialog

	case app.RegisterResultMsg:
		// Handle registration result
		if msg.Success {
			m.dialog = m.dialog.ShowRegisterSuccess(msg.JID, msg.Password, msg.Server, msg.Port)
		} else {
			m.dialog = m.dialog.ShowError("Registration failed: " + msg.Error)
		}
		m.focus = FocusDialog

	case restoreViewerMsg:
		// Restore CAPTCHA viewer to default state after showing status
		m.dialog = m.dialog.RestoreViewer()
	}

	// Update status bar with current state
	m.statusbar = m.statusbar.SetMode(m.keys.Mode())
	m.statusbar = m.statusbar.SetAccount(m.app.CurrentAccount())
	m.statusbar = m.statusbar.SetStatus(m.app.Status())
	m.statusbar = m.statusbar.SetConnected(m.app.Connected())
	m.statusbar = m.statusbar.SetWindows(m.getWindowInfos())

	// Update roster with connected accounts
	m.roster = m.roster.SetAccounts(m.getAccountDisplays())

	return m, tea.Batch(cmds...)
}

// View renders the UI
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	if m.quitting {
		return "Goodbye!\n"
	}

	styles := m.themes.Styles()

	// Calculate dimensions
	statusHeight := 1
	cmdHeight := 1
	mainHeight := m.height - statusHeight - cmdHeight

	// Build main area
	var mainView string
	rosterWidth := m.app.Config().UI.RosterWidth
	if !m.showRoster {
		rosterWidth = 0
	}
	chatWidth := m.width - rosterWidth

	if m.showRoster && rosterWidth > 0 {
		rosterView := m.roster.View()
		chatView := m.chat.View()

		// Apply focus styling
		if m.focus == FocusRoster {
			rosterView = styles.WindowActive.Width(rosterWidth - 2).Height(mainHeight - 2).Render(rosterView)
		} else {
			rosterView = styles.WindowInactive.Width(rosterWidth - 2).Height(mainHeight - 2).Render(rosterView)
		}

		if m.focus == FocusChat {
			chatView = styles.WindowActive.Width(chatWidth - 2).Height(mainHeight - 2).Render(chatView)
		} else {
			chatView = styles.WindowInactive.Width(chatWidth - 2).Height(mainHeight - 2).Render(chatView)
		}

		mainView = lipgloss.JoinHorizontal(lipgloss.Top, rosterView, chatView)
	} else {
		chatView := m.chat.View()
		if m.focus == FocusChat {
			chatView = styles.WindowActive.Width(m.width - 2).Height(mainHeight - 2).Render(chatView)
		} else {
			chatView = styles.WindowInactive.Width(m.width - 2).Height(mainHeight - 2).Render(chatView)
		}
		mainView = chatView
	}

	// Build command/input line
	var cmdView string
	if m.keys.Mode() == keybindings.ModeCommand || m.keys.Mode() == keybindings.ModeSearch {
		cmdView = m.commandline.View()
	} else if m.focus == FocusChat && m.keys.Mode() == keybindings.ModeInsert {
		cmdView = m.chat.InputView()
	} else {
		cmdView = ""
	}

	// Build status bar
	statusView := m.statusbar.View()

	// Combine all views
	result := lipgloss.JoinVertical(lipgloss.Left,
		mainView,
		cmdView,
		statusView,
	)

	// Overlay dialog if active
	if m.dialog.Active() {
		result = m.overlayDialog(result)
	}

	// Overlay settings if active
	if m.showSettings {
		result = m.overlaySettings(result)
	}

	return result
}

// handleAction processes keybinding actions
func (m *Model) handleAction(action keybindings.Action, msg tea.KeyMsg) tea.Cmd {
	switch action {
	case keybindings.ActionQuit:
		m.quitting = true
		return tea.Quit

	case keybindings.ActionEnterInsert:
		m.keys.SetMode(keybindings.ModeInsert)
		if m.focus == FocusRoster {
			m.focus = FocusChat
		}

	case keybindings.ActionEnterCommand:
		m.keys.SetMode(keybindings.ModeCommand)
		m.focus = FocusCommandLine
		m.commandline = m.commandline.SetPrefix(":")
		m.commandline = m.commandline.Clear()

	case keybindings.ActionEnterSearch:
		m.keys.SetMode(keybindings.ModeSearch)
		m.focus = FocusCommandLine
		m.commandline = m.commandline.SetPrefix("/")
		m.commandline = m.commandline.Clear()

	case keybindings.ActionExitMode, keybindings.ActionCancelCommand:
		m.keys.SetMode(keybindings.ModeNormal)
		m.commandline = m.commandline.Clear()
		if m.focus == FocusCommandLine {
			m.focus = FocusRoster
		}

	case keybindings.ActionMoveUp:
		count := m.keys.Count()
		for i := 0; i < count; i++ {
			if m.focus == FocusRoster {
				m.roster = m.roster.MoveUp()
			} else if m.focus == FocusChat {
				m.chat = m.chat.ScrollUp()
			}
		}

	case keybindings.ActionMoveDown:
		count := m.keys.Count()
		for i := 0; i < count; i++ {
			if m.focus == FocusRoster {
				m.roster = m.roster.MoveDown()
			} else if m.focus == FocusChat {
				m.chat = m.chat.ScrollDown()
			}
		}

	case keybindings.ActionMoveTop:
		if m.focus == FocusRoster {
			m.roster = m.roster.MoveToTop()
		} else if m.focus == FocusChat {
			m.chat = m.chat.ScrollToTop()
		}

	case keybindings.ActionMoveBottom:
		if m.focus == FocusRoster {
			m.roster = m.roster.MoveToBottom()
		} else if m.focus == FocusChat {
			m.chat = m.chat.ScrollToBottom()
		}

	case keybindings.ActionHalfPageUp:
		if m.focus == FocusRoster {
			m.roster = m.roster.PageUp()
		} else if m.focus == FocusChat {
			m.chat = m.chat.HalfPageUp()
		}

	case keybindings.ActionHalfPageDown:
		if m.focus == FocusRoster {
			m.roster = m.roster.PageDown()
		} else if m.focus == FocusChat {
			m.chat = m.chat.HalfPageDown()
		}

	case keybindings.ActionOpenChat:
		if m.focus == FocusRoster {
			if jid := m.roster.SelectedJID(); jid != "" {
				m.openChat(jid)
				m.focus = FocusChat
				m.keys.SetMode(keybindings.ModeInsert)
			}
		}

	case keybindings.ActionCloseChat:
		m.windows = m.windows.CloseActive()
		m.focus = FocusRoster

	case keybindings.ActionToggleRoster:
		m.showRoster = !m.showRoster
		m.updateComponentSizes()

	case keybindings.ActionFocusRoster:
		m.focus = FocusRoster

	case keybindings.ActionFocusChat:
		m.focus = FocusChat

	case keybindings.ActionNextWindow:
		m.windows = m.windows.Next()
		m.loadActiveWindow()

	case keybindings.ActionPrevWindow:
		m.windows = m.windows.Prev()
		m.loadActiveWindow()

	case keybindings.ActionWindow1, keybindings.ActionWindow2, keybindings.ActionWindow3,
		keybindings.ActionWindow4, keybindings.ActionWindow5, keybindings.ActionWindow6,
		keybindings.ActionWindow7, keybindings.ActionWindow8, keybindings.ActionWindow9,
		keybindings.ActionWindow10, keybindings.ActionWindow11, keybindings.ActionWindow12,
		keybindings.ActionWindow13, keybindings.ActionWindow14, keybindings.ActionWindow15,
		keybindings.ActionWindow16, keybindings.ActionWindow17, keybindings.ActionWindow18,
		keybindings.ActionWindow19, keybindings.ActionWindow20:
		windowNum := int(action - keybindings.ActionWindow1)
		m.windows = m.windows.GoTo(windowNum)
		m.loadActiveWindow()

	case keybindings.ActionSearchNext:
		query := m.keys.SearchQuery()
		if query != "" {
			if m.focus == FocusRoster {
				m.roster = m.roster.SearchNext(query)
			} else if m.focus == FocusChat {
				m.chat = m.chat.SearchNext(query)
			}
		}

	case keybindings.ActionSearchPrev:
		query := m.keys.SearchQuery()
		if query != "" {
			if m.focus == FocusRoster {
				m.roster = m.roster.SearchPrev(query)
			} else if m.focus == FocusChat {
				m.chat = m.chat.SearchPrev(query)
			}
		}

	case keybindings.ActionAddContact:
		m.dialog = m.dialog.ShowAddContact()
		m.focus = FocusDialog

	case keybindings.ActionJoinRoom:
		m.dialog = m.dialog.ShowJoinRoom()
		m.focus = FocusDialog

	case keybindings.ActionShowInfo:
		if m.focus == FocusRoster {
			if jid := m.roster.SelectedJID(); jid != "" {
				m.dialog = m.dialog.ShowContactInfo(jid)
				m.focus = FocusDialog
			}
		}

	case keybindings.ActionShowSettings:
		m.showSettings = true
		m.focus = FocusSettings
		m.settings = m.settings.SetSize(m.width-4, m.height-4)

	case keybindings.ActionSaveWindows:
		m.saveWindows()

	case keybindings.ActionFocusAccounts:
		m.focus = FocusAccounts
		m.roster = m.roster.MoveToAccounts()

	case keybindings.ActionToggleAccountList:
		m.roster = m.roster.ToggleAccountList()

	case keybindings.ActionShowContextHelp:
		m.showContextHelp()
	}

	return nil
}

// showContextHelp shows context-sensitive help popup based on current focus
func (m *Model) showContextHelp() {
	var title, content string

	switch m.focus {
	case FocusChat:
		// Show recent messages
		title = "Recent Messages"
		jid := m.windows.ActiveJID()
		if jid == "" {
			content = "No active chat.\n\nOpen a chat with Enter on a contact,\nor use :1 to switch to window 1."
		} else {
			history := m.app.GetChatHistory(jid)
			if len(history) == 0 {
				content = "No messages in this chat yet.\n\nPress 'i' to enter insert mode and type a message."
			} else {
				// Show last 10 messages
				start := len(history) - 10
				if start < 0 {
					start = 0
				}
				var sb strings.Builder
				for _, msg := range history[start:] {
					from := msg.From
					if msg.Outgoing {
						from = "You"
					}
					sb.WriteString(from + ": " + truncate(msg.Body, 40) + "\n")
				}
				content = sb.String()
			}
		}

	case FocusRoster:
		// Show selected contact info
		title = "Contact Info"
		if m.roster.FocusSection() == roster.SectionAccounts {
			title = "Account Info"
			jid := m.roster.SelectedAccountJID()
			if jid == "" {
				content = "No account selected.\n\nUse j/k to navigate,\ngA to focus accounts section."
			} else {
				accounts := m.app.GetConnectedAccounts()
				for _, acc := range accounts {
					if acc.JID == jid {
						content = "JID: " + acc.JID + "\n"
						content += "Status: " + acc.Status + "\n"
						content += "Unread: " + strconv.Itoa(acc.Unread)
						break
					}
				}
			}
		} else {
			jid := m.roster.SelectedJID()
			if jid == "" {
				content = "No contact selected.\n\nUse j/k to navigate,\nEnter to open chat."
			} else {
				contacts := m.app.GetContacts()
				for _, c := range contacts {
					if c.JID == jid {
						content = "JID: " + c.JID + "\n"
						if c.Name != "" {
							content += "Name: " + c.Name + "\n"
						}
						content += "Status: " + c.Status + "\n"
						if c.StatusMsg != "" {
							content += "Message: " + c.StatusMsg + "\n"
						}
						content += "Unread: " + strconv.Itoa(c.Unread)
						break
					}
				}
			}
		}

	case FocusAccounts:
		// Show account details
		title = "Account Details"
		jid := m.roster.SelectedAccountJID()
		if jid == "" {
			content = "No account selected."
		} else {
			accounts := m.app.GetConnectedAccounts()
			for _, acc := range accounts {
				if acc.JID == jid {
					content = "JID: " + acc.JID + "\n"
					content += "Status: " + acc.Status + "\n"
					content += "Unread: " + strconv.Itoa(acc.Unread)
					break
				}
			}
		}

	default:
		// General help
		title = "Quick Reference"
		content = "Navigation:\n"
		content += "  j/k     Move down/up\n"
		content += "  gr      Focus roster\n"
		content += "  gc      Focus chat\n"
		content += "  gA      Focus accounts\n"
		content += "  gl      Toggle account list\n"
		content += "  H       Show this help\n\n"
		content += "Actions:\n"
		content += "  i       Insert mode\n"
		content += "  :       Command mode\n"
		content += "  /       Search\n"
		content += "  Enter   Open/select\n"
		content += "  Esc     Exit mode"
	}

	m.dialog = m.dialog.ShowContextHelp(title, content)
	m.focus = FocusDialog
}

// truncate truncates a string to maxLen and adds ellipsis
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// updateFocusedComponent sends the key message to the focused component
func (m *Model) updateFocusedComponent(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	switch m.focus {
	case FocusChat:
		var cmd tea.Cmd
		m.chat, cmd = m.chat.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case FocusCommandLine:
		var cmd tea.Cmd
		m.commandline, cmd = m.commandline.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case FocusRoster:
		var cmd tea.Cmd
		m.roster, cmd = m.roster.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return cmds
}

// handleAppEvent handles events from the application layer
func (m *Model) handleAppEvent(event app.EventMsg) tea.Cmd {
	switch event.Type {
	case app.EventRosterUpdate:
		contacts := m.app.GetContacts()
		m.roster = m.roster.SetContacts(contacts)

	case app.EventMessage:
		if msg, ok := event.Data.(app.ChatMessage); ok {
			m.chat = m.chat.AddMessage(msg)
		}

	case app.EventPresence:
		if presence, ok := event.Data.(app.PresenceUpdate); ok {
			m.roster = m.roster.UpdatePresence(presence.JID, presence.Status)
		}
		// Check if we're connecting
		if m.app.Status() == "connecting" {
			m.chat = m.chat.SetStatusMsg("Connecting to " + m.app.CurrentAccount() + "...")
		}

	case app.EventConnected:
		m.statusbar = m.statusbar.SetConnected(true)
		m.chat = m.chat.ClearStatusMsg()

	case app.EventDisconnected:
		m.statusbar = m.statusbar.SetConnected(false)
		m.chat = m.chat.ClearStatusMsg()

	case app.EventError:
		if errMsg, ok := event.Data.(string); ok {
			m.dialog = m.dialog.ShowError(errMsg)
			m.focus = FocusDialog
		}
	}

	return nil
}

// executeCommand executes a command from the command line
func (m *Model) executeCommand(cmd string, args []string) tea.Cmd {
	return m.app.ExecuteCommand(cmd, args)
}

// openChat opens a chat window for the given JID
func (m *Model) openChat(jid string) {
	m.windows = m.windows.OpenChat(jid)
	history := m.app.GetChatHistory(jid)
	m.chat = m.chat.SetJID(jid)
	m.chat = m.chat.SetHistory(history)
}

// loadActiveWindow loads the content for the active window
func (m *Model) loadActiveWindow() {
	jid := m.windows.ActiveJID()
	if jid != "" {
		history := m.app.GetChatHistory(jid)
		m.chat = m.chat.SetJID(jid)
		m.chat = m.chat.SetHistory(history)
	} else {
		// Console window - clear chat
		m.chat = m.chat.SetJID("")
		m.chat = m.chat.SetHistory(nil)
	}
}

// updateComponentSizes updates component dimensions based on window size
func (m *Model) updateComponentSizes() {
	rosterWidth := m.app.Config().UI.RosterWidth
	if !m.showRoster {
		rosterWidth = 0
	}
	chatWidth := m.width - rosterWidth

	statusHeight := 1
	cmdHeight := 1
	mainHeight := m.height - statusHeight - cmdHeight

	m.roster = m.roster.SetSize(rosterWidth, mainHeight)
	m.chat = m.chat.SetSize(chatWidth, mainHeight)
	m.statusbar = m.statusbar.SetWidth(m.width)
	m.commandline = m.commandline.SetWidth(m.width)
}

// getAccountDisplays converts connected accounts to roster display format
func (m Model) getAccountDisplays() []roster.AccountDisplay {
	accounts := m.app.GetConnectedAccounts()
	displays := make([]roster.AccountDisplay, len(accounts))
	for i, acc := range accounts {
		displays[i] = roster.AccountDisplay{
			JID:    acc.JID,
			Status: acc.Status,
			Unread: acc.Unread,
		}
	}
	return displays
}

// getWindowInfos returns window information for the status bar
func (m *Model) getWindowInfos() []statusbar.WindowInfo {
	wins := m.windows.GetWindows()
	activeNum := m.windows.ActiveNum()

	infos := make([]statusbar.WindowInfo, len(wins))
	for i, w := range wins {
		title := w.Title
		if title == "" {
			title = w.JID
		}
		// Extract just the username part from JID
		if idx := strings.Index(title, "@"); idx > 0 {
			title = title[:idx]
		}

		infos[i] = statusbar.WindowInfo{
			Num:    i + 1, // 1-indexed for display
			Title:  title,
			Active: i == activeNum,
			Unread: w.Unread,
		}
	}
	return infos
}

// saveWindows saves the current window state
func (m *Model) saveWindows() {
	wins := m.windows.GetWindows()
	activeNum := m.windows.ActiveNum()

	states := make([]app.WindowState, len(wins))
	for i, w := range wins {
		windowType := "console"
		switch w.Type {
		case windows.WindowChat:
			windowType = "chat"
		case windows.WindowMUC:
			windowType = "muc"
		case windows.WindowPrivate:
			windowType = "private"
		}

		states[i] = app.WindowState{
			Type:   windowType,
			JID:    w.JID,
			Title:  w.Title,
			Active: i == activeNum,
		}
	}

	_ = m.app.SaveWindowState(states)
}

// loadWindows loads the saved window state
func (m *Model) loadWindows() {
	states, err := m.app.LoadWindowState()
	if err != nil || len(states) == 0 {
		return
	}

	// Restore windows
	activeIdx := 0
	for i, state := range states {
		if state.Type == "console" {
			continue // Console is always window 0
		}

		if state.JID != "" {
			switch state.Type {
			case "chat", "private":
				m.windows = m.windows.OpenChat(state.JID)
			case "muc":
				m.windows = m.windows.OpenMUC(state.JID, "")
			}
		}

		if state.Active {
			activeIdx = i
		}
	}

	// Switch to the active window
	if activeIdx > 0 {
		m.windows = m.windows.GoTo(activeIdx)
		m.loadActiveWindow()
	}
}

// overlayDialog overlays the dialog on top of the main view
func (m *Model) overlayDialog(base string) string {
	dialogView := m.dialog.View()

	// Center the dialog
	dialogWidth := 50
	dialogHeight := 10

	x := (m.width - dialogWidth) / 2
	y := (m.height - dialogHeight) / 2

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		dialogView,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("0")),
	)
}

// overlaySettings overlays the settings menu on top of the main view
func (m *Model) overlaySettings(base string) string {
	settingsView := m.settings.View()

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		settingsView,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("0")),
	)
}

// handleCommandAction handles command actions that need UI interaction
func (m *Model) handleCommandAction(msg app.CommandActionMsg) {
	switch msg.Action {
	case app.ActionShowHelp:
		m.dialog = m.dialog.ShowHelp(nil)
		m.focus = FocusDialog

	case app.ActionShowAccountList:
		appAccounts := m.app.GetAccountInfos()
		accounts := make([]dialogs.AccountInfo, len(appAccounts))
		for i, a := range appAccounts {
			accounts[i] = dialogs.AccountInfo{JID: a.JID, Session: a.Session}
		}
		m.dialog = m.dialog.ShowAccountList(accounts, m.app.CurrentAccount())
		m.focus = FocusDialog

	case app.ActionShowAccountAdd:
		m.dialog = m.dialog.ShowAccountAdd()
		m.focus = FocusDialog

	case app.ActionShowAccountEdit:
		if jid, ok := msg.Data["jid"].(string); ok {
			acc := m.app.GetAccount(jid)
			if acc != nil {
				m.dialog = m.dialog.ShowAccountEdit(acc.JID, acc.Server, acc.Port)
				m.focus = FocusDialog
			}
		}

	case app.ActionShowPassword:
		if jid, ok := msg.Data["jid"].(string); ok {
			m.dialog = m.dialog.ShowPassword(jid)
			m.focus = FocusDialog
		}

	case app.ActionShowSettings:
		settings := m.app.GetSettings()
		m.dialog = m.dialog.ShowSettingsList(settings)
		m.focus = FocusDialog

	case app.ActionSwitchWindow:
		if winStr, ok := msg.Data["window"].(string); ok {
			if win, err := strconv.Atoi(winStr); err == nil && win >= 1 && win <= 20 {
				var ok bool
				m.windows, ok = m.windows.GoToResult(win - 1)
				if ok {
					m.loadActiveWindow()
				} else {
					m.dialog = m.dialog.ShowError("Window " + winStr + " does not exist. Open a chat first with Enter on a contact.")
					m.focus = FocusDialog
				}
			}
		}

	case app.ActionWindowNext:
		m.windows = m.windows.Next()
		m.loadActiveWindow()

	case app.ActionWindowPrev:
		m.windows = m.windows.Prev()
		m.loadActiveWindow()

	case app.ActionSaveWindows:
		m.saveWindows()

	case app.ActionLoadWindows:
		m.loadWindows()

	case app.ActionShowRegister:
		m.dialog = m.dialog.ShowRegister()
		m.focus = FocusDialog
	}
}

// handleDialogResult handles dialog results
func (m *Model) handleDialogResult(result dialogs.DialogResult) tea.Cmd {
	switch result.Type {
	case dialogs.DialogAccountAdd:
		if result.Confirmed {
			jid := result.Values["jid"]
			password := result.Values["password"]
			server := result.Values["server"]
			portStr := result.Values["port"]
			port := 5222
			if portStr != "" {
				if p, err := strconv.Atoi(portStr); err == nil {
					port = p
				}
			}

			acc := config.Account{
				JID:         jid,
				Password:    password,
				Server:      server,
				Port:        port,
				AutoConnect: true,
				OMEMO:       true,
				Resource:    "roster",
			}
			m.app.AddAccount(acc)
		}

	case dialogs.DialogAccountEdit:
		if result.Confirmed {
			jid := result.Values["jid"]
			password := result.Values["password"]
			server := result.Values["server"]
			portStr := result.Values["port"]
			port := 5222
			if portStr != "" {
				if p, err := strconv.Atoi(portStr); err == nil {
					port = p
				}
			}

			acc := m.app.GetAccount(result.Values["original_jid"])
			if acc != nil {
				acc.JID = jid
				if password != "" {
					acc.Password = password
				}
				acc.Server = server
				acc.Port = port
				m.app.AddAccount(*acc)
			}
		}

	case dialogs.DialogPassword:
		if result.Confirmed {
			jid := result.Values["jid"]
			password := result.Values["password"]
			acc := m.app.GetAccount(jid)
			if acc != nil {
				acc.Password = password
				if acc.Session {
					m.app.AddSessionAccount(*acc)
				} else {
					m.app.AddAccount(*acc)
				}
			} else {
				// Create a new session-only account
				newAcc := config.Account{
					JID:      jid,
					Password: password,
					Port:     5222,
					Resource: "roster",
					OMEMO:    true,
					Session:  true,
				}
				m.app.AddSessionAccount(newAcc)
			}
			// Now try to connect
			return m.app.ExecuteCommand("connect", []string{jid})
		}

	case dialogs.DialogAddContact:
		if result.Confirmed {
			// TODO: Add contact via XMPP
		}

	case dialogs.DialogJoinRoom:
		if result.Confirmed {
			// TODO: Join room via XMPP
		}

	case dialogs.DialogRegister:
		if result.Confirmed {
			server := result.Values["server"]
			portStr := result.Values["port"]
			port := 5222
			if portStr != "" {
				if p, err := strconv.Atoi(portStr); err == nil {
					port = p
				}
			}
			if server != "" {
				return m.app.FetchRegistrationForm(server, port)
			}
		}

	case dialogs.DialogRegisterForm:
		// Handle 'V' key for viewing CAPTCHA
		if result.Action == dialogs.ActionViewCaptcha {
			// View CAPTCHA - open the media
			errMsg := m.openCaptcha(result.Values["_captchaURL"])
			if errMsg != "" {
				// Failed to open - show error with instructions
				m.dialog = m.dialog.ShowError(errMsg)
				m.focus = FocusDialog
			}
			// Dialog stays open - user can continue filling the form
			return nil
		}

		// Handle 'C' key for copying CAPTCHA URL
		if result.Action == dialogs.ActionCopyURL {
			url := result.Values["_captchaURL"]
			if url != "" {
				if err := m.copyToClipboard(url); err != nil {
					m.dialog = m.dialog.ShowError("Failed to copy URL: " + err.Error() + "\n\nURL: " + url)
					m.focus = FocusDialog
				} else {
					// Show success feedback temporarily
					m.dialog = m.dialog.SetViewerStatus("Copied!")
					// Return a command to restore after 1 second
					return tea.Tick(time.Second, func(t time.Time) tea.Msg {
						return restoreViewerMsg{}
					})
				}
				// Dialog stays open - user can continue filling the form
			}
			return nil
		}

		if result.Confirmed {
			server := result.Values["server"]
			portStr := result.Values["port"]
			port := 5222
			if portStr != "" {
				if p, err := strconv.Atoi(portStr); err == nil {
					port = p
				}
			}

			// Get data form info
			isDataForm := result.Values["_isDataForm"] == "true"
			formType := result.Values["_formType"]

			// Build fields map, including hidden fields
			fields := make(map[string]string)
			for k, v := range result.Values {
				// Skip internal metadata fields (start with underscore but not _hidden_)
				if strings.HasPrefix(k, "_") && !strings.HasPrefix(k, "_hidden_") {
					continue
				}
				// Skip the captcha viewer field
				if k == "_captcha_viewer" {
					continue
				}
				// Include hidden fields (prefixed with _hidden_)
				if strings.HasPrefix(k, "_hidden_") {
					fields[strings.TrimPrefix(k, "_hidden_")] = v
				} else {
					fields[k] = v
				}
			}
			return m.app.SubmitRegistration(server, port, fields, isDataForm, formType)
		}

	case dialogs.DialogRegisterSuccess:
		jid := result.Values["jid"]
		password := result.Values["password"]
		server := result.Values["server"]
		portStr := result.Values["port"]
		port := 5222
		if portStr != "" {
			if p, err := strconv.Atoi(portStr); err == nil {
				port = p
			}
		}

		// Button 0 = Save & Connect, Button 1 = Save Only, Button 2 = Close
		switch result.Button {
		case 0: // Save & Connect
			acc := config.Account{
				JID:         jid,
				Password:    password,
				Server:      server,
				Port:        port,
				AutoConnect: true,
				OMEMO:       true,
				Resource:    "roster",
			}
			m.app.AddAccount(acc)
			return m.app.ExecuteCommand("connect", []string{jid})

		case 1: // Save Only
			acc := config.Account{
				JID:         jid,
				Password:    password,
				Server:      server,
				Port:        port,
				AutoConnect: false,
				OMEMO:       true,
				Resource:    "roster",
			}
			m.app.AddAccount(acc)
		}
		// Button 2 (Close) just closes the dialog without saving
	}

	m.focus = FocusRoster
	return nil
}

// openCaptcha opens a CAPTCHA media for the user to view/play
// It handles URLs, data: URIs, and raw embedded data
// Returns an error message if the CAPTCHA couldn't be opened, or empty string on success
func (m *Model) openCaptcha(url string) string {
	// Use stored URL if not provided directly (CID URIs are filtered out earlier)
	if url == "" || strings.HasPrefix(url, "cid:") {
		url = m.captchaURL
	}

	if url != "" {
		// Open URL in browser/media player
		if m.openURL(url) {
			return ""
		}
		return "No application found to open media.\n\nPlease manually open this URL:\n" + url
	}

	// If we have raw media data, save it to a temp file and open
	if len(m.captchaData) > 0 {
		// Determine file extension and media type from MIME type
		ext, mediaType := getExtensionForMime(m.captchaMime)

		// Create temp file
		tmpDir := os.TempDir()
		tmpFile := filepath.Join(tmpDir, "roster-captcha"+ext)

		if err := os.WriteFile(tmpFile, m.captchaData, 0600); err != nil {
			return "Failed to save CAPTCHA " + mediaType + ": " + err.Error()
		}

		// Open with system viewer/player
		if m.openFile(tmpFile) {
			return ""
		}
		return "No " + mediaType + " viewer found.\n\nCAPTCHA " + mediaType + " saved to:\n" + tmpFile + "\n\nPlease open it manually."
	}

	return "No CAPTCHA data available."
}

// getExtensionForMime returns the file extension and media type description for a MIME type
func getExtensionForMime(mimeType string) (ext string, mediaType string) {
	// Default values
	ext = ".bin"
	mediaType = "file"

	// Normalize MIME type (remove parameters)
	if idx := strings.Index(mimeType, ";"); idx > 0 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}

	switch mimeType {
	// Images
	case "image/png":
		return ".png", "image"
	case "image/jpeg", "image/jpg":
		return ".jpg", "image"
	case "image/gif":
		return ".gif", "image"
	case "image/webp":
		return ".webp", "image"
	case "image/bmp":
		return ".bmp", "image"
	case "image/svg+xml":
		return ".svg", "image"

	// Audio
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav", "audio"
	case "audio/mpeg", "audio/mp3":
		return ".mp3", "audio"
	case "audio/ogg":
		return ".ogg", "audio"
	case "audio/webm":
		return ".webm", "audio"
	case "audio/flac":
		return ".flac", "audio"
	case "audio/aac":
		return ".aac", "audio"
	case "audio/x-speex", "audio/speex", "audio/ogg-speex", "audio/ogg; codecs=speex":
		return ".spx", "audio"

	// Video
	case "video/mp4":
		return ".mp4", "video"
	case "video/mpeg":
		return ".mpg", "video"
	case "video/webm":
		return ".webm", "video"
	case "video/ogg":
		return ".ogv", "video"
	case "video/quicktime":
		return ".mov", "video"
	case "video/x-msvideo":
		return ".avi", "video"

	default:
		// Try to determine from prefix
		if strings.HasPrefix(mimeType, "image/") {
			return ".png", "image"
		}
		if strings.HasPrefix(mimeType, "audio/") {
			return ".wav", "audio"
		}
		if strings.HasPrefix(mimeType, "video/") {
			return ".mp4", "video"
		}
		return ".bin", "media"
	}
}

// openURL opens a URL in the default browser
// Returns true if successful, false if no browser was found
func (m *Model) openURL(url string) bool {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start() == nil
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start() == nil
	case "linux", "freebsd", "openbsd", "netbsd":
		// Try multiple openers in order of preference
		openers := [][]string{
			{"xdg-open", url},
			{"sensible-browser", url},
			{"x-www-browser", url},
			{"gnome-open", url},
			{"kde-open", url},
			{"firefox", url},
			{"chromium", url},
			{"chromium-browser", url},
			{"google-chrome", url},
			{"google-chrome-stable", url},
		}
		for _, opener := range openers {
			if path, err := exec.LookPath(opener[0]); err == nil {
				if exec.Command(path, opener[1:]...).Start() == nil {
					return true
				}
			}
		}
	}
	return false
}

// openFile opens a file with the default system application
// Returns true if successful, false if no viewer was found
func (m *Model) openFile(path string) bool {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Start() == nil
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start() == nil
	case "linux", "freebsd", "openbsd", "netbsd":
		// Try multiple openers in order of preference
		// First try generic openers, then image-specific viewers
		openers := [][]string{
			{"xdg-open", path},
			{"sensible-browser", path},
			{"gnome-open", path},
			{"kde-open", path},
			// Image viewers as fallback
			{"eog", path},           // GNOME Eye of GNOME
			{"gwenview", path},      // KDE
			{"feh", path},           // Lightweight
			{"sxiv", path},          // Simple X Image Viewer
			{"imv", path},           // Wayland-native
			{"display", path},       // ImageMagick
		}
		for _, opener := range openers {
			if execPath, err := exec.LookPath(opener[0]); err == nil {
				if exec.Command(execPath, opener[1:]...).Start() == nil {
					return true
				}
			}
		}
	}
	return false
}

// copyToClipboard copies text to the system clipboard
func (m *Model) copyToClipboard(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "windows":
		cmd = exec.Command("cmd", "/c", "clip")
	case "linux", "freebsd", "openbsd", "netbsd":
		// Try xclip first, then xsel, then wl-copy (Wayland)
		if path, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command(path, "-selection", "clipboard")
		} else if path, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command(path, "--clipboard", "--input")
		} else if path, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command(path)
		} else {
			return fmt.Errorf("no clipboard tool found (install xclip, xsel, or wl-copy)")
		}
	default:
		return fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
