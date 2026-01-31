package dialogs

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/meszmate/roster/internal/ui/theme"
)

// DialogType represents the type of dialog
type DialogType int

const (
	DialogNone DialogType = iota
	DialogError
	DialogConfirm
	DialogInputType
	DialogAddContact
	DialogJoinRoom
	DialogContactInfo
	DialogFingerprint
	DialogSubscription
	DialogHelp
	DialogAccountAdd
	DialogAccountEdit
	DialogAccountList
	DialogPassword
	DialogSettings
	DialogContextHelp
	DialogAccountRemove
)

// DialogResult is sent when a dialog is closed
type DialogResult struct {
	Type      DialogType
	Confirmed bool
	Values    map[string]string
}

// Model represents the dialog component
type Model struct {
	dialogType  DialogType
	title       string
	message     string
	inputs      []DialogInput
	activeInput int
	buttons     []string
	activeBtn   int
	width       int
	height      int
	styles      *theme.Styles
	data        map[string]string
}

// DialogInput represents an input field in a dialog
type DialogInput struct {
	Label    string
	Key      string
	Value    string
	Cursor   int
	Password bool
}

// New creates a new dialog model
func New(styles *theme.Styles) Model {
	return Model{
		dialogType: DialogNone,
		styles:     styles,
		buttons:    []string{"OK", "Cancel"},
		data:       make(map[string]string),
	}
}

// Active returns whether a dialog is active
func (m Model) Active() bool {
	return m.dialogType != DialogNone
}

// Type returns the dialog type
func (m Model) Type() DialogType {
	return m.dialogType
}

// ShowError shows an error dialog
func (m Model) ShowError(message string) Model {
	m.dialogType = DialogError
	m.title = "Error"
	m.message = message
	m.buttons = []string{"OK"}
	m.activeBtn = 0
	m.inputs = nil
	return m
}

// ShowConfirm shows a confirmation dialog
func (m Model) ShowConfirm(title, message string) Model {
	m.dialogType = DialogConfirm
	m.title = title
	m.message = message
	m.buttons = []string{"Yes", "No"}
	m.activeBtn = 0
	m.inputs = nil
	return m
}

// ShowAddContact shows the add contact dialog
func (m Model) ShowAddContact() Model {
	m.dialogType = DialogAddContact
	m.title = "Add Contact"
	m.message = ""
	m.inputs = []DialogInput{
		{Label: "JID", Key: "jid", Value: ""},
		{Label: "Name", Key: "name", Value: ""},
		{Label: "Group", Key: "group", Value: ""},
	}
	m.activeInput = 0
	m.buttons = []string{"Add", "Cancel"}
	m.activeBtn = 0
	return m
}

// ShowJoinRoom shows the join room dialog
func (m Model) ShowJoinRoom() Model {
	m.dialogType = DialogJoinRoom
	m.title = "Join Room"
	m.message = ""
	m.inputs = []DialogInput{
		{Label: "Room JID", Key: "room", Value: ""},
		{Label: "Nickname", Key: "nick", Value: ""},
		{Label: "Password", Key: "password", Value: "", Password: true},
	}
	m.activeInput = 0
	m.buttons = []string{"Join", "Cancel"}
	m.activeBtn = 0
	return m
}

// ShowContactInfo shows contact info dialog
func (m Model) ShowContactInfo(jid string) Model {
	m.dialogType = DialogContactInfo
	m.title = "Contact Info"
	m.message = jid
	m.buttons = []string{"Close"}
	m.activeBtn = 0
	m.inputs = nil
	m.data["jid"] = jid
	return m
}

// ShowFingerprint shows fingerprint verification dialog
func (m Model) ShowFingerprint(jid string, fingerprints []string) Model {
	m.dialogType = DialogFingerprint
	m.title = "Verify Fingerprint"
	m.message = "Fingerprints for " + jid + ":\n\n" + strings.Join(fingerprints, "\n")
	m.buttons = []string{"Trust", "Untrust", "Close"}
	m.activeBtn = 0
	m.inputs = nil
	m.data["jid"] = jid
	return m
}

// ShowSubscription shows subscription request dialog
func (m Model) ShowSubscription(jid string) Model {
	m.dialogType = DialogSubscription
	m.title = "Subscription Request"
	m.message = jid + " wants to add you to their roster"
	m.buttons = []string{"Accept", "Deny"}
	m.activeBtn = 0
	m.inputs = nil
	m.data["jid"] = jid
	return m
}

// ShowHelp shows the help dialog with all commands
func (m Model) ShowHelp(commands map[string]string) Model {
	m.dialogType = DialogHelp
	m.title = "Help - Available Commands"

	// Build help message
	var sb strings.Builder
	sb.WriteString("Keybindings (Normal Mode):\n")
	sb.WriteString("  j/k       Move down/up\n")
	sb.WriteString("  gg/G      Top/bottom\n")
	sb.WriteString("  Ctrl+u/d  Half page up/down\n")
	sb.WriteString("  /         Search\n")
	sb.WriteString("  n/N       Next/prev search result\n")
	sb.WriteString("  :         Command mode\n")
	sb.WriteString("  i         Insert mode (chat)\n")
	sb.WriteString("  Enter     Open chat\n")
	sb.WriteString("  q         Close chat\n")
	sb.WriteString("  Esc       Back to normal mode\n")
	sb.WriteString("  H         Context help popup\n")
	sb.WriteString("\nFocus (g prefix):\n")
	sb.WriteString("  gr        Focus roster\n")
	sb.WriteString("  gc        Focus chat\n")
	sb.WriteString("  gA        Focus accounts\n")
	sb.WriteString("  gl        Toggle account list\n")
	sb.WriteString("\nAccount Actions (in accounts section):\n")
	sb.WriteString("  H         Show account info tooltip\n")
	sb.WriteString("  C         Connect account\n")
	sb.WriteString("  D         Disconnect account\n")
	sb.WriteString("  E         Edit account\n")
	sb.WriteString("  X         Remove account\n")
	sb.WriteString("\nActions (g prefix):\n")
	sb.WriteString("  ga        Add contact\n")
	sb.WriteString("  gx        Remove contact\n")
	sb.WriteString("  gR        Rename contact\n")
	sb.WriteString("  gj        Join room\n")
	sb.WriteString("  gs/S      Settings\n")
	sb.WriteString("  gw        Save windows\n")
	sb.WriteString("\nWindows:\n")
	sb.WriteString("  Alt+1-0   Windows 1-10\n")
	sb.WriteString("  Tab       Next window\n")
	sb.WriteString("\nCommands (press : first):\n")

	// Add command summaries
	cmdList := []string{
		"connect <jid> <pass> [server] [port]",
		"account add    - Add saved account",
		"account edit <jid> - Edit account",
		"account resource <jid> <name> - Set resource",
		"disconnect     - Disconnect",
		"1-20           - Switch window",
		"set <k> <v>    - Change setting",
		"quit           - Exit",
	}
	for _, cmd := range cmdList {
		sb.WriteString("  :" + cmd + "\n")
	}
	sb.WriteString("\nThemes: matrix, nord, gruvbox, dracula, rainbow")

	m.message = sb.String()
	m.buttons = []string{"Close"}
	m.activeBtn = 0
	m.inputs = nil
	return m
}

// ShowAccountAdd shows the add account dialog
func (m Model) ShowAccountAdd() Model {
	m.dialogType = DialogAccountAdd
	m.title = "Add Account"
	m.message = ""
	m.inputs = []DialogInput{
		{Label: "JID (user@server.com)", Key: "jid", Value: ""},
		{Label: "Password", Key: "password", Value: "", Password: true},
		{Label: "Server (optional)", Key: "server", Value: ""},
		{Label: "Port (default: 5222)", Key: "port", Value: ""},
		{Label: "Resource (default: roster)", Key: "resource", Value: ""},
	}
	m.activeInput = 0
	m.buttons = []string{"Add", "Cancel"}
	m.activeBtn = 0
	return m
}

// ShowAccountEdit shows the edit account dialog
func (m Model) ShowAccountEdit(jid, server string, port int, resource string) Model {
	m.dialogType = DialogAccountEdit
	m.title = "Edit Account"
	m.message = ""
	portStr := ""
	if port > 0 && port != 5222 {
		portStr = strconv.Itoa(port)
	}
	m.inputs = []DialogInput{
		{Label: "JID", Key: "jid", Value: jid},
		{Label: "New Password (leave empty to keep)", Key: "password", Value: "", Password: true},
		{Label: "Server", Key: "server", Value: server},
		{Label: "Port", Key: "port", Value: portStr},
		{Label: "Resource", Key: "resource", Value: resource},
	}
	m.activeInput = 0
	m.buttons = []string{"Save", "Cancel"}
	m.activeBtn = 0
	m.data["original_jid"] = jid
	return m
}

// AccountInfo holds account info for display
type AccountInfo struct {
	JID     string
	Session bool
}

// ShowAccountList shows the account list dialog
func (m Model) ShowAccountList(accounts []AccountInfo, currentAccount string) Model {
	m.dialogType = DialogAccountList
	m.title = "Accounts"

	var sb strings.Builder
	if len(accounts) == 0 {
		sb.WriteString("No accounts configured.\n\n")
		sb.WriteString("Quick connect (session only):\n")
		sb.WriteString("  :connect user@server.com password\n\n")
		sb.WriteString("Add saved account:\n")
		sb.WriteString("  :account add")
	} else {
		for _, acc := range accounts {
			prefix := "  "
			suffix := ""
			if acc.JID == currentAccount {
				prefix = "→ "
				suffix = " (active)"
			}
			if acc.Session {
				suffix += " [session]"
			} else {
				suffix += " [saved]"
			}
			sb.WriteString(prefix + acc.JID + suffix + "\n")
		}
		sb.WriteString("\nCommands:\n")
		sb.WriteString("  :connect <jid> <pass> - Session connect\n")
		sb.WriteString("  :account add          - Add saved account\n")
		sb.WriteString("  :account remove <jid> - Remove account")
	}

	m.message = sb.String()
	m.buttons = []string{"Close"}
	m.activeBtn = 0
	m.inputs = nil
	return m
}

// ShowPassword shows a password prompt dialog
func (m Model) ShowPassword(jid string) Model {
	m.dialogType = DialogPassword
	m.title = "Enter Password"
	m.message = "Password for " + jid
	m.inputs = []DialogInput{
		{Label: "Password", Key: "password", Value: "", Password: true},
	}
	m.activeInput = 0
	m.buttons = []string{"Connect", "Cancel"}
	m.activeBtn = 0
	m.data["jid"] = jid
	return m
}

// ShowSettingsList shows available settings
func (m Model) ShowSettingsList(settings map[string]string) Model {
	m.dialogType = DialogSettings
	m.title = "Settings"

	var sb strings.Builder
	sb.WriteString("Current Settings:\n\n")

	// List common settings
	settingsList := []struct{ key, desc string }{
		{"theme", "UI theme (rainbow, matrix, nord, gruvbox, dracula)"},
		{"roster_width", "Roster panel width"},
		{"roster_position", "Roster position (left, right)"},
		{"show_timestamps", "Show message timestamps"},
		{"time_format", "Time format (e.g., 15:04)"},
		{"notifications", "Desktop notifications"},
		{"encryption", "Default encryption (omemo, none)"},
		{"require_encryption", "Require encryption"},
	}

	for _, s := range settingsList {
		val := settings[s.key]
		if val == "" {
			val = "(default)"
		}
		sb.WriteString("  " + s.key + " = " + val + "\n")
		sb.WriteString("    " + s.desc + "\n")
	}

	sb.WriteString("\nUse :set <setting> <value> to change")

	m.message = sb.String()
	m.buttons = []string{"Close"}
	m.activeBtn = 0
	m.inputs = nil
	return m
}

// ShowContextHelp shows context-sensitive help based on current focus
func (m Model) ShowContextHelp(context string, content string) Model {
	m.dialogType = DialogContextHelp
	m.title = context
	m.message = content
	m.buttons = []string{"Close"}
	m.activeBtn = 0
	m.inputs = nil
	return m
}

// ShowAccountRemoveConfirm shows a confirmation dialog for removing an account
func (m Model) ShowAccountRemoveConfirm(jid string, isSession bool) Model {
	m.dialogType = DialogAccountRemove
	m.title = "Remove Account"

	accountType := "saved"
	if isSession {
		accountType = "session"
	}

	m.message = "Are you sure you want to remove this " + accountType + " account?\n\n" +
		"  " + jid + "\n\n" +
		"This action cannot be undone."

	m.buttons = []string{"Remove", "Cancel"}
	m.activeBtn = 1 // Default to Cancel for safety
	m.inputs = nil
	m.data["jid"] = jid
	return m
}

// Hide hides the dialog
func (m Model) Hide() Model {
	m.dialogType = DialogNone
	m.inputs = nil
	m.data = make(map[string]string)
	return m
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.Active() {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			// Cancel dialog
			result := DialogResult{
				Type:      m.dialogType,
				Confirmed: false,
			}
			m = m.Hide()
			return m, func() tea.Msg { return result }

		case tea.KeyTab:
			// Cycle through inputs/buttons
			if len(m.inputs) > 0 {
				m.activeInput++
				if m.activeInput >= len(m.inputs) {
					m.activeInput = 0
				}
			}

		case tea.KeyShiftTab:
			// Cycle backwards
			if len(m.inputs) > 0 {
				m.activeInput--
				if m.activeInput < 0 {
					m.activeInput = len(m.inputs) - 1
				}
			}

		case tea.KeyLeft:
			if len(m.inputs) == 0 || m.activeInput >= len(m.inputs) {
				// Navigate buttons
				m.activeBtn--
				if m.activeBtn < 0 {
					m.activeBtn = len(m.buttons) - 1
				}
			} else if m.inputs[m.activeInput].Cursor > 0 {
				m.inputs[m.activeInput].Cursor--
			}

		case tea.KeyRight:
			if len(m.inputs) == 0 || m.activeInput >= len(m.inputs) {
				// Navigate buttons
				m.activeBtn++
				if m.activeBtn >= len(m.buttons) {
					m.activeBtn = 0
				}
			} else if m.inputs[m.activeInput].Cursor < len(m.inputs[m.activeInput].Value) {
				m.inputs[m.activeInput].Cursor++
			}

		case tea.KeyEnter:
			// Confirm action
			values := make(map[string]string)
			for _, input := range m.inputs {
				values[input.Key] = input.Value
			}
			for k, v := range m.data {
				values[k] = v
			}

			confirmed := m.activeBtn == 0 // First button is confirm
			result := DialogResult{
				Type:      m.dialogType,
				Confirmed: confirmed,
				Values:    values,
			}
			m = m.Hide()
			return m, func() tea.Msg { return result }

		case tea.KeyBackspace:
			if len(m.inputs) > 0 && m.activeInput < len(m.inputs) {
				input := &m.inputs[m.activeInput]
				if input.Cursor > 0 {
					input.Value = input.Value[:input.Cursor-1] + input.Value[input.Cursor:]
					input.Cursor--
				}
			}

		case tea.KeyRunes:
			if len(m.inputs) > 0 && m.activeInput < len(m.inputs) {
				input := &m.inputs[m.activeInput]
				input.Value = input.Value[:input.Cursor] + string(msg.Runes) + input.Value[input.Cursor:]
				input.Cursor += len(msg.Runes)
			}

		case tea.KeySpace:
			if len(m.inputs) > 0 && m.activeInput < len(m.inputs) {
				input := &m.inputs[m.activeInput]
				input.Value = input.Value[:input.Cursor] + " " + input.Value[input.Cursor:]
				input.Cursor++
			}
		}
	}

	return m, nil
}

// View renders the dialog
func (m Model) View() string {
	if !m.Active() {
		return ""
	}

	var b strings.Builder

	// Title
	title := m.styles.DialogTitle.Render(m.title)
	b.WriteString(title)
	b.WriteString("\n\n")

	// Message
	if m.message != "" {
		b.WriteString(m.styles.DialogContent.Render(m.message))
		b.WriteString("\n\n")
	}

	// Inputs
	for i, input := range m.inputs {
		label := input.Label + ": "

		// Value with cursor
		value := input.Value
		if input.Password {
			value = strings.Repeat("*", len(value))
		}

		var rendered string
		if i == m.activeInput {
			// Show cursor
			beforeCursor := value
			cursorChar := " "
			afterCursor := ""
			if input.Cursor < len(value) {
				beforeCursor = value[:input.Cursor]
				cursorChar = string(value[input.Cursor])
				afterCursor = value[input.Cursor+1:]
			}
			cursor := lipgloss.NewStyle().Reverse(true).Render(cursorChar)
			rendered = m.styles.InputFocused.Render(label + beforeCursor + cursor + afterCursor)
		} else {
			rendered = m.styles.InputNormal.Render(label + value)
		}

		b.WriteString(rendered)
		b.WriteString("\n")
	}

	if len(m.inputs) > 0 {
		b.WriteString("\n")
	}

	// Buttons
	var buttons []string
	for i, btn := range m.buttons {
		var style lipgloss.Style
		if i == m.activeBtn {
			style = m.styles.DialogButtonActive
		} else {
			style = m.styles.DialogButton
		}
		buttons = append(buttons, style.Render(btn))
	}
	b.WriteString(strings.Join(buttons, "  "))

	// Wrap in border - use smaller size for context help
	content := b.String()
	width := 50
	padding := 1
	if m.dialogType == DialogContextHelp {
		width = 40 // Smaller width for context help
		padding = 0
	}
	return m.styles.DialogBorder.
		Width(width).
		Padding(padding, 2).
		Render(content)
}
