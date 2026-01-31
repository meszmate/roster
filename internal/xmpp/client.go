package xmpp

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"mellium.im/sasl"
	"mellium.im/xmpp"
	"mellium.im/xmpp/jid"
	"mellium.im/xmpp/stanza"
)

// Client wraps the Mellium XMPP client
type Client struct {
	session    *xmpp.Session
	jid        jid.JID
	password   string
	server     string
	port       int
	resource   string
	priority   int
	connected  bool
	mu         sync.RWMutex

	// Handlers
	onMessage    func(msg Message)
	onPresence   func(p Presence)
	onRoster     func(items []RosterItem)
	onConnect    func()
	onDisconnect func(err error)
	onError      func(err error)

	ctx    context.Context
	cancel context.CancelFunc
}

// Message represents an XMPP message
type Message struct {
	ID        string
	From      jid.JID
	To        jid.JID
	Body      string
	Type      stanza.MessageType
	Timestamp time.Time
	Thread    string
	Encrypted bool
}

// Presence represents an XMPP presence
type Presence struct {
	From     jid.JID
	To       jid.JID
	Type     stanza.PresenceType
	Show     string
	Status   string
	Priority int
}

// RosterItem represents a roster entry
type RosterItem struct {
	JID          jid.JID
	Name         string
	Subscription string
	Groups       []string
}

// ClientConfig contains configuration for the XMPP client
type ClientConfig struct {
	JID      string
	Password string
	Server   string
	Port     int
	Resource string
	Priority int
}

// NewClient creates a new XMPP client
func NewClient(cfg ClientConfig) (*Client, error) {
	j, err := jid.Parse(cfg.JID)
	if err != nil {
		return nil, fmt.Errorf("invalid JID: %w", err)
	}

	if cfg.Resource != "" {
		j, err = j.WithResource(cfg.Resource)
		if err != nil {
			return nil, fmt.Errorf("invalid resource: %w", err)
		}
	}

	if cfg.Port == 0 {
		cfg.Port = 5222
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Client{
		jid:      j,
		password: cfg.Password,
		server:   cfg.Server,
		port:     cfg.Port,
		resource: cfg.Resource,
		priority: cfg.Priority,
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// Connect establishes a connection to the XMPP server
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	server := c.server
	if server == "" {
		server = c.jid.Domain().String()
	}

	addr := fmt.Sprintf("%s:%d", server, c.port)

	// Dial TCP connection
	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return fmt.Errorf("failed to dial server: %w", err)
	}

	// Create TLS config
	tlsConfig := &tls.Config{
		ServerName: c.jid.Domain().String(),
		MinVersion: tls.VersionTLS12,
	}

	// Create negotiator with SASL and resource binding
	negotiator := xmpp.NewNegotiator(func(_ *xmpp.Session, _ *xmpp.StreamConfig) xmpp.StreamConfig {
		return xmpp.StreamConfig{
			Features: []xmpp.StreamFeature{
				xmpp.StartTLS(tlsConfig),
				xmpp.SASL("", c.password, sasl.ScramSha256Plus, sasl.ScramSha256, sasl.ScramSha1Plus, sasl.ScramSha1, sasl.Plain),
				xmpp.BindResource(),
			},
		}
	})

	// Negotiate XMPP session
	session, err := xmpp.NewSession(
		c.ctx,
		c.jid.Domain(),
		c.jid,
		conn,
		0,
		negotiator,
	)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to negotiate session: %w", err)
	}

	c.session = session
	c.connected = true

	// Update JID with resource from server
	c.jid = session.LocalAddr()

	// Start message handler
	go c.handleStanzas()

	if c.onConnect != nil {
		c.onConnect()
	}

	return nil
}

// Disconnect closes the XMPP connection
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.cancel()

	if c.session != nil {
		// Send unavailable presence
		_ = c.session.Encode(c.ctx, stanza.Presence{Type: stanza.UnavailablePresence})
		_ = c.session.Close()
	}

	c.connected = false
	c.session = nil

	if c.onDisconnect != nil {
		c.onDisconnect(nil)
	}

	return nil
}

// handleStanzas processes incoming stanzas
func (c *Client) handleStanzas() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.mu.RLock()
		session := c.session
		c.mu.RUnlock()

		if session == nil {
			return
		}

		// Read next token from the session
		tok, err := session.TokenReader().Token()
		if err != nil {
			if err == io.EOF {
				c.handleDisconnect(nil)
				return
			}
			if c.onError != nil {
				c.onError(err)
			}
			c.handleDisconnect(err)
			return
		}

		// Process start elements
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		// Handle different stanza types
		switch start.Name.Local {
		case "message":
			c.handleMessage(session, start)
		case "presence":
			c.handlePresenceStanza(session, start)
		case "iq":
			c.handleIQ(session, start)
		}
	}
}

// handleMessage processes a message stanza
func (c *Client) handleMessage(session *xmpp.Session, start xml.StartElement) {
	// Skip the message content for now
	// In a full implementation, we would decode the message body
	if c.onMessage != nil {
		msg := Message{
			Timestamp: time.Now(),
		}
		// Parse attributes
		for _, attr := range start.Attr {
			switch attr.Name.Local {
			case "from":
				msg.From, _ = jid.Parse(attr.Value)
			case "to":
				msg.To, _ = jid.Parse(attr.Value)
			case "id":
				msg.ID = attr.Value
			case "type":
				msg.Type = stanza.MessageType(attr.Value)
			}
		}
		c.onMessage(msg)
	}
}

// handlePresenceStanza processes a presence stanza
func (c *Client) handlePresenceStanza(session *xmpp.Session, start xml.StartElement) {
	if c.onPresence != nil {
		p := Presence{}
		for _, attr := range start.Attr {
			switch attr.Name.Local {
			case "from":
				p.From, _ = jid.Parse(attr.Value)
			case "to":
				p.To, _ = jid.Parse(attr.Value)
			case "type":
				p.Type = stanza.PresenceType(attr.Value)
			}
		}
		c.onPresence(p)
	}
}

// handleIQ processes an IQ stanza
func (c *Client) handleIQ(session *xmpp.Session, start xml.StartElement) {
	// IQ handling would go here
}

// handleDisconnect handles unexpected disconnection
func (c *Client) handleDisconnect(err error) {
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()

	if c.onDisconnect != nil {
		c.onDisconnect(err)
	}
}

// SendMessage sends a message
func (c *Client) SendMessage(to string, body string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	toJID, err := jid.Parse(to)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	msg := stanza.Message{
		To:   toJID,
		Type: stanza.ChatMessage,
	}

	// Encode the message
	return session.Encode(c.ctx, msg)
}

// SendPresence sends a presence update
func (c *Client) SendPresence(show, status string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	p := stanza.Presence{}
	return session.Encode(c.ctx, p)
}

// RequestRoster requests the roster from the server
func (c *Client) RequestRoster() error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	// Roster request would be implemented here
	// Using mellium.im/xmpp/roster package
	return nil
}

// AddContact adds a contact to the roster
func (c *Client) AddContact(contactJID, name string, groups []string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	// Implementation would use roster package
	return nil
}

// RemoveContact removes a contact from the roster
func (c *Client) RemoveContact(contactJID string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	// Implementation would use roster package
	return nil
}

// Subscribe sends a subscription request
func (c *Client) Subscribe(contactJID string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	to, err := jid.Parse(contactJID)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	p := stanza.Presence{
		To:   to,
		Type: stanza.SubscribePresence,
	}

	return session.Encode(c.ctx, p)
}

// Unsubscribe sends an unsubscribe request
func (c *Client) Unsubscribe(contactJID string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	to, err := jid.Parse(contactJID)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	p := stanza.Presence{
		To:   to,
		Type: stanza.UnsubscribePresence,
	}

	return session.Encode(c.ctx, p)
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// JID returns the client's JID
func (c *Client) JID() jid.JID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.jid
}

// SetMessageHandler sets the message handler
func (c *Client) SetMessageHandler(handler func(msg Message)) {
	c.onMessage = handler
}

// SetPresenceHandler sets the presence handler
func (c *Client) SetPresenceHandler(handler func(p Presence)) {
	c.onPresence = handler
}

// SetRosterHandler sets the roster handler
func (c *Client) SetRosterHandler(handler func(items []RosterItem)) {
	c.onRoster = handler
}

// SetConnectHandler sets the connect handler
func (c *Client) SetConnectHandler(handler func()) {
	c.onConnect = handler
}

// SetDisconnectHandler sets the disconnect handler
func (c *Client) SetDisconnectHandler(handler func(err error)) {
	c.onDisconnect = handler
}

// SetErrorHandler sets the error handler
func (c *Client) SetErrorHandler(handler func(err error)) {
	c.onError = handler
}
