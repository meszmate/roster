package omemo

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/crypto/curve25519"
)

// TrustLevel represents the trust level of an identity
type TrustLevel int

const (
	TrustUndecided TrustLevel = iota
	TrustTrusted
	TrustUntrusted
	TrustVerified
)

// String returns the string representation of the trust level
func (t TrustLevel) String() string {
	switch t {
	case TrustUndecided:
		return "undecided"
	case TrustTrusted:
		return "trusted"
	case TrustUntrusted:
		return "untrusted"
	case TrustVerified:
		return "verified"
	default:
		return "unknown"
	}
}

// Identity represents an OMEMO identity
type Identity struct {
	DeviceID    uint32
	IdentityKey []byte
	TrustLevel  TrustLevel
}

// PreKey represents a one-time prekey
type PreKey struct {
	ID         uint32
	PrivateKey []byte
	PublicKey  []byte
}

// SignedPreKey represents a signed prekey
type SignedPreKey struct {
	ID         uint32
	PrivateKey []byte
	PublicKey  []byte
	Signature  []byte
	Timestamp  int64
}

// Bundle represents an OMEMO bundle (published to PEP)
type Bundle struct {
	DeviceID        uint32
	IdentityKey     []byte
	SignedPreKey    *SignedPreKey
	SignedPreKeySig []byte
	PreKeys         []PreKey
}

// Session represents an encrypted session with a device
type Session struct {
	RemoteJID       string
	RemoteDeviceID  uint32
	ChainKey        []byte
	MessageKeys     map[uint32][]byte
	SendingChain    *Chain
	ReceivingChains []*Chain
}

// Chain represents a message chain in the Double Ratchet
type Chain struct {
	RatchetKey []byte
	ChainKey   []byte
	MessageNum uint32
}

// EncryptedMessage represents an OMEMO encrypted message
type EncryptedMessage struct {
	SenderDeviceID uint32
	IV             []byte
	Payload        []byte // AES-GCM encrypted
	Keys           map[uint32][]byte // Device ID -> Encrypted key
}

// Manager manages OMEMO encryption for an account
type Manager struct {
	mu            sync.RWMutex
	jid           string
	deviceID      uint32
	identityKey   *KeyPair
	signedPreKey  *SignedPreKey
	preKeys       map[uint32]*PreKey
	sessions      map[string]map[uint32]*Session // JID -> DeviceID -> Session
	identities    map[string]map[uint32]*Identity // JID -> DeviceID -> Identity
	trustOnFirst  bool
	store         Store
}

// KeyPair represents a public/private key pair
type KeyPair struct {
	Private []byte
	Public  []byte
}

// Store is the interface for OMEMO persistent storage
type Store interface {
	SaveIdentity(jid string, deviceID uint32, identityKey []byte, trust TrustLevel) error
	GetIdentity(jid string, deviceID uint32) (*Identity, error)
	GetIdentities(jid string) ([]Identity, error)
	SetTrustLevel(jid string, deviceID uint32, trust TrustLevel) error

	SaveSession(jid string, deviceID uint32, sessionData []byte) error
	GetSession(jid string, deviceID uint32) ([]byte, error)
	DeleteSession(jid string, deviceID uint32) error

	SavePreKey(keyID uint32, keyData []byte) error
	GetPreKey(keyID uint32) ([]byte, error)
	DeletePreKey(keyID uint32) error

	SaveSignedPreKey(keyID uint32, keyData, signature []byte, timestamp int64) error
	GetSignedPreKey(keyID uint32) ([]byte, []byte, error)
}

// NewManager creates a new OMEMO manager
func NewManager(jid string, store Store, trustOnFirst bool) (*Manager, error) {
	m := &Manager{
		jid:          jid,
		preKeys:      make(map[uint32]*PreKey),
		sessions:     make(map[string]map[uint32]*Session),
		identities:   make(map[string]map[uint32]*Identity),
		trustOnFirst: trustOnFirst,
		store:        store,
	}

	// Generate or load device ID and keys
	if err := m.initializeKeys(); err != nil {
		return nil, err
	}

	return m, nil
}

// initializeKeys generates or loads identity keys
func (m *Manager) initializeKeys() error {
	// Generate identity key pair
	identityKey, err := generateKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate identity key: %w", err)
	}
	m.identityKey = identityKey

	// Generate device ID
	deviceID := make([]byte, 4)
	if _, err := rand.Read(deviceID); err != nil {
		return fmt.Errorf("failed to generate device ID: %w", err)
	}
	m.deviceID = uint32(deviceID[0])<<24 | uint32(deviceID[1])<<16 | uint32(deviceID[2])<<8 | uint32(deviceID[3])

	// Generate signed prekey
	signedPreKey, err := m.generateSignedPreKey(1)
	if err != nil {
		return fmt.Errorf("failed to generate signed prekey: %w", err)
	}
	m.signedPreKey = signedPreKey

	// Generate prekeys
	for i := uint32(1); i <= 100; i++ {
		preKey, err := generatePreKey(i)
		if err != nil {
			return fmt.Errorf("failed to generate prekey: %w", err)
		}
		m.preKeys[i] = preKey
	}

	return nil
}

// generateKeyPair generates a Curve25519 key pair
func generateKeyPair() (*KeyPair, error) {
	var private, public [32]byte

	if _, err := rand.Read(private[:]); err != nil {
		return nil, err
	}

	// Clamp private key
	private[0] &= 248
	private[31] &= 127
	private[31] |= 64

	curve25519.ScalarBaseMult(&public, &private)

	return &KeyPair{
		Private: private[:],
		Public:  public[:],
	}, nil
}

// generatePreKey generates a one-time prekey
func generatePreKey(id uint32) (*PreKey, error) {
	keyPair, err := generateKeyPair()
	if err != nil {
		return nil, err
	}

	return &PreKey{
		ID:         id,
		PrivateKey: keyPair.Private,
		PublicKey:  keyPair.Public,
	}, nil
}

// generateSignedPreKey generates a signed prekey
func (m *Manager) generateSignedPreKey(id uint32) (*SignedPreKey, error) {
	keyPair, err := generateKeyPair()
	if err != nil {
		return nil, err
	}

	// In a real implementation, this would be an XEdDSA signature
	signature := make([]byte, 64)
	if _, err := rand.Read(signature); err != nil {
		return nil, err
	}

	return &SignedPreKey{
		ID:         id,
		PrivateKey: keyPair.Private,
		PublicKey:  keyPair.Public,
		Signature:  signature,
	}, nil
}

// DeviceID returns the device ID
func (m *Manager) DeviceID() uint32 {
	return m.deviceID
}

// GetBundle returns the OMEMO bundle for publishing
func (m *Manager) GetBundle() *Bundle {
	m.mu.RLock()
	defer m.mu.RUnlock()

	preKeys := make([]PreKey, 0, len(m.preKeys))
	for _, pk := range m.preKeys {
		preKeys = append(preKeys, PreKey{
			ID:        pk.ID,
			PublicKey: pk.PublicKey,
		})
	}

	return &Bundle{
		DeviceID:        m.deviceID,
		IdentityKey:     m.identityKey.Public,
		SignedPreKey:    m.signedPreKey,
		SignedPreKeySig: m.signedPreKey.Signature,
		PreKeys:         preKeys,
	}
}

// GetFingerprint returns the fingerprint of our identity key
func (m *Manager) GetFingerprint() string {
	return formatFingerprint(m.identityKey.Public)
}

// GetContactFingerprints returns fingerprints for a contact
func (m *Manager) GetContactFingerprints(jid string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	devices := m.identities[jid]
	if devices == nil {
		return nil
	}

	var fingerprints []string
	for _, identity := range devices {
		fingerprints = append(fingerprints, formatFingerprint(identity.IdentityKey))
	}
	return fingerprints
}

// formatFingerprint formats a public key as a fingerprint
func formatFingerprint(publicKey []byte) string {
	encoded := base64.StdEncoding.EncodeToString(publicKey)
	// Format in groups of 8 for readability
	var formatted string
	for i, c := range encoded {
		if i > 0 && i%8 == 0 {
			formatted += " "
		}
		formatted += string(c)
	}
	return formatted
}

// ProcessBundle processes a received bundle and establishes a session
func (m *Manager) ProcessBundle(jid string, bundle *Bundle) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store identity
	if m.identities[jid] == nil {
		m.identities[jid] = make(map[uint32]*Identity)
	}

	trustLevel := TrustUndecided
	if m.trustOnFirst {
		trustLevel = TrustTrusted
	}

	m.identities[jid][bundle.DeviceID] = &Identity{
		DeviceID:    bundle.DeviceID,
		IdentityKey: bundle.IdentityKey,
		TrustLevel:  trustLevel,
	}

	// Save to store
	if m.store != nil {
		if err := m.store.SaveIdentity(jid, bundle.DeviceID, bundle.IdentityKey, trustLevel); err != nil {
			return err
		}
	}

	// Create session using X3DH
	// This is a simplified version - real implementation would do full X3DH
	session := &Session{
		RemoteJID:      jid,
		RemoteDeviceID: bundle.DeviceID,
		MessageKeys:    make(map[uint32][]byte),
	}

	if m.sessions[jid] == nil {
		m.sessions[jid] = make(map[uint32]*Session)
	}
	m.sessions[jid][bundle.DeviceID] = session

	return nil
}

// Encrypt encrypts a message for all devices of a recipient
func (m *Manager) Encrypt(jid, plaintext string) (*EncryptedMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := m.sessions[jid]
	if len(sessions) == 0 {
		return nil, errors.New("no sessions established with recipient")
	}

	// Generate message key and IV
	messageKey := make([]byte, 32)
	iv := make([]byte, 12)
	if _, err := rand.Read(messageKey); err != nil {
		return nil, err
	}
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}

	// Encrypt message with AES-GCM (simplified - real impl would use proper AES-GCM)
	payload := []byte(plaintext) // Placeholder - would be AES-GCM encrypted

	// Encrypt message key for each device
	keys := make(map[uint32][]byte)
	for deviceID, session := range sessions {
		// Check trust level
		if identity := m.identities[jid][deviceID]; identity != nil {
			if identity.TrustLevel == TrustUntrusted {
				continue
			}
		}

		// Encrypt key for this device (placeholder)
		encryptedKey := make([]byte, 32)
		copy(encryptedKey, messageKey)
		_ = session // Would use session for actual encryption
		keys[deviceID] = encryptedKey
	}

	if len(keys) == 0 {
		return nil, errors.New("no trusted devices to encrypt for")
	}

	return &EncryptedMessage{
		SenderDeviceID: m.deviceID,
		IV:             iv,
		Payload:        payload,
		Keys:           keys,
	}, nil
}

// Decrypt decrypts a received OMEMO message
func (m *Manager) Decrypt(jid string, msg *EncryptedMessage) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Find our encrypted key
	encryptedKey, ok := msg.Keys[m.deviceID]
	if !ok {
		return "", errors.New("message not encrypted for this device")
	}

	// Decrypt message key (placeholder)
	messageKey := make([]byte, 32)
	copy(messageKey, encryptedKey)

	// Decrypt payload with AES-GCM (placeholder)
	plaintext := string(msg.Payload)

	return plaintext, nil
}

// SetTrustLevel sets the trust level for a device
func (m *Manager) SetTrustLevel(jid string, deviceID uint32, trust TrustLevel) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.identities[jid] == nil {
		return errors.New("unknown JID")
	}

	identity := m.identities[jid][deviceID]
	if identity == nil {
		return errors.New("unknown device")
	}

	identity.TrustLevel = trust

	if m.store != nil {
		return m.store.SetTrustLevel(jid, deviceID, trust)
	}

	return nil
}

// GetTrustLevel returns the trust level for a device
func (m *Manager) GetTrustLevel(jid string, deviceID uint32) TrustLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.identities[jid] == nil {
		return TrustUndecided
	}

	identity := m.identities[jid][deviceID]
	if identity == nil {
		return TrustUndecided
	}

	return identity.TrustLevel
}

// HasSession returns whether a session exists with a device
func (m *Manager) HasSession(jid string, deviceID uint32) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.sessions[jid] == nil {
		return false
	}
	return m.sessions[jid][deviceID] != nil
}

// DeleteSession deletes a session with a device
func (m *Manager) DeleteSession(jid string, deviceID uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sessions[jid] != nil {
		delete(m.sessions[jid], deviceID)
	}

	if m.store != nil {
		return m.store.DeleteSession(jid, deviceID)
	}

	return nil
}
