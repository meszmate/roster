package client

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"fmt"
	"sync"
	"time"

	cryptoomemo "github.com/meszmate/xmpp-go/crypto/omemo"
)

type OMEMOStore struct {
	mu       sync.RWMutex
	jid      string
	deviceID uint32
	db       *sql.DB

	identityKey   *cryptoomemo.IdentityKeyPair
	remoteKeys    map[cryptoomemo.Address]ed25519.PublicKey
	preKeys       map[uint32]*cryptoomemo.PreKeyRecord
	signedPreKeys map[uint32]*cryptoomemo.SignedPreKeyRecord
	sessions      map[cryptoomemo.Address][]byte
}

func NewOMEMOStore(jid string, deviceID uint32) *OMEMOStore {
	return &OMEMOStore{
		jid:           jid,
		deviceID:      deviceID,
		remoteKeys:    make(map[cryptoomemo.Address]ed25519.PublicKey),
		preKeys:       make(map[uint32]*cryptoomemo.PreKeyRecord),
		signedPreKeys: make(map[uint32]*cryptoomemo.SignedPreKeyRecord),
		sessions:      make(map[cryptoomemo.Address][]byte),
	}
}

func NewOMEMOStoreWithDB(jid string, deviceID uint32, db *sql.DB) *OMEMOStore {
	s := &OMEMOStore{
		jid:           jid,
		deviceID:      deviceID,
		db:            db,
		remoteKeys:    make(map[cryptoomemo.Address]ed25519.PublicKey),
		preKeys:       make(map[uint32]*cryptoomemo.PreKeyRecord),
		signedPreKeys: make(map[uint32]*cryptoomemo.SignedPreKeyRecord),
		sessions:      make(map[cryptoomemo.Address][]byte),
	}
	s.loadFromDB()
	return s
}

func (s *OMEMOStore) loadFromDB() {
	if s.db == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var ikpPrivate, ikpPublic []byte
	err := s.db.QueryRow(`
		SELECT private_key, public_key FROM omemo_identity 
		WHERE jid = ? AND device_id = ?`, s.jid, s.deviceID).Scan(&ikpPrivate, &ikpPublic)
	if err == nil && len(ikpPrivate) > 0 && len(ikpPublic) > 0 {
		s.identityKey = &cryptoomemo.IdentityKeyPair{
			PrivateKey: ed25519.PrivateKey(ikpPrivate),
			PublicKey:  ed25519.PublicKey(ikpPublic),
		}
	}

	rows, err := s.db.Query(`
		SELECT jid, device_id, identity_key FROM omemo_remote_identities 
		WHERE account_jid = ?`, s.jid)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var jid string
			var deviceID uint32
			var identityKey []byte
			if err := rows.Scan(&jid, &deviceID, &identityKey); err == nil {
				addr := cryptoomemo.Address{JID: jid, DeviceID: deviceID}
				s.remoteKeys[addr] = ed25519.PublicKey(identityKey)
			}
		}
	}

	rows, err = s.db.Query(`
		SELECT key_id, private_key, public_key FROM omemo_prekeys 
		WHERE jid = ?`, s.jid)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var keyID uint32
			var privateKey, publicKey []byte
			if err := rows.Scan(&keyID, &privateKey, &publicKey); err == nil {
				s.preKeys[keyID] = &cryptoomemo.PreKeyRecord{
					ID:         keyID,
					PrivateKey: privateKey,
					PublicKey:  publicKey,
				}
			}
		}
	}

	rows, err = s.db.Query(`
		SELECT key_id, private_key, public_key, signature FROM omemo_signed_prekeys 
		WHERE jid = ?`, s.jid)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var keyID uint32
			var privateKey, publicKey, signature []byte
			if err := rows.Scan(&keyID, &privateKey, &publicKey, &signature); err == nil {
				s.signedPreKeys[keyID] = &cryptoomemo.SignedPreKeyRecord{
					ID:         keyID,
					PrivateKey: privateKey,
					PublicKey:  publicKey,
					Signature:  signature,
				}
			}
		}
	}

	rows, err = s.db.Query(`
		SELECT jid, device_id, session_data FROM omemo_sessions 
		WHERE account_jid = ?`, s.jid)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var jid string
			var deviceID uint32
			var sessionData []byte
			if err := rows.Scan(&jid, &deviceID, &sessionData); err == nil {
				addr := cryptoomemo.Address{JID: jid, DeviceID: deviceID}
				s.sessions[addr] = sessionData
			}
		}
	}
}

func (s *OMEMOStore) GetIdentityKeyPair() (*cryptoomemo.IdentityKeyPair, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.identityKey, nil
}

func (s *OMEMOStore) SaveIdentityKeyPair(ikp *cryptoomemo.IdentityKeyPair) error {
	s.mu.Lock()
	s.identityKey = ikp
	s.mu.Unlock()

	if s.db != nil {
		_, err := s.db.Exec(`
			INSERT OR REPLACE INTO omemo_identity (jid, device_id, private_key, public_key, created_at)
			VALUES (?, ?, ?, ?, ?)`,
			s.jid, s.deviceID, []byte(ikp.PrivateKey), []byte(ikp.PublicKey), time.Now().Unix())
		return err
	}
	return nil
}

func (s *OMEMOStore) GetLocalDeviceID() (uint32, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.deviceID, nil
}

func (s *OMEMOStore) GetRemoteIdentity(addr cryptoomemo.Address) (ed25519.PublicKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.remoteKeys[addr], nil
}

func (s *OMEMOStore) SaveRemoteIdentity(addr cryptoomemo.Address, key ed25519.PublicKey) error {
	s.mu.Lock()
	s.remoteKeys[addr] = key
	s.mu.Unlock()

	if s.db != nil {
		_, err := s.db.Exec(`
			INSERT OR REPLACE INTO omemo_remote_identities (account_jid, jid, device_id, identity_key, trust_level, first_seen)
			VALUES (?, ?, ?, ?, 1, ?)`,
			s.jid, addr.JID, addr.DeviceID, []byte(key), time.Now().Unix())
		return err
	}
	return nil
}

func (s *OMEMOStore) IsTrusted(addr cryptoomemo.Address, key ed25519.PublicKey) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	existing, ok := s.remoteKeys[addr]
	if !ok {
		return true, nil
	}
	return bytes.Equal(existing, key), nil
}

func (s *OMEMOStore) GetPreKey(id uint32) (*cryptoomemo.PreKeyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pk, ok := s.preKeys[id]
	if !ok {
		return nil, cryptoomemo.ErrNoPreKey
	}
	return pk, nil
}

func (s *OMEMOStore) SavePreKey(record *cryptoomemo.PreKeyRecord) error {
	s.mu.Lock()
	s.preKeys[record.ID] = record
	s.mu.Unlock()

	if s.db != nil {
		_, err := s.db.Exec(`
			INSERT OR REPLACE INTO omemo_prekeys (jid, key_id, private_key, public_key)
			VALUES (?, ?, ?, ?)`,
			s.jid, record.ID, record.PrivateKey, record.PublicKey)
		return err
	}
	return nil
}

func (s *OMEMOStore) RemovePreKey(id uint32) error {
	s.mu.Lock()
	delete(s.preKeys, id)
	s.mu.Unlock()

	if s.db != nil {
		_, err := s.db.Exec(`DELETE FROM omemo_prekeys WHERE jid = ? AND key_id = ?`, s.jid, id)
		return err
	}
	return nil
}

func (s *OMEMOStore) GetSignedPreKey(id uint32) (*cryptoomemo.SignedPreKeyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	spk, ok := s.signedPreKeys[id]
	if !ok {
		return nil, cryptoomemo.ErrNoPreKey
	}
	return spk, nil
}

func (s *OMEMOStore) SaveSignedPreKey(record *cryptoomemo.SignedPreKeyRecord) error {
	s.mu.Lock()
	s.signedPreKeys[record.ID] = record
	s.mu.Unlock()

	if s.db != nil {
		_, err := s.db.Exec(`
			INSERT OR REPLACE INTO omemo_signed_prekeys (jid, key_id, private_key, public_key, signature, timestamp)
			VALUES (?, ?, ?, ?, ?, ?)`,
			s.jid, record.ID, record.PrivateKey, record.PublicKey, record.Signature, time.Now().Unix())
		return err
	}
	return nil
}

func (s *OMEMOStore) GetSession(addr cryptoomemo.Address) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, ok := s.sessions[addr]
	if !ok {
		return nil, cryptoomemo.ErrNoSession
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	return cp, nil
}

func (s *OMEMOStore) SaveSession(addr cryptoomemo.Address, data []byte) error {
	s.mu.Lock()
	cp := make([]byte, len(data))
	copy(cp, data)
	s.sessions[addr] = cp
	s.mu.Unlock()

	if s.db != nil {
		_, err := s.db.Exec(`
			INSERT OR REPLACE INTO omemo_sessions (account_jid, jid, device_id, session_data)
			VALUES (?, ?, ?, ?)`,
			s.jid, addr.JID, addr.DeviceID, data)
		return err
	}
	return nil
}

func (s *OMEMOStore) ContainsSession(addr cryptoomemo.Address) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.sessions[addr]
	return ok, nil
}

func GenerateDeviceID() uint32 {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

const (
	TrustUndecided = 0
	TrustTrusted   = 1
	TrustVerified  = 2
	TrustUntrusted = 3
)

func TrustLevelString(level int) string {
	switch level {
	case TrustTrusted:
		return "trusted"
	case TrustVerified:
		return "verified"
	case TrustUntrusted:
		return "untrusted"
	default:
		return "undecided"
	}
}

func (s *OMEMOStore) GetFingerprint() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.identityKey == nil {
		return ""
	}
	return formatFingerprint(s.identityKey.PublicKey)
}

func formatFingerprint(key ed25519.PublicKey) string {
	encoded := fmt.Sprintf("%x", key)
	var formatted string
	for i, c := range encoded {
		if i > 0 && i%8 == 0 {
			formatted += " "
		}
		formatted += string(c)
	}
	return formatted
}

type RemoteIdentity struct {
	DeviceID    uint32
	IdentityKey []byte
	TrustLevel  int
}

func (s *OMEMOStore) GetRemoteIdentitiesForJID(jid string) []RemoteIdentity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var identities []RemoteIdentity
	for addr, key := range s.remoteKeys {
		if addr.JID == jid {
			trustLevel := TrustUndecided
			if s.db != nil {
				var trust int
				err := s.db.QueryRow(`
					SELECT trust_level FROM omemo_remote_identities 
					WHERE account_jid = ? AND jid = ? AND device_id = ?`,
					s.jid, jid, addr.DeviceID).Scan(&trust)
				if err == nil {
					trustLevel = trust
				}
			}
			identities = append(identities, RemoteIdentity{
				DeviceID:    addr.DeviceID,
				IdentityKey: []byte(key),
				TrustLevel:  trustLevel,
			})
		}
	}
	return identities
}

func (s *OMEMOStore) SetTrustLevel(jid string, deviceID uint32, level int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		_, err := s.db.Exec(`
			UPDATE omemo_remote_identities 
			SET trust_level = ? 
			WHERE account_jid = ? AND jid = ? AND device_id = ?`,
			level, s.jid, jid, deviceID)
		return err
	}
	return nil
}

func (s *OMEMOStore) GetTrustLevel(jid string, deviceID uint32) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db != nil {
		var trust int
		err := s.db.QueryRow(`
			SELECT trust_level FROM omemo_remote_identities 
			WHERE account_jid = ? AND jid = ? AND device_id = ?`,
			s.jid, jid, deviceID).Scan(&trust)
		if err == nil {
			return trust
		}
	}
	return TrustUndecided
}

func (s *OMEMOStore) DeleteDevice(jid string, deviceID uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	addr := cryptoomemo.Address{JID: jid, DeviceID: deviceID}
	delete(s.remoteKeys, addr)
	delete(s.sessions, addr)

	if s.db != nil {
		_, err := s.db.Exec(`
			DELETE FROM omemo_remote_identities 
			WHERE account_jid = ? AND jid = ? AND device_id = ?`,
			s.jid, jid, deviceID)
		return err
	}
	return nil
}

func (s *OMEMOStore) GetAllRemoteIdentities() map[string][]RemoteIdentity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]RemoteIdentity)
	for addr, key := range s.remoteKeys {
		result[addr.JID] = append(result[addr.JID], RemoteIdentity{
			DeviceID:    addr.DeviceID,
			IdentityKey: []byte(key),
			TrustLevel:  1,
		})
	}
	return result
}
