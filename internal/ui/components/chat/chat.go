package chat

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/meszmate/roster/internal/ui/theme"
)

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

// Spinner frames for the sending animation
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// SpinnerTickMsg is sent to animate the spinner
type SpinnerTickMsg struct{}

// SpinnerTick returns a command that sends a spinner tick after a delay
func SpinnerTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return SpinnerTickMsg{}
	})
}

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
	Status    MessageStatus // Delivery status for outgoing messages

	// File transfer fields
	FileURL  string // URL if message contains a file
	FileName string // Extracted filename
	FileSize int64  // Size in bytes if known
	FileMIME string // MIME type if known
}

// SendMsg is sent when a message should be sent
type SendMsg struct {
	To   string
	Body string
}

// Model represents the chat component
type Model struct {
	messages      []Message
	jid           string
	input         string
	cursorPos     int
	offset        int
	width         int
	height        int
	styles        *theme.Styles
	encrypted     bool
	typing        bool
	peerTyping    bool
	searchQuery   string
	searchMatches []int
	searchIndex   int
	statusMsg     string // Current activity/status message
	spinnerIdx    int    // Current spinner frame index
	selectedMsg   int    // Currently selected message index (for file operations)

	// Chat header state
	headerFocused  bool
	headerSelected int                // 0=edit, 1=sharing, 2=verify, 3=details
	contactData    *ContactDetailData // Contact info for header display
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

// SetHeaderFocused sets whether the header is focused
func (m Model) SetHeaderFocused(focused bool) Model {
	m.headerFocused = focused
	if focused {
		m.headerSelected = 0
	}
	return m
}

// IsHeaderFocused returns whether the header is focused
func (m Model) IsHeaderFocused() bool {
	return m.headerFocused
}

// SetContactData sets the contact data for header display
func (m Model) SetContactData(data *ContactDetailData) Model {
	m.contactData = data
	return m
}

// HeaderNavigateLeft moves header selection left
func (m Model) HeaderNavigateLeft() Model {
	if m.headerSelected > 0 {
		m.headerSelected--
	}
	return m
}

// HeaderNavigateRight moves header selection right
func (m Model) HeaderNavigateRight() Model {
	if m.headerSelected < 3 {
		m.headerSelected++
	}
	return m
}

// HeaderSelectedAction returns the currently selected header action
// 0=edit, 1=sharing, 2=verify, 3=details
func (m Model) HeaderSelectedAction() int {
	return m.headerSelected
}

// statusIcon returns the status icon for a message
func (m Model) statusIcon(status MessageStatus) string {
	switch status {
	case StatusSending:
		return spinnerFrames[m.spinnerIdx%len(spinnerFrames)]
	case StatusSent:
		return "✓"
	case StatusDelivered:
		return "✓✓"
	case StatusRead:
		return m.styles.PresenceOnline.Render("✓✓")
	case StatusFailed:
		return m.styles.PresenceDND.Render("✗")
	default:
		return ""
	}
}

// hasSendingMessages checks if there are any messages being sent
func (m Model) hasSendingMessages() bool {
	for _, msg := range m.messages {
		if msg.Status == StatusSending {
			return true
		}
	}
	return false
}

// UpdateMessageStatus updates the status of a message by ID
func (m Model) UpdateMessageStatus(msgID string, status MessageStatus) Model {
	for i, msg := range m.messages {
		if msg.ID == msgID {
			m.messages[i].Status = status
			break
		}
	}
	return m
}

// SelectedMessage returns the currently selected message (for file operations)
func (m Model) SelectedMessage() *Message {
	if m.selectedMsg >= 0 && m.selectedMsg < len(m.messages) {
		return &m.messages[m.selectedMsg]
	}
	return nil
}

// SelectNextFileMessage selects the next message that contains a file
func (m Model) SelectNextFileMessage() Model {
	for i := m.selectedMsg + 1; i < len(m.messages); i++ {
		if m.messages[i].FileURL != "" {
			m.selectedMsg = i
			return m
		}
	}
	// Wrap around
	for i := 0; i < m.selectedMsg; i++ {
		if m.messages[i].FileURL != "" {
			m.selectedMsg = i
			return m
		}
	}
	return m
}

// SelectPrevFileMessage selects the previous message that contains a file
func (m Model) SelectPrevFileMessage() Model {
	for i := m.selectedMsg - 1; i >= 0; i-- {
		if m.messages[i].FileURL != "" {
			m.selectedMsg = i
			return m
		}
	}
	// Wrap around
	for i := len(m.messages) - 1; i > m.selectedMsg; i-- {
		if m.messages[i].FileURL != "" {
			m.selectedMsg = i
			return m
		}
	}
	return m
}

// extractFileURL extracts HTTPS URLs from message body
func extractFileURL(body string) (string, bool) {
	// Match HTTPS URLs only (security)
	urlRegex := regexp.MustCompile(`https://[^\s<>"]+`)
	match := urlRegex.FindString(body)
	if match != "" {
		return match, true
	}
	return "", false
}

// extractFileName extracts filename from URL
func extractFileName(url string) string {
	// Get the path after the last /
	idx := strings.LastIndex(url, "/")
	if idx >= 0 && idx < len(url)-1 {
		name := url[idx+1:]
		// Remove query parameters
		if qIdx := strings.Index(name, "?"); qIdx > 0 {
			name = name[:qIdx]
		}
		return name
	}
	return ""
}

// humanizeBytes converts bytes to human readable format
func humanizeBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case SpinnerTickMsg:
		m.spinnerIdx++
		if m.hasSendingMessages() {
			return m, SpinnerTick()
		}
		return m, nil

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

	// Header with JID and contact info
	header := m.jid
	if header == "" {
		header = "No chat selected"
	}

	// Build header line 1: Name + status icon + encryption icon + fingerprint
	if m.jid != "" && m.contactData != nil {
		// Show contact name if available
		displayName := m.contactData.Name
		if displayName == "" {
			displayName = m.contactData.JID
		}

		// Status icon
		var statusIcon string
		switch m.contactData.Status {
		case "online":
			statusIcon = m.styles.PresenceOnline.Render("●")
		case "away":
			statusIcon = m.styles.PresenceAway.Render("◐")
		case "dnd":
			statusIcon = m.styles.PresenceDND.Render("⊘")
		case "xa":
			statusIcon = m.styles.PresenceXA.Render("◯")
		default:
			statusIcon = m.styles.PresenceOffline.Render("○")
		}

		header = fmt.Sprintf("%s %s", statusIcon, displayName)

		// Encryption indicator
		if m.encrypted {
			header += " " + m.styles.ChatEncrypted.Render("🔒")
		} else {
			header += " " + m.styles.ChatUnencrypted.Render("🔓")
		}
	} else if m.jid != "" {
		// Encryption indicator (fallback when no contact data)
		if m.encrypted {
			header += " " + m.styles.ChatEncrypted.Render("🔒")
		} else {
			header += " " + m.styles.ChatUnencrypted.Render("🔓")
		}
	}

	b.WriteString(m.styles.ChatNick.Render(header))
	b.WriteString("\n")

	// Header line 2: Action buttons (when focused)
	if m.headerFocused && m.jid != "" {
		actions := []string{"[E]dit", "[S]haring", "[V]erify", "[D]etails"}
		var actionLine strings.Builder
		actionLine.WriteString("  ")
		for i, action := range actions {
			if i == m.headerSelected {
				actionLine.WriteString(m.styles.RosterSelected.Render(action))
			} else {
				actionLine.WriteString(m.styles.ChatSystem.Render(action))
			}
			if i < len(actions)-1 {
				actionLine.WriteString("  ")
			}
		}
		b.WriteString(actionLine.String())
		b.WriteString("\n")
	}

	b.WriteString(strings.Repeat("─", m.width-2))
	b.WriteString("\n")

	// Calculate visible area
	visibleHeight := m.height - 4 // header + separator + typing indicator
	if m.headerFocused && m.jid != "" {
		visibleHeight-- // Account for action buttons line
	}
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

	// Status icon for outgoing messages
	statusStr := ""
	if msg.Outgoing && msg.Status != StatusNone {
		statusStr = " " + m.statusIcon(msg.Status)
	}

	// Message body style
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

	// Check if message contains a file URL
	if msg.FileURL != "" || (msg.Body != "" && strings.HasPrefix(msg.Body, "https://")) {
		return m.renderFileMessage(msg, timestamp, nickStr, statusStr)
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
			formatted = fmt.Sprintf("%s %s: %s%s", timestamp, nickStr, bodyStyle.Render(line), statusStr)
		} else {
			padding := strings.Repeat(" ", 6+len(nick)+2)
			formatted = padding + bodyStyle.Render(line)
		}
		lines = append(lines, formatted)
	}

	return lines
}

// renderFileMessage renders a message that contains a file URL
func (m Model) renderFileMessage(msg Message, timestamp, nickStr, statusStr string) []string {
	var lines []string

	// Extract URL and filename
	fileURL := msg.FileURL
	if fileURL == "" {
		if url, ok := extractFileURL(msg.Body); ok {
			fileURL = url
		}
	}

	fileName := msg.FileName
	if fileName == "" {
		fileName = extractFileName(fileURL)
	}
	if fileName == "" {
		fileName = "file"
	}

	// First line: timestamp + nick + file icon
	firstLine := fmt.Sprintf("%s %s: 📎 %s", timestamp, nickStr, fileName)
	if msg.FileSize > 0 {
		firstLine += fmt.Sprintf(" (%s)", humanizeBytes(msg.FileSize))
	}
	firstLine += statusStr
	lines = append(lines, firstLine)

	// Second line: URL (truncated if needed)
	urlDisplay := fileURL
	maxURLWidth := m.width - 10
	if len(urlDisplay) > maxURLWidth && maxURLWidth > 20 {
		urlDisplay = urlDisplay[:maxURLWidth-3] + "..."
	}
	urlLine := strings.Repeat(" ", 6+4+2) + m.styles.ChatSystem.Render(urlDisplay)
	lines = append(lines, urlLine)

	// Third line: actions hint
	actionsLine := strings.Repeat(" ", 6+4+2) + m.styles.ChatTimestamp.Render("[o=open  c=copy URL]")
	lines = append(lines, actionsLine)

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

// formatFingerprint formats a fingerprint for display (in 4-char chunks)
func formatFingerprint(fp string) string {
	if len(fp) == 0 {
		return ""
	}
	var result strings.Builder
	for i, c := range fp {
		if i > 0 && i%4 == 0 {
			result.WriteString(" ")
		}
		result.WriteRune(c)
	}
	return result.String()
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

// AccountDetailData holds data for rendering account details
type AccountDetailData struct {
	JID              string
	Status           string // online, connecting, failed, offline
	Server           string
	Port             int
	Resource         string
	OMEMO            bool
	AutoConnect      bool
	Session          bool
	UnreadMsgs       int
	UnreadChats      int
	OMEMOFingerprint string // Own OMEMO fingerprint
	OMEMODeviceID    uint32 // Own device ID
}

// ContactDetailData holds data for rendering contact details
type ContactDetailData struct {
	JID           string
	Name          string
	Status        string // online, away, dnd, xa, offline
	StatusMsg     string
	Groups        []string
	Subscription  string
	MyPresence    string // Your custom presence for this contact (empty = default)
	MyPresenceMsg string
	LastSeen      time.Time
	StatusSharing bool // Whether you share your status with this contact
	OMEMOEnabled  bool // Whether OMEMO is enabled for this contact
	Fingerprints  []FingerprintDisplay
}

// FingerprintDisplay holds fingerprint info for display
type FingerprintDisplay struct {
	DeviceID    uint32
	Fingerprint string
	Trust       string // "verified", "trusted", "untrusted", "undecided"
}

// RenderAccountDetails renders the account details view
func (m Model) RenderAccountDetails(acc AccountDetailData) string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	var b strings.Builder

	// Header
	header := "Account Details"
	b.WriteString(m.styles.ChatNick.Render(header))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", m.width-2))
	b.WriteString("\n\n")

	// Status with icon
	var statusIcon, statusText string
	switch acc.Status {
	case "online":
		statusIcon = m.styles.PresenceOnline.Render("●")
		statusText = "Online"
	case "connecting":
		statusIcon = m.styles.PresenceAway.Render("◐")
		statusText = "Connecting..."
	case "disconnecting":
		statusIcon = m.styles.PresenceAway.Render("◐")
		statusText = "Disconnecting..."
	case "failed":
		statusIcon = m.styles.PresenceDND.Render("✘")
		statusText = "Connection Failed"
	default:
		statusIcon = m.styles.PresenceOffline.Render("○")
		statusText = "Offline"
	}
	b.WriteString(fmt.Sprintf("  Status: %s %s\n", statusIcon, statusText))

	// JID
	b.WriteString(fmt.Sprintf("  JID: %s\n", acc.JID))

	// Server info
	if acc.Server != "" {
		serverStr := acc.Server
		if acc.Port > 0 && acc.Port != 5222 {
			serverStr = fmt.Sprintf("%s:%d", acc.Server, acc.Port)
		}
		b.WriteString(fmt.Sprintf("  Server: %s\n", serverStr))
	}

	// Resource
	if acc.Resource != "" {
		b.WriteString(fmt.Sprintf("  Resource: %s\n", acc.Resource))
	}

	b.WriteString("\n")

	// Features
	omemoStr := "OFF"
	if acc.OMEMO {
		omemoStr = m.styles.ChatEncrypted.Render("ON")
	}
	b.WriteString(fmt.Sprintf("  OMEMO: %s\n", omemoStr))

	// Show OMEMO fingerprint if available
	if acc.OMEMO {
		if acc.OMEMODeviceID > 0 && acc.OMEMOFingerprint != "" {
			b.WriteString(fmt.Sprintf("  Device ID: %d\n", acc.OMEMODeviceID))
			fpFormatted := formatFingerprint(acc.OMEMOFingerprint)
			b.WriteString("  Fingerprint:\n")
			b.WriteString(fmt.Sprintf("    %s\n", fpFormatted))
		} else if acc.Status == "online" {
			b.WriteString(m.styles.ChatSystem.Render("  (OMEMO keys will be generated on first message)") + "\n")
		} else {
			b.WriteString(m.styles.ChatSystem.Render("  (Connect to generate OMEMO keys)") + "\n")
		}
	}

	autoStr := "OFF"
	if acc.AutoConnect {
		autoStr = "ON"
	}
	b.WriteString(fmt.Sprintf("  AutoConnect: [%s]\n", autoStr))

	// Account type
	typeStr := "Saved"
	if acc.Session {
		typeStr = "Session (not saved)"
	}
	b.WriteString(fmt.Sprintf("  Type: %s\n", typeStr))

	b.WriteString("\n")

	// Unread summary
	if acc.UnreadMsgs > 0 {
		unreadStr := fmt.Sprintf("  Unread: %d messages", acc.UnreadMsgs)
		if acc.UnreadChats > 0 {
			unreadStr += fmt.Sprintf(" from %d chats", acc.UnreadChats)
		}
		b.WriteString(m.styles.RosterUnread.Render(unreadStr) + "\n")
		b.WriteString("\n")
	}

	// Calculate remaining height for padding
	visibleHeight := m.height - 4
	currentLines := strings.Count(b.String(), "\n")
	for i := currentLines; i < visibleHeight-3; i++ {
		b.WriteString("\n")
	}

	// Actions hint at bottom
	b.WriteString(strings.Repeat("─", m.width-2))
	b.WriteString("\n")
	b.WriteString(m.styles.ChatSystem.Render("  E=edit  C=connect  D=disconnect  T=toggle auto  X=remove  esc=back"))
	b.WriteString("\n")

	return b.String()
}

// RenderContactDetails renders the contact details view
func (m Model) RenderContactDetails(contact ContactDetailData) string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	var b strings.Builder

	// Header
	header := "Roster Details"
	b.WriteString(m.styles.ChatNick.Render(header))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", m.width-2))
	b.WriteString("\n\n")

	// Name and JID
	if contact.Name != "" && contact.Name != contact.JID {
		b.WriteString(fmt.Sprintf("  Name: %s\n", contact.Name))
	}
	b.WriteString(fmt.Sprintf("  JID: %s\n", contact.JID))

	b.WriteString("\n")

	// Status with icon and friendly text
	var statusIcon, statusText string
	switch contact.Status {
	case "online":
		statusIcon = m.styles.PresenceOnline.Render("●")
		statusText = "Online"
	case "away":
		statusIcon = m.styles.PresenceAway.Render("◐")
		statusText = "Away"
	case "dnd":
		statusIcon = m.styles.PresenceDND.Render("⊘")
		statusText = "Do Not Disturb"
	case "xa":
		statusIcon = m.styles.PresenceXA.Render("◯")
		statusText = "Extended Away"
	default:
		statusIcon = m.styles.PresenceOffline.Render("○")
		statusText = "Offline"
	}

	b.WriteString(fmt.Sprintf("  Status: %s %s\n", statusIcon, statusText))

	// Status message if set
	if contact.StatusMsg != "" {
		b.WriteString(fmt.Sprintf("  Message: \"%s\"\n", contact.StatusMsg))
	}

	// For offline contacts, show last seen or indicate status not shared
	if contact.Status == "offline" {
		if !contact.LastSeen.IsZero() {
			lastSeenStr := contact.LastSeen.Format("2006-01-02 15:04")
			b.WriteString(fmt.Sprintf("  Last seen: %s\n", lastSeenStr))
		} else {
			// Contact is offline and we have no last seen info - status not shared
			b.WriteString(m.styles.ChatSystem.Render("  (status not shared)") + "\n")
		}
	}

	b.WriteString("\n")

	// Groups
	if len(contact.Groups) > 0 {
		b.WriteString(fmt.Sprintf("  Groups: %s\n", strings.Join(contact.Groups, ", ")))
	} else {
		b.WriteString("  Groups: (none)\n")
	}

	// Subscription
	if contact.Subscription != "" {
		b.WriteString(fmt.Sprintf("  Subscription: %s\n", contact.Subscription))
	}

	b.WriteString("\n")

	// Status sharing toggle
	sharingStr := "[OFF]"
	if contact.StatusSharing {
		sharingStr = m.styles.PresenceOnline.Render("[ON]")
	}
	b.WriteString(fmt.Sprintf("  Status sharing: %s\n", sharingStr))

	// Your presence for this roster entry
	if contact.MyPresence != "" {
		presenceStr := contact.MyPresence
		if contact.MyPresenceMsg != "" {
			presenceStr += ": " + contact.MyPresenceMsg
		}
		b.WriteString(fmt.Sprintf("  Your presence for this roster entry: [%s]\n", presenceStr))
	} else {
		b.WriteString("  Your presence for this roster entry: [default]\n")
	}

	b.WriteString("\n")

	// Encryption section
	b.WriteString("  ── Encryption ──\n")
	if contact.OMEMOEnabled {
		b.WriteString("  OMEMO: " + m.styles.ChatEncrypted.Render("Enabled") + "\n")
	} else {
		b.WriteString("  OMEMO: Disabled\n")
	}

	// Fingerprints
	if len(contact.Fingerprints) > 0 {
		b.WriteString("  Devices:\n")
		for _, fp := range contact.Fingerprints {
			var trustStr string
			switch fp.Trust {
			case "verified":
				trustStr = m.styles.PresenceOnline.Render("[verified]")
			case "trusted":
				trustStr = m.styles.PresenceAway.Render("[trusted]")
			case "untrusted":
				trustStr = m.styles.PresenceDND.Render("[untrusted]")
			default:
				trustStr = "[undecided]"
			}
			b.WriteString(fmt.Sprintf("    Device %d: %s\n", fp.DeviceID, trustStr))
			// Show fingerprint in chunks
			fpFormatted := formatFingerprint(fp.Fingerprint)
			b.WriteString(fmt.Sprintf("      %s\n", fpFormatted))
		}
	}

	// Calculate remaining height for padding
	visibleHeight := m.height - 4
	currentLines := strings.Count(b.String(), "\n")
	for i := currentLines; i < visibleHeight-3; i++ {
		b.WriteString("\n")
	}

	// Actions hint at bottom
	b.WriteString(strings.Repeat("─", m.width-2))
	b.WriteString("\n")
	b.WriteString(m.styles.ChatSystem.Render("  E=edit  enter=chat  s=toggle sharing  v=verify  esc=back"))
	b.WriteString("\n")

	return b.String()
}

// AccountEditData holds data for editing an account
type AccountEditData struct {
	JID           string
	Server        string
	Port          int
	Resource      string
	AutoConnect   bool
	OMEMO         bool
	SelectedField int    // 0=server, 1=port, 2=resource, 3=autoconnect, 4=omemo
	EditingField  bool   // true when actively editing a text field
	EditBuffer    string // current edit buffer for text fields
	CursorPos     int    // cursor position in edit buffer
}

// RenderAccountEdit renders the account edit view
func (m Model) RenderAccountEdit(edit AccountEditData) string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	var b strings.Builder

	// Header
	header := "Edit Account"
	b.WriteString(m.styles.ChatNick.Render(header))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", m.width-2))
	b.WriteString("\n\n")

	// JID (read-only)
	b.WriteString(fmt.Sprintf("  JID: %s (read-only)\n\n", edit.JID))

	// Editable fields
	fields := []struct {
		label string
		value string
		idx   int
	}{
		{"Server", edit.Server, 0},
		{"Port", fmt.Sprintf("%d", edit.Port), 1},
		{"Resource", edit.Resource, 2},
	}

	for _, field := range fields {
		prefix := "  "
		if edit.SelectedField == field.idx {
			prefix = "> "
		}

		if edit.SelectedField == field.idx && edit.EditingField {
			// Show edit buffer with cursor
			beforeCursor := edit.EditBuffer[:edit.CursorPos]
			afterCursor := ""
			cursorChar := " "
			if edit.CursorPos < len(edit.EditBuffer) {
				cursorChar = string(edit.EditBuffer[edit.CursorPos])
				afterCursor = edit.EditBuffer[edit.CursorPos+1:]
			}
			cursor := lipgloss.NewStyle().Reverse(true).Render(cursorChar)
			b.WriteString(fmt.Sprintf("%s%s: %s%s%s\n", prefix, field.label, beforeCursor, cursor, afterCursor))
		} else {
			value := field.value
			if value == "" {
				value = "(empty)"
			}
			if edit.SelectedField == field.idx {
				b.WriteString(m.styles.RosterSelected.Render(fmt.Sprintf("%s%s: %s", prefix, field.label, value)) + "\n")
			} else {
				b.WriteString(fmt.Sprintf("%s%s: %s\n", prefix, field.label, value))
			}
		}
	}

	b.WriteString("\n")

	// Toggle fields
	toggleFields := []struct {
		label string
		value bool
		idx   int
	}{
		{"AutoConnect", edit.AutoConnect, 3},
		{"OMEMO", edit.OMEMO, 4},
	}

	for _, field := range toggleFields {
		prefix := "  "
		if edit.SelectedField == field.idx {
			prefix = "> "
		}

		valueStr := "[OFF]"
		if field.value {
			valueStr = "[ON]"
		}

		if edit.SelectedField == field.idx {
			b.WriteString(m.styles.RosterSelected.Render(fmt.Sprintf("%s%s: %s", prefix, field.label, valueStr)) + "\n")
		} else {
			b.WriteString(fmt.Sprintf("%s%s: %s\n", prefix, field.label, valueStr))
		}
	}

	// Calculate remaining height for padding
	visibleHeight := m.height - 4
	currentLines := strings.Count(b.String(), "\n")
	for i := currentLines; i < visibleHeight-4; i++ {
		b.WriteString("\n")
	}

	// Actions hint at bottom
	b.WriteString(strings.Repeat("─", m.width-2))
	b.WriteString("\n")
	if edit.EditingField {
		b.WriteString(m.styles.ChatSystem.Render("  enter=save field  esc=cancel field"))
	} else {
		b.WriteString(m.styles.ChatSystem.Render("  j/k=navigate  enter=edit/toggle  S=save all  esc=cancel"))
	}
	b.WriteString("\n")

	return b.String()
}
