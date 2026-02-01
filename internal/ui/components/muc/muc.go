package muc

import (
	"strings"

	"github.com/meszmate/roster/internal/ui/theme"
)

// Role represents a MUC participant role
type Role string

const (
	RoleModerator   Role = "moderator"
	RoleParticipant Role = "participant"
	RoleVisitor     Role = "visitor"
	RoleNone        Role = "none"
)

// Affiliation represents a MUC participant affiliation
type Affiliation string

const (
	AffiliationOwner   Affiliation = "owner"
	AffiliationAdmin   Affiliation = "admin"
	AffiliationMember  Affiliation = "member"
	AffiliationOutcast Affiliation = "outcast"
	AffiliationNone    Affiliation = "none"
)

// Participant represents a MUC room participant
type Participant struct {
	Nick        string
	JID         string
	Role        Role
	Affiliation Affiliation
	Status      string
	StatusMsg   string
}

// Room represents a MUC room
type Room struct {
	JID          string
	Name         string
	Nick         string
	Subject      string
	Participants []Participant
	Joined       bool
	Password     string
}

// Model represents the MUC component
type Model struct {
	rooms           map[string]*Room
	activeRoom      string
	showParticipants bool
	participantWidth int
	width           int
	height          int
	styles          *theme.Styles
}

// New creates a new MUC model
func New(styles *theme.Styles) Model {
	return Model{
		rooms:            make(map[string]*Room),
		showParticipants: true,
		participantWidth: 20,
		styles:           styles,
	}
}

// JoinRoom joins a MUC room
func (m Model) JoinRoom(roomJID, nick, password string) Model {
	room := &Room{
		JID:      roomJID,
		Nick:     nick,
		Password: password,
		Joined:   false,
	}
	m.rooms[roomJID] = room
	m.activeRoom = roomJID
	return m
}

// LeaveRoom leaves a MUC room
func (m Model) LeaveRoom(roomJID string) Model {
	delete(m.rooms, roomJID)
	if m.activeRoom == roomJID {
		m.activeRoom = ""
	}
	return m
}

// SetRoomJoined marks a room as joined
func (m Model) SetRoomJoined(roomJID string) Model {
	if room, ok := m.rooms[roomJID]; ok {
		room.Joined = true
	}
	return m
}

// AddParticipant adds a participant to a room
func (m Model) AddParticipant(roomJID string, p Participant) Model {
	if room, ok := m.rooms[roomJID]; ok {
		// Check if participant already exists
		for i, existing := range room.Participants {
			if existing.Nick == p.Nick {
				room.Participants[i] = p
				return m
			}
		}
		room.Participants = append(room.Participants, p)
	}
	return m
}

// RemoveParticipant removes a participant from a room
func (m Model) RemoveParticipant(roomJID, nick string) Model {
	if room, ok := m.rooms[roomJID]; ok {
		for i, p := range room.Participants {
			if p.Nick == nick {
				room.Participants = append(room.Participants[:i], room.Participants[i+1:]...)
				break
			}
		}
	}
	return m
}

// SetSubject sets the room subject
func (m Model) SetSubject(roomJID, subject string) Model {
	if room, ok := m.rooms[roomJID]; ok {
		room.Subject = subject
	}
	return m
}

// GetRoom returns a room by JID
func (m Model) GetRoom(roomJID string) *Room {
	return m.rooms[roomJID]
}

// GetActiveRoom returns the active room
func (m Model) GetActiveRoom() *Room {
	return m.rooms[m.activeRoom]
}

// SetActiveRoom sets the active room
func (m Model) SetActiveRoom(roomJID string) Model {
	if _, ok := m.rooms[roomJID]; ok {
		m.activeRoom = roomJID
	}
	return m
}

// ToggleParticipants toggles the participant list visibility
func (m Model) ToggleParticipants() Model {
	m.showParticipants = !m.showParticipants
	return m
}

// SetSize sets the component size
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	return m
}

// ParticipantsView renders the participant list
func (m Model) ParticipantsView() string {
	if !m.showParticipants {
		return ""
	}

	room := m.GetActiveRoom()
	if room == nil {
		return ""
	}

	var b strings.Builder

	// Header
	header := m.styles.RosterHeader.Width(m.participantWidth).Render("Participants")
	b.WriteString(header)
	b.WriteString("\n")

	// Group participants by role
	moderators := []Participant{}
	participants := []Participant{}
	visitors := []Participant{}

	for _, p := range room.Participants {
		switch p.Role {
		case RoleModerator:
			moderators = append(moderators, p)
		case RoleParticipant:
			participants = append(participants, p)
		case RoleVisitor:
			visitors = append(visitors, p)
		}
	}

	// Render each group
	if len(moderators) > 0 {
		b.WriteString(m.styles.RosterGroup.Render("Moderators"))
		b.WriteString("\n")
		for _, p := range moderators {
			b.WriteString(m.renderParticipant(p))
			b.WriteString("\n")
		}
	}

	if len(participants) > 0 {
		b.WriteString(m.styles.RosterGroup.Render("Participants"))
		b.WriteString("\n")
		for _, p := range participants {
			b.WriteString(m.renderParticipant(p))
			b.WriteString("\n")
		}
	}

	if len(visitors) > 0 {
		b.WriteString(m.styles.RosterGroup.Render("Visitors"))
		b.WriteString("\n")
		for _, p := range visitors {
			b.WriteString(m.renderParticipant(p))
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderParticipant renders a single participant
func (m Model) renderParticipant(p Participant) string {
	// Presence indicator
	var indicator string
	switch p.Status {
	case "online", "":
		indicator = m.styles.PresenceOnline.Render("●")
	case "away":
		indicator = m.styles.PresenceAway.Render("◐")
	case "dnd":
		indicator = m.styles.PresenceDND.Render("⊘")
	case "xa":
		indicator = m.styles.PresenceXA.Render("◯")
	default:
		indicator = m.styles.PresenceOffline.Render("○")
	}

	// Affiliation badge
	badge := ""
	switch p.Affiliation {
	case AffiliationOwner:
		badge = "&"
	case AffiliationAdmin:
		badge = "@"
	case AffiliationMember:
		badge = "+"
	}

	nick := p.Nick
	if len(nick) > m.participantWidth-5 {
		nick = nick[:m.participantWidth-6] + "…"
	}

	return " " + indicator + badge + nick
}
