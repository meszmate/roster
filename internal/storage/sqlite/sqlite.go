package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	db *sql.DB
}

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

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) migrate() error {
	migrations := []string{
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

		`CREATE TABLE IF NOT EXISTS sessions (
			account TEXT PRIMARY KEY,
			resource TEXT,
			last_connected INTEGER,
			status TEXT,
			status_msg TEXT,
			priority INTEGER DEFAULT 0,
			session_data BLOB
		)`,

		`CREATE TABLE IF NOT EXISTS window_state (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account TEXT NOT NULL,
			window_type TEXT NOT NULL,
			jid TEXT,
			position INTEGER,
			active INTEGER DEFAULT 0
		)`,

		`CREATE TABLE IF NOT EXISTS app_state (
			key TEXT PRIMARY KEY,
			value TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS chat_state (
			account TEXT NOT NULL,
			jid TEXT NOT NULL,
			unread INTEGER DEFAULT 0,
			last_read INTEGER,
			PRIMARY KEY (account, jid)
		)`,
		`CREATE TABLE IF NOT EXISTS roster_cache (
			account TEXT NOT NULL,
			jid TEXT NOT NULL,
			name TEXT,
			groups_json TEXT,
			subscription TEXT,
			last_updated INTEGER NOT NULL,
			PRIMARY KEY (account, jid)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_roster_cache_account ON roster_cache(account)`,
		`CREATE TABLE IF NOT EXISTS mam_sync (
			account TEXT NOT NULL,
			jid TEXT NOT NULL,
			last_stanza_id TEXT,
			last_timestamp INTEGER,
			last_synced INTEGER NOT NULL,
			PRIMARY KEY (account, jid)
		)`,

		`CREATE TABLE IF NOT EXISTS contact_presence_settings (
			account TEXT NOT NULL,
			contact_jid TEXT NOT NULL,
			my_show TEXT,
			my_status_msg TEXT,
			PRIMARY KEY (account, contact_jid)
		)`,

		`CREATE TABLE IF NOT EXISTS contact_last_presence (
			account TEXT NOT NULL,
			contact_jid TEXT NOT NULL,
			their_show TEXT,
			their_status_msg TEXT,
			last_updated INTEGER,
			PRIMARY KEY (account, contact_jid)
		)`,

		`CREATE TABLE IF NOT EXISTS status_sharing (
			account TEXT NOT NULL,
			contact_jid TEXT NOT NULL,
			share_enabled INTEGER DEFAULT 0,
			PRIMARY KEY (account, contact_jid)
		)`,

		`CREATE TABLE IF NOT EXISTS omemo_identity (
			jid TEXT NOT NULL,
			device_id INTEGER NOT NULL,
			private_key BLOB NOT NULL,
			public_key BLOB NOT NULL,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (jid, device_id)
		)`,

		`CREATE TABLE IF NOT EXISTS omemo_remote_identities (
			account_jid TEXT NOT NULL,
			jid TEXT NOT NULL,
			device_id INTEGER NOT NULL,
			identity_key BLOB NOT NULL,
			trust_level INTEGER DEFAULT 1,
			first_seen INTEGER NOT NULL,
			PRIMARY KEY (account_jid, jid, device_id)
		)`,

		`CREATE TABLE IF NOT EXISTS omemo_prekeys (
			jid TEXT NOT NULL,
			key_id INTEGER NOT NULL,
			private_key BLOB NOT NULL,
			public_key BLOB NOT NULL,
			PRIMARY KEY (jid, key_id)
		)`,

		`CREATE TABLE IF NOT EXISTS omemo_signed_prekeys (
			jid TEXT NOT NULL,
			key_id INTEGER NOT NULL,
			private_key BLOB NOT NULL,
			public_key BLOB NOT NULL,
			signature BLOB NOT NULL,
			timestamp INTEGER NOT NULL,
			PRIMARY KEY (jid, key_id)
		)`,

		`CREATE TABLE IF NOT EXISTS omemo_sessions (
			account_jid TEXT NOT NULL,
			jid TEXT NOT NULL,
			device_id INTEGER NOT NULL,
			session_data BLOB NOT NULL,
			PRIMARY KEY (account_jid, jid, device_id)
		)`,
	}

	for _, migration := range migrations {
		if _, err := d.db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	// Backward-compatible migration for older DBs that predate stanza_id.
	if _, err := d.db.Exec(`ALTER TABLE messages ADD COLUMN stanza_id TEXT`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return fmt.Errorf("failed to ensure stanza_id column: %w", err)
		}
	}
	if _, err := d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_stanza_id ON messages(stanza_id)`); err != nil {
		return fmt.Errorf("failed to ensure stanza_id index: %w", err)
	}

	return nil
}

func (d *DB) SaveMessage(account, jid, id, body, msgType string, timestamp time.Time, outgoing, encrypted bool) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO messages (id, account, jid, body, timestamp, outgoing, encrypted, type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, account, jid, body, timestamp.Unix(), outgoing, encrypted, msgType)
	return err
}

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

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

func (d *DB) MarkMessageReceived(id string) error {
	_, err := d.db.Exec("UPDATE messages SET received = 1 WHERE id = ?", id)
	return err
}

func (d *DB) MarkMessageDisplayed(id string) error {
	_, err := d.db.Exec("UPDATE messages SET displayed = 1 WHERE id = ?", id)
	return err
}

func (d *DB) DeleteMessages(account, jid string) error {
	_, err := d.db.Exec("DELETE FROM messages WHERE account = ? AND jid = ?", account, jid)
	return err
}

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

func (d *DB) SetUnreadCount(account, jid string, count int) error {
	_, err := d.db.Exec(`
		INSERT INTO chat_state (account, jid, unread)
		VALUES (?, ?, ?)
		ON CONFLICT(account, jid) DO UPDATE SET unread = excluded.unread
	`, account, jid, count)
	return err
}

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

func (d *DB) MarkRead(account, jid string) error {
	now := time.Now().Unix()
	_, err := d.db.Exec(`
		INSERT INTO chat_state (account, jid, unread, last_read)
		VALUES (?, ?, 0, ?)
		ON CONFLICT(account, jid) DO UPDATE SET unread = 0, last_read = excluded.last_read
	`, account, jid, now)
	return err
}

type Session struct {
	Account       string
	Resource      string
	LastConnected time.Time
	Status        string
	StatusMsg     string
	Priority      int
	SessionData   []byte
}

func (d *DB) SaveSession(session Session) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO sessions (account, resource, last_connected, status, status_msg, priority, session_data)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, session.Account, session.Resource, time.Now().Unix(), session.Status, session.StatusMsg, session.Priority, session.SessionData)
	return err
}

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

func (d *DB) DeleteSession(account string) error {
	_, err := d.db.Exec("DELETE FROM sessions WHERE account = ?", account)
	return err
}

type WindowState struct {
	ID         int
	WindowType string
	JID        string
	Position   int
	Active     bool
}

func (d *DB) SaveWindowState(account string, windows []WindowState) error {
	_, err := d.db.Exec("DELETE FROM window_state WHERE account = ?", account)
	if err != nil {
		return err
	}

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

func (d *DB) SetAppState(key, value string) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO app_state (key, value)
		VALUES (?, ?)
	`, key, value)
	return err
}

func (d *DB) GetAppState(key string) (string, error) {
	var value string
	err := d.db.QueryRow("SELECT value FROM app_state WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (d *DB) DeleteAppState(key string) error {
	_, err := d.db.Exec("DELETE FROM app_state WHERE key = ?", key)
	return err
}

func (d *DB) DeleteOldMessages(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()
	result, err := d.db.Exec("DELETE FROM messages WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (d *DB) GetMessageCount() (int64, error) {
	var count int64
	err := d.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count)
	return count, err
}

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

func (d *DB) Vacuum() error {
	_, err := d.db.Exec("VACUUM")
	return err
}

func (d *DB) SaveMyPresenceForContact(account, contactJID, show, statusMsg string) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO contact_presence_settings (account, contact_jid, my_show, my_status_msg)
		VALUES (?, ?, ?, ?)
	`, account, contactJID, show, statusMsg)
	return err
}

func (d *DB) GetMyPresenceForContact(account, contactJID string) (show, statusMsg string, err error) {
	var showNull, statusNull sql.NullString
	err = d.db.QueryRow(`
		SELECT my_show, my_status_msg FROM contact_presence_settings
		WHERE account = ? AND contact_jid = ?
	`, account, contactJID).Scan(&showNull, &statusNull)

	if err == sql.ErrNoRows {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}

	if showNull.Valid {
		show = showNull.String
	}
	if statusNull.Valid {
		statusMsg = statusNull.String
	}
	return show, statusMsg, nil
}

func (d *DB) DeleteMyPresenceForContact(account, contactJID string) error {
	_, err := d.db.Exec(`
		DELETE FROM contact_presence_settings
		WHERE account = ? AND contact_jid = ?
	`, account, contactJID)
	return err
}

func (d *DB) SaveContactLastPresence(account, contactJID, show, statusMsg string) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO contact_last_presence (account, contact_jid, their_show, their_status_msg, last_updated)
		VALUES (?, ?, ?, ?, ?)
	`, account, contactJID, show, statusMsg, time.Now().Unix())
	return err
}

func (d *DB) GetContactLastPresence(account, contactJID string) (show, statusMsg string, lastUpdated time.Time, err error) {
	var showNull, statusNull sql.NullString
	var lastUpdatedUnix int64

	err = d.db.QueryRow(`
		SELECT their_show, their_status_msg, last_updated FROM contact_last_presence
		WHERE account = ? AND contact_jid = ?
	`, account, contactJID).Scan(&showNull, &statusNull, &lastUpdatedUnix)

	if err == sql.ErrNoRows {
		return "", "", time.Time{}, nil
	}
	if err != nil {
		return "", "", time.Time{}, err
	}

	if showNull.Valid {
		show = showNull.String
	}
	if statusNull.Valid {
		statusMsg = statusNull.String
	}
	lastUpdated = time.Unix(lastUpdatedUnix, 0)
	return show, statusMsg, lastUpdated, nil
}

func (d *DB) SetStatusSharing(account, contactJID string, enabled bool) error {
	val := 0
	if enabled {
		val = 1
	}
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO status_sharing (account, contact_jid, share_enabled)
		VALUES (?, ?, ?)
	`, account, contactJID, val)
	return err
}

func (d *DB) GetStatusSharing(account, contactJID string) (bool, error) {
	var enabled int
	err := d.db.QueryRow(`
		SELECT share_enabled FROM status_sharing
		WHERE account = ? AND contact_jid = ?
	`, account, contactJID).Scan(&enabled)

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return enabled == 1, nil
}

func (d *DB) GetContactsWithStatusSharing(account string) ([]string, error) {
	rows, err := d.db.Query(`
		SELECT contact_jid FROM status_sharing
		WHERE account = ? AND share_enabled = 1
	`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []string
	for rows.Next() {
		var jid string
		if err := rows.Scan(&jid); err != nil {
			return nil, err
		}
		contacts = append(contacts, jid)
	}
	return contacts, nil
}

func (d *DB) SaveMessageWithStanzaID(account, jid, id, stanzaID, body, msgType string, timestamp time.Time, outgoing, encrypted bool) error {
	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO messages (id, stanza_id, account, jid, body, timestamp, outgoing, encrypted, type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, stanzaID, account, jid, body, timestamp.Unix(), outgoing, encrypted, msgType)
	return err
}

type MAMSync struct {
	Account       string
	JID           string
	LastStanzaID  string
	LastTimestamp int64
	LastSynced    int64
}

func (d *DB) GetMAMSync(account, jid string) (*MAMSync, error) {
	var sync MAMSync
	err := d.db.QueryRow(`
		SELECT account, jid, last_stanza_id, last_timestamp, last_synced
		FROM mam_sync
		WHERE account = ? AND jid = ?
	`, account, jid).Scan(&sync.Account, &sync.JID, &sync.LastStanzaID, &sync.LastTimestamp, &sync.LastSynced)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &sync, err
}

func (d *DB) SaveMAMSync(sync MAMSync) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO mam_sync (account, jid, last_stanza_id, last_timestamp, last_synced)
		VALUES (?, ?, ?, ?, ?)
	`, sync.Account, sync.JID, sync.LastStanzaID, sync.LastTimestamp, time.Now().Unix())
	return err
}

func (d *DB) DeleteMAMSync(account, jid string) error {
	_, err := d.db.Exec(`
		DELETE FROM mam_sync
		WHERE account = ? AND jid = ?
	`, account, jid)
	return err
}

func (d *DB) MessageExists(stanzaID string) (bool, error) {
	var one int
	err := d.db.QueryRow("SELECT 1 FROM messages WHERE stanza_id = ?", stanzaID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return one == 1, nil
}

type RosterEntry struct {
	JID          string
	Name         string
	Groups       []string
	Subscription string
}

func (d *DB) SaveRoster(account string, entries []RosterEntry) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM roster_cache WHERE account = ?", account); err != nil {
		return err
	}

	for _, entry := range entries {
		groupsJSON := "[]"
		if len(entry.Groups) > 0 {
			encoded, err := json.Marshal(entry.Groups)
			if err != nil {
				return err
			}
			groupsJSON = string(encoded)
		}

		_, err := tx.Exec(`
			INSERT INTO roster_cache (account, jid, name, groups_json, subscription, last_updated)
			VALUES (?, ?, ?, ?, ?, ?)
		`, account, entry.JID, entry.Name, groupsJSON, entry.Subscription, time.Now().Unix())
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *DB) GetRoster(account string) ([]RosterEntry, error) {
	rows, err := d.db.Query(`
		SELECT jid, name, groups_json, subscription
		FROM roster_cache
		WHERE account = ?
		ORDER BY COALESCE(name, jid), jid
	`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []RosterEntry
	for rows.Next() {
		var entry RosterEntry
		var groupsJSON sql.NullString
		var name, subscription sql.NullString

		if err := rows.Scan(&entry.JID, &name, &groupsJSON, &subscription); err != nil {
			return nil, err
		}

		if name.Valid {
			entry.Name = name.String
		}
		if subscription.Valid {
			entry.Subscription = subscription.String
		}
		if groupsJSON.Valid && groupsJSON.String != "" {
			_ = json.Unmarshal([]byte(groupsJSON.String), &entry.Groups)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}
