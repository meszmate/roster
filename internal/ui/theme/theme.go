package theme

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/lipgloss"
)

// Theme represents a complete UI theme
type Theme struct {
	Name        string           `toml:"name"`
	Description string           `toml:"description"`
	Colors      ColorsConfig     `toml:"colors"`
	Roster      RosterConfig     `toml:"roster"`
	Chat        ChatConfig       `toml:"chat"`
	StatusBar   StatusBarConfig  `toml:"statusbar"`
	CommandLine CommandLineConfig `toml:"commandline"`
	Dialogs     DialogsConfig    `toml:"dialogs"`
}

// ColorsConfig contains the base color palette
type ColorsConfig struct {
	Primary    string `toml:"primary"`
	Secondary  string `toml:"secondary"`
	Accent     string `toml:"accent"`
	Background string `toml:"background"`
	Foreground string `toml:"foreground"`
	Muted      string `toml:"muted"`
	Border     string `toml:"border"`
	Error      string `toml:"error"`
	Warning    string `toml:"warning"`
	Success    string `toml:"success"`
	Online     string `toml:"online"`
	Away       string `toml:"away"`
	DND        string `toml:"dnd"`
	XA         string `toml:"xa"`
	Offline    string `toml:"offline"`
}

// RosterConfig contains roster-specific styles
type RosterConfig struct {
	HeaderFg   string `toml:"header_fg"`
	HeaderBg   string `toml:"header_bg"`
	SelectedFg string `toml:"selected_fg"`
	SelectedBg string `toml:"selected_bg"`
	ContactFg  string `toml:"contact_fg"`
	GroupFg    string `toml:"group_fg"`
	BorderFg   string `toml:"border_fg"`
	UnreadFg   string `toml:"unread_fg"`
}

// ChatConfig contains chat-specific styles
type ChatConfig struct {
	MyMessageFg         string `toml:"my_message_fg"`
	MyMessageBg         string `toml:"my_message_bg"`
	TheirMessageFg      string `toml:"their_message_fg"`
	TheirMessageBg      string `toml:"their_message_bg"`
	TimestampFg         string `toml:"timestamp_fg"`
	NickFg              string `toml:"nick_fg"`
	EncryptedIndicator  string `toml:"encrypted_indicator"`
	UnencryptedIndicator string `toml:"unencrypted_indicator"`
	SystemMessageFg     string `toml:"system_message_fg"`
	TypingIndicatorFg   string `toml:"typing_indicator_fg"`
}

// StatusBarConfig contains status bar styles
type StatusBarConfig struct {
	Fg          string `toml:"fg"`
	Bg          string `toml:"bg"`
	ModeNormal  string `toml:"mode_normal"`
	ModeInsert  string `toml:"mode_insert"`
	ModeCommand string `toml:"mode_command"`
	AccountFg   string `toml:"account_fg"`
	StatusFg    string `toml:"status_fg"`
}

// CommandLineConfig contains command line styles
type CommandLineConfig struct {
	PromptFg   string `toml:"prompt_fg"`
	InputFg    string `toml:"input_fg"`
	InputBg    string `toml:"input_bg"`
	CursorFg   string `toml:"cursor_fg"`
	CompletionFg string `toml:"completion_fg"`
	CompletionBg string `toml:"completion_bg"`
}

// DialogsConfig contains dialog styles
type DialogsConfig struct {
	BorderFg    string `toml:"border_fg"`
	TitleFg     string `toml:"title_fg"`
	ContentFg   string `toml:"content_fg"`
	ButtonFg    string `toml:"button_fg"`
	ButtonBg    string `toml:"button_bg"`
	ButtonActiveFg string `toml:"button_active_fg"`
	ButtonActiveBg string `toml:"button_active_bg"`
}

// Styles contains the compiled lipgloss styles for a theme
type Styles struct {
	// Base styles
	Base       lipgloss.Style
	Focused    lipgloss.Style
	Border     lipgloss.Style

	// Roster styles
	RosterHeader   lipgloss.Style
	RosterSelected lipgloss.Style
	RosterContact  lipgloss.Style
	RosterGroup    lipgloss.Style
	RosterUnread   lipgloss.Style

	// Presence styles
	PresenceOnline  lipgloss.Style
	PresenceAway    lipgloss.Style
	PresenceDND     lipgloss.Style
	PresenceXA      lipgloss.Style
	PresenceOffline lipgloss.Style

	// Chat styles
	ChatMyMessage    lipgloss.Style
	ChatTheirMessage lipgloss.Style
	ChatTimestamp    lipgloss.Style
	ChatNick         lipgloss.Style
	ChatEncrypted    lipgloss.Style
	ChatUnencrypted  lipgloss.Style
	ChatSystem       lipgloss.Style
	ChatTyping       lipgloss.Style

	// Status bar styles
	StatusBar        lipgloss.Style
	StatusModeNormal lipgloss.Style
	StatusModeInsert lipgloss.Style
	StatusModeCommand lipgloss.Style
	StatusAccount    lipgloss.Style

	// Command line styles
	CommandPrompt     lipgloss.Style
	CommandInput      lipgloss.Style
	CommandCompletion lipgloss.Style

	// Dialog styles
	DialogBorder lipgloss.Style
	DialogTitle  lipgloss.Style
	DialogContent lipgloss.Style
	DialogButton lipgloss.Style
	DialogButtonActive lipgloss.Style

	// Input styles
	InputNormal lipgloss.Style
	InputFocused lipgloss.Style

	// Window styles
	WindowActive   lipgloss.Style
	WindowInactive lipgloss.Style
}

// Manager handles theme loading and switching
type Manager struct {
	themes       map[string]*Theme
	current      *Theme
	currentName  string
	styles       *Styles
	themeDirs    []string
}

// NewManager creates a new theme manager
func NewManager(themeDirs ...string) *Manager {
	m := &Manager{
		themes:    make(map[string]*Theme),
		themeDirs: themeDirs,
	}

	// Load built-in themes
	m.themes["rainbow"] = RainbowTheme()
	m.themes["matrix"] = MatrixTheme()
	m.themes["nord"] = NordTheme()
	m.themes["gruvbox"] = GruvboxTheme()
	m.themes["dracula"] = DraculaTheme()

	// Set rainbow as default
	m.current = m.themes["rainbow"]
	m.currentName = "rainbow"
	m.styles = m.compileStyles(m.current)

	return m
}

// LoadTheme loads a theme from a TOML file
func (m *Manager) LoadTheme(name string) error {
	for _, dir := range m.themeDirs {
		path := filepath.Join(dir, name+".toml")
		if _, err := os.Stat(path); err == nil {
			var theme Theme
			if _, err := toml.DecodeFile(path, &theme); err != nil {
				return fmt.Errorf("failed to parse theme file %s: %w", path, err)
			}
			theme.Name = name
			m.themes[name] = &theme
			return nil
		}
	}
	return fmt.Errorf("theme %s not found", name)
}

// SetTheme switches to a different theme
func (m *Manager) SetTheme(name string) error {
	theme, ok := m.themes[name]
	if !ok {
		// Try to load it
		if err := m.LoadTheme(name); err != nil {
			return err
		}
		theme = m.themes[name]
	}
	m.current = theme
	m.currentName = name
	m.styles = m.compileStyles(theme)
	return nil
}

// Current returns the current theme
func (m *Manager) Current() *Theme {
	return m.current
}

// CurrentName returns the current theme name
func (m *Manager) CurrentName() string {
	return m.currentName
}

// Styles returns the compiled styles for the current theme
func (m *Manager) Styles() *Styles {
	return m.styles
}

// AvailableThemes returns a list of available theme names
func (m *Manager) AvailableThemes() []string {
	names := make([]string, 0, len(m.themes))
	for name := range m.themes {
		names = append(names, name)
	}
	return names
}

// compileStyles compiles a theme into lipgloss styles
func (m *Manager) compileStyles(t *Theme) *Styles {
	s := &Styles{}

	// Base styles
	s.Base = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Colors.Foreground)).
		Background(lipgloss.Color(t.Colors.Background))

	s.Focused = s.Base.
		BorderForeground(lipgloss.Color(t.Colors.Primary))

	s.Border = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(t.Colors.Border))

	// Roster styles
	s.RosterHeader = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Roster.HeaderFg)).
		Background(lipgloss.Color(t.Roster.HeaderBg)).
		Bold(true).
		Padding(0, 1)

	s.RosterSelected = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Roster.SelectedFg)).
		Background(lipgloss.Color(t.Roster.SelectedBg)).
		Bold(true)

	s.RosterContact = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Roster.ContactFg))

	s.RosterGroup = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Roster.GroupFg)).
		Bold(true)

	s.RosterUnread = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Roster.UnreadFg)).
		Bold(true)

	// Presence styles
	s.PresenceOnline = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Colors.Online))

	s.PresenceAway = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Colors.Away))

	s.PresenceDND = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Colors.DND))

	s.PresenceXA = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Colors.XA))

	s.PresenceOffline = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Colors.Offline))

	// Chat styles
	s.ChatMyMessage = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Chat.MyMessageFg)).
		Background(lipgloss.Color(t.Chat.MyMessageBg))

	s.ChatTheirMessage = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Chat.TheirMessageFg)).
		Background(lipgloss.Color(t.Chat.TheirMessageBg))

	s.ChatTimestamp = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Chat.TimestampFg))

	s.ChatNick = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Chat.NickFg)).
		Bold(true)

	s.ChatEncrypted = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Chat.EncryptedIndicator))

	s.ChatUnencrypted = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Chat.UnencryptedIndicator))

	s.ChatSystem = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Chat.SystemMessageFg)).
		Italic(true)

	s.ChatTyping = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Chat.TypingIndicatorFg)).
		Italic(true)

	// Status bar styles
	s.StatusBar = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.StatusBar.Fg)).
		Background(lipgloss.Color(t.StatusBar.Bg))

	s.StatusModeNormal = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Colors.Background)).
		Background(lipgloss.Color(t.StatusBar.ModeNormal)).
		Bold(true).
		Padding(0, 1)

	s.StatusModeInsert = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Colors.Background)).
		Background(lipgloss.Color(t.StatusBar.ModeInsert)).
		Bold(true).
		Padding(0, 1)

	s.StatusModeCommand = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Colors.Background)).
		Background(lipgloss.Color(t.StatusBar.ModeCommand)).
		Bold(true).
		Padding(0, 1)

	s.StatusAccount = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.StatusBar.AccountFg)).
		Background(lipgloss.Color(t.StatusBar.Bg))

	// Command line styles
	s.CommandPrompt = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.CommandLine.PromptFg))

	s.CommandInput = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.CommandLine.InputFg)).
		Background(lipgloss.Color(t.CommandLine.InputBg))

	s.CommandCompletion = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.CommandLine.CompletionFg)).
		Background(lipgloss.Color(t.CommandLine.CompletionBg))

	// Dialog styles
	s.DialogBorder = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(t.Dialogs.BorderFg))

	s.DialogTitle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Dialogs.TitleFg)).
		Bold(true)

	s.DialogContent = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Dialogs.ContentFg))

	s.DialogButton = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Dialogs.ButtonFg)).
		Background(lipgloss.Color(t.Dialogs.ButtonBg)).
		Padding(0, 2)

	s.DialogButtonActive = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Dialogs.ButtonActiveFg)).
		Background(lipgloss.Color(t.Dialogs.ButtonActiveBg)).
		Bold(true).
		Padding(0, 2)

	// Input styles
	s.InputNormal = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(t.Colors.Border)).
		Padding(0, 1)

	s.InputFocused = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(t.Colors.Primary)).
		Padding(0, 1)

	// Window styles
	s.WindowActive = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(t.Colors.Primary))

	s.WindowInactive = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(t.Colors.Border))

	return s
}
