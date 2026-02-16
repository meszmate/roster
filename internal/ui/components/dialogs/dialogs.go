package dialogs

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/meszmate/roster/internal/ui/theme"
)

// DialogType represents the type of dialog
type DialogType int

const (
	DialogNone DialogType = iota
	DialogError
	DialogConfirm
	DialogInputType
	DialogAddContact
	DialogJoinRoom
	DialogCreateRoom
	DialogContactInfo
	DialogFingerprint
	DialogOMEMODevices
	DialogSubscription
	DialogHelp
	DialogAccountAdd
	DialogAccountEdit
	DialogAccountList
	DialogPassword
	DialogSettings
	DialogContextHelp
	DialogAccountRemove
	DialogLoading
	DialogRegister
	DialogRegisterForm
	DialogRegisterSuccess
	DialogConfirmSaveMessages
)

// DialogAction represents what action triggered the dialog result
type DialogAction int

const (
	ActionConfirm DialogAction = iota
	ActionCancel
	ActionViewCaptcha
	ActionCopyURL
)

// OperationType identifies which async operation is in progress
type OperationType string

const (
	OpNone           OperationType = ""
	OpAddContact     OperationType = "add_contact"
	OpJoinRoom       OperationType = "join_room"
	OpCreateRoom     OperationType = "create_room"
	OpConnect        OperationType = "connect"
	OpDisconnect     OperationType = "disconnect"
	OpRegisterFetch  OperationType = "register_fetch"
	OpRegisterSubmit OperationType = "register_submit"
)

// Spinner frames for loading animation
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// DialogResult is sent when a dialog is closed
type DialogResult struct {
	Type      DialogType
	Confirmed bool
	Button    int // Index of the button that was pressed
	Action    DialogAction
	Values    map[string]string
}

// SpinnerTickMsg is sent to animate the loading spinner
type SpinnerTickMsg struct{}

// CancelOperationMsg is sent when user cancels a loading operation
type CancelOperationMsg struct {
	Operation OperationType
}

// Model represents the dialog component
type Model struct {
	dialogType     DialogType
	title          string
	message        string
	inputs         []DialogInput
	checkboxes     []DialogCheckbox
	activeInput    int
	activeCheckbox int
	inCheckboxes   bool
	buttons        []string
	activeBtn      int
	width          int
	height         int
	styles         *theme.Styles
	data           map[string]string

	// Loading dialog state
	spinnerFrame  int
	operationType OperationType

	// Scroll state for help dialog
	scrollOffset    int
	maxVisibleLines int

	// OMEMO devices
	omemoDevices   []OMEMODeviceInfo
	selectedDevice int
}

// OMEMODeviceInfo represents info about an OMEMO device
type OMEMODeviceInfo struct {
	DeviceID    uint32
	Fingerprint string
	TrustLevel  int
	TrustString string
}

// DialogInput represents an input field in a dialog
type DialogInput struct {
	Label    string
	Key      string
	Value    string
	Cursor   int
	Password bool
	ReadOnly bool // If true, field is not editable (for display only)
}

// DialogCheckbox represents a checkbox in a dialog
type DialogCheckbox struct {
	Label   string
	Key     string
	Checked bool
}

// New creates a new dialog model
func New(styles *theme.Styles) Model {
	return Model{
		dialogType: DialogNone,
		styles:     styles,
		buttons:    []string{"OK", "Cancel"},
		data:       make(map[string]string),
	}
}

// Active returns whether a dialog is active
func (m Model) Active() bool {
	return m.dialogType != DialogNone
}

// Type returns the dialog type
func (m Model) Type() DialogType {
	return m.dialogType
}

// ShowError shows an error dialog
func (m Model) ShowError(message string) Model {
	m.dialogType = DialogError
	m.title = "Error"
	m.message = message
	m.buttons = []string{"OK"}
	m.activeBtn = 0
	m.inputs = nil
	return m
}

// ShowConfirm shows a confirmation dialog
func (m Model) ShowConfirm(title, message string) Model {
	m.dialogType = DialogConfirm
	m.title = title
	m.message = message
	m.buttons = []string{"Yes", "No"}
	m.activeBtn = 0
	m.inputs = nil
	return m
}

// ShowConfirmSaveMessages shows the confirmation dialog for enabling message persistence
func (m Model) ShowConfirmSaveMessages() Model {
	m.dialogType = DialogConfirmSaveMessages
	m.title = "Enable Message Persistence"
	m.message = "This will save your chat messages to a local SQLite database.\n\n" +
		"Messages will be stored locally on your device and persist across sessions.\n\n" +
		"Are you sure you want to enable message saving?"
	m.buttons = []string{"Enable", "Cancel"}
	m.activeBtn = 1 // Default to Cancel for safety
	m.inputs = nil
	return m
}

// ShowAddContact shows the add contact dialog
func (m Model) ShowAddContact() Model {
	m.dialogType = DialogAddContact
	m.title = "Add to Roster"
	m.message = ""
	m.inputs = []DialogInput{
		{Label: "JID", Key: "jid", Value: ""},
		{Label: "Name", Key: "name", Value: ""},
		{Label: "Group", Key: "group", Value: ""},
	}
	m.checkboxes = nil
	m.inCheckboxes = false
	m.activeInput = 0
	m.buttons = []string{"Add", "Cancel"}
	m.activeBtn = 0
	return m
}

// ShowJoinRoom shows the join room dialog
func (m Model) ShowJoinRoom() Model {
	m.dialogType = DialogJoinRoom
	m.title = "Join Room"
	m.message = ""
	m.inputs = []DialogInput{
		{Label: "Room JID", Key: "room", Value: ""},
		{Label: "Nickname", Key: "nick", Value: ""},
		{Label: "Password", Key: "password", Value: "", Password: true},
	}
	m.checkboxes = nil
	m.inCheckboxes = false
	m.activeInput = 0
	m.buttons = []string{"Join", "Cancel"}
	m.activeBtn = 0
	return m
}

// ShowContactInfo shows contact info dialog
func (m Model) ShowContactInfo(jid string) Model {
	m.dialogType = DialogContactInfo
	m.title = "Roster Info"
	m.message = jid
	m.buttons = []string{"Close"}
	m.activeBtn = 0
	m.inputs = nil
	m.data["jid"] = jid
	return m
}

// ShowFingerprint shows fingerprint verification dialog
func (m Model) ShowFingerprint(jid string, fingerprints []string) Model {
	m.dialogType = DialogFingerprint
	m.title = "Verify Fingerprint"
	m.message = "Fingerprints for " + jid + ":\n\n" + strings.Join(fingerprints, "\n")
	m.buttons = []string{"Trust", "Untrust", "Close"}
	m.activeBtn = 0
	m.inputs = nil
	m.data["jid"] = jid
	return m
}

// ShowOMEMODevices shows OMEMO device management dialog
func (m Model) ShowOMEMODevices(jid string, devices []OMEMODeviceInfo) Model {
	m.dialogType = DialogOMEMODevices
	m.title = "OMEMO Devices - " + jid
	m.omemoDevices = devices
	m.selectedDevice = 0
	m.buttons = []string{"Trust", "Verify", "Untrust", "Delete", "Close"}
	m.activeBtn = 4
	m.inputs = nil
	m.data["jid"] = jid
	return m
}

// GetSelectedOMEMODevice returns the currently selected OMEMO device
func (m Model) GetSelectedOMEMODevice() (OMEMODeviceInfo, int, bool) {
	if len(m.omemoDevices) == 0 || m.selectedDevice >= len(m.omemoDevices) {
		return OMEMODeviceInfo{}, 0, false
	}
	return m.omemoDevices[m.selectedDevice], m.selectedDevice, true
}

// ShowSubscription shows subscription request dialog
func (m Model) ShowSubscription(jid string) Model {
	m.dialogType = DialogSubscription
	m.title = "Subscription Request"
	m.message = jid + " wants to add you to their roster"
	m.buttons = []string{"Accept", "Deny"}
	m.activeBtn = 0
	m.inputs = nil
	m.data["jid"] = jid
	return m
}

// ShowHelp shows the help dialog with all commands
func (m Model) ShowHelp(commands map[string]string) Model {
	m.dialogType = DialogHelp
	m.title = "Help - Available Commands"

	// Build help message
	var sb strings.Builder
	sb.WriteString("Keybindings (Normal Mode):\n")
	sb.WriteString("  j/k       Move down/up\n")
	sb.WriteString("  gg/G      Top/bottom\n")
	sb.WriteString("  Ctrl+u/d  Half page up/down\n")
	sb.WriteString("  /         Search\n")
	sb.WriteString("  n/N       Next/prev search result\n")
	sb.WriteString("  :         Command mode\n")
	sb.WriteString("  i         Insert mode (chat)\n")
	sb.WriteString("  Enter     Open chat\n")
	sb.WriteString("  q         Close chat\n")
	sb.WriteString("  Esc       Back to normal mode\n")
	sb.WriteString("  H         Context help popup\n")
	sb.WriteString("\nFocus (g prefix):\n")
	sb.WriteString("  gr        Focus roster\n")
	sb.WriteString("  gc        Focus chat\n")
	sb.WriteString("  gA        Focus accounts\n")
	sb.WriteString("  gl        Toggle account list\n")
	sb.WriteString("\nAccount Actions (in accounts section):\n")
	sb.WriteString("  H         Show account info tooltip\n")
	sb.WriteString("  C         Connect account\n")
	sb.WriteString("  D         Disconnect account\n")
	sb.WriteString("  E         Edit account\n")
	sb.WriteString("  X         Remove account\n")
	sb.WriteString("\nActions (g prefix):\n")
	sb.WriteString("  ga        Add to roster\n")
	sb.WriteString("  gx        Remove from roster\n")
	sb.WriteString("  gR        Rename roster entry\n")
	sb.WriteString("  gj        Join room\n")
	sb.WriteString("  gC        Create room\n")
	sb.WriteString("  gs/S      Settings\n")
	sb.WriteString("  gw        Save windows\n")
	sb.WriteString("\nRoster Details:\n")
	sb.WriteString("  s         Toggle status sharing\n")
	sb.WriteString("  v         Verify fingerprint\n")
	sb.WriteString("  Space     Bind account to window\n")
	sb.WriteString("\nWindows:\n")
	sb.WriteString("  Alt+1-0   Windows 1-10\n")
	sb.WriteString("  Tab       Next window\n")
	sb.WriteString("\nCommands (press : first):\n")

	// Add command summaries
	cmdList := []string{
		"connect <jid> <pass> [server] [port]",
		"account add    - Add saved account",
		"account edit <jid> - Edit account",
		"account resource <jid> <name> - Set resource",
		"disconnect     - Disconnect",
		"1-20           - Switch window",
		"set <k> <v>    - Change setting",
		"quit           - Exit",
	}
	for _, cmd := range cmdList {
		sb.WriteString("  :" + cmd + "\n")
	}
	sb.WriteString("\nThemes: matrix, nord, gruvbox, dracula, rainbow")

	m.message = sb.String()
	m.buttons = []string{"Close"}
	m.activeBtn = 0
	m.inputs = nil
	m.scrollOffset = 0
	m.maxVisibleLines = 20 // Number of visible lines in help dialog
	return m
}

// ShowAccountAdd shows the add account dialog
func (m Model) ShowAccountAdd() Model {
	m.dialogType = DialogAccountAdd
	m.title = "Add Account"
	m.message = ""
	m.inputs = []DialogInput{
		{Label: "JID (user@server.com)", Key: "jid", Value: ""},
		{Label: "Password", Key: "password", Value: "", Password: true},
		{Label: "Server (optional)", Key: "server", Value: ""},
		{Label: "Port (default: 5222)", Key: "port", Value: ""},
		{Label: "Resource (default: roster)", Key: "resource", Value: ""},
	}
	m.checkboxes = nil
	m.inCheckboxes = false
	m.activeInput = 0
	m.buttons = []string{"Add", "Cancel"}
	m.activeBtn = 0
	return m
}

// ShowAccountEdit shows the edit account dialog
func (m Model) ShowAccountEdit(jid, server string, port int, resource string) Model {
	m.dialogType = DialogAccountEdit
	m.title = "Edit Account"
	m.message = ""
	portStr := ""
	if port > 0 && port != 5222 {
		portStr = strconv.Itoa(port)
	}
	m.inputs = []DialogInput{
		{Label: "JID", Key: "jid", Value: jid},
		{Label: "New Password (leave empty to keep)", Key: "password", Value: "", Password: true},
		{Label: "Server", Key: "server", Value: server},
		{Label: "Port", Key: "port", Value: portStr},
		{Label: "Resource", Key: "resource", Value: resource},
	}
	m.checkboxes = nil
	m.inCheckboxes = false
	m.activeInput = 0
	m.buttons = []string{"Save", "Cancel"}
	m.activeBtn = 0
	m.data["original_jid"] = jid
	return m
}

// AccountInfo holds account info for display
type AccountInfo struct {
	JID     string
	Session bool
}

// ShowAccountList shows the account list dialog
func (m Model) ShowAccountList(accounts []AccountInfo, currentAccount string) Model {
	m.dialogType = DialogAccountList
	m.title = "Accounts"

	var sb strings.Builder
	if len(accounts) == 0 {
		sb.WriteString("No accounts configured.\n\n")
		sb.WriteString("Quick connect (session only):\n")
		sb.WriteString("  :connect user@server.com password\n\n")
		sb.WriteString("Add saved account:\n")
		sb.WriteString("  :account add")
	} else {
		for _, acc := range accounts {
			prefix := "  "
			suffix := ""
			if acc.JID == currentAccount {
				prefix = "→ "
				suffix = " (active)"
			}
			if acc.Session {
				suffix += " [session]"
			} else {
				suffix += " [saved]"
			}
			sb.WriteString(prefix + acc.JID + suffix + "\n")
		}
		sb.WriteString("\nCommands:\n")
		sb.WriteString("  :connect <jid> <pass> - Session connect\n")
		sb.WriteString("  :account add          - Add saved account\n")
		sb.WriteString("  :account remove <jid> - Remove account")
	}

	m.message = sb.String()
	m.buttons = []string{"Close"}
	m.activeBtn = 0
	m.inputs = nil
	return m
}

// ShowPassword shows a password prompt dialog
func (m Model) ShowPassword(jid string) Model {
	m.dialogType = DialogPassword
	m.title = "Enter Password"
	m.message = "Password for " + jid
	m.inputs = []DialogInput{
		{Label: "Password", Key: "password", Value: "", Password: true},
	}
	m.checkboxes = nil
	m.inCheckboxes = false
	m.activeInput = 0
	m.buttons = []string{"Connect", "Cancel"}
	m.activeBtn = 0
	m.data["jid"] = jid
	return m
}

// ShowSettingsList shows available settings
func (m Model) ShowSettingsList(settings map[string]string) Model {
	m.dialogType = DialogSettings
	m.title = "Settings"

	var sb strings.Builder
	sb.WriteString("Current Settings:\n\n")

	// List common settings
	settingsList := []struct{ key, desc string }{
		{"theme", "UI theme (rainbow, matrix, nord, gruvbox, dracula)"},
		{"roster_width", "Roster panel width"},
		{"roster_position", "Roster position (left, right)"},
		{"show_timestamps", "Show message timestamps"},
		{"time_format", "Time format (e.g., 15:04)"},
		{"notifications", "Desktop notifications"},
		{"encryption", "Default encryption (omemo, none)"},
		{"require_encryption", "Require encryption"},
	}

	for _, s := range settingsList {
		val := settings[s.key]
		if val == "" {
			val = "(default)"
		}
		sb.WriteString("  " + s.key + " = " + val + "\n")
		sb.WriteString("    " + s.desc + "\n")
	}

	sb.WriteString("\nUse :set <setting> <value> to change")

	m.message = sb.String()
	m.buttons = []string{"Close"}
	m.activeBtn = 0
	m.inputs = nil
	return m
}

// ShowContextHelp shows context-sensitive help based on current focus
func (m Model) ShowContextHelp(context string, content string) Model {
	m.dialogType = DialogContextHelp
	m.title = context
	m.message = content
	m.buttons = []string{"Close"}
	m.activeBtn = 0
	m.inputs = nil
	return m
}

// ShowAccountRemoveConfirm shows a confirmation dialog for removing an account
func (m Model) ShowAccountRemoveConfirm(jid string, isSession bool) Model {
	m.dialogType = DialogAccountRemove
	m.title = "Remove Account"

	accountType := "saved"
	if isSession {
		accountType = "session"
	}

	m.message = "Are you sure you want to remove this " + accountType + " account?\n\n" +
		"  " + jid + "\n\n" +
		"This action cannot be undone."

	m.buttons = []string{"Remove", "Cancel"}
	m.activeBtn = 1 // Default to Cancel for safety
	m.inputs = nil
	m.checkboxes = nil
	m.data["jid"] = jid
	return m
}

// ShowCreateRoom shows the create room dialog
func (m Model) ShowCreateRoom() Model {
	m.dialogType = DialogCreateRoom
	m.title = "Create Room"
	m.message = ""
	m.inputs = []DialogInput{
		{Label: "Room JID (room@conference.server)", Key: "room_jid", Value: ""},
		{Label: "Nickname", Key: "nick", Value: ""},
		{Label: "Room Name (optional)", Key: "name", Value: ""},
		{Label: "Description (optional)", Key: "desc", Value: ""},
		{Label: "Password (optional)", Key: "password", Value: "", Password: true},
	}
	m.checkboxes = []DialogCheckbox{
		{Label: "Use defaults (instant room)", Key: "defaults", Checked: true},
		{Label: "Members only", Key: "members_only", Checked: false},
		{Label: "Persistent", Key: "persistent", Checked: false},
	}
	m.activeInput = 0
	m.activeCheckbox = 0
	m.inCheckboxes = false
	m.buttons = []string{"Create", "Cancel"}
	m.activeBtn = 0
	return m
}

// Hide hides the dialog
func (m Model) Hide() Model {
	m.dialogType = DialogNone
	m.inputs = nil
	m.checkboxes = nil
	m.inCheckboxes = false
	m.operationType = OpNone
	m.spinnerFrame = 0
	m.scrollOffset = 0
	m.maxVisibleLines = 0
	m.data = make(map[string]string)
	return m
}

// RegistrationField represents a field in the registration form
type RegistrationField struct {
	Name     string
	Label    string
	Required bool
	Password bool
	Type     string // text-single, text-private, hidden, etc.
	Value    string // Pre-filled or default value
}

// CaptchaInfo holds CAPTCHA display information
type CaptchaInfo struct {
	Type      string // "image", "audio", "video", "qa", "hashcash"
	Challenge string // Challenge type (ocr, audio_recog, etc.)
	MimeType  string
	Data      []byte   // Raw media data
	URLs      []string // All available URLs
	URL       string   // Primary URL to fetch from
	Question  string   // For QA type or challenge description
	FieldVar  string   // Field var for answer submission
}

// ShowRegister shows the server input dialog for registration
func (m Model) ShowRegister() Model {
	m.dialogType = DialogRegister
	m.title = "Register Account"
	m.message = "Enter the server to register on"
	m.inputs = []DialogInput{
		{Label: "Server (e.g., example.com)", Key: "server", Value: ""},
		{Label: "Port (default: 5222)", Key: "port", Value: ""},
	}
	m.checkboxes = nil
	m.inCheckboxes = false
	m.activeInput = 0
	m.buttons = []string{"Fetch Form", "Cancel"}
	m.activeBtn = 0
	return m
}

// ShowRegisterForm shows the registration form with dynamic fields
func (m Model) ShowRegisterForm(server string, port int, fields []RegistrationField, instructions string, isDataForm bool, formType string, captcha *CaptchaInfo) Model {
	m.dialogType = DialogRegisterForm
	m.title = "Register on " + server

	// Build message with CAPTCHA info if present
	message := instructions
	if captcha != nil {
		message += formatCaptchaInfo(captcha)
	}
	m.message = message

	// Separate visible fields from hidden fields
	var visibleInputs []DialogInput

	// Add read-only CAPTCHA viewer field if CAPTCHA has URL or media data
	if captcha != nil {
		hasURL := captcha.URL != "" && !strings.HasPrefix(captcha.URL, "cid:")
		hasData := len(captcha.Data) > 0
		isMediaType := captcha.Type == "image" || captcha.Type == "audio" || captcha.Type == "video"

		// Show viewer field if we have something to open/copy
		if hasURL || hasData || isMediaType {
			label := "Open CAPTCHA"
			switch captcha.Type {
			case "audio":
				label = "Play Audio"
			case "video":
				label = "Play Video"
			}
			var value string
			if hasURL {
				value = "[ V: Open | C: Copy URL ]"
			} else if hasData {
				value = "[ V: Open ]"
			} else {
				value = "[ V: Open ]"
			}
			visibleInputs = append(visibleInputs, DialogInput{
				Label:    label,
				Key:      "_captcha_viewer",
				Value:    value,
				ReadOnly: true,
			})
		}
	}

	for _, f := range fields {
		// Store field type for later use in submission
		if f.Type != "" {
			m.data["_type_"+f.Name] = f.Type
		}

		if f.Type == "hidden" {
			// Store hidden fields in data map
			m.data["_hidden_"+f.Name] = f.Value
		} else if f.Type == "fixed" {
			// Fixed fields are just display text, don't show as input
			continue
		} else {
			// Check if this is a CAPTCHA answer field
			fieldLower := strings.ToLower(f.Name)
			isCaptchaAnswerField := fieldLower == "ocr" || fieldLower == "captcha" || fieldLower == "qa" ||
				strings.Contains(fieldLower, "captcha") ||
				fieldLower == "audio_recog" || fieldLower == "video_recog" ||
				fieldLower == "picture_recog" || fieldLower == "picture_q" ||
				fieldLower == "speech_q" || fieldLower == "speech_recog" || fieldLower == "video_q"

			// Skip fields that just display the URL (already shown in message)
			if strings.HasPrefix(f.Label, "http://") || strings.HasPrefix(f.Label, "https://") {
				continue
			}
			if strings.HasPrefix(f.Value, "http://") || strings.HasPrefix(f.Value, "https://") {
				// This is a URL display field, skip it
				continue
			}

			// For CAPTCHA answer fields, use a clear label
			label := f.Label
			if isCaptchaAnswerField && label == "" {
				label = "CAPTCHA Answer"
			}

			visibleInputs = append(visibleInputs, DialogInput{
				Label:    label,
				Key:      f.Name,
				Value:    f.Value, // Pre-fill with default value
				Password: f.Password,
			})
		}
	}
	m.inputs = visibleInputs
	m.checkboxes = nil
	m.inCheckboxes = false
	m.activeInput = 0

	m.buttons = []string{"Register", "Cancel"}
	m.activeBtn = 0
	m.data["server"] = server
	m.data["port"] = strconv.Itoa(port)
	m.data["_isDataForm"] = strconv.FormatBool(isDataForm)
	m.data["_formType"] = formType

	// Store CAPTCHA info for later use
	if captcha != nil {
		m.data["_captchaType"] = captcha.Type
		m.data["_captchaURL"] = captcha.URL
		m.data["_captchaMime"] = captcha.MimeType
		m.data["_captchaFieldVar"] = captcha.FieldVar
		m.data["_captchaChallenge"] = captcha.Challenge
	}
	return m
}

// ShowRegisterSuccess shows the success dialog after registration
func (m Model) ShowRegisterSuccess(jid, password, server string, port int) Model {
	m.dialogType = DialogRegisterSuccess
	m.title = "Registration Successful"
	m.message = "Account created: " + jid + "\n\n[1] Save & Connect  [2] Save Only\n[3] Session Only    [4] Close"
	m.inputs = nil
	m.checkboxes = nil
	m.inCheckboxes = false
	m.buttons = []string{"Save & Connect", "Save Only", "Session Only", "Close"}
	m.activeBtn = 0
	m.data["jid"] = jid
	m.data["password"] = password
	m.data["server"] = server
	m.data["port"] = strconv.Itoa(port)
	return m
}

// formatBytes formats byte size to human readable string
func formatBytes(bytes int) string {
	if bytes < 1024 {
		return strconv.Itoa(bytes) + " B"
	} else if bytes < 1024*1024 {
		return strconv.FormatFloat(float64(bytes)/1024, 'f', 1, 64) + " KB"
	} else {
		return strconv.FormatFloat(float64(bytes)/(1024*1024), 'f', 1, 64) + " MB"
	}
}

// formatCaptchaInfo formats CAPTCHA information for display
func formatCaptchaInfo(captcha *CaptchaInfo) string {
	var msg string

	switch captcha.Type {
	case "qa":
		// Question-answer CAPTCHA - show the question
		if captcha.Question != "" {
			msg += "\n\nSecurity Question: " + captcha.Question
		}

	case "audio":
		msg += "\n\n-- Audio CAPTCHA Required --"
		msg += formatMediaDetails(captcha, "audio")
		msg += "\n\nPress V to play the audio challenge."

	case "video":
		msg += "\n\n-- Video CAPTCHA Required --"
		msg += formatMediaDetails(captcha, "video")
		msg += "\n\nPress V to play the video challenge."

	case "image":
		msg += "\n\n-- Image CAPTCHA Required --"
		msg += formatMediaDetails(captcha, "image")

	default:
		// Unknown type - show what we have
		if captcha.Question != "" {
			msg += "\n\nChallenge: " + captcha.Question
		}
		if len(captcha.Data) > 0 || captcha.URL != "" {
			msg += "\n\n-- CAPTCHA Required --"
			msg += formatMediaDetails(captcha, "media")
		}
	}

	// Show challenge type hint if available
	if captcha.Challenge != "" {
		challengeDesc := getChallengeDescription(captcha.Challenge)
		if challengeDesc != "" {
			msg += "\nTask: " + challengeDesc
		}
	}

	return msg
}

// formatMediaDetails formats the media source information
func formatMediaDetails(captcha *CaptchaInfo, mediaType string) string {
	var msg string
	hasURL := captcha.URL != "" && !strings.HasPrefix(captcha.URL, "cid:")
	hasEmbeddedData := len(captcha.Data) > 0

	// Show URL if it's a real HTTP URL
	if hasURL {
		msg += "\nURL: " + captcha.URL
	}

	// Show embedded data info
	if hasEmbeddedData {
		msg += "\nSource: Embedded " + mediaType
		msg += "\nSize: " + formatBytes(len(captcha.Data))
	}

	// Show MIME type
	if captcha.MimeType != "" {
		msg += "\nFormat: " + captcha.MimeType
	}

	// Show alternative URLs count
	if len(captcha.URLs) > 1 {
		msg += "\nAlternatives: " + strconv.Itoa(len(captcha.URLs)) + " sources available"
	}

	// Fallback message if no source info
	if !hasURL && !hasEmbeddedData {
		msg += "\nSource: Server-provided " + mediaType
	}

	// Security warning only for URLs opened in browser (not for embedded data)
	if hasURL && !hasEmbeddedData {
		msg += "\n\nNote: Opens in browser. URL may track your IP."
	}

	return msg
}

// getInputViewport returns the visible portion of input text with cursor in view
// Returns: (displayText, cursorPosInDisplay, startOffset)
func getInputViewport(value string, cursor int, maxWidth int) (string, int, int) {
	if len(value) <= maxWidth {
		return value, cursor, 0
	}

	// Keep cursor visible with some context around it
	halfWidth := maxWidth / 2

	var start, end int
	if cursor <= halfWidth {
		// Cursor near start - show from beginning
		start = 0
		end = maxWidth
	} else if cursor >= len(value)-halfWidth {
		// Cursor near end - show last portion
		start = len(value) - maxWidth
		end = len(value)
	} else {
		// Cursor in middle - center around cursor
		start = cursor - halfWidth
		end = start + maxWidth
	}

	displayText := value[start:end]
	cursorInDisplay := cursor - start

	return displayText, cursorInDisplay, start
}

// getChallengeDescription returns a human-readable description of the challenge type
func getChallengeDescription(challenge string) string {
	switch challenge {
	case "ocr":
		return "Enter the text you see"
	case "audio_recog":
		return "Describe the sound you hear"
	case "video_recog":
		return "Identify the video content"
	case "picture_recog":
		return "Identify what you see in the picture"
	case "picture_q":
		return "Answer the question about the picture"
	case "speech_q":
		return "Answer the question you hear"
	case "speech_recog":
		return "Enter the words you hear"
	case "video_q":
		return "Answer the question in the video"
	case "qa":
		return "Answer the security question"
	default:
		return ""
	}
}

// SetViewerStatus updates the CAPTCHA viewer field with a status message
func (m Model) SetViewerStatus(status string) Model {
	for i := range m.inputs {
		if m.inputs[i].Key == "_captcha_viewer" {
			m.inputs[i].Value = "[ " + status + " ]"
			break
		}
	}
	return m
}

// RestoreViewer restores the CAPTCHA viewer field to its default state
func (m Model) RestoreViewer() Model {
	hasURL := m.data["_captchaURL"] != "" && !strings.HasPrefix(m.data["_captchaURL"], "cid:")
	var value string
	if hasURL {
		value = "[ V: Open | C: Copy URL ]"
	} else {
		value = "[ V: Open ]"
	}
	for i := range m.inputs {
		if m.inputs[i].Key == "_captcha_viewer" {
			m.inputs[i].Value = value
			break
		}
	}
	return m
}

// ShowLoading shows a loading dialog with spinner and cancel button
func (m Model) ShowLoading(message string, operation OperationType) Model {
	m.dialogType = DialogLoading
	m.title = "Please Wait"
	m.message = message
	m.inputs = nil
	m.checkboxes = nil
	m.inCheckboxes = false
	m.buttons = []string{"Cancel"}
	m.activeBtn = 0
	m.spinnerFrame = 0
	m.operationType = operation
	return m
}

// IsLoading returns true if a loading dialog is active
func (m Model) IsLoading() bool {
	return m.dialogType == DialogLoading
}

// HideLoading hides the loading dialog without affecting other state
func (m Model) HideLoading() Model {
	if m.dialogType == DialogLoading {
		m.dialogType = DialogNone
		m.operationType = OpNone
		m.spinnerFrame = 0
	}
	return m
}

// GetOperationType returns the current operation type
func (m Model) GetOperationType() OperationType {
	return m.operationType
}

// SpinnerTick returns a command that sends a spinner tick after a delay
func SpinnerTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return SpinnerTickMsg{}
	})
}

// AdvanceSpinner advances the spinner frame
func (m Model) AdvanceSpinner() Model {
	m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
	return m
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.Active() {
		return m, nil
	}

	// Handle spinner tick for loading dialog
	if _, ok := msg.(SpinnerTickMsg); ok && m.dialogType == DialogLoading {
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		return m, SpinnerTick()
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Special handling for loading dialog - only allow cancel
		if m.dialogType == DialogLoading {
			if msg.Type == tea.KeyEsc || msg.Type == tea.KeyEnter {
				op := m.operationType
				m = m.Hide()
				return m, func() tea.Msg { return CancelOperationMsg{Operation: op} }
			}
			// Ignore other keys in loading dialog
			return m, nil
		}

		// Handle 'v' key for viewing CAPTCHA in registration form
		// Only trigger when focused on the read-only CAPTCHA viewer field
		keyStr := strings.ToLower(msg.String())
		if keyStr == "v" && m.dialogType == DialogRegisterForm {
			// Check if current input is the CAPTCHA viewer field
			if m.activeInput >= 0 && m.activeInput < len(m.inputs) {
				if m.inputs[m.activeInput].Key == "_captcha_viewer" {
					// Collect current values before sending
					values := make(map[string]string)
					for _, input := range m.inputs {
						values[input.Key] = input.Value
					}
					for k, v := range m.data {
						values[k] = v
					}
					result := DialogResult{
						Type:      m.dialogType,
						Confirmed: false,
						Action:    ActionViewCaptcha,
						Values:    values,
					}
					// Don't hide dialog - just send the action
					return m, func() tea.Msg { return result }
				}
			}
		}

		// Handle 'c' key for copying CAPTCHA URL to clipboard
		// Only when focused on the CAPTCHA viewer field
		if keyStr == "c" && m.dialogType == DialogRegisterForm {
			if m.activeInput >= 0 && m.activeInput < len(m.inputs) {
				if m.inputs[m.activeInput].Key == "_captcha_viewer" {
					captchaURL := m.data["_captchaURL"]
					if captchaURL != "" && !strings.HasPrefix(captchaURL, "cid:") {
						values := make(map[string]string)
						for _, input := range m.inputs {
							values[input.Key] = input.Value
						}
						for k, v := range m.data {
							values[k] = v
						}
						result := DialogResult{
							Type:      m.dialogType,
							Confirmed: false,
							Action:    ActionCopyURL,
							Values:    values,
						}
						// Don't hide dialog - just send the action
						return m, func() tea.Msg { return result }
					}
				}
			}
		}

		// Handle scrolling for help dialog
		if m.dialogType == DialogHelp {
			lines := strings.Split(m.message, "\n")
			maxScroll := len(lines) - m.maxVisibleLines
			if maxScroll < 0 {
				maxScroll = 0
			}

			switch msg.String() {
			case "j", "down":
				if m.scrollOffset < maxScroll {
					m.scrollOffset++
				}
				return m, nil
			case "k", "up":
				if m.scrollOffset > 0 {
					m.scrollOffset--
				}
				return m, nil
			case "g":
				// Go to top
				m.scrollOffset = 0
				return m, nil
			case "G":
				// Go to bottom
				m.scrollOffset = maxScroll
				return m, nil
			case "ctrl+d":
				// Half page down
				m.scrollOffset += m.maxVisibleLines / 2
				if m.scrollOffset > maxScroll {
					m.scrollOffset = maxScroll
				}
				return m, nil
			case "ctrl+u":
				// Half page up
				m.scrollOffset -= m.maxVisibleLines / 2
				if m.scrollOffset < 0 {
					m.scrollOffset = 0
				}
				return m, nil
			}
		}

		// Handle OMEMO devices dialog
		if m.dialogType == DialogOMEMODevices {
			switch msg.String() {
			case "j", "down":
				if m.selectedDevice < len(m.omemoDevices)-1 {
					m.selectedDevice++
				}
				return m, nil
			case "k", "up":
				if m.selectedDevice > 0 {
					m.selectedDevice--
				}
				return m, nil
			}
		}

		// Handle number keys 1-9 for button selection (when not in input fields)
		if len(m.inputs) == 0 || m.dialogType == DialogRegisterSuccess {
			if keyStr >= "1" && keyStr <= "9" {
				btnIndex := int(keyStr[0] - '1') // Convert "1" to 0, "2" to 1, etc.
				if btnIndex < len(m.buttons) {
					m.activeBtn = btnIndex
					// Trigger the button action
					values := make(map[string]string)
					for _, input := range m.inputs {
						values[input.Key] = input.Value
					}
					for _, cb := range m.checkboxes {
						if cb.Checked {
							values[cb.Key] = "true"
						} else {
							values[cb.Key] = "false"
						}
					}
					for k, v := range m.data {
						values[k] = v
					}
					confirmed := btnIndex == 0
					action := ActionCancel
					if confirmed {
						action = ActionConfirm
					}
					result := DialogResult{
						Type:      m.dialogType,
						Confirmed: confirmed,
						Button:    btnIndex,
						Action:    action,
						Values:    values,
					}
					m = m.Hide()
					return m, func() tea.Msg { return result }
				}
			}
		}

		switch msg.Type {
		case tea.KeyEsc:
			// Cancel dialog
			result := DialogResult{
				Type:      m.dialogType,
				Confirmed: false,
				Action:    ActionCancel,
			}
			m = m.Hide()
			return m, func() tea.Msg { return result }

		case tea.KeyTab:
			// Cycle through inputs, then checkboxes
			if m.inCheckboxes {
				m.activeCheckbox++
				if m.activeCheckbox >= len(m.checkboxes) {
					m.activeCheckbox = 0
					m.inCheckboxes = false
					m.activeInput = 0
				}
			} else if len(m.inputs) > 0 {
				m.activeInput++
				if m.activeInput >= len(m.inputs) {
					if len(m.checkboxes) > 0 {
						m.inCheckboxes = true
						m.activeCheckbox = 0
					} else {
						m.activeInput = 0
					}
				}
			} else if len(m.checkboxes) > 0 {
				m.inCheckboxes = true
				m.activeCheckbox++
				if m.activeCheckbox >= len(m.checkboxes) {
					m.activeCheckbox = 0
				}
			}

		case tea.KeyShiftTab:
			// Cycle backwards
			if m.inCheckboxes {
				m.activeCheckbox--
				if m.activeCheckbox < 0 {
					if len(m.inputs) > 0 {
						m.inCheckboxes = false
						m.activeInput = len(m.inputs) - 1
					} else {
						m.activeCheckbox = len(m.checkboxes) - 1
					}
				}
			} else if len(m.inputs) > 0 {
				m.activeInput--
				if m.activeInput < 0 {
					if len(m.checkboxes) > 0 {
						m.inCheckboxes = true
						m.activeCheckbox = len(m.checkboxes) - 1
					} else {
						m.activeInput = len(m.inputs) - 1
					}
				}
			}

		case tea.KeyLeft:
			if len(m.inputs) == 0 || m.activeInput >= len(m.inputs) {
				// Navigate buttons
				m.activeBtn--
				if m.activeBtn < 0 {
					m.activeBtn = len(m.buttons) - 1
				}
			} else if !m.inputs[m.activeInput].ReadOnly && m.inputs[m.activeInput].Cursor > 0 {
				// Only move cursor in editable fields
				m.inputs[m.activeInput].Cursor--
			}

		case tea.KeyRight:
			if len(m.inputs) == 0 || m.activeInput >= len(m.inputs) {
				// Navigate buttons
				m.activeBtn++
				if m.activeBtn >= len(m.buttons) {
					m.activeBtn = 0
				}
			} else if !m.inputs[m.activeInput].ReadOnly && m.inputs[m.activeInput].Cursor < len(m.inputs[m.activeInput].Value) {
				// Only move cursor in editable fields
				m.inputs[m.activeInput].Cursor++
			}

		case tea.KeyEnter:
			// If in checkboxes, toggle the checkbox instead of confirming
			if m.inCheckboxes && m.activeCheckbox < len(m.checkboxes) {
				m.checkboxes[m.activeCheckbox].Checked = !m.checkboxes[m.activeCheckbox].Checked
				return m, nil
			}

			// Confirm action
			values := make(map[string]string)
			for _, input := range m.inputs {
				values[input.Key] = input.Value
			}
			for _, cb := range m.checkboxes {
				if cb.Checked {
					values[cb.Key] = "true"
				} else {
					values[cb.Key] = "false"
				}
			}
			for k, v := range m.data {
				values[k] = v
			}

			confirmed := m.activeBtn == 0 // First button is confirm
			action := ActionCancel
			if confirmed {
				action = ActionConfirm
			}
			result := DialogResult{
				Type:      m.dialogType,
				Confirmed: confirmed,
				Button:    m.activeBtn,
				Action:    action,
				Values:    values,
			}
			m = m.Hide()
			return m, func() tea.Msg { return result }

		case tea.KeySpace:
			// Toggle checkbox if in checkbox mode
			if m.inCheckboxes && m.activeCheckbox < len(m.checkboxes) {
				m.checkboxes[m.activeCheckbox].Checked = !m.checkboxes[m.activeCheckbox].Checked
				return m, nil
			}
			// Otherwise, add space to input (skip read-only fields)
			if len(m.inputs) > 0 && m.activeInput < len(m.inputs) && !m.inCheckboxes {
				input := &m.inputs[m.activeInput]
				if !input.ReadOnly {
					input.Value = input.Value[:input.Cursor] + " " + input.Value[input.Cursor:]
					input.Cursor++
				}
			}

		case tea.KeyBackspace:
			if len(m.inputs) > 0 && m.activeInput < len(m.inputs) {
				input := &m.inputs[m.activeInput]
				// Skip read-only fields
				if !input.ReadOnly && input.Cursor > 0 {
					input.Value = input.Value[:input.Cursor-1] + input.Value[input.Cursor:]
					input.Cursor--
				}
			}

		case tea.KeyRunes:
			if len(m.inputs) > 0 && m.activeInput < len(m.inputs) && !m.inCheckboxes {
				input := &m.inputs[m.activeInput]
				// Skip read-only fields
				if !input.ReadOnly {
					input.Value = input.Value[:input.Cursor] + string(msg.Runes) + input.Value[input.Cursor:]
					input.Cursor += len(msg.Runes)
				}
			}
		}
	}

	return m, nil
}

// View renders the dialog
func (m Model) View() string {
	if !m.Active() {
		return ""
	}

	var b strings.Builder

	// Special rendering for loading dialog
	if m.dialogType == DialogLoading {
		// Title
		title := m.styles.DialogTitle.Render(m.title)
		b.WriteString(title)
		b.WriteString("\n\n")

		// Spinner and message
		spinner := spinnerFrames[m.spinnerFrame]
		b.WriteString(m.styles.DialogContent.Render(spinner + " " + m.message))
		b.WriteString("\n\n")

		// Cancel button
		cancelBtn := m.styles.DialogButtonActive.Render("Cancel")
		b.WriteString(cancelBtn)
		b.WriteString("\n")
		b.WriteString(m.styles.DialogContent.Render("(Press Esc or Enter to cancel)"))

		return m.styles.DialogBorder.
			Width(50).
			Padding(1, 2).
			Render(b.String())
	}

	// Title
	title := m.styles.DialogTitle.Render(m.title)
	b.WriteString(title)
	b.WriteString("\n\n")

	// Message (with scroll support for help dialog)
	if m.message != "" {
		if m.dialogType == DialogHelp && m.maxVisibleLines > 0 {
			// Scrollable help content
			lines := strings.Split(m.message, "\n")
			totalLines := len(lines)

			// Calculate visible range
			start := m.scrollOffset
			end := start + m.maxVisibleLines
			if end > totalLines {
				end = totalLines
			}

			// Show "more above" indicator
			if start > 0 {
				b.WriteString(m.styles.DialogContent.Render("+" + strconv.Itoa(start) + " more above (k/up to scroll)"))
				b.WriteString("\n")
			}

			// Show visible lines - render each line separately to preserve colors
			visibleLines := lines[start:end]
			for _, line := range visibleLines {
				b.WriteString(m.styles.DialogContent.Render(line))
				b.WriteString("\n")
			}

			// Show "more below" indicator
			remaining := totalLines - end
			if remaining > 0 {
				b.WriteString(m.styles.DialogContent.Render("+" + strconv.Itoa(remaining) + " more below (j/down to scroll)"))
			}
			b.WriteString("\n")
		} else {
			// Render each line separately to preserve colors across line breaks
			for _, line := range strings.Split(m.message, "\n") {
				b.WriteString(m.styles.DialogContent.Render(line))
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}

	// OMEMO Devices list
	if m.dialogType == DialogOMEMODevices && len(m.omemoDevices) > 0 {
		b.WriteString("Devices (j/k to select):\n\n")
		for i, dev := range m.omemoDevices {
			prefix := "  "
			if i == m.selectedDevice {
				prefix = "> "
			}

			trustStyle := m.styles.PresenceOffline
			switch dev.TrustLevel {
			case 2:
				trustStyle = m.styles.PresenceOnline
			case 1:
				trustStyle = m.styles.PresenceAway
			case 3:
				trustStyle = m.styles.PresenceDND
			}

			line := prefix + "Device " + strconv.FormatUint(uint64(dev.DeviceID), 10) + " [" + trustStyle.Render(dev.TrustString) + "]"
			b.WriteString(m.styles.DialogContent.Render(line))
			b.WriteString("\n")

			fpLine := "   " + dev.Fingerprint
			b.WriteString(m.styles.DialogContent.Render(fpLine))
			b.WriteString("\n\n")
		}
	}

	// Inputs
	for i, input := range m.inputs {
		label := input.Label + ": "

		// Value with cursor
		value := input.Value
		if input.Password {
			value = strings.Repeat("*", len(value))
		}

		// Calculate available width for input value
		// Dialog width 50 - border (2) - padding (4) = 44 content area
		// Input styling adds border (2) + padding (2) = 4 overhead
		// So label + value can be up to 40 chars
		availableWidth := 40 - len(label)
		if availableWidth < 6 {
			availableWidth = 6 // Minimum usable width
		}

		// Apply viewport for long text
		displayValue := value
		displayCursor := input.Cursor
		var viewportStart int
		if len(value) > availableWidth {
			displayValue, displayCursor, viewportStart = getInputViewport(value, input.Cursor, availableWidth)
		}

		// Add overflow indicators
		hasLeftOverflow := viewportStart > 0
		hasRightOverflow := viewportStart+len(displayValue) < len(value)

		prefix := ""
		suffix := ""
		if hasLeftOverflow {
			prefix = "<"
			if len(displayValue) > 1 {
				displayValue = displayValue[1:] // Make room for indicator
			}
		}
		if hasRightOverflow {
			suffix = ">"
			if len(displayValue) > 1 {
				displayValue = displayValue[:len(displayValue)-1] // Make room for indicator
			}
		}

		// Adjust cursor position if we added left overflow indicator
		if hasLeftOverflow && displayCursor > 0 {
			displayCursor--
		}

		var rendered string
		if input.ReadOnly {
			// Read-only field - no cursor, just highlight when focused
			if i == m.activeInput && !m.inCheckboxes {
				rendered = m.styles.InputFocused.Render(label + prefix + displayValue + suffix)
			} else {
				rendered = m.styles.InputNormal.Render(label + prefix + displayValue + suffix)
			}
		} else if i == m.activeInput && !m.inCheckboxes {
			// Show cursor for editable fields
			beforeCursor := displayValue
			cursorChar := " "
			afterCursor := ""
			if displayCursor < len(displayValue) {
				beforeCursor = displayValue[:displayCursor]
				cursorChar = string(displayValue[displayCursor])
				afterCursor = displayValue[displayCursor+1:]
			}
			cursor := lipgloss.NewStyle().Reverse(true).Render(cursorChar)
			rendered = m.styles.InputFocused.Render(label + prefix + beforeCursor + cursor + afterCursor + suffix)
		} else {
			rendered = m.styles.InputNormal.Render(label + prefix + displayValue + suffix)
		}

		b.WriteString(rendered)
		b.WriteString("\n")
	}

	if len(m.inputs) > 0 {
		b.WriteString("\n")
	}

	// Checkboxes
	for i, cb := range m.checkboxes {
		checkMark := "[ ]"
		if cb.Checked {
			checkMark = "[x]"
		}

		var rendered string
		if i == m.activeCheckbox && m.inCheckboxes {
			rendered = m.styles.InputFocused.Render(checkMark + " " + cb.Label)
		} else {
			rendered = m.styles.InputNormal.Render(checkMark + " " + cb.Label)
		}

		b.WriteString(rendered)
		b.WriteString("\n")
	}

	if len(m.checkboxes) > 0 {
		b.WriteString("\n")
	}

	// Buttons
	var buttons []string
	for i, btn := range m.buttons {
		var style lipgloss.Style
		if i == m.activeBtn {
			style = m.styles.DialogButtonActive
		} else {
			style = m.styles.DialogButton
		}
		buttons = append(buttons, style.Render(btn))
	}
	b.WriteString(strings.Join(buttons, "  "))

	// Wrap in border - use smaller size for context help
	content := b.String()
	width := 50
	padding := 1
	if m.dialogType == DialogContextHelp {
		width = 40 // Smaller width for context help
		padding = 0
	}
	return m.styles.DialogBorder.
		Width(width).
		Padding(padding, 2).
		Render(content)
}
