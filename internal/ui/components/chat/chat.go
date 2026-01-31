package chat

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/meszmate/roster/internal/ui/theme"
)

// Message represents a chat message
type Message struct {
	ID        string
	From      string
	To        string
	Body      string
	Timestamp time.Time
	Encrypted bool
	Read      bool
	Type      string // chat, groupchat, system
	Outgoing  bool
}

// SendMsg is sent when a message should be sent
type SendMsg struct {
	To   string
	Body string
}

// Model represents the chat component
type Model struct {
	messages    []Message
	jid         string
	input       string
	cursorPos   int
	offset      int
	width       int
	height      int
	styles      *theme.Styles
	encrypted   bool
	typing      bool
	peerTyping  bool
	searchQuery string
	searchMatches []int
	searchIndex int
	statusMsg   string  // Current activity/status message
}

// New creates a new chat model
func New(styles *theme.Styles) Model {
	return Model{
		messages:  []Message{},
		styles:    styles,
		encrypted: true,
	}
}

// SetJID sets the current chat JID
func (m Model) SetJID(jid string) Model {
	m.jid = jid
	m.input = ""
	m.cursorPos = 0
	return m
}

// SetHistory sets the chat history
func (m Model) SetHistory(messages []Message) Model {
	m.messages = messages
	m.offset = len(messages) - m.height + 3
	if m.offset < 0 {
		m.offset = 0
	}
	return m
}

// AddMessage adds a new message to the chat
func (m Model) AddMessage(msg interface{}) Model {
	if chatMsg, ok := msg.(Message); ok {
		m.messages = append(m.messages, chatMsg)
		// Auto-scroll to bottom if we were already at bottom
		if m.offset >= len(m.messages)-m.height {
			m.offset = len(m.messages) - m.height + 3
			if m.offset < 0 {
				m.offset = 0
			}
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

// ScrollUp scrolls the chat up
func (m Model) ScrollUp() Model {
	if m.offset > 0 {
		m.offset--
	}
	return m
}

// ScrollDown scrolls the chat down
func (m Model) ScrollDown() Model {
	maxOffset := len(m.messages) - m.height + 3
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset < maxOffset {
		m.offset++
	}
	return m
}

// ScrollToTop scrolls to the top of the chat
func (m Model) ScrollToTop() Model {
	m.offset = 0
	return m
}

// ScrollToBottom scrolls to the bottom of the chat
func (m Model) ScrollToBottom() Model {
	m.offset = len(m.messages) - m.height + 3
	if m.offset < 0 {
		m.offset = 0
	}
	return m
}

// HalfPageUp scrolls up by half a page
func (m Model) HalfPageUp() Model {
	m.offset -= m.height / 2
	if m.offset < 0 {
		m.offset = 0
	}
	return m
}

// HalfPageDown scrolls down by half a page
func (m Model) HalfPageDown() Model {
	m.offset += m.height / 2
	maxOffset := len(m.messages) - m.height + 3
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	return m
}

// SearchNext finds the next message matching the query
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
		matchIdx := m.searchMatches[m.searchIndex]
		if matchIdx < m.offset {
			m.offset = matchIdx
		} else if matchIdx >= m.offset+m.height-3 {
			m.offset = matchIdx - m.height + 4
		}
	}

	return m
}

// SearchPrev finds the previous message matching the query
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
		matchIdx := m.searchMatches[m.searchIndex]
		if matchIdx < m.offset {
			m.offset = matchIdx
		} else if matchIdx >= m.offset+m.height-3 {
			m.offset = matchIdx - m.height + 4
		}
	}

	return m
}

// findMatches finds all messages matching the query
func (m Model) findMatches(query string) []int {
	var matches []int
	query = strings.ToLower(query)
	for i, msg := range m.messages {
		body := strings.ToLower(msg.Body)
		if strings.Contains(body, query) {
			matches = append(matches, i)
		}
	}
	return matches
}

// SetEncrypted sets the encryption state
func (m Model) SetEncrypted(encrypted bool) Model {
	m.encrypted = encrypted
	return m
}

// SetPeerTyping sets whether the peer is typing
func (m Model) SetPeerTyping(typing bool) Model {
	m.peerTyping = typing
	return m
}

// SetStatusMsg sets a status message to display
func (m Model) SetStatusMsg(msg string) Model {
	m.statusMsg = msg
	return m
}

// ClearStatusMsg clears the status message
func (m Model) ClearStatusMsg() Model {
	m.statusMsg = ""
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
			m.typing = true

		case tea.KeyBackspace:
			if m.cursorPos > 0 {
				m.input = m.input[:m.cursorPos-1] + m.input[m.cursorPos:]
				m.cursorPos--
			}

		case tea.KeyDelete:
			if m.cursorPos < len(m.input) {
				m.input = m.input[:m.cursorPos] + m.input[m.cursorPos+1:]
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

		case tea.KeyEnter:
			if m.input != "" {
				sendMsg := SendMsg{
					To:   m.jid,
					Body: m.input,
				}
				m.input = ""
				m.cursorPos = 0
				m.typing = false
				return m, func() tea.Msg {
					return sendMsg
				}
			}

		case tea.KeySpace:
			m.input = m.input[:m.cursorPos] + " " + m.input[m.cursorPos:]
			m.cursorPos++
		}
	}

	return m, nil
}

// View renders the chat messages
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	var b strings.Builder

	// Header with JID
	header := m.jid
	if header == "" {
		header = "No chat selected"
	}

	// Encryption indicator
	if m.jid != "" {
		if m.encrypted {
			header += " " + m.styles.ChatEncrypted.Render("🔒")
		} else {
			header += " " + m.styles.ChatUnencrypted.Render("🔓")
		}
	}

	b.WriteString(m.styles.ChatNick.Render(header))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", m.width-2))
	b.WriteString("\n")

	// Calculate visible area
	visibleHeight := m.height - 4 // header + separator + typing indicator
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	// Show welcome message if no chat selected
	if m.jid == "" {
		welcomeLines := []string{
			"",
			"Welcome to Roster!",
			"",
			"A modern XMPP/Jabber client",
			"",
		}

		// Add status message if present
		if m.statusMsg != "" {
			welcomeLines = append(welcomeLines, ">>> "+m.statusMsg+" <<<", "")
		}

		welcomeLines = append(welcomeLines,
			"Quick Connect (session only):",
			"  :connect user@server.com password",
			"",
			"Or save an account:",
			"  :account add",
			"",
			"Then:",
			"  j/k + Enter - Open chat",
			"  i           - Type message",
			"  :help       - All commands",
			"  Esc         - Normal mode",
			"",
			"Windows: :1 :2 :wn :wp Alt+1-0",
		)
		for i, line := range welcomeLines {
			if i < visibleHeight {
				if strings.HasPrefix(line, ">>>") {
					b.WriteString(m.styles.PresenceAway.Render(line))
				} else {
					b.WriteString(m.styles.ChatSystem.Render(line))
				}
				b.WriteString("\n")
			}
		}
		// Pad remaining
		for i := len(welcomeLines); i < visibleHeight; i++ {
			b.WriteString("\n")
		}
		return b.String()
	}

	// Render messages
	msgCount := 0
	for i := m.offset; i < len(m.messages) && msgCount < visibleHeight; i++ {
		msg := m.messages[i]
		lines := m.renderMessage(msg)
		for _, line := range lines {
			if msgCount < visibleHeight {
				b.WriteString(line)
				b.WriteString("\n")
				msgCount++
			}
		}
	}

	// Show hint if no messages
	if len(m.messages) == 0 && msgCount == 0 {
		hint := m.styles.ChatSystem.Render("No messages yet. Press i to start typing.")
		b.WriteString(hint)
		b.WriteString("\n")
		msgCount++
	}

	// Pad remaining lines
	for i := msgCount; i < visibleHeight; i++ {
		b.WriteString("\n")
	}

	// Typing indicator
	if m.peerTyping {
		b.WriteString(m.styles.ChatTyping.Render("typing..."))
	}

	return b.String()
}

// renderMessage renders a single message
func (m Model) renderMessage(msg Message) []string {
	var lines []string

	// Timestamp
	timestamp := m.styles.ChatTimestamp.Render(msg.Timestamp.Format("15:04"))

	// Sender nick
	nick := msg.From
	if msg.Outgoing {
		nick = "me"
	}
	nickStr := m.styles.ChatNick.Render(nick)

	// Message body
	var bodyStyle lipgloss.Style
	if msg.Outgoing {
		bodyStyle = m.styles.ChatMyMessage
	} else {
		bodyStyle = m.styles.ChatTheirMessage
	}

	// Handle system messages
	if msg.Type == "system" {
		line := m.styles.ChatSystem.Render(fmt.Sprintf("*** %s", msg.Body))
		return []string{line}
	}

	// Word wrap message body
	maxWidth := m.width - 15 // timestamp + nick + padding
	if maxWidth < 10 {
		maxWidth = 10
	}

	wrapped := wordWrap(msg.Body, maxWidth)
	for i, line := range wrapped {
		var formatted string
		if i == 0 {
			formatted = fmt.Sprintf("%s %s: %s", timestamp, nickStr, bodyStyle.Render(line))
		} else {
			padding := strings.Repeat(" ", 6+len(nick)+2)
			formatted = padding + bodyStyle.Render(line)
		}
		lines = append(lines, formatted)
	}

	return lines
}

// InputView renders the input line
func (m Model) InputView() string {
	// Build prompt
	prompt := "> "
	if m.encrypted {
		prompt = m.styles.ChatEncrypted.Render("🔒 ") + prompt
	}

	// Render input with cursor
	beforeCursor := m.input[:m.cursorPos]
	afterCursor := ""
	cursorChar := " "
	if m.cursorPos < len(m.input) {
		cursorChar = string(m.input[m.cursorPos])
		afterCursor = m.input[m.cursorPos+1:]
	}

	cursor := lipgloss.NewStyle().Reverse(true).Render(cursorChar)
	input := prompt + beforeCursor + cursor + afterCursor

	return m.styles.CommandInput.Width(m.width).Render(input)
}

// wordWrap wraps text to the specified width
func wordWrap(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	var currentLine strings.Builder
	for _, word := range words {
		if currentLine.Len() == 0 {
			currentLine.WriteString(word)
		} else if currentLine.Len()+1+len(word) <= width {
			currentLine.WriteString(" ")
			currentLine.WriteString(word)
		} else {
			lines = append(lines, currentLine.String())
			currentLine.Reset()
			currentLine.WriteString(word)
		}
	}
	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return lines
}
