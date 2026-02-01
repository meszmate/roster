package keybindings

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Mode represents the current input mode
type Mode int

const (
	ModeNormal Mode = iota
	ModeInsert
	ModeCommand
	ModeSearch
)

// String returns the string representation of the mode
func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeInsert:
		return "INSERT"
	case ModeCommand:
		return "COMMAND"
	case ModeSearch:
		return "SEARCH"
	default:
		return "UNKNOWN"
	}
}

// Action represents a keybinding action
type Action int

const (
	ActionNone Action = iota
	// Navigation
	ActionMoveUp
	ActionMoveDown
	ActionMoveLeft
	ActionMoveRight
	ActionMoveTop
	ActionMoveBottom
	ActionPageUp
	ActionPageDown
	ActionHalfPageUp
	ActionHalfPageDown
	ActionScrollUp
	ActionScrollDown

	// Mode switching
	ActionEnterInsert
	ActionEnterInsertAfter
	ActionEnterInsertLineStart
	ActionEnterInsertLineEnd
	ActionEnterCommand
	ActionEnterSearch
	ActionEnterSearchBackward
	ActionExitMode

	// Selection and interaction
	ActionSelect
	ActionOpenChat
	ActionCloseChat
	ActionNextWindow
	ActionPrevWindow
	ActionWindow1
	ActionWindow2
	ActionWindow3
	ActionWindow4
	ActionWindow5
	ActionWindow6
	ActionWindow7
	ActionWindow8
	ActionWindow9
	ActionWindow10
	ActionWindow11
	ActionWindow12
	ActionWindow13
	ActionWindow14
	ActionWindow15
	ActionWindow16
	ActionWindow17
	ActionWindow18
	ActionWindow19
	ActionWindow20

	// Search
	ActionSearchNext
	ActionSearchPrev
	ActionClearSearch

	// Text editing
	ActionDeleteChar
	ActionDeleteWord
	ActionDeleteLine
	ActionUndo
	ActionRedo
	ActionYank
	ActionPaste

	// Commands
	ActionExecuteCommand
	ActionCancelCommand
	ActionCompleteCommand

	// UI
	ActionToggleRoster
	ActionToggleHelp
	ActionFocusRoster
	ActionFocusChat
	ActionRefresh
	ActionQuit

	// Chat
	ActionSendMessage
	ActionNewLine
	ActionCycleEncryption

	// Roster
	ActionAddContact
	ActionRemoveContact
	ActionRenameContact
	ActionShowInfo

	// MUC
	ActionJoinRoom
	ActionLeaveRoom
	ActionShowParticipants

	// Misc
	ActionMark
	ActionJumpToMark

	// Settings
	ActionShowSettings

	// Window management
	ActionSaveWindows

	// Focus
	ActionFocusAccounts
	ActionFocusInput
	ActionToggleAccountList

	// Context help
	ActionShowContextHelp

	// Account actions (when focused on accounts section)
	ActionAccountConnect
	ActionAccountDisconnect
	ActionAccountRemove
	ActionAccountEdit

	// Detail view actions
	ActionShowDetails
	ActionToggleAutoConnect

	// Multi-account window binding
	ActionSetWindowAccount

	// MUC room creation
	ActionCreateRoom

	// Status sharing
	ActionToggleStatusSharing

	// Fingerprint verification
	ActionVerifyFingerprint

	// Chat header focus
	ActionFocusHeader
)

// KeyBinding represents a key binding
type KeyBinding struct {
	Key    string
	Action Action
	Mode   Mode
}

// Manager handles keybindings and mode management
type Manager struct {
	mode           Mode
	bindings       map[Mode]map[string]Action
	pendingKeys    string
	searchQuery    string
	searchBackward bool
	marks          map[rune]int
	count          int
	countBuffer    string
}

// NewManager creates a new keybinding manager
func NewManager() *Manager {
	m := &Manager{
		mode:     ModeNormal,
		bindings: make(map[Mode]map[string]Action),
		marks:    make(map[rune]int),
	}
	m.setupDefaultBindings()
	return m
}

// setupDefaultBindings sets up the default vim-like keybindings
func (m *Manager) setupDefaultBindings() {
	// Normal mode bindings
	m.bindings[ModeNormal] = map[string]Action{
		// Movement
		"j":      ActionMoveDown,
		"k":      ActionMoveUp,
		"h":      ActionMoveLeft,
		"l":      ActionMoveRight,
		"down":   ActionMoveDown,
		"up":     ActionMoveUp,
		"left":   ActionMoveLeft,
		"right":  ActionMoveRight,
		"gg":     ActionMoveTop,
		"G":      ActionMoveBottom,
		"ctrl+u": ActionHalfPageUp,
		"ctrl+d": ActionHalfPageDown,
		"ctrl+b": ActionPageUp,
		"ctrl+f": ActionPageDown,
		"ctrl+y": ActionScrollUp,
		"ctrl+e": ActionScrollDown,

		// Mode switching
		"i":      ActionEnterInsert,
		"a":      ActionEnterInsertAfter,
		"I":      ActionEnterInsertLineStart,
		"A":      ActionEnterInsertLineEnd,
		":":      ActionEnterCommand,
		"/":      ActionEnterSearch,
		"?":      ActionEnterSearchBackward,
		"escape": ActionExitMode,

		// Selection
		"enter":  ActionOpenChat,
		"o":      ActionOpenChat,
		"q":      ActionCloseChat,

		// Search
		"n":      ActionSearchNext,
		"N":      ActionSearchPrev,

		// Windows (Alt+1-0, Alt+q-p for windows 11-20)
		"alt+1":  ActionWindow1,
		"alt+2":  ActionWindow2,
		"alt+3":  ActionWindow3,
		"alt+4":  ActionWindow4,
		"alt+5":  ActionWindow5,
		"alt+6":  ActionWindow6,
		"alt+7":  ActionWindow7,
		"alt+8":  ActionWindow8,
		"alt+9":  ActionWindow9,
		"alt+0":  ActionWindow10,
		"alt+q":  ActionWindow11,
		"alt+w":  ActionWindow12,
		"alt+e":  ActionWindow13,
		"alt+r":  ActionWindow14,
		"alt+t":  ActionWindow15,
		"alt+y":  ActionWindow16,
		"alt+u":  ActionWindow17,
		"alt+i":  ActionWindow18,
		"alt+o":  ActionWindow19,
		"alt+p":  ActionWindow20,

		// Tab navigation
		"tab":       ActionNextWindow,
		"shift+tab": ActionPrevWindow,
		"gt":        ActionNextWindow,
		"gT":        ActionPrevWindow,

		// UI
		"ctrl+r":    ActionToggleRoster,
		"ctrl+h":    ActionToggleHelp,
		"ctrl+l":    ActionRefresh,
		"ctrl+c":    ActionQuit,
		"ZZ":        ActionQuit,
		"ZQ":        ActionQuit,

		// Actions
		"dd":       ActionDeleteLine,
		"u":        ActionUndo,
		"ctrl+r_":  ActionRedo,
		"yy":       ActionYank,
		"p":        ActionPaste,
		"m":        ActionMark,
		"'":        ActionJumpToMark,

		// Roster actions (avoiding ctrl+a for tmux users)
		"ga":       ActionAddContact,    // 'g' prefix + 'a' for add
		"gx":       ActionRemoveContact, // 'g' prefix + 'x' for remove
		"gR":       ActionRenameContact, // 'g' prefix + 'R' for rename (capital)
		"gi":       ActionShowInfo,      // 'g' prefix + 'i' for info

		// MUC (avoiding ctrl conflicts for tmux)
		"gj":       ActionJoinRoom,         // 'g' prefix + 'j' for join
		"gC":       ActionCreateRoom,       // 'g' prefix + 'C' for create room
		"gp":       ActionShowParticipants, // 'g' prefix + 'p' for participants

		// Settings
		"gs":       ActionShowSettings, // 'g' prefix + 's' for settings
		"S":        ActionShowSettings,

		// Window management
		"gw":       ActionSaveWindows,  // 'g' prefix + 'w' for save windows

		// Focus keybindings
		"gr":       ActionFocusRoster,       // 'g' prefix + 'r' for roster focus
		"gc":       ActionFocusChat,         // 'g' prefix + 'c' for chat focus
		"gA":       ActionFocusAccounts,     // 'g' prefix + 'A' for accounts focus
		"gl":       ActionToggleAccountList, // 'g' prefix + 'l' for account list toggle

		// Context help
		"H":        ActionShowContextHelp,   // Show context-sensitive help/info popup

		// Account actions (work when focused on accounts section)
		"C":        ActionAccountConnect,    // Connect to selected account
		"D":        ActionAccountDisconnect, // Disconnect selected account
		"X":        ActionAccountRemove,     // Remove selected account (with confirmation)
		"E":        ActionAccountEdit,       // Edit selected account
		"T":        ActionToggleAutoConnect, // Toggle auto-connect for selected account

		// Multi-account window binding
		"space":    ActionSetWindowAccount,  // Bind selected account to current window

		// Status sharing (in contact details)
		"s":        ActionToggleStatusSharing, // Toggle status sharing for contact

		// Fingerprint verification (in contact details)
		"v":        ActionVerifyFingerprint,   // Verify fingerprint

		// Chat header focus
		"gh":       ActionFocusHeader,         // Focus chat header for contact actions
	}

	// Insert mode bindings
	m.bindings[ModeInsert] = map[string]Action{
		"escape":    ActionExitMode,
		"ctrl+c":    ActionExitMode,
		"enter":     ActionSendMessage,
		"shift+enter": ActionNewLine,
		"ctrl+e":    ActionCycleEncryption,
		"up":        ActionMoveUp,
		"down":      ActionMoveDown,
		"ctrl+u":    ActionDeleteLine,
		"ctrl+w":    ActionDeleteWord,
		"ctrl+h":    ActionDeleteChar,
		"backspace": ActionDeleteChar,
	}

	// Command mode bindings
	m.bindings[ModeCommand] = map[string]Action{
		"escape":    ActionCancelCommand,
		"ctrl+c":    ActionCancelCommand,
		"enter":     ActionExecuteCommand,
		"tab":       ActionCompleteCommand,
		"ctrl+h":    ActionDeleteChar,
		"backspace": ActionDeleteChar,
		"ctrl+u":    ActionDeleteLine,
		"ctrl+w":    ActionDeleteWord,
	}

	// Search mode bindings
	m.bindings[ModeSearch] = map[string]Action{
		"escape":    ActionCancelCommand,
		"ctrl+c":    ActionCancelCommand,
		"enter":     ActionExecuteCommand,
		"ctrl+h":    ActionDeleteChar,
		"backspace": ActionDeleteChar,
		"ctrl+u":    ActionDeleteLine,
	}
}

// Mode returns the current mode
func (m *Manager) Mode() Mode {
	return m.mode
}

// SetMode sets the current mode
func (m *Manager) SetMode(mode Mode) {
	m.mode = mode
	m.pendingKeys = ""
	m.countBuffer = ""
	m.count = 0
}

// Count returns the current count prefix (for commands like 5j)
func (m *Manager) Count() int {
	if m.count == 0 {
		return 1
	}
	return m.count
}

// SearchQuery returns the current search query
func (m *Manager) SearchQuery() string {
	return m.searchQuery
}

// SetSearchQuery sets the search query
func (m *Manager) SetSearchQuery(query string) {
	m.searchQuery = query
}

// IsSearchBackward returns whether search is backward
func (m *Manager) IsSearchBackward() bool {
	return m.searchBackward
}

// HandleKey processes a key message and returns the corresponding action
func (m *Manager) HandleKey(msg tea.KeyMsg) Action {
	key := keyToString(msg)

	// Handle count prefix in normal mode
	if m.mode == ModeNormal && isDigit(key) && m.pendingKeys == "" {
		if key != "0" || m.countBuffer != "" {
			m.countBuffer += key
			return ActionNone
		}
	}

	// Parse count buffer
	if m.countBuffer != "" {
		m.count = parseInt(m.countBuffer)
	}

	// Check for multi-key bindings (like gg, ZZ)
	m.pendingKeys += key

	// Look for exact match first
	if action, ok := m.bindings[m.mode][m.pendingKeys]; ok {
		m.pendingKeys = ""
		m.countBuffer = ""
		return action
	}

	// Check if pending keys could be a prefix of a multi-key binding
	if m.hasPendingPrefix() {
		return ActionNone
	}

	// No match found, reset pending keys
	m.pendingKeys = ""
	m.countBuffer = ""
	m.count = 0

	return ActionNone
}

// hasPendingPrefix checks if pending keys could be a prefix of a binding
func (m *Manager) hasPendingPrefix() bool {
	for binding := range m.bindings[m.mode] {
		if strings.HasPrefix(binding, m.pendingKeys) && binding != m.pendingKeys {
			return true
		}
	}
	return false
}

// SetMark sets a mark at the given position
func (m *Manager) SetMark(mark rune, position int) {
	m.marks[mark] = position
}

// GetMark returns the position for a mark
func (m *Manager) GetMark(mark rune) (int, bool) {
	pos, ok := m.marks[mark]
	return pos, ok
}

// Bind adds or updates a key binding
func (m *Manager) Bind(mode Mode, key string, action Action) {
	if m.bindings[mode] == nil {
		m.bindings[mode] = make(map[string]Action)
	}
	m.bindings[mode][key] = action
}

// Unbind removes a key binding
func (m *Manager) Unbind(mode Mode, key string) {
	if m.bindings[mode] != nil {
		delete(m.bindings[mode], key)
	}
}

// keyToString converts a tea.KeyMsg to a string representation
func keyToString(msg tea.KeyMsg) string {
	switch msg.Type {
	case tea.KeyRunes:
		return string(msg.Runes)
	case tea.KeySpace:
		return "space"
	case tea.KeyEnter:
		return "enter"
	case tea.KeyBackspace:
		return "backspace"
	case tea.KeyTab:
		return "tab"
	case tea.KeyShiftTab:
		return "shift+tab"
	case tea.KeyEscape:
		return "escape"
	case tea.KeyUp:
		return "up"
	case tea.KeyDown:
		return "down"
	case tea.KeyLeft:
		return "left"
	case tea.KeyRight:
		return "right"
	case tea.KeyHome:
		return "home"
	case tea.KeyEnd:
		return "end"
	case tea.KeyPgUp:
		return "pgup"
	case tea.KeyPgDown:
		return "pgdown"
	case tea.KeyDelete:
		return "delete"
	case tea.KeyCtrlA:
		return "ctrl+a"
	case tea.KeyCtrlB:
		return "ctrl+b"
	case tea.KeyCtrlC:
		return "ctrl+c"
	case tea.KeyCtrlD:
		return "ctrl+d"
	case tea.KeyCtrlE:
		return "ctrl+e"
	case tea.KeyCtrlF:
		return "ctrl+f"
	case tea.KeyCtrlG:
		return "ctrl+g"
	case tea.KeyCtrlH:
		return "ctrl+h"
	// Note: tea.KeyCtrlI is the same as tea.KeyTab, handled above
	case tea.KeyCtrlJ:
		return "ctrl+j"
	case tea.KeyCtrlK:
		return "ctrl+k"
	case tea.KeyCtrlL:
		return "ctrl+l"
	case tea.KeyCtrlN:
		return "ctrl+n"
	case tea.KeyCtrlO:
		return "ctrl+o"
	case tea.KeyCtrlP:
		return "ctrl+p"
	case tea.KeyCtrlQ:
		return "ctrl+q"
	case tea.KeyCtrlR:
		return "ctrl+r"
	case tea.KeyCtrlS:
		return "ctrl+s"
	case tea.KeyCtrlT:
		return "ctrl+t"
	case tea.KeyCtrlU:
		return "ctrl+u"
	case tea.KeyCtrlV:
		return "ctrl+v"
	case tea.KeyCtrlW:
		return "ctrl+w"
	case tea.KeyCtrlX:
		return "ctrl+x"
	case tea.KeyCtrlY:
		return "ctrl+y"
	case tea.KeyCtrlZ:
		return "ctrl+z"
	default:
		if msg.Alt {
			return "alt+" + string(msg.Runes)
		}
		return msg.String()
	}
}

// isDigit checks if a string is a single digit
func isDigit(s string) bool {
	if len(s) != 1 {
		return false
	}
	return s[0] >= '0' && s[0] <= '9'
}

// parseInt parses an integer from a string
func parseInt(s string) int {
	result := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}

// ActionName returns a human-readable name for an action
func ActionName(action Action) string {
	names := map[Action]string{
		ActionNone:              "none",
		ActionMoveUp:            "move up",
		ActionMoveDown:          "move down",
		ActionMoveLeft:          "move left",
		ActionMoveRight:         "move right",
		ActionMoveTop:           "move to top",
		ActionMoveBottom:        "move to bottom",
		ActionPageUp:            "page up",
		ActionPageDown:          "page down",
		ActionHalfPageUp:        "half page up",
		ActionHalfPageDown:      "half page down",
		ActionEnterInsert:       "enter insert mode",
		ActionEnterCommand:      "enter command mode",
		ActionEnterSearch:       "search",
		ActionExitMode:          "exit mode",
		ActionOpenChat:          "open chat",
		ActionCloseChat:         "close chat",
		ActionSendMessage:       "send message",
		ActionQuit:              "quit",
	}
	if name, ok := names[action]; ok {
		return name
	}
	return "unknown"
}
