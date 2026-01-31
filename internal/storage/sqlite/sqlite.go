package sqlite

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB represents the SQLite database
type DB struct {
	db *sql.DB
}

// New creates a new SQLite database connection
func New(dataDir string) (*DB, error) {
	dbPath := filepath.Join(dataDir, "roster.db")

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &DB{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return store, nil
}

// Close closes the database connection
func (d *DB) Close() error {
	return d.db.Close()
}

// migrate runs database migrations
func (d *DB) migrate() error {
	migrations := []string{
		// Messages table
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			account TEXT NOT NULL,
			jid TEXT NOT NULL,
			body TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			outgoing INTEGER NOT NULL,
			encrypted INTEGER NOT NULL,
			type TEXT NOT NULL,
			received INTEGER DEFAULT 0,
			displayed INTEGER DEFAULT 0,
			corrected INTEGER DEFAULT 0,
			corrected_id TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_jid ON messages(account, jid)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp)`,

		// Roster cache
		`CREATE TABLE IF NOT EXISTS roster (
			account TEXT NOT NULL,
			jid TEXT NOT NULL,
			name TEXT,
			subscription TEXT,
			groups TEXT,
			PRIMARY KEY (account, jid)
		)`,

		// OMEMO identities
		`CREATE TABLE IF NOT EXISTS omemo_identities (
			account TEXT NOT NULL,
			jid TEXT NOT NULL,
			device_id INTEGER NOT NULL,
			identity_key BLOB NOT NULL,
			trust_level INTEGER DEFAULT 0,
			first_seen INTEGER NOT NULL,
			last_seen INTEGER NOT NULL,
			PRIMARY KEY (account, jid, device_id)
		)`,

		// OMEMO sessions
		`CREATE TABLE IF NOT EXISTS omemo_sessions (
			account TEXT NOT NULL,
			jid TEXT NOT NULL,
			device_id INTEGER NOT NULL,
			session_data BLOB NOT NULL,
			PRIMARY KEY (account, jid, device_id)
		)`,

		// OMEMO prekeys
		`CREATE TABLE IF NOT EXISTS omemo_prekeys (
			account TEXT NOT NULL,
			key_id INTEGER NOT NULL,
			key_data BLOB NOT NULL,
			PRIMARY KEY (account, key_id)
		)`,

		// OMEMO signed prekeys
		`CREATE TABLE IF NOT EXISTS omemo_signed_prekeys (
			account TEXT NOT NULL,
			key_id INTEGER NOT NULL,
			key_data BLOB NOT NULL,
			signature BLOB NOT NULL,
			timestamp INTEGER NOT NULL,
			PRIMARY KEY (account, key_id)
		)`,

		// MUC bookmarks
		`CREATE TABLE IF NOT EXISTS bookmarks (
			account TEXT NOT NULL,
			jid TEXT NOT NULL,
			name TEXT,
			nick TEXT,
			password TEXT,
			autojoin INTEGER DEFAULT 0,
			PRIMARY KEY (account, jid)
		)`,

		// Session state (persists login sessions)
		`CREATE TABLE IF NOT EXISTS sessions (
			account TEXT PRIMARY KEY,
			resource TEXT,
			last_connected INTEGER,
			status TEXT,
			status_msg TEXT,
			priority INTEGER DEFAULT 0,
			session_data BLOB
		)`,

		// Window state (persists open windows/chats)
		`CREATE TABLE IF NOT EXISTS window_state (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account TEXT NOT NULL,
			window_type TEXT NOT NULL,
			jid TEXT,
			position INTEGER,
			active INTEGER DEFAULT 0
		)`,

		// Application state (UI preferences)
		`CREATE TABLE IF NOT EXISTS app_state (
			key TEXT PRIMARY KEY,
			value TEXT
		)`,

		// Chat state (unread counts, etc.)
		`CREATE TABLE IF NOT EXISTS chat_state (
			account TEXT NOT NULL,
			jid TEXT NOT NULL,
			unread INTEGER DEFAULT 0,
			last_read INTEGER,
			PRIMARY KEY (account, jid)
		)`,
	}

	for _, migration := range migrations {
		if _, err := d.db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

// Message operations

// SaveMessage saves a message to the database
func (d *DB) SaveMessage(account, jid, id, body, msgType string, timestamp time.Time, outgoing, encrypted bool) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO messages (id, account, jid, body, timestamp, outgoing, encrypted, type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, account, jid, body, timestamp.Unix(), outgoing, encrypted, msgType)
	return err
}

// GetMessages retrieves messages for a JID
func (d *DB) GetMessages(account, jid string, limit, offset int) ([]Message, error) {
	rows, err := d.db.Query(`
		SELECT id, body, timestamp, outgoing, encrypted, type, received, displayed, corrected, corrected_id
		FROM messages
		WHERE account = ? AND jid = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`, account, jid, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var ts int64
		var correctedID sql.NullString

		err := rows.Scan(&msg.ID, &msg.Body, &ts, &msg.Outgoing, &msg.Encrypted,
			&msg.Type, &msg.Received, &msg.Displayed, &msg.Corrected, &correctedID)
		if err != nil {
			return nil, err
		}

		msg.Timestamp = time.Unix(ts, 0)
		if correctedID.Valid {
			msg.CorrectedID = correctedID.String
		}
		messages = append(messages, msg)
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// MarkMessageReceived marks a message as received
func (d *DB) MarkMessageReceived(id string) error {
	_, err := d.db.Exec("UPDATE messages SET received = 1 WHERE id = ?", id)
	return err
}

// MarkMessageDisplayed marks a message as displayed
func (d *DB) MarkMessageDisplayed(id string) error {
	_, err := d.db.Exec("UPDATE messages SET displayed = 1 WHERE id = ?", id)
	return err
}

// DeleteMessages deletes messages for a JID
func (d *DB) DeleteMessages(account, jid string) error {
	_, err := d.db.Exec("DELETE FROM messages WHERE account = ? AND jid = ?", account, jid)
	return err
}

// Message represents a stored message
type Message struct {
	ID          string
	Body        string
	Timestamp   time.Time
	Outgoing    bool
	Encrypted   bool
	Type        string
	Received    bool
	Displayed   bool
	Corrected   bool
	CorrectedID string
}

// Roster operations

// SaveRosterItem saves a roster item
func (d *DB) SaveRosterItem(account, jid, name, subscription, groups string) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO roster (account, jid, name, subscription, groups)
		VALUES (?, ?, ?, ?, ?)
	`, account, jid, name, subscription, groups)
	return err
}

// GetRoster retrieves the roster for an account
func (d *DB) GetRoster(account string) ([]RosterItem, error) {
	rows, err := d.db.Query(`
		SELECT jid, name, subscription, groups
		FROM roster
		WHERE account = ?
	`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []RosterItem
	for rows.Next() {
		var item RosterItem
		var name, groups sql.NullString

		err := rows.Scan(&item.JID, &name, &item.Subscription, &groups)
		if err != nil {
			return nil, err
		}

		if name.Valid {
			item.Name = name.String
		}
		if groups.Valid {
			item.Groups = groups.String
		}
		items = append(items, item)
	}

	return items, nil
}

// DeleteRosterItem deletes a roster item
func (d *DB) DeleteRosterItem(account, jid string) error {
	_, err := d.db.Exec("DELETE FROM roster WHERE account = ? AND jid = ?", account, jid)
	return err
}

// ClearRoster clears the roster for an account
func (d *DB) ClearRoster(account string) error {
	_, err := d.db.Exec("DELETE FROM roster WHERE account = ?", account)
	return err
}

// RosterItem represents a stored roster item
type RosterItem struct {
	JID          string
	Name         string
	Subscription string
	Groups       string
}

// OMEMO operations

// SaveOMEMOIdentity saves an OMEMO identity
func (d *DB) SaveOMEMOIdentity(account, jid string, deviceID int, identityKey []byte, trustLevel int) error {
	now := time.Now().Unix()
	_, err := d.db.Exec(`
		INSERT INTO omemo_identities (account, jid, device_id, identity_key, trust_level, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account, jid, device_id) DO UPDATE SET
			identity_key = excluded.identity_key,
			trust_level = excluded.trust_level,
			last_seen = excluded.last_seen
	`, account, jid, deviceID, identityKey, trustLevel, now, now)
	return err
}

// GetOMEMOIdentity retrieves an OMEMO identity
func (d *DB) GetOMEMOIdentity(account, jid string, deviceID int) (*OMEMOIdentity, error) {
	var identity OMEMOIdentity
	var firstSeen, lastSeen int64

	err := d.db.QueryRow(`
		SELECT identity_key, trust_level, first_seen, last_seen
		FROM omemo_identities
		WHERE account = ? AND jid = ? AND device_id = ?
	`, account, jid, deviceID).Scan(&identity.IdentityKey, &identity.TrustLevel, &firstSeen, &lastSeen)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	identity.DeviceID = deviceID
	identity.FirstSeen = time.Unix(firstSeen, 0)
	identity.LastSeen = time.Unix(lastSeen, 0)
	return &identity, nil
}

// GetOMEMOIdentities retrieves all OMEMO identities for a JID
func (d *DB) GetOMEMOIdentities(account, jid string) ([]OMEMOIdentity, error) {
	rows, err := d.db.Query(`
		SELECT device_id, identity_key, trust_level, first_seen, last_seen
		FROM omemo_identities
		WHERE account = ? AND jid = ?
	`, account, jid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var identities []OMEMOIdentity
	for rows.Next() {
		var identity OMEMOIdentity
		var firstSeen, lastSeen int64

		err := rows.Scan(&identity.DeviceID, &identity.IdentityKey, &identity.TrustLevel, &firstSeen, &lastSeen)
		if err != nil {
			return nil, err
		}

		identity.FirstSeen = time.Unix(firstSeen, 0)
		identity.LastSeen = time.Unix(lastSeen, 0)
		identities = append(identities, identity)
	}

	return identities, nil
}

// SetOMEMOTrustLevel sets the trust level for an identity
func (d *DB) SetOMEMOTrustLevel(account, jid string, deviceID, trustLevel int) error {
	_, err := d.db.Exec(`
		UPDATE omemo_identities SET trust_level = ?
		WHERE account = ? AND jid = ? AND device_id = ?
	`, trustLevel, account, jid, deviceID)
	return err
}

// OMEMOIdentity represents a stored OMEMO identity
type OMEMOIdentity struct {
	DeviceID    int
	IdentityKey []byte
	TrustLevel  int
	FirstSeen   time.Time
	LastSeen    time.Time
}

// SaveOMEMOSession saves an OMEMO session
func (d *DB) SaveOMEMOSession(account, jid string, deviceID int, sessionData []byte) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO omemo_sessions (account, jid, device_id, session_data)
		VALUES (?, ?, ?, ?)
	`, account, jid, deviceID, sessionData)
	return err
}

// GetOMEMOSession retrieves an OMEMO session
func (d *DB) GetOMEMOSession(account, jid string, deviceID int) ([]byte, error) {
	var data []byte
	err := d.db.QueryRow(`
		SELECT session_data FROM omemo_sessions
		WHERE account = ? AND jid = ? AND device_id = ?
	`, account, jid, deviceID).Scan(&data)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	return data, err
}

// DeleteOMEMOSession deletes an OMEMO session
func (d *DB) DeleteOMEMOSession(account, jid string, deviceID int) error {
	_, err := d.db.Exec(`
		DELETE FROM omemo_sessions
		WHERE account = ? AND jid = ? AND device_id = ?
	`, account, jid, deviceID)
	return err
}

// Bookmark operations

// SaveBookmark saves a bookmark
func (d *DB) SaveBookmark(account, jid, name, nick, password string, autojoin bool) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO bookmarks (account, jid, name, nick, password, autojoin)
		VALUES (?, ?, ?, ?, ?, ?)
	`, account, jid, name, nick, password, autojoin)
	return err
}

// GetBookmarks retrieves all bookmarks for an account
func (d *DB) GetBookmarks(account string) ([]Bookmark, error) {
	rows, err := d.db.Query(`
		SELECT jid, name, nick, password, autojoin
		FROM bookmarks
		WHERE account = ?
	`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookmarks []Bookmark
	for rows.Next() {
		var b Bookmark
		var name, nick, password sql.NullString

		err := rows.Scan(&b.JID, &name, &nick, &password, &b.Autojoin)
		if err != nil {
			return nil, err
		}

		if name.Valid {
			b.Name = name.String
		}
		if nick.Valid {
			b.Nick = nick.String
		}
		if password.Valid {
			b.Password = password.String
		}
		bookmarks = append(bookmarks, b)
	}

	return bookmarks, nil
}

// DeleteBookmark deletes a bookmark
func (d *DB) DeleteBookmark(account, jid string) error {
	_, err := d.db.Exec("DELETE FROM bookmarks WHERE account = ? AND jid = ?", account, jid)
	return err
}

// Bookmark represents a stored bookmark
type Bookmark struct {
	JID      string
	Name     string
	Nick     string
	Password string
	Autojoin bool
}

// Chat state operations

// SetUnreadCount sets the unread count for a JID
func (d *DB) SetUnreadCount(account, jid string, count int) error {
	_, err := d.db.Exec(`
		INSERT INTO chat_state (account, jid, unread)
		VALUES (?, ?, ?)
		ON CONFLICT(account, jid) DO UPDATE SET unread = excluded.unread
	`, account, jid, count)
	return err
}

// GetUnreadCount gets the unread count for a JID
func (d *DB) GetUnreadCount(account, jid string) (int, error) {
	var count int
	err := d.db.QueryRow(`
		SELECT unread FROM chat_state
		WHERE account = ? AND jid = ?
	`, account, jid).Scan(&count)

	if err == sql.ErrNoRows {
		return 0, nil
	}
	return count, err
}

// MarkRead marks a chat as read
func (d *DB) MarkRead(account, jid string) error {
	now := time.Now().Unix()
	_, err := d.db.Exec(`
		INSERT INTO chat_state (account, jid, unread, last_read)
		VALUES (?, ?, 0, ?)
		ON CONFLICT(account, jid) DO UPDATE SET unread = 0, last_read = excluded.last_read
	`, account, jid, now)
	return err
}

// Session operations (for persisting login state)

// Session represents a saved login session
type Session struct {
	Account       string
	Resource      string
	LastConnected time.Time
	Status        string
	StatusMsg     string
	Priority      int
	SessionData   []byte
}

// SaveSession saves a login session
func (d *DB) SaveSession(session Session) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO sessions (account, resource, last_connected, status, status_msg, priority, session_data)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, session.Account, session.Resource, time.Now().Unix(), session.Status, session.StatusMsg, session.Priority, session.SessionData)
	return err
}

// GetSession retrieves a saved session
func (d *DB) GetSession(account string) (*Session, error) {
	var session Session
	var lastConnected int64
	var resource, status, statusMsg sql.NullString
	var sessionData []byte

	err := d.db.QueryRow(`
		SELECT account, resource, last_connected, status, status_msg, priority, session_data
		FROM sessions
		WHERE account = ?
	`, account).Scan(&session.Account, &resource, &lastConnected, &status, &statusMsg, &session.Priority, &sessionData)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if resource.Valid {
		session.Resource = resource.String
	}
	if status.Valid {
		session.Status = status.String
	}
	if statusMsg.Valid {
		session.StatusMsg = statusMsg.String
	}
	session.LastConnected = time.Unix(lastConnected, 0)
	session.SessionData = sessionData

	return &session, nil
}

// GetAllSessions retrieves all saved sessions
func (d *DB) GetAllSessions() ([]Session, error) {
	rows, err := d.db.Query(`
		SELECT account, resource, last_connected, status, status_msg, priority, session_data
		FROM sessions
		ORDER BY last_connected DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var session Session
		var lastConnected int64
		var resource, status, statusMsg sql.NullString
		var sessionData []byte

		err := rows.Scan(&session.Account, &resource, &lastConnected, &status, &statusMsg, &session.Priority, &sessionData)
		if err != nil {
			return nil, err
		}

		if resource.Valid {
			session.Resource = resource.String
		}
		if status.Valid {
			session.Status = status.String
		}
		if statusMsg.Valid {
			session.StatusMsg = statusMsg.String
		}
		session.LastConnected = time.Unix(lastConnected, 0)
		session.SessionData = sessionData

		sessions = append(sessions, session)
	}

	return sessions, nil
}

// DeleteSession deletes a saved session
func (d *DB) DeleteSession(account string) error {
	_, err := d.db.Exec("DELETE FROM sessions WHERE account = ?", account)
	return err
}

// Window state operations

// WindowState represents the state of a window
type WindowState struct {
	ID         int
	WindowType string
	JID        string
	Position   int
	Active     bool
}

// SaveWindowState saves the current window state
func (d *DB) SaveWindowState(account string, windows []WindowState) error {
	// Clear existing state for this account
	_, err := d.db.Exec("DELETE FROM window_state WHERE account = ?", account)
	if err != nil {
		return err
	}

	// Insert new state
	for _, w := range windows {
		_, err := d.db.Exec(`
			INSERT INTO window_state (account, window_type, jid, position, active)
			VALUES (?, ?, ?, ?, ?)
		`, account, w.WindowType, w.JID, w.Position, w.Active)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetWindowState retrieves the window state for an account
func (d *DB) GetWindowState(account string) ([]WindowState, error) {
	rows, err := d.db.Query(`
		SELECT id, window_type, jid, position, active
		FROM window_state
		WHERE account = ?
		ORDER BY position
	`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var windows []WindowState
	for rows.Next() {
		var w WindowState
		var jid sql.NullString

		err := rows.Scan(&w.ID, &w.WindowType, &jid, &w.Position, &w.Active)
		if err != nil {
			return nil, err
		}

		if jid.Valid {
			w.JID = jid.String
		}
		windows = append(windows, w)
	}

	return windows, nil
}

// App state operations (key-value store for UI state)

// SetAppState sets an app state value
func (d *DB) SetAppState(key, value string) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO app_state (key, value)
		VALUES (?, ?)
	`, key, value)
	return err
}

// GetAppState gets an app state value
func (d *DB) GetAppState(key string) (string, error) {
	var value string
	err := d.db.QueryRow("SELECT value FROM app_state WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// DeleteAppState deletes an app state value
func (d *DB) DeleteAppState(key string) error {
	_, err := d.db.Exec("DELETE FROM app_state WHERE key = ?", key)
	return err
}

// Message retention operations

// DeleteOldMessages deletes messages older than the specified number of days
func (d *DB) DeleteOldMessages(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()
	result, err := d.db.Exec("DELETE FROM messages WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetMessageCount returns the total number of messages
func (d *DB) GetMessageCount() (int64, error) {
	var count int64
	err := d.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count)
	return count, err
}

// GetDatabaseSize returns the approximate size of the database
func (d *DB) GetDatabaseSize() (int64, error) {
	var pageCount, pageSize int64
	err := d.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	if err != nil {
		return 0, err
	}
	err = d.db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	if err != nil {
		return 0, err
	}
	return pageCount * pageSize, nil
}

// Vacuum compacts the database
func (d *DB) Vacuum() error {
	_, err := d.db.Exec("VACUUM")
	return err
}
