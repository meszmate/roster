package roster

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/meszmate/roster/internal/ui/theme"
)

// Contact represents a roster contact
type Contact struct {
	JID      string
	Name     string
	Groups   []string
	Status   string // online, away, dnd, xa, offline
	StatusMsg string
	Unread   int
}

// AccountDisplay represents an account for display in the sidebar
type AccountDisplay struct {
	JID      string
	Status   string // online, connecting, failed, offline
	Unread   int
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
	contacts    []Contact
	groups      map[string][]Contact
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
	accounts        []AccountDisplay
	showAccountList bool  // Full account list mode
	accountSelected int   // Selection in account section
	focusSection    Section // Contacts or Accounts
	maxVisibleAccounts int // Maximum visible accounts before "+N more"
}

// New creates a new roster model
func New(styles *theme.Styles) Model {
	return Model{
		contacts:           []Contact{},
		groups:             make(map[string][]Contact),
		styles:             styles,
		showGroups:         true,
		expandedGroups:     make(map[string]bool),
		accounts:           []AccountDisplay{},
		focusSection:       SectionContacts,
		maxVisibleAccounts: 3,
	}
}

// SetContacts sets the roster contacts
func (m Model) SetContacts(contacts []Contact) Model {
	m.contacts = contacts
	m.groups = make(map[string][]Contact)

	// Group contacts
	for _, c := range contacts {
		if len(c.Groups) == 0 {
			m.groups["Ungrouped"] = append(m.groups["Ungrouped"], c)
		} else {
			for _, g := range c.Groups {
				m.groups[g] = append(m.groups[g], c)
			}
		}
	}

	// Expand all groups by default
	for g := range m.groups {
		m.expandedGroups[g] = true
	}

	return m
}

// UpdatePresence updates a contact's presence status
func (m Model) UpdatePresence(jid, status string) Model {
	for i, c := range m.contacts {
		if c.JID == jid {
			m.contacts[i].Status = status
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

// SetAccounts sets the connected accounts
func (m Model) SetAccounts(accounts []AccountDisplay) Model {
	m.accounts = accounts
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
		} else {
			// Move to contacts section
			m.focusSection = SectionContacts
			if len(m.contacts) > 0 {
				m.selected = len(m.contacts) - 1
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
		if m.selected < len(m.contacts)-1 {
			m.selected++
			if m.selected >= m.offset+m.getContactsHeight()-2 {
				m.offset = m.selected - m.getContactsHeight() + 3
			}
		} else if len(m.accounts) > 0 {
			// Move to accounts section
			m.focusSection = SectionAccounts
			m.accountSelected = 0
		}
	} else {
		// In accounts section
		maxAccounts := m.maxVisibleAccounts
		if m.showAccountList {
			maxAccounts = len(m.accounts)
		}
		if m.accountSelected < maxAccounts-1 && m.accountSelected < len(m.accounts)-1 {
			m.accountSelected++
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
	m.selected = len(m.contacts) - 1
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
	if m.selected >= len(m.contacts) {
		m.selected = len(m.contacts) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	m.offset += pageSize
	maxOffset := len(m.contacts) - m.height + 2
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	return m
}

// SelectedJID returns the JID of the selected contact
func (m Model) SelectedJID() string {
	if m.selected >= 0 && m.selected < len(m.contacts) {
		return m.contacts[m.selected].JID
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

// findMatches finds all contacts matching the query
func (m Model) findMatches(query string) []int {
	var matches []int
	query = strings.ToLower(query)
	for i, c := range m.contacts {
		name := strings.ToLower(c.Name)
		jid := strings.ToLower(c.JID)
		if strings.Contains(name, query) || strings.Contains(jid, query) {
			matches = append(matches, i)
		}
	}
	return matches
}

// getAccountSectionHeight returns the height needed for the accounts section
func (m Model) getAccountSectionHeight() int {
	if len(m.accounts) == 0 {
		return 0
	}
	// Header (1) + separator (1) + accounts (up to 3) + "+N more" line if needed (1)
	numVisible := m.maxVisibleAccounts
	if m.showAccountList || len(m.accounts) <= m.maxVisibleAccounts {
		numVisible = len(m.accounts)
	}
	height := 2 + numVisible // header + separator + visible accounts
	if len(m.accounts) > m.maxVisibleAccounts && !m.showAccountList {
		height++ // "+N more" line
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

	// Show help message if no contacts
	if len(m.contacts) == 0 {
		helpLines := []string{
			"",
			"No contacts yet",
			"",
			"Getting Started:",
			"  :account add - Add account",
			"  :connect     - Connect",
			"  :help        - All commands",
			"",
			"Quick Keys:",
			"  ga  Add contact",
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
				b.WriteString(m.styles.RosterContact.Render(" " + line))
				b.WriteString("\n")
			}
		}
		// Pad remaining
		for i := len(helpLines); i < visibleHeight; i++ {
			b.WriteString("\n")
		}
	} else {
		// Render contacts
		for i := m.offset; i < len(m.contacts) && i < m.offset+visibleHeight; i++ {
			c := m.contacts[i]
			selected := i == m.selected && m.focusSection == SectionContacts
			line := m.renderContact(c, selected)
			b.WriteString(line)
			b.WriteString("\n")
		}

		// Pad remaining contact lines
		contactsRendered := 0
		if len(m.contacts) > 0 {
			end := m.offset + visibleHeight
			if end > len(m.contacts) {
				end = len(m.contacts)
			}
			contactsRendered = end - m.offset
		}
		for i := contactsRendered; i < visibleHeight; i++ {
			b.WriteString("\n")
		}
	}

	// Render accounts section if there are accounts
	if len(m.accounts) > 0 {
		b.WriteString(m.renderAccountsSection())
	}

	return b.String()
}

// renderAccountsSection renders the accounts section at the bottom
func (m Model) renderAccountsSection() string {
	var b strings.Builder

	// Separator
	separator := strings.Repeat("═", m.width-4)
	b.WriteString(m.styles.RosterContact.Render(" " + separator))
	b.WriteString("\n")

	// Header
	b.WriteString(m.styles.RosterGroup.Width(m.width - 2).Render(" Accounts:"))
	b.WriteString("\n")

	// Determine how many accounts to show
	numToShow := m.maxVisibleAccounts
	if m.showAccountList || len(m.accounts) <= m.maxVisibleAccounts {
		numToShow = len(m.accounts)
	}

	// Render visible accounts
	for i := 0; i < numToShow && i < len(m.accounts); i++ {
		acc := m.accounts[i]
		selected := i == m.accountSelected && m.focusSection == SectionAccounts
		line := m.renderAccount(acc, selected)
		b.WriteString(line)
		if i < numToShow-1 || (len(m.accounts) > m.maxVisibleAccounts && !m.showAccountList) {
			b.WriteString("\n")
		}
	}

	// Show "+N more" if there are more accounts
	if len(m.accounts) > m.maxVisibleAccounts && !m.showAccountList {
		more := len(m.accounts) - m.maxVisibleAccounts
		moreText := fmt.Sprintf(" +%d more...", more)
		b.WriteString(m.styles.RosterContact.Width(m.width - 2).Render(moreText))
	}

	return b.String()
}

// renderAccount renders a single account line
func (m Model) renderAccount(acc AccountDisplay, selected bool) string {
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
	case "failed":
		indicator = "✘"
		indicatorStyle = m.styles.PresenceDND
	default:
		indicator = "○"
		indicatorStyle = m.styles.PresenceOffline
	}

	presence := indicatorStyle.Render(indicator)

	// JID (truncate if needed)
	jid := acc.JID
	maxWidth := m.width - 10 // presence + padding + unread
	if len(jid) > maxWidth && maxWidth > 0 {
		jid = jid[:maxWidth-1] + "…"
	}

	// Unread indicator
	unread := ""
	if acc.Unread > 0 {
		unread = m.styles.RosterUnread.Render(fmt.Sprintf(" (%d)", acc.Unread))
	}

	// Build line
	var style lipgloss.Style
	if selected {
		style = m.styles.RosterSelected
	} else {
		style = m.styles.RosterContact
	}

	content := fmt.Sprintf(" %s %s%s", presence, jid, unread)

	// Pad to width
	if len(content) < m.width-2 {
		content += strings.Repeat(" ", m.width-2-len(content))
	}

	return style.Width(m.width - 2).Render(content)
}

// renderContact renders a single contact line
func (m Model) renderContact(c Contact, selected bool) string {
	// Presence indicator
	var presenceStyle lipgloss.Style
	var indicator string
	switch c.Status {
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

	presence := presenceStyle.Render(indicator)

	// Contact name
	name := c.Name
	if name == "" {
		name = c.JID
	}

	// Truncate if needed
	maxWidth := m.width - 6 // presence + padding + unread
	if len(name) > maxWidth && maxWidth > 0 {
		name = name[:maxWidth-1] + "…"
	}

	// Unread indicator
	unread := ""
	if c.Unread > 0 {
		unread = m.styles.RosterUnread.Render(fmt.Sprintf(" (%d)", c.Unread))
	}

	// Build line
	var style lipgloss.Style
	if selected {
		style = m.styles.RosterSelected
	} else {
		style = m.styles.RosterContact
	}

	content := fmt.Sprintf(" %s %s%s", presence, name, unread)

	// Pad to width
	if len(content) < m.width-2 {
		content += strings.Repeat(" ", m.width-2-len(content))
	}

	return style.Width(m.width - 2).Render(content)
}
