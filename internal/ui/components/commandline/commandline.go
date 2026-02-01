package commandline

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/meszmate/roster/internal/ui/theme"
)

// CommandMsg is sent when a command is executed
type CommandMsg struct {
	Command string
	Args    []string
}

// CancelMsg is sent when command mode should be exited (backspace on empty input)
type CancelMsg struct{}

// Command represents a registered command
type Command struct {
	Name        string
	Description string
	Args        []string
	Handler     func(args []string) tea.Cmd
}

// Model represents the command line component
type Model struct {
	input       string
	cursorPos   int
	prefix      string
	width       int
	styles      *theme.Styles
	commands    map[string]Command
	completions []string
	compIndex   int
	history     []string
	historyPos  int
}

// New creates a new command line model
func New(styles *theme.Styles) Model {
	m := Model{
		styles:     styles,
		prefix:     ":",
		commands:   make(map[string]Command),
		history:    []string{},
		historyPos: -1,
	}

	// Register default commands
	m.registerDefaultCommands()

	return m
}

// registerDefaultCommands registers the default set of commands
func (m *Model) registerDefaultCommands() {
	commands := []Command{
		// General
		{Name: "help", Description: "Show help for all commands or a specific command", Args: []string{"[command]"}},
		{Name: "quit", Description: "Quit the application", Args: []string{}},
		{Name: "q", Description: "Quit the application (alias)", Args: []string{}},

		// Account management
		{Name: "account", Description: "Manage accounts: list, add, remove, edit, default", Args: []string{"subcommand", "[args...]"}},
		{Name: "connect", Description: "Connect to an account (prompts for password if needed)", Args: []string{"[jid]"}},
		{Name: "disconnect", Description: "Disconnect from current account", Args: []string{}},

		// Settings
		{Name: "set", Description: "View or change settings: theme, roster_width, notifications, etc.", Args: []string{"[setting]", "[value]"}},
		{Name: "settings", Description: "Open settings menu", Args: []string{}},

		// Contacts
		{Name: "add", Description: "Add a contact to roster", Args: []string{"jid", "[name]"}},
		{Name: "remove", Description: "Remove a contact from roster", Args: []string{"jid"}},
		{Name: "rename", Description: "Rename a contact", Args: []string{"jid", "name"}},
		{Name: "info", Description: "Show contact info", Args: []string{"[jid]"}},
		{Name: "roster", Description: "Toggle roster panel visibility", Args: []string{}},

		// Messaging
		{Name: "msg", Description: "Send a message to a JID", Args: []string{"jid", "message"}},
		{Name: "clear", Description: "Clear current chat history", Args: []string{}},
		{Name: "close", Description: "Close current chat window", Args: []string{}},

		// Status
		{Name: "status", Description: "Set your status (online, away, dnd, xa, offline)", Args: []string{"status", "[message]"}},
		{Name: "away", Description: "Set away status with optional message", Args: []string{"[message]"}},
		{Name: "dnd", Description: "Set do-not-disturb status", Args: []string{"[message]"}},
		{Name: "xa", Description: "Set extended away status", Args: []string{"[message]"}},
		{Name: "online", Description: "Set online status", Args: []string{}},
		{Name: "offline", Description: "Go offline", Args: []string{}},

		// MUC (Multi-User Chat)
		{Name: "join", Description: "Join a MUC room", Args: []string{"room@server", "[nick]"}},
		{Name: "leave", Description: "Leave current room", Args: []string{}},
		{Name: "invite", Description: "Invite someone to current room", Args: []string{"jid"}},
		{Name: "kick", Description: "Kick user from room (moderator)", Args: []string{"nick", "[reason]"}},
		{Name: "ban", Description: "Ban user from room (admin)", Args: []string{"jid", "[reason]"}},
		{Name: "subject", Description: "Set room subject/topic", Args: []string{"text"}},
		{Name: "nick", Description: "Change your nickname in room", Args: []string{"nick"}},
		{Name: "affiliation", Description: "Set user affiliation (owner/admin/member/none)", Args: []string{"jid", "affiliation"}},
		{Name: "role", Description: "Set user role (moderator/participant/visitor)", Args: []string{"nick", "role"}},
		{Name: "bookmark", Description: "Manage room bookmarks: list, add, remove", Args: []string{"subcommand", "[args...]"}},

		// Encryption
		{Name: "omemo", Description: "OMEMO encryption: status, enable, disable, fingerprints", Args: []string{"subcommand", "[args...]"}},
		{Name: "fingerprint", Description: "Show OMEMO fingerprints for a contact", Args: []string{"[jid]"}},
		{Name: "trust", Description: "Trust an OMEMO fingerprint", Args: []string{"jid", "fingerprint"}},
		{Name: "untrust", Description: "Untrust an OMEMO fingerprint", Args: []string{"jid", "fingerprint"}},

		// Windows
		{Name: "window", Description: "Switch to window by number (1-20)", Args: []string{"number"}},
		{Name: "win", Description: "Switch to window (alias)", Args: []string{"number"}},

		// Plugins
		{Name: "plugins", Description: "List installed plugins", Args: []string{}},
		{Name: "plugin", Description: "Manage plugins: enable, disable, info", Args: []string{"subcommand", "name"}},
	}

	for _, cmd := range commands {
		m.commands[cmd.Name] = cmd
	}
}

// SetWidth sets the command line width
func (m Model) SetWidth(width int) Model {
	m.width = width
	return m
}

// SetPrefix sets the command line prefix
func (m Model) SetPrefix(prefix string) Model {
	m.prefix = prefix
	return m
}

// Clear clears the input
func (m Model) Clear() Model {
	m.input = ""
	m.cursorPos = 0
	m.completions = nil
	m.compIndex = 0
	return m
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyRunes:
			// Insert text at cursor
			m.input = m.input[:m.cursorPos] + string(msg.Runes) + m.input[m.cursorPos:]
			m.cursorPos += len(msg.Runes)
			m.completions = nil

		case tea.KeyBackspace:
			if m.cursorPos > 0 {
				m.input = m.input[:m.cursorPos-1] + m.input[m.cursorPos:]
				m.cursorPos--
				m.completions = nil
			} else if m.input == "" {
				// Backspace on empty input - exit command mode
				return m, func() tea.Msg { return CancelMsg{} }
			}

		case tea.KeyDelete:
			if m.cursorPos < len(m.input) {
				m.input = m.input[:m.cursorPos] + m.input[m.cursorPos+1:]
				m.completions = nil
			}

		case tea.KeyLeft:
			if m.cursorPos > 0 {
				m.cursorPos--
			}

		case tea.KeyRight:
			if m.cursorPos < len(m.input) {
				m.cursorPos++
			}

		case tea.KeyHome:
			m.cursorPos = 0

		case tea.KeyEnd:
			m.cursorPos = len(m.input)

		case tea.KeyUp:
			// History navigation
			if m.historyPos < len(m.history)-1 {
				m.historyPos++
				m.input = m.history[len(m.history)-1-m.historyPos]
				m.cursorPos = len(m.input)
			}

		case tea.KeyDown:
			// History navigation
			if m.historyPos > 0 {
				m.historyPos--
				m.input = m.history[len(m.history)-1-m.historyPos]
				m.cursorPos = len(m.input)
			} else if m.historyPos == 0 {
				m.historyPos = -1
				m.input = ""
				m.cursorPos = 0
			}

		case tea.KeyTab:
			// Tab completion
			m = m.complete()

		case tea.KeyEnter:
			if m.input != "" {
				// Add to history
				m.history = append(m.history, m.input)
				m.historyPos = -1

				// Parse and execute command
				cmd, args := m.parseCommand()
				m.input = ""
				m.cursorPos = 0
				m.completions = nil

				return m, func() tea.Msg {
					return CommandMsg{Command: cmd, Args: args}
				}
			}

		case tea.KeySpace:
			m.input = m.input[:m.cursorPos] + " " + m.input[m.cursorPos:]
			m.cursorPos++
			m.completions = nil

		case tea.KeyCtrlU:
			// Delete to beginning
			m.input = m.input[m.cursorPos:]
			m.cursorPos = 0
			m.completions = nil

		case tea.KeyCtrlW:
			// Delete word
			if m.cursorPos > 0 {
				pos := m.cursorPos - 1
				for pos > 0 && m.input[pos] == ' ' {
					pos--
				}
				for pos > 0 && m.input[pos] != ' ' {
					pos--
				}
				if m.input[pos] == ' ' {
					pos++
				}
				m.input = m.input[:pos] + m.input[m.cursorPos:]
				m.cursorPos = pos
				m.completions = nil
			}
		}
	}

	return m, nil
}

// complete performs tab completion
func (m Model) complete() Model {
	if m.completions == nil {
		// Generate completions
		m.completions = m.getCompletions()
		m.compIndex = 0
	} else {
		// Cycle through completions
		m.compIndex++
		if m.compIndex >= len(m.completions) {
			m.compIndex = 0
		}
	}

	if len(m.completions) > 0 {
		parts := strings.Fields(m.input)
		if len(parts) == 0 {
			m.input = m.completions[m.compIndex]
		} else if len(parts) == 1 && !strings.HasSuffix(m.input, " ") {
			m.input = m.completions[m.compIndex]
		} else {
			// Replace last word
			lastSpace := strings.LastIndex(m.input, " ")
			if lastSpace >= 0 {
				m.input = m.input[:lastSpace+1] + m.completions[m.compIndex]
			}
		}
		m.cursorPos = len(m.input)
	}

	return m
}

// getCompletions returns completions for the current input
func (m Model) getCompletions() []string {
	var completions []string
	parts := strings.Fields(m.input)

	if len(parts) == 0 || (len(parts) == 1 && !strings.HasSuffix(m.input, " ")) {
		// Complete command name
		prefix := ""
		if len(parts) == 1 {
			prefix = parts[0]
		}

		for name := range m.commands {
			if strings.HasPrefix(name, prefix) {
				completions = append(completions, name)
			}
		}
	}

	return completions
}

// parseCommand parses the input into command and arguments
func (m Model) parseCommand() (string, []string) {
	parts := strings.Fields(m.input)
	if len(parts) == 0 {
		return "", nil
	}

	cmd := parts[0]
	args := parts[1:]
	return cmd, args
}

// View renders the command line
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	// Prompt
	prompt := m.styles.CommandPrompt.Render(m.prefix)

	// Input with cursor
	beforeCursor := m.input[:m.cursorPos]
	afterCursor := ""
	cursorChar := " "
	if m.cursorPos < len(m.input) {
		cursorChar = string(m.input[m.cursorPos])
		afterCursor = m.input[m.cursorPos+1:]
	}

	cursor := lipgloss.NewStyle().Reverse(true).Render(cursorChar)
	input := beforeCursor + cursor + afterCursor

	// Completions hint
	var hint string
	if len(m.completions) > 1 {
		hint = " (" + strings.Join(m.completions, " | ") + ")"
		hint = m.styles.CommandCompletion.Render(hint)
	}

	return prompt + input + hint
}

// RegisterCommand registers a new command
func (m *Model) RegisterCommand(cmd Command) {
	m.commands[cmd.Name] = cmd
}

// GetCommands returns all registered commands
func (m Model) GetCommands() map[string]Command {
	return m.commands
}
