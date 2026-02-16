package settings

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/meszmate/roster/internal/config"
	"github.com/meszmate/roster/internal/ui/theme"
)

// Section represents a settings section
type Section int

const (
	SectionGeneral Section = iota
	SectionUI
	SectionEncryption
	SectionPlugins
	SectionAccounts
	SectionAbout
)

// String returns the section name
func (s Section) String() string {
	switch s {
	case SectionGeneral:
		return "General"
	case SectionUI:
		return "User Interface"
	case SectionEncryption:
		return "Encryption"
	case SectionPlugins:
		return "Plugins"
	case SectionAccounts:
		return "Accounts"
	case SectionAbout:
		return "About"
	default:
		return "Unknown"
	}
}

// SettingType represents the type of a setting
type SettingType int

const (
	SettingBool SettingType = iota
	SettingString
	SettingSelect
	SettingNumber
)

// Setting represents a single setting
type Setting struct {
	Key         string
	Label       string
	Description string
	Type        SettingType
	Value       interface{}
	Options     []string // For SettingSelect
	Min, Max    int      // For SettingNumber
}

// SaveMsg is sent when settings are saved
type SaveMsg struct {
	Config *config.Config
}

// ConfirmSaveMessagesMsg is sent when user tries to enable message saving
type ConfirmSaveMessagesMsg struct{}

// EnableSaveMessagesMsg is sent to actually enable message saving after confirmation
type EnableSaveMessagesMsg struct{}

// Model represents the settings component
type Model struct {
	cfg        *config.Config
	section    Section
	settings   []Setting
	selected   int
	editing    bool
	editValue  string
	editCursor int
	width      int
	height     int
	styles     *theme.Styles
	changed    bool
	themes     []string
}

// New creates a new settings model
func New(cfg *config.Config, styles *theme.Styles, themes []string) Model {
	m := Model{
		cfg:     cfg,
		styles:  styles,
		section: SectionGeneral,
		themes:  themes,
	}
	m.loadSettings()
	return m
}

// loadSettings loads settings for the current section
func (m *Model) loadSettings() {
	m.settings = nil
	m.selected = 0

	switch m.section {
	case SectionGeneral:
		m.settings = []Setting{
			{
				Key:         "save_messages",
				Label:       "Save Messages",
				Description: "Save chat messages to database (requires confirmation)",
				Type:        SettingBool,
				Value:       m.cfg.Storage.SaveMessages,
			},
			{
				Key:         "save_window_state",
				Label:       "Save Window State",
				Description: "Remember open windows between sessions",
				Type:        SettingBool,
				Value:       m.cfg.Storage.SaveWindowState,
			},
		}

	case SectionUI:
		m.settings = []Setting{
			{
				Key:         "theme",
				Label:       "Theme",
				Description: "Color theme",
				Type:        SettingSelect,
				Value:       m.cfg.UI.Theme,
				Options:     m.themes,
			},
			{
				Key:         "roster_position",
				Label:       "Roster Position",
				Description: "Position of the roster panel",
				Type:        SettingSelect,
				Value:       m.cfg.UI.RosterPosition,
				Options:     []string{"left", "right"},
			},
			{
				Key:         "roster_width",
				Label:       "Roster Width",
				Description: "Width of the roster panel",
				Type:        SettingNumber,
				Value:       m.cfg.UI.RosterWidth,
				Min:         20,
				Max:         60,
			},
			{
				Key:         "show_timestamps",
				Label:       "Show Timestamps",
				Description: "Show message timestamps",
				Type:        SettingBool,
				Value:       m.cfg.UI.ShowTimestamps,
			},
			{
				Key:         "time_format",
				Label:       "Time Format",
				Description: "Format for timestamps (Go time format)",
				Type:        SettingString,
				Value:       m.cfg.UI.TimeFormat,
			},
			{
				Key:         "notifications",
				Label:       "Desktop Notifications",
				Description: "Show desktop notifications",
				Type:        SettingBool,
				Value:       m.cfg.UI.Notifications,
			},
		}

	case SectionEncryption:
		m.settings = []Setting{
			{
				Key:         "default_encryption",
				Label:       "Default Encryption",
				Description: "Default encryption method",
				Type:        SettingSelect,
				Value:       m.cfg.Encryption.Default,
				Options:     []string{"omemo", "otr", "pgp", "none"},
			},
			{
				Key:         "require_encryption",
				Label:       "Require Encryption",
				Description: "Require encryption for all messages",
				Type:        SettingBool,
				Value:       m.cfg.Encryption.RequireEncryption,
			},
			{
				Key:         "omemo_tofu",
				Label:       "OMEMO Trust on First Use",
				Description: "Automatically trust new devices",
				Type:        SettingBool,
				Value:       m.cfg.Encryption.OMEMOTOFU,
			},
		}

	case SectionPlugins:
		m.settings = []Setting{
			{
				Key:         "enabled_plugins",
				Label:       "Enabled Plugins",
				Description: "Comma-separated list of enabled plugins",
				Type:        SettingString,
				Value:       strings.Join(m.cfg.Plugins.Enabled, ","),
			},
			{
				Key:         "plugin_dir",
				Label:       "Plugin Directory",
				Description: "Custom plugin directory",
				Type:        SettingString,
				Value:       m.cfg.Plugins.PluginDir,
			},
		}

	case SectionAccounts:
		// Accounts are shown as a list with add/edit/remove options
		m.settings = []Setting{
			{
				Key:         "manage_accounts",
				Label:       "Manage Accounts",
				Description: "Press Enter to manage accounts",
				Type:        SettingBool,
				Value:       false,
			},
		}

	case SectionAbout:
		m.settings = []Setting{
			{
				Key:         "version",
				Label:       "Version",
				Description: "Roster XMPP Client",
				Type:        SettingString,
				Value:       "1.0.0",
			},
			{
				Key:         "website",
				Label:       "Website",
				Description: "Project homepage",
				Type:        SettingString,
				Value:       "https://github.com/meszmate/roster",
			},
			{
				Key:         "license",
				Label:       "License",
				Description: "Open source license",
				Type:        SettingString,
				Value:       "MIT",
			},
		}
	}
}

// SetSize sets the component size
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	return m
}

// SetSection changes to a different section
func (m Model) SetSection(section Section) Model {
	m.section = section
	m.loadSettings()
	return m
}

// NextSection moves to the next section
func (m Model) NextSection() Model {
	m.section = (m.section + 1) % 6
	m.loadSettings()
	return m
}

// PrevSection moves to the previous section
func (m Model) PrevSection() Model {
	if m.section == 0 {
		m.section = 5
	} else {
		m.section--
	}
	m.loadSettings()
	return m
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.editing {
			return m.handleEditMode(msg)
		}
		return m.handleNormalMode(msg)
	}
	return m, nil
}

// handleNormalMode handles keys in normal mode
func (m Model) handleNormalMode(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.selected < len(m.settings)-1 {
			m.selected++
		}

	case "k", "up":
		if m.selected > 0 {
			m.selected--
		}

	case "h", "left":
		m = m.PrevSection()

	case "l", "right", "tab":
		m = m.NextSection()

	case "enter", " ":
		if m.selected < len(m.settings) {
			setting := &m.settings[m.selected]
			switch setting.Type {
			case SettingBool:
				// Check if trying to enable save_messages - requires confirmation
				if setting.Key == "save_messages" && !setting.Value.(bool) {
					// Emit message to show confirmation dialog
					return m, func() tea.Msg {
						return ConfirmSaveMessagesMsg{}
					}
				}
				// Toggle boolean
				setting.Value = !setting.Value.(bool)
				m.changed = true
				m.applyChange(setting)

			case SettingSelect:
				// Cycle through options
				current := setting.Value.(string)
				for i, opt := range setting.Options {
					if opt == current {
						next := (i + 1) % len(setting.Options)
						setting.Value = setting.Options[next]
						m.changed = true
						m.applyChange(setting)
						break
					}
				}

			case SettingString, SettingNumber:
				// Enter edit mode
				m.editing = true
				m.editValue = fmt.Sprintf("%v", setting.Value)
				m.editCursor = len(m.editValue)
			}
		}

	case "s", "ctrl+s":
		// Save settings
		if m.changed {
			if err := config.Save(m.cfg); err == nil {
				m.changed = false
				return m, func() tea.Msg {
					return SaveMsg{Config: m.cfg}
				}
			}
		}
	}

	return m, nil
}

// handleEditMode handles keys in edit mode
func (m Model) handleEditMode(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.editing = false

	case tea.KeyEnter:
		// Apply edit
		if m.selected < len(m.settings) {
			setting := &m.settings[m.selected]
			if setting.Type == SettingNumber {
				// Parse number
				var num int
				_, _ = fmt.Sscanf(m.editValue, "%d", &num)
				if num < setting.Min {
					num = setting.Min
				}
				if num > setting.Max {
					num = setting.Max
				}
				setting.Value = num
			} else {
				setting.Value = m.editValue
			}
			m.changed = true
			m.applyChange(setting)
		}
		m.editing = false

	case tea.KeyBackspace:
		if m.editCursor > 0 {
			m.editValue = m.editValue[:m.editCursor-1] + m.editValue[m.editCursor:]
			m.editCursor--
		}

	case tea.KeyLeft:
		if m.editCursor > 0 {
			m.editCursor--
		}

	case tea.KeyRight:
		if m.editCursor < len(m.editValue) {
			m.editCursor++
		}

	case tea.KeyRunes:
		m.editValue = m.editValue[:m.editCursor] + string(msg.Runes) + m.editValue[m.editCursor:]
		m.editCursor += len(msg.Runes)
	}

	return m, nil
}

// applyChange applies a setting change to the config
func (m *Model) applyChange(setting *Setting) {
	switch setting.Key {
	// UI
	case "theme":
		m.cfg.UI.Theme = setting.Value.(string)
	case "roster_position":
		m.cfg.UI.RosterPosition = setting.Value.(string)
	case "roster_width":
		m.cfg.UI.RosterWidth = setting.Value.(int)
	case "show_timestamps":
		m.cfg.UI.ShowTimestamps = setting.Value.(bool)
	case "time_format":
		m.cfg.UI.TimeFormat = setting.Value.(string)
	case "notifications":
		m.cfg.UI.Notifications = setting.Value.(bool)

	// Encryption
	case "default_encryption":
		m.cfg.Encryption.Default = setting.Value.(string)
	case "require_encryption":
		m.cfg.Encryption.RequireEncryption = setting.Value.(bool)
	case "omemo_tofu":
		m.cfg.Encryption.OMEMOTOFU = setting.Value.(bool)

	// Plugins
	case "enabled_plugins":
		plugins := strings.Split(setting.Value.(string), ",")
		var cleaned []string
		for _, p := range plugins {
			p = strings.TrimSpace(p)
			if p != "" {
				cleaned = append(cleaned, p)
			}
		}
		m.cfg.Plugins.Enabled = cleaned
	case "plugin_dir":
		m.cfg.Plugins.PluginDir = setting.Value.(string)

	// Storage
	case "save_messages":
		m.cfg.Storage.SaveMessages = setting.Value.(bool)
	case "save_window_state":
		m.cfg.Storage.SaveWindowState = setting.Value.(bool)
	}
}

// View renders the settings
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	var b strings.Builder

	// Title
	title := m.styles.DialogTitle.Render("Settings")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Section tabs
	var tabs []string
	for i := Section(0); i <= SectionAbout; i++ {
		tab := i.String()
		if i == m.section {
			tab = m.styles.RosterSelected.Render(" " + tab + " ")
		} else {
			tab = m.styles.RosterContact.Render(" " + tab + " ")
		}
		tabs = append(tabs, tab)
	}
	b.WriteString(strings.Join(tabs, ""))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", m.width-4))
	b.WriteString("\n\n")

	// Settings list
	for i, setting := range m.settings {
		line := m.renderSetting(setting, i == m.selected)
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	if m.changed {
		b.WriteString(m.styles.ChatSystem.Render("Press 's' to save changes"))
	} else {
		b.WriteString(m.styles.ChatSystem.Render("j/k: navigate | Enter: edit | h/l: sections | Esc: close"))
	}

	// Wrap in border
	content := b.String()
	return m.styles.DialogBorder.
		Width(m.width).
		Height(m.height).
		Padding(1, 2).
		Render(content)
}

// renderSetting renders a single setting
func (m Model) renderSetting(setting Setting, selected bool) string {
	var value string

	switch setting.Type {
	case SettingBool:
		if setting.Value.(bool) {
			value = m.styles.PresenceOnline.Render("[✓]")
		} else {
			value = m.styles.PresenceOffline.Render("[ ]")
		}

	case SettingSelect:
		value = fmt.Sprintf("< %s >", setting.Value)

	case SettingString:
		if m.editing && selected {
			// Show cursor
			before := m.editValue[:m.editCursor]
			after := ""
			cursor := " "
			if m.editCursor < len(m.editValue) {
				cursor = string(m.editValue[m.editCursor])
				after = m.editValue[m.editCursor+1:]
			}
			cursorStyle := lipgloss.NewStyle().Reverse(true)
			value = before + cursorStyle.Render(cursor) + after
		} else {
			value = fmt.Sprintf("%v", setting.Value)
		}

	case SettingNumber:
		if m.editing && selected {
			before := m.editValue[:m.editCursor]
			after := ""
			cursor := " "
			if m.editCursor < len(m.editValue) {
				cursor = string(m.editValue[m.editCursor])
				after = m.editValue[m.editCursor+1:]
			}
			cursorStyle := lipgloss.NewStyle().Reverse(true)
			value = before + cursorStyle.Render(cursor) + after
		} else {
			value = fmt.Sprintf("%d", setting.Value)
		}
	}

	// Build line
	label := setting.Label
	desc := m.styles.ChatTimestamp.Render(setting.Description)

	var style lipgloss.Style
	if selected {
		style = m.styles.RosterSelected
	} else {
		style = m.styles.RosterContact
	}

	line := fmt.Sprintf("  %-25s %s", label, value)
	return style.Render(line) + "\n  " + desc
}

// HasChanges returns whether there are unsaved changes
func (m Model) HasChanges() bool {
	return m.changed
}

// Config returns the current config
func (m Model) Config() *config.Config {
	return m.cfg
}

// EnableSaveMessages enables message saving after user confirmation
func (m Model) EnableSaveMessages() Model {
	m.cfg.Storage.SaveMessages = true
	m.changed = true
	// Update the setting value in the current view if in General section
	for i := range m.settings {
		if m.settings[i].Key == "save_messages" {
			m.settings[i].Value = true
			break
		}
	}
	return m
}
