package statusbar

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/meszmate/roster/internal/ui/keybindings"
	"github.com/meszmate/roster/internal/ui/theme"
)

// WindowInfo represents a window for display in status bar
type WindowInfo struct {
	Num    int
	Title  string
	Active bool
	Unread int
}

// Model represents the status bar component
type Model struct {
	width         int
	mode          keybindings.Mode
	account       string
	status        string
	connected     bool
	styles        *theme.Styles
	extraInfo     string
	windows       []WindowInfo
	windowAccount string
	syncing       bool
	syncProgress  string
	rosterLoading bool
	rosterSpinner string
}

// New creates a new status bar model
func New(styles *theme.Styles) Model {
	return Model{
		styles:    styles,
		mode:      keybindings.ModeNormal,
		connected: false,
	}
}

// SetWidth sets the status bar width
func (m Model) SetWidth(width int) Model {
	m.width = width
	return m
}

// SetMode sets the current mode
func (m Model) SetMode(mode keybindings.Mode) Model {
	m.mode = mode
	return m
}

// SetAccount sets the current account
func (m Model) SetAccount(account string) Model {
	m.account = account
	return m
}

// SetStatus sets the current status
func (m Model) SetStatus(status string) Model {
	m.status = status
	return m
}

// SetConnected sets the connection state
func (m Model) SetConnected(connected bool) Model {
	m.connected = connected
	return m
}

// SetExtraInfo sets extra info to display
func (m Model) SetExtraInfo(info string) Model {
	m.extraInfo = info
	return m
}

// SetWindows sets the window list for display
func (m Model) SetWindows(windows []WindowInfo) Model {
	m.windows = windows
	return m
}

// SetWindowAccount sets the account bound to the current window
func (m Model) SetWindowAccount(acc string) Model {
	m.windowAccount = acc
	return m
}

// SetSyncing sets the MAM sync state
func (m Model) SetSyncing(syncing bool, progress string) Model {
	m.syncing = syncing
	m.syncProgress = progress
	return m
}

// SetRosterLoading sets roster loading state.
func (m Model) SetRosterLoading(loading bool, spinner string) Model {
	m.rosterLoading = loading
	m.rosterSpinner = spinner
	return m
}

// View renders the status bar
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	// Mode indicator
	var modeStyle lipgloss.Style
	switch m.mode {
	case keybindings.ModeNormal:
		modeStyle = m.styles.StatusModeNormal
	case keybindings.ModeInsert:
		modeStyle = m.styles.StatusModeInsert
	case keybindings.ModeCommand:
		modeStyle = m.styles.StatusModeCommand
	case keybindings.ModeSearch:
		modeStyle = m.styles.StatusModeCommand
	default:
		modeStyle = m.styles.StatusModeNormal
	}

	modeText := modeStyle.Render(m.mode.String())

	// Extra info (like encryption status, typing, etc.)
	extra := ""
	if m.extraInfo != "" {
		extra = " | " + m.extraInfo
	}

	// Sync indicator
	syncIndicator := ""
	if m.syncing {
		syncIndicator = m.styles.PresenceAway.Render(" [Syncing" + m.syncProgress + "...]")
	}

	// Roster loading indicator
	rosterIndicator := ""
	if m.rosterLoading {
		spin := m.rosterSpinner
		if spin == "" {
			spin = "..."
		}
		rosterIndicator = m.styles.PresenceAway.Render(" [Roster " + spin + " loading...]")
	}

	// Window indicators
	var windowsStr string
	if len(m.windows) > 0 {
		var parts []string
		for _, w := range m.windows {
			label := fmt.Sprintf("%d", w.Num)
			if w.Title != "" && w.Title != "Console" {
				// Shorten title
				title := w.Title
				if len(title) > 10 {
					title = title[:10]
				}
				label = fmt.Sprintf("%d:%s", w.Num, title)
			}

			if w.Unread > 0 {
				label = fmt.Sprintf("%s(%d)", label, w.Unread)
			}

			if w.Active {
				label = m.styles.StatusModeInsert.Render(label)
			} else if w.Unread > 0 {
				label = m.styles.PresenceAway.Render(label)
			} else {
				label = m.styles.StatusAccount.Render(label)
			}
			parts = append(parts, label)
		}
		windowsStr = "[" + strings.Join(parts, " ") + "]"
	}

	// Build account section only if an account is active
	var accountSection string
	if m.account != "" {
		// Connection status indicator and text
		var connStatus string
		var statusText string
		switch m.status {
		case "online":
			connStatus = m.styles.PresenceOnline.Render("●")
		case "connecting":
			connStatus = m.styles.PresenceAway.Render("◐")
			statusText = m.styles.PresenceAway.Render(" [connecting...]")
		case "failed":
			connStatus = m.styles.PresenceDND.Render("✗")
			statusText = m.styles.PresenceDND.Render(" [failed]")
		default:
			connStatus = m.styles.PresenceOffline.Render("○")
			if m.status != "" && m.status != "offline" {
				statusText = fmt.Sprintf(" [%s]", m.status)
			}
		}

		accountText := m.styles.StatusAccount.Render(m.account)

		// Window account indicator (if different from main account)
		windowAccStr := ""
		if m.windowAccount != "" && m.windowAccount != m.account {
			// Show just the user part of the JID
			windowAcc := m.windowAccount
			if idx := strings.Index(windowAcc, "@"); idx > 0 {
				windowAcc = windowAcc[:idx]
			}
			windowAccStr = " | Win:" + windowAcc
		}

		accountSection = fmt.Sprintf(" %s %s%s%s", connStatus, accountText, statusText, windowAccStr)
	}

	// Build left and right parts
	left := fmt.Sprintf(" %s%s%s%s", modeText, accountSection, syncIndicator, rosterIndicator)
	right := windowsStr + extra + " "

	// Calculate padding
	padding := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 0 {
		padding = 0
	}

	// Combine
	result := left + strings.Repeat(" ", padding) + right

	return m.styles.StatusBar.Width(m.width).Render(result)
}
