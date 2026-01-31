package windows

import (
	"github.com/meszmate/roster/internal/ui/theme"
)

// WindowType represents the type of window
type WindowType int

const (
	WindowConsole WindowType = iota
	WindowChat
	WindowMUC
	WindowPrivate
)

// Window represents a single window
type Window struct {
	ID         int
	Type       WindowType
	JID        string
	Title      string
	Unread     int
	Active     bool
	AccountJID string // Which account this window uses
}

// Model represents the window manager
type Model struct {
	windows    []Window
	active     int
	maxWindows int
	styles     *theme.Styles
}

// New creates a new window manager
func New(styles *theme.Styles) Model {
	return Model{
		windows: []Window{
			{ID: 0, Type: WindowConsole, Title: "Console", Active: true},
		},
		active:     0,
		maxWindows: 20,
		styles:     styles,
	}
}

// OpenChat opens a new chat window for a JID
func (m Model) OpenChat(jid string) Model {
	return m.OpenChatWithAccount(jid, "")
}

// OpenChatWithAccount opens a new chat window for a JID with a specific account
func (m Model) OpenChatWithAccount(jid, accountJID string) Model {
	// Check if window already exists
	for i, w := range m.windows {
		if w.JID == jid {
			m.active = i
			return m
		}
	}

	// Create new window
	if len(m.windows) < m.maxWindows {
		window := Window{
			ID:         len(m.windows),
			Type:       WindowChat,
			JID:        jid,
			Title:      jid,
			AccountJID: accountJID,
		}
		m.windows = append(m.windows, window)
		m.active = len(m.windows) - 1
	}

	return m
}

// OpenMUC opens a new MUC window
func (m Model) OpenMUC(roomJID, nick string) Model {
	return m.OpenMUCWithAccount(roomJID, nick, "")
}

// OpenMUCWithAccount opens a new MUC window with a specific account
func (m Model) OpenMUCWithAccount(roomJID, nick, accountJID string) Model {
	// Check if window already exists
	for i, w := range m.windows {
		if w.JID == roomJID {
			m.active = i
			return m
		}
	}

	// Create new window
	if len(m.windows) < m.maxWindows {
		window := Window{
			ID:         len(m.windows),
			Type:       WindowMUC,
			JID:        roomJID,
			Title:      roomJID,
			AccountJID: accountJID,
		}
		m.windows = append(m.windows, window)
		m.active = len(m.windows) - 1
	}

	return m
}

// CloseActive closes the active window
func (m Model) CloseActive() Model {
	if m.active == 0 {
		// Can't close console
		return m
	}

	m.windows = append(m.windows[:m.active], m.windows[m.active+1:]...)

	// Update IDs
	for i := range m.windows {
		m.windows[i].ID = i
	}

	// Adjust active window
	if m.active >= len(m.windows) {
		m.active = len(m.windows) - 1
	}

	return m
}

// Close closes a window by ID
func (m Model) Close(id int) Model {
	if id == 0 || id >= len(m.windows) {
		return m
	}

	m.windows = append(m.windows[:id], m.windows[id+1:]...)

	// Update IDs
	for i := range m.windows {
		m.windows[i].ID = i
	}

	// Adjust active window
	if m.active >= len(m.windows) {
		m.active = len(m.windows) - 1
	}

	return m
}

// Next moves to the next window
func (m Model) Next() Model {
	m.active++
	if m.active >= len(m.windows) {
		m.active = 0
	}
	return m
}

// Prev moves to the previous window
func (m Model) Prev() Model {
	m.active--
	if m.active < 0 {
		m.active = len(m.windows) - 1
	}
	return m
}

// GoTo goes to a specific window by number
func (m Model) GoTo(num int) Model {
	if num >= 0 && num < len(m.windows) {
		m.active = num
	}
	return m
}

// GoToResult goes to a specific window and returns whether it succeeded
func (m Model) GoToResult(num int) (Model, bool) {
	if num >= 0 && num < len(m.windows) {
		m.active = num
		return m, true
	}
	return m, false
}

// Active returns the active window
func (m Model) Active() *Window {
	if m.active >= 0 && m.active < len(m.windows) {
		return &m.windows[m.active]
	}
	return nil
}

// ActiveJID returns the JID of the active window
func (m Model) ActiveJID() string {
	if w := m.Active(); w != nil {
		return w.JID
	}
	return ""
}

// ActiveNum returns the active window number
func (m Model) ActiveNum() int {
	return m.active
}

// Count returns the number of windows
func (m Model) Count() int {
	return len(m.windows)
}

// IncrementUnread increments unread count for a JID
func (m Model) IncrementUnread(jid string) Model {
	for i, w := range m.windows {
		if w.JID == jid {
			m.windows[i].Unread++
			break
		}
	}
	return m
}

// ClearUnread clears unread count for a window
func (m Model) ClearUnread(id int) Model {
	if id >= 0 && id < len(m.windows) {
		m.windows[id].Unread = 0
	}
	return m
}

// GetWindows returns all windows
func (m Model) GetWindows() []Window {
	return m.windows
}

// View renders the window list (for tab bar)
func (m Model) View() string {
	// This could render a tab bar at the top
	return ""
}
