package roster

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/meszmate/roster/internal/ui/theme"
)

// Roster represents a roster entry (contact)
type Roster struct {
	JID          string
	Name         string
	Groups       []string
	Status       string // online, away, dnd, xa, offline
	StatusMsg    string
	Unread       int
	AccountJID   string // Which account owns this contact
	StatusHidden bool   // True if we don't share status with this contact
	Subscription string // "none", "to", "from", "both"
}

// AccountDisplay represents an account for display in the sidebar
type AccountDisplay struct {
	JID         string
	Status      string // online, connecting, failed, offline
	UnreadMsgs  int    // Total unread messages
	UnreadChats int    // Number of contacts with unread messages
	Server      string // Server address
	Port        int    // Port number
	Resource    string // Client resource name
	OMEMO       bool   // Encryption enabled
	Session     bool   // Session-only (not saved)
	AutoConnect bool   // Auto-connect on startup
}

// Section represents which section is focused in the roster
type Section int

const (
	SectionContacts Section = iota
	SectionAccounts
)

// SelectMsg is sent when a contact is selected
type SelectMsg struct {
	JID string
}

// AccountSelectMsg is sent when an account is selected
type AccountSelectMsg struct {
	JID string
}

// Model represents the roster component
type Model struct {
	rosters     []Roster
	groups      map[string][]Roster
	selected    int
	offset      int
	width       int
	height      int
	styles      *theme.Styles
	showGroups  bool
	expandedGroups map[string]bool
	searchQuery string
	searchMatches []int
	searchIndex int

	// Account section
	accounts           []AccountDisplay
	showAccountList    bool    // Full account list mode
	accountSelected    int     // Selection in account section
	accountOffset      int     // Scroll offset for accounts
	focusSection       Section // Contacts or Accounts
	maxVisibleAccounts int     // Maximum visible accounts when not focused
	maxExpandedAccounts int    // Maximum visible accounts when focused (auto-expand)
}

// New creates a new roster model
func New(styles *theme.Styles) Model {
	return Model{
		rosters:             []Roster{},
		groups:              make(map[string][]Roster),
		styles:              styles,
		showGroups:          true,
		expandedGroups:      make(map[string]bool),
		accounts:            []AccountDisplay{},
		focusSection:        SectionContacts,
		maxVisibleAccounts:  3,
		maxExpandedAccounts: 6, // Show more when focused, but don't take all space
	}
}

// SetContacts sets the roster entries (alias for SetRosters for compatibility)
func (m Model) SetContacts(rosters []Roster) Model {
	return m.SetRosters(rosters)
}

// SetRosters sets the roster entries
func (m Model) SetRosters(rosters []Roster) Model {
	m.rosters = rosters
	m.groups = make(map[string][]Roster)

	// Group rosters
	for _, r := range rosters {
		if len(r.Groups) == 0 {
			m.groups["Ungrouped"] = append(m.groups["Ungrouped"], r)
		} else {
			for _, g := range r.Groups {
				m.groups[g] = append(m.groups[g], r)
			}
		}
	}

	// Expand all groups by default
	for g := range m.groups {
		m.expandedGroups[g] = true
	}

	return m
}

// UpdatePresence updates a roster entry's presence status
func (m Model) UpdatePresence(jid, status string) Model {
	for i, r := range m.rosters {
		if r.JID == jid {
			m.rosters[i].Status = status
			break
		}
	}
	return m
}

// UpdatePresenceMessage updates a roster entry's status message
func (m Model) UpdatePresenceMessage(jid, statusMsg string) Model {
	for i, r := range m.rosters {
		if r.JID == jid {
			m.rosters[i].StatusMsg = statusMsg
			break
		}
	}
	return m
}

// SetContactStatusHidden sets the StatusHidden flag for a roster entry
func (m Model) SetContactStatusHidden(jid string, hidden bool) Model {
	for i, r := range m.rosters {
		if r.JID == jid {
			m.rosters[i].StatusHidden = hidden
			break
		}
	}
	return m
}

// SetSize sets the component size
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	return m
}

// SetAccounts sets the accounts (both saved and session) and sorts them
func (m Model) SetAccounts(accounts []AccountDisplay) Model {
	// Separate into saved and session accounts
	var saved, session []AccountDisplay
	for _, acc := range accounts {
		if acc.Session {
			session = append(session, acc)
		} else {
			saved = append(saved, acc)
		}
	}
	// Combine: saved first, then session
	m.accounts = append(saved, session...)
	return m
}

// FocusSection returns the currently focused section
func (m Model) FocusSection() Section {
	return m.focusSection
}

// SetFocusSection sets the focused section
func (m Model) SetFocusSection(section Section) Model {
	m.focusSection = section
	return m
}

// ToggleAccountList toggles full account list mode
func (m Model) ToggleAccountList() Model {
	m.showAccountList = !m.showAccountList
	return m
}


// IsAccountListExpanded returns whether the account list is expanded
func (m Model) IsAccountListExpanded() bool {
	return m.showAccountList
}

// SelectedAccountJID returns the JID of the selected account
func (m Model) SelectedAccountJID() string {
	if m.accountSelected >= 0 && m.accountSelected < len(m.accounts) {
		return m.accounts[m.accountSelected].JID
	}
	return ""
}

// MoveToAccounts switches focus to the accounts section
func (m Model) MoveToAccounts() Model {
	if len(m.accounts) > 0 {
		m.focusSection = SectionAccounts
		m.accountSelected = 0
	}
	return m
}

// MoveToContacts switches focus to the contacts section
func (m Model) MoveToContacts() Model {
	m.focusSection = SectionContacts
	return m
}

// MoveUp moves the selection up
func (m Model) MoveUp() Model {
	if m.focusSection == SectionAccounts {
		// In accounts section
		if m.accountSelected > 0 {
			m.accountSelected--
			// Adjust offset to keep selection in view
			if m.accountSelected < m.accountOffset {
				m.accountOffset = m.accountSelected
			}
		} else {
			// Move to contacts section
			m.focusSection = SectionContacts
			m.accountOffset = 0 // Reset offset when leaving accounts
			if len(m.rosters) > 0 {
				m.selected = len(m.rosters) - 1
				// Adjust offset to show selected
				if m.selected >= m.offset+m.getContactsHeight()-2 {
					m.offset = m.selected - m.getContactsHeight() + 3
				}
			}
		}
	} else {
		// In contacts section
		if m.selected > 0 {
			m.selected--
			if m.selected < m.offset {
				m.offset = m.selected
			}
		}
	}
	return m
}

// MoveDown moves the selection down
func (m Model) MoveDown() Model {
	if m.focusSection == SectionContacts {
		// In contacts section
		if m.selected < len(m.rosters)-1 {
			m.selected++
			if m.selected >= m.offset+m.getContactsHeight()-2 {
				m.offset = m.selected - m.getContactsHeight() + 3
			}
		} else if len(m.accounts) > 0 {
			// Move to accounts section
			m.focusSection = SectionAccounts
			m.accountSelected = 0
			m.accountOffset = 0
		}
	} else {
		// In accounts section - allow full navigation through all accounts
		if m.accountSelected < len(m.accounts)-1 {
			m.accountSelected++
			// Adjust offset to keep selection in view
			maxVisible := m.getMaxVisibleAccounts()
			if m.accountSelected >= m.accountOffset+maxVisible {
				m.accountOffset = m.accountSelected - maxVisible + 1
			}
		}
	}
	return m
}

// getContactsHeight returns the height available for contacts
func (m Model) getContactsHeight() int {
	accountSectionHeight := m.getAccountSectionHeight()
	return m.height - accountSectionHeight
}

// MoveToTop moves selection to the top
func (m Model) MoveToTop() Model {
	m.selected = 0
	m.offset = 0
	return m
}

// MoveToBottom moves selection to the bottom
func (m Model) MoveToBottom() Model {
	m.selected = len(m.rosters) - 1
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= m.offset+m.height-2 {
		m.offset = m.selected - m.height + 3
	}
	if m.offset < 0 {
		m.offset = 0
	}
	return m
}

// PageUp moves up by half a page
func (m Model) PageUp() Model {
	pageSize := m.height / 2
	m.selected -= pageSize
	if m.selected < 0 {
		m.selected = 0
	}
	m.offset -= pageSize
	if m.offset < 0 {
		m.offset = 0
	}
	return m
}

// PageDown moves down by half a page
func (m Model) PageDown() Model {
	pageSize := m.height / 2
	m.selected += pageSize
	if m.selected >= len(m.rosters) {
		m.selected = len(m.rosters) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	m.offset += pageSize
	maxOffset := len(m.rosters) - m.height + 2
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	return m
}

// SelectedJID returns the JID of the selected roster entry
func (m Model) SelectedJID() string {
	if m.selected >= 0 && m.selected < len(m.rosters) {
		return m.rosters[m.selected].JID
	}
	return ""
}

// SearchNext finds the next match
func (m Model) SearchNext(query string) Model {
	if query != m.searchQuery {
		m.searchQuery = query
		m.searchMatches = m.findMatches(query)
		m.searchIndex = 0
	} else {
		m.searchIndex++
		if m.searchIndex >= len(m.searchMatches) {
			m.searchIndex = 0
		}
	}

	if len(m.searchMatches) > 0 {
		m.selected = m.searchMatches[m.searchIndex]
		if m.selected < m.offset {
			m.offset = m.selected
		} else if m.selected >= m.offset+m.height-2 {
			m.offset = m.selected - m.height + 3
		}
	}

	return m
}

// SearchPrev finds the previous match
func (m Model) SearchPrev(query string) Model {
	if query != m.searchQuery {
		m.searchQuery = query
		m.searchMatches = m.findMatches(query)
		m.searchIndex = len(m.searchMatches) - 1
	} else {
		m.searchIndex--
		if m.searchIndex < 0 {
			m.searchIndex = len(m.searchMatches) - 1
		}
	}

	if len(m.searchMatches) > 0 && m.searchIndex >= 0 {
		m.selected = m.searchMatches[m.searchIndex]
		if m.selected < m.offset {
			m.offset = m.selected
		} else if m.selected >= m.offset+m.height-2 {
			m.offset = m.selected - m.height + 3
		}
	}

	return m
}

// findMatches finds all roster entries matching the query
func (m Model) findMatches(query string) []int {
	var matches []int
	query = strings.ToLower(query)
	for i, r := range m.rosters {
		name := strings.ToLower(r.Name)
		jid := strings.ToLower(r.JID)
		if strings.Contains(name, query) || strings.Contains(jid, query) {
			matches = append(matches, i)
		}
	}
	return matches
}

// getMaxVisibleAccounts returns the max accounts to show based on focus state
func (m Model) getMaxVisibleAccounts() int {
	if m.focusSection == SectionAccounts {
		return m.maxExpandedAccounts
	}
	return m.maxVisibleAccounts
}

// getAccountSectionHeight returns the height needed for the accounts section
func (m Model) getAccountSectionHeight() int {
	if len(m.accounts) == 0 {
		return 0
	}

	// Separate saved and session accounts
	var savedCount, sessionCount int
	for _, acc := range m.accounts {
		if acc.Session {
			sessionCount++
		} else {
			savedCount++
		}
	}

	maxVisible := m.getMaxVisibleAccounts()
	height := 0

	// Saved accounts section
	if savedCount > 0 {
		height += 2 // separator + header
		numVisible := maxVisible
		if m.showAccountList || savedCount <= maxVisible {
			numVisible = savedCount
		}
		height += numVisible * 2 // 2 lines per account (JID + stats)
		// Add height for ↑N more indicator if there are hidden accounts above
		if m.accountOffset > 0 && m.focusSection == SectionAccounts {
			height++ // "↑N more" line
		}
		// Add height for ↓N more indicator if there are hidden accounts below
		if savedCount > maxVisible && !m.showAccountList {
			height++ // "↓N more" line
		}
	}

	// Session accounts section
	if sessionCount > 0 {
		height += 2 // separator + header
		numVisible := maxVisible
		if m.showAccountList {
			numVisible = sessionCount
		}
		if sessionCount < numVisible {
			numVisible = sessionCount
		}
		height += numVisible * 2 // 2 lines per account
		if sessionCount > numVisible && !m.showAccountList {
			height++ // "↓N more" line
		}
	}

	return height
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.focusSection == SectionAccounts {
				if jid := m.SelectedAccountJID(); jid != "" {
					return m, func() tea.Msg {
						return AccountSelectMsg{JID: jid}
					}
				}
			} else {
				if jid := m.SelectedJID(); jid != "" {
					return m, func() tea.Msg {
						return SelectMsg{JID: jid}
					}
				}
			}
		}
	}
	return m, nil
}

// View renders the roster
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	var b strings.Builder

	// Header
	header := m.styles.RosterHeader.Width(m.width - 2).Render("Roster")
	b.WriteString(header)
	b.WriteString("\n")

	// Calculate visible area for contacts
	accountSectionHeight := m.getAccountSectionHeight()
	visibleHeight := m.height - 3 - accountSectionHeight // header + padding - accounts
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	// Show help message if no roster entries
	if len(m.rosters) == 0 {
		helpLines := []string{
			"",
			"Roster is empty",
			"",
			"Getting Started:",
			"  :account add - Add account",
			"  :connect     - Connect",
			"  :help        - All commands",
			"",
			"Quick Keys:",
			"  ga  Add to roster",
			"  gj  Join room",
			"  gs  Settings",
			"  :1  Window 1",
			"",
			": = commands",
			"Esc = cancel",
		}
		for i, line := range helpLines {
			if i < visibleHeight {
				if len(line) > m.width-4 {
					line = line[:m.width-4]
				}
				b.WriteString(m.styles.RosterContact.Width(m.width - 2).Render(" " + line))
				b.WriteString("\n")
			}
		}
		// Pad remaining - use actual lines rendered, not len(helpLines)
		blankLine := strings.Repeat(" ", m.width-2)
		linesRendered := len(helpLines)
		if linesRendered > visibleHeight {
			linesRendered = visibleHeight
		}
		for i := linesRendered; i < visibleHeight; i++ {
			b.WriteString(blankLine)
			b.WriteString("\n")
		}
	} else {
		// Calculate actual visible height accounting for scroll indicators
		actualVisibleHeight := visibleHeight
		if m.offset > 0 {
			actualVisibleHeight-- // Reserve line for "↑N more"
		}
		hiddenBelow := len(m.rosters) - m.offset - actualVisibleHeight
		if hiddenBelow > 0 {
			actualVisibleHeight-- // Reserve line for "↓N more"
			hiddenBelow = len(m.rosters) - m.offset - actualVisibleHeight
		}

		// Show "↑N more" indicator if there are hidden contacts above
		if m.offset > 0 {
			moreText := fmt.Sprintf("  ↑ %d more", m.offset)
			b.WriteString(m.styles.RosterContact.Foreground(lipgloss.Color("242")).Width(m.width - 2).Render(moreText))
			b.WriteString("\n")
		}

		// Render visible roster entries
		for i := m.offset; i < len(m.rosters) && i < m.offset+actualVisibleHeight; i++ {
			r := m.rosters[i]
			selected := i == m.selected && m.focusSection == SectionContacts
			line := m.renderRoster(r, selected)
			b.WriteString(line)
			b.WriteString("\n")
		}

		// Show "↓N more" indicator if there are hidden contacts below
		if hiddenBelow > 0 {
			moreText := fmt.Sprintf("  ↓ %d more", hiddenBelow)
			b.WriteString(m.styles.RosterContact.Foreground(lipgloss.Color("242")).Width(m.width - 2).Render(moreText))
			b.WriteString("\n")
		}

		// Pad remaining roster lines
		rostersRendered := 0
		if len(m.rosters) > 0 {
			end := m.offset + actualVisibleHeight
			if end > len(m.rosters) {
				end = len(m.rosters)
			}
			rostersRendered = end - m.offset
		}
		// Account for indicator lines in padding calculation
		linesUsed := rostersRendered
		if m.offset > 0 {
			linesUsed++
		}
		if hiddenBelow > 0 {
			linesUsed++
		}
		blankLine := strings.Repeat(" ", m.width-2)
		for i := linesUsed; i < visibleHeight; i++ {
			b.WriteString(blankLine)
			b.WriteString("\n")
		}
	}

	// Render accounts section if there are accounts
	if len(m.accounts) > 0 {
		b.WriteString(m.renderAccountsSection())
	}

	// Count lines rendered and pad to full height to clear old content
	result := b.String()
	lineCount := strings.Count(result, "\n")
	if lineCount < m.height-1 {
		blankLine := strings.Repeat(" ", m.width-2)
		for i := lineCount; i < m.height-1; i++ {
			result += blankLine + "\n"
		}
	}

	return result
}

// renderAccountsSection renders the accounts section at the bottom with separate saved/session sections
func (m Model) renderAccountsSection() string {
	var b strings.Builder

	// Separate saved and session accounts
	var saved, session []AccountDisplay
	for _, acc := range m.accounts {
		if acc.Session {
			session = append(session, acc)
		} else {
			saved = append(saved, acc)
		}
	}

	maxVisible := m.getMaxVisibleAccounts()
	isFocused := m.focusSection == SectionAccounts

	// Track global account index for selection highlighting
	globalIdx := 0

	// Render Saved Accounts section if there are any
	if len(saved) > 0 {
		// Separator
		separator := strings.Repeat("─", m.width-4)
		b.WriteString(m.styles.RosterContact.Foreground(lipgloss.Color("242")).Width(m.width - 2).Render(" " + separator))
		b.WriteString("\n")

		// Header with position indicator when focused
		headerText := " Saved Accounts"
		if isFocused && len(saved) > maxVisible {
			headerText = fmt.Sprintf(" Saved Accounts [%d/%d]", m.accountSelected+1, len(m.accounts))
		}
		b.WriteString(m.styles.RosterGroup.Width(m.width - 2).Render(headerText))
		b.WriteString("\n")

		// Calculate visible range with scroll offset
		startIdx := 0
		endIdx := len(saved)
		if len(saved) > maxVisible && !m.showAccountList {
			// Apply scroll offset when focused
			if isFocused {
				// Keep selection in view
				if m.accountSelected < m.accountOffset {
					m.accountOffset = m.accountSelected
				} else if m.accountSelected >= m.accountOffset+maxVisible {
					m.accountOffset = m.accountSelected - maxVisible + 1
				}
				startIdx = m.accountOffset
				endIdx = startIdx + maxVisible
				if endIdx > len(saved) {
					endIdx = len(saved)
				}
			} else {
				endIdx = maxVisible
			}
		}

		// Show "↑N more" indicator if there are hidden accounts above
		if startIdx > 0 {
			moreText := fmt.Sprintf("  ↑ %d more", startIdx)
			b.WriteString(m.styles.RosterContact.Foreground(lipgloss.Color("242")).Width(m.width - 2).Render(moreText))
			b.WriteString("\n")
		}

		// Render visible saved accounts
		for i := startIdx; i < endIdx && i < len(saved); i++ {
			acc := saved[i]
			selected := globalIdx+i-startIdx == m.accountSelected && isFocused

			// Build position indicator for selected account
			posIndicator := ""
			if selected && len(m.accounts) > maxVisible {
				aboveCount := m.accountSelected
				belowCount := len(m.accounts) - m.accountSelected - 1
				if aboveCount > 0 || belowCount > 0 {
					posIndicator = fmt.Sprintf(" %d↑ %d↓", aboveCount, belowCount)
				}
			}

			line := m.renderAccountWithIndicator(acc, selected, posIndicator)
			b.WriteString(line)
			b.WriteString("\n")
		}
		globalIdx += endIdx - startIdx

		// Show "↓N more" indicator if there are hidden accounts below
		hiddenBelow := len(saved) - endIdx
		if hiddenBelow > 0 {
			moreText := fmt.Sprintf("  ↓ %d more", hiddenBelow)
			b.WriteString(m.styles.RosterContact.Foreground(lipgloss.Color("242")).Width(m.width - 2).Render(moreText))
			b.WriteString("\n")
		}

		// Update globalIdx to account for all saved accounts (for session section selection)
		globalIdx = len(saved)
	}

	// Render Session Accounts section if there are any
	if len(session) > 0 {
		// Separator
		separator := strings.Repeat("─", m.width-4)
		b.WriteString(m.styles.RosterContact.Foreground(lipgloss.Color("242")).Width(m.width - 2).Render(" " + separator))
		b.WriteString("\n")

		// Header
		b.WriteString(m.styles.RosterGroup.Width(m.width - 2).Render(" Session"))
		b.WriteString("\n")

		// Determine how many to show
		maxForSession := maxVisible
		if m.showAccountList {
			maxForSession = len(session)
		}

		// Render session accounts
		for i := 0; i < maxForSession && i < len(session); i++ {
			acc := session[i]
			selected := globalIdx == m.accountSelected && isFocused

			// Build position indicator for selected account
			posIndicator := ""
			if selected && len(m.accounts) > maxVisible {
				aboveCount := m.accountSelected
				belowCount := len(m.accounts) - m.accountSelected - 1
				if aboveCount > 0 || belowCount > 0 {
					posIndicator = fmt.Sprintf(" %d↑ %d↓", aboveCount, belowCount)
				}
			}

			line := m.renderAccountWithIndicator(acc, selected, posIndicator)
			b.WriteString(line)
			b.WriteString("\n")
			globalIdx++
		}

		// Show "↓N more" for session accounts
		if len(session) > maxForSession && !m.showAccountList {
			more := len(session) - maxForSession
			moreText := fmt.Sprintf("  ↓ %d more", more)
			b.WriteString(m.styles.RosterContact.Foreground(lipgloss.Color("242")).Width(m.width - 2).Render(moreText))
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderAccountWithIndicator renders an account with optional position indicator
func (m Model) renderAccountWithIndicator(acc AccountDisplay, selected bool, posIndicator string) string {
	// Status indicator
	var indicator string
	var indicatorStyle lipgloss.Style
	switch acc.Status {
	case "online":
		indicator = "●"
		indicatorStyle = m.styles.PresenceOnline
	case "connecting":
		indicator = "◐"
		indicatorStyle = m.styles.PresenceAway
	case "disconnecting":
		indicator = "◐"
		indicatorStyle = m.styles.PresenceAway
	case "failed":
		indicator = "✘"
		indicatorStyle = m.styles.PresenceDND
	default:
		indicator = "○"
		indicatorStyle = m.styles.PresenceOffline
	}

	presence := indicatorStyle.Render(indicator)

	// JID (truncate if needed, accounting for position indicator)
	jid := acc.JID
	maxWidth := m.width - 6 - len(posIndicator) // presence + padding + indicator
	if len(jid) > maxWidth && maxWidth > 0 {
		jid = jid[:maxWidth-1] + "…"
	}

	// Build line style
	var style lipgloss.Style
	var dimStyle lipgloss.Style
	if selected {
		style = m.styles.RosterSelected
		dimStyle = m.styles.RosterSelected
	} else if acc.Status == "offline" || acc.Status == "failed" {
		// Dimmed style for offline/failed accounts
		style = m.styles.RosterContact.Foreground(lipgloss.Color("242"))
		dimStyle = m.styles.RosterContact.Foreground(lipgloss.Color("242"))
	} else {
		style = m.styles.RosterContact
		dimStyle = m.styles.RosterContact.Foreground(lipgloss.Color("242"))
	}

	// First line: indicator + JID + position indicator (if any)
	line1 := fmt.Sprintf(" %s %s", presence, jid)

	// Add position indicator on the right side if present
	if posIndicator != "" {
		// Calculate padding to right-align the position indicator
		padLen := m.width - 2 - len(line1) - len(posIndicator)
		if padLen > 0 {
			line1 += strings.Repeat(" ", padLen) + posIndicator
		}
	} else {
		// Pad first line to width
		if len(line1) < m.width-2 {
			line1 += strings.Repeat(" ", m.width-2-len(line1))
		}
	}

	// Second line: stats
	var statsLine string
	if acc.Status == "connecting" {
		statsLine = "  connecting..."
	} else if acc.Status == "disconnecting" {
		statsLine = "  disconnecting..."
	} else if acc.Status == "offline" {
		statsLine = "  offline"
		if acc.OMEMO {
			statsLine += " · OMEMO"
		}
	} else if acc.Status == "failed" {
		statsLine = "  connection failed"
	} else {
		// Online: show unread stats
		var parts []string
		if acc.UnreadMsgs > 0 {
			parts = append(parts, fmt.Sprintf("%d msgs", acc.UnreadMsgs))
		}
		if acc.UnreadChats > 0 {
			parts = append(parts, fmt.Sprintf("%d chats", acc.UnreadChats))
		}
		if acc.OMEMO {
			parts = append(parts, "OMEMO")
		} else {
			parts = append(parts, "Plain")
		}
		if len(parts) > 0 {
			statsLine = "  " + strings.Join(parts, " · ")
		}
	}

	// Pad stats line to width
	if len(statsLine) < m.width-2 {
		statsLine += strings.Repeat(" ", m.width-2-len(statsLine))
	}

	// Combine lines
	result := style.Width(m.width - 2).Render(line1) + "\n" + dimStyle.Width(m.width - 2).Render(statsLine)
	return result
}

// renderRoster renders a single roster entry line
func (m Model) renderRoster(r Roster, selected bool) string {
	// Presence indicator
	var presenceStyle lipgloss.Style
	var indicator string

	// If status is hidden, show "?" indicator in gray
	if r.StatusHidden {
		presenceStyle = m.styles.RosterContact.Foreground(lipgloss.Color("242"))
		indicator = "?"
	} else {
		switch r.Status {
		case "online":
			presenceStyle = m.styles.PresenceOnline
			indicator = "●"
		case "away":
			presenceStyle = m.styles.PresenceAway
			indicator = "◐"
		case "dnd":
			presenceStyle = m.styles.PresenceDND
			indicator = "⊘"
		case "xa":
			presenceStyle = m.styles.PresenceXA
			indicator = "◯"
		default:
			presenceStyle = m.styles.PresenceOffline
			indicator = "○"
		}
	}

	presence := presenceStyle.Render(indicator)

	// Roster entry name
	name := r.Name
	if name == "" {
		name = r.JID
	}

	// Build status suffix for non-online entries with status message
	statusSuffix := ""
	if r.StatusHidden {
		// Show "(hidden)" for entries where status is not shared
		statusSuffix = " (hidden)"
	} else if r.StatusMsg != "" && r.Status != "online" {
		// Show abbreviated status with message: "(away: lunch)"
		shortStatus := r.Status
		if r.Status == "offline" {
			shortStatus = "off"
		}
		statusSuffix = fmt.Sprintf(" (%s: %s)", shortStatus, r.StatusMsg)
	} else if r.StatusMsg != "" && r.Status == "online" {
		// Online with status message: just show the message
		statusSuffix = fmt.Sprintf(" (%s)", r.StatusMsg)
	}

	// Unread indicator
	unread := ""
	if r.Unread > 0 {
		unread = fmt.Sprintf(" [%d]", r.Unread)
	}

	// Calculate available width for name + status
	// Format: " ● name (status) [N]"
	// Reserve: 3 (space+indicator+space) + unread length
	unreadLen := len(unread)
	maxWidth := m.width - 5 - unreadLen // presence + padding + unread
	if maxWidth < 5 {
		maxWidth = 5
	}

	// Combine name and status suffix, then truncate if needed
	displayText := name + statusSuffix
	if len(displayText) > maxWidth {
		// Truncate, preferring to keep the name
		if len(name) >= maxWidth {
			displayText = name[:maxWidth-1] + "…"
		} else {
			// Truncate status message
			remaining := maxWidth - len(name) - 1
			if remaining > 3 && len(statusSuffix) > 0 {
				displayText = name + statusSuffix[:remaining] + "…"
			} else {
				displayText = name
			}
		}
	}

	// Build line style
	var style lipgloss.Style
	if selected {
		style = m.styles.RosterSelected
	} else {
		style = m.styles.RosterContact
	}

	// Build the content
	var content string
	if r.Unread > 0 {
		content = fmt.Sprintf(" %s %s%s", presence, displayText, m.styles.RosterUnread.Render(unread))
	} else {
		content = fmt.Sprintf(" %s %s", presence, displayText)
	}

	return style.Width(m.width - 2).Render(content)
}
