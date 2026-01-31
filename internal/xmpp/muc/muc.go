package muc

import (
	"sync"
	"time"

	"mellium.im/xmpp/jid"
)

// Affiliation represents a MUC affiliation
type Affiliation string

const (
	AffiliationOwner   Affiliation = "owner"
	AffiliationAdmin   Affiliation = "admin"
	AffiliationMember  Affiliation = "member"
	AffiliationOutcast Affiliation = "outcast"
	AffiliationNone    Affiliation = "none"
)

// Role represents a MUC role
type Role string

const (
	RoleModerator   Role = "moderator"
	RoleParticipant Role = "participant"
	RoleVisitor     Role = "visitor"
	RoleNone        Role = "none"
)

// Occupant represents a room occupant
type Occupant struct {
	Nick        string
	JID         jid.JID // Real JID if known
	Affiliation Affiliation
	Role        Role
	Show        string
	Status      string
}

// Room represents a MUC room
type Room struct {
	JID         jid.JID
	Name        string
	Nick        string
	Subject     string
	SubjectBy   string
	Password    string
	Joined      bool
	Occupants   map[string]*Occupant
	Messages    []Message
	LastActive  time.Time
	Unread      int
}

// Message represents a MUC message
type Message struct {
	ID        string
	From      string // Nick
	Body      string
	Timestamp time.Time
	Type      string // groupchat, private
	Delayed   bool
}

// Manager manages MUC rooms
type Manager struct {
	mu    sync.RWMutex
	rooms map[string]*Room
}

// NewManager creates a new MUC manager
func NewManager() *Manager {
	return &Manager{
		rooms: make(map[string]*Room),
	}
}

// JoinRoom creates a room entry for joining
func (m *Manager) JoinRoom(roomJID jid.JID, nick, password string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := roomJID.Bare().String()
	room := &Room{
		JID:       roomJID.Bare(),
		Nick:      nick,
		Password:  password,
		Occupants: make(map[string]*Occupant),
		Messages:  []Message{},
	}
	m.rooms[bare] = room
	return room
}

// LeaveRoom removes a room
func (m *Manager) LeaveRoom(roomJID jid.JID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := roomJID.Bare().String()
	delete(m.rooms, bare)
}

// GetRoom returns a room by JID
func (m *Manager) GetRoom(roomJID jid.JID) *Room {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bare := roomJID.Bare().String()
	return m.rooms[bare]
}

// SetJoined marks a room as joined
func (m *Manager) SetJoined(roomJID jid.JID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := roomJID.Bare().String()
	if room, ok := m.rooms[bare]; ok {
		room.Joined = true
	}
}

// AddOccupant adds or updates an occupant
func (m *Manager) AddOccupant(roomJID jid.JID, occupant Occupant) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := roomJID.Bare().String()
	if room, ok := m.rooms[bare]; ok {
		room.Occupants[occupant.Nick] = &occupant
	}
}

// RemoveOccupant removes an occupant
func (m *Manager) RemoveOccupant(roomJID jid.JID, nick string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := roomJID.Bare().String()
	if room, ok := m.rooms[bare]; ok {
		delete(room.Occupants, nick)
	}
}

// GetOccupant returns an occupant by nick
func (m *Manager) GetOccupant(roomJID jid.JID, nick string) *Occupant {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bare := roomJID.Bare().String()
	if room, ok := m.rooms[bare]; ok {
		return room.Occupants[nick]
	}
	return nil
}

// SetSubject sets the room subject
func (m *Manager) SetSubject(roomJID jid.JID, subject, by string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := roomJID.Bare().String()
	if room, ok := m.rooms[bare]; ok {
		room.Subject = subject
		room.SubjectBy = by
	}
}

// AddMessage adds a message to a room
func (m *Manager) AddMessage(roomJID jid.JID, msg Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := roomJID.Bare().String()
	if room, ok := m.rooms[bare]; ok {
		room.Messages = append(room.Messages, msg)
		room.LastActive = time.Now()
		room.Unread++
	}
}

// MarkRead marks a room as read
func (m *Manager) MarkRead(roomJID jid.JID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := roomJID.Bare().String()
	if room, ok := m.rooms[bare]; ok {
		room.Unread = 0
	}
}

// GetAllRooms returns all rooms
func (m *Manager) GetAllRooms() []*Room {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rooms := make([]*Room, 0, len(m.rooms))
	for _, room := range m.rooms {
		rooms = append(rooms, room)
	}
	return rooms
}

// GetJoinedRooms returns only joined rooms
func (m *Manager) GetJoinedRooms() []*Room {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var rooms []*Room
	for _, room := range m.rooms {
		if room.Joined {
			rooms = append(rooms, room)
		}
	}
	return rooms
}

// ClearHistory clears message history for a room
func (m *Manager) ClearHistory(roomJID jid.JID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := roomJID.Bare().String()
	if room, ok := m.rooms[bare]; ok {
		room.Messages = []Message{}
	}
}

// RenameOccupant handles a nick change
func (m *Manager) RenameOccupant(roomJID jid.JID, oldNick, newNick string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bare := roomJID.Bare().String()
	if room, ok := m.rooms[bare]; ok {
		if occupant, ok := room.Occupants[oldNick]; ok {
			delete(room.Occupants, oldNick)
			occupant.Nick = newNick
			room.Occupants[newNick] = occupant

			// Update our own nick if needed
			if room.Nick == oldNick {
				room.Nick = newNick
			}
		}
	}
}
