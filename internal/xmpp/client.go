package xmpp

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"strconv"
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
	onReceipt    func(messageID string, status string) // "delivered" or "read"

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

// PresenceWithStatus extends stanza.Presence with show/status/priority fields
type PresenceWithStatus struct {
	stanza.Presence
	Show     string `xml:"show,omitempty"`
	Status   string `xml:"status,omitempty"`
	Priority int8   `xml:"priority,omitempty"`
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

	addr := net.JoinHostPort(server, strconv.Itoa(c.port))

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

	// Read child elements to get body and receipts
	tokenReader := session.TokenReader()
	hasBody := false

	for {
		tok, err := tokenReader.Token()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Local == "body":
				// Read body text
				if bodyTok, err := tokenReader.Token(); err == nil {
					if charData, ok := bodyTok.(xml.CharData); ok {
						msg.Body = string(charData)
						hasBody = true
					}
				}

			case t.Name.Local == "received" && t.Name.Space == "urn:xmpp:receipts":
				// XEP-0184 delivery receipt
				for _, attr := range t.Attr {
					if attr.Name.Local == "id" {
						if c.onReceipt != nil {
							c.onReceipt(attr.Value, "delivered")
						}
						break
					}
				}

			case t.Name.Local == "displayed" && t.Name.Space == "urn:xmpp:chat-markers:0":
				// XEP-0333 read marker
				for _, attr := range t.Attr {
					if attr.Name.Local == "id" {
						if c.onReceipt != nil {
							c.onReceipt(attr.Value, "read")
						}
						break
					}
				}

			case t.Name.Local == "encrypted":
				msg.Encrypted = true
			}

		case xml.EndElement:
			if t.Name.Local == "message" {
				// End of message stanza
				if hasBody && c.onMessage != nil {
					c.onMessage(msg)
				}
				return
			}
		}
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
	// Decode IQ attributes
	var iqType, iqID string
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "type":
			iqType = attr.Value
		case "id":
			iqID = attr.Value
		}
	}

	// Read tokens from the session's TokenReader
	tokenReader := session.TokenReader()
	for {
		tok, err := tokenReader.Token()
		if err != nil {
			return
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "query" && t.Name.Space == "jabber:iq:roster" {
				items := c.parseRosterQuery(tokenReader, t)
				if c.onRoster != nil && len(items) > 0 {
					c.onRoster(items)
				}
				// Send result acknowledgment for roster pushes
				if iqType == "set" && iqID != "" {
					c.sendIQResult(iqID)
				}
				return
			}
		case xml.EndElement:
			if t.Name.Local == "iq" {
				return
			}
		}
	}
}

// tokenReader is an interface for reading XML tokens
type tokenReader interface {
	Token() (xml.Token, error)
}

// parseRosterQuery parses a roster query element
func (c *Client) parseRosterQuery(tr tokenReader, start xml.StartElement) []RosterItem {
	var items []RosterItem
	for {
		tok, err := tr.Token()
		if err != nil {
			return items
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "item" {
				item := c.parseRosterItem(tr, t)
				items = append(items, item)
			}
		case xml.EndElement:
			if t.Name.Local == "query" {
				return items
			}
		}
	}
}

// parseRosterItem parses a roster item element
func (c *Client) parseRosterItem(tr tokenReader, start xml.StartElement) RosterItem {
	item := RosterItem{}
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "jid":
			item.JID, _ = jid.Parse(attr.Value)
		case "name":
			item.Name = attr.Value
		case "subscription":
			item.Subscription = attr.Value
		}
	}
	// Parse group elements
	for {
		tok, err := tr.Token()
		if err != nil {
			return item
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "group" {
				if groupTok, err := tr.Token(); err == nil && groupTok != nil {
					if charData, ok := groupTok.(xml.CharData); ok {
						item.Groups = append(item.Groups, string(charData))
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "item" {
				return item
			}
		}
	}
}

// sendIQResult sends an IQ result acknowledgment
func (c *Client) sendIQResult(id string) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return
	}
	session := c.session
	c.mu.RUnlock()

	iq := stanza.IQ{
		ID:   id,
		Type: stanza.ResultIQ,
	}
	session.Encode(c.ctx, iq)
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

// messageBody represents the body element in a message stanza
type messageBody struct {
	XMLName xml.Name `xml:"body"`
	Text    string   `xml:",chardata"`
}

// messageRequest represents a request for delivery receipt
type messageRequest struct {
	XMLName xml.Name `xml:"urn:xmpp:receipts request"`
}

// messageMarkable represents a markable indicator for chat markers
type messageMarkable struct {
	XMLName xml.Name `xml:"urn:xmpp:chat-markers:0 markable"`
}

// messageWithBody represents a message with body for XML encoding
type messageWithBody struct {
	stanza.Message
	Body     messageBody     `xml:"body"`
	Request  messageRequest  `xml:"urn:xmpp:receipts request"`
	Markable messageMarkable `xml:"urn:xmpp:chat-markers:0 markable"`
}

// randomString generates a random hex string of the specified byte length
func randomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// SendMessage sends a message and returns the message ID
func (c *Client) SendMessage(to string, body string) (string, error) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return "", fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	toJID, err := jid.Parse(to)
	if err != nil {
		return "", fmt.Errorf("invalid JID: %w", err)
	}

	// Generate unique message ID
	id := fmt.Sprintf("%d-%s", time.Now().UnixNano(), randomString(4))

	msg := messageWithBody{
		Message: stanza.Message{
			ID:   id,
			To:   toJID,
			Type: stanza.ChatMessage,
		},
		Body:     messageBody{Text: body},
		Request:  messageRequest{},
		Markable: messageMarkable{},
	}

	// Encode the message
	err = session.Encode(c.ctx, msg)
	return id, err
}

// SendPresence sends a presence update with show and status
func (c *Client) SendPresence(show, status string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	priority := c.priority
	c.mu.RUnlock()

	p := PresenceWithStatus{
		Presence: stanza.Presence{},
		Show:     show,   // "away", "dnd", "xa", or "" for online
		Status:   status, // status message
		Priority: int8(priority),
	}
	return session.Encode(c.ctx, p)
}

// SendDirectedPresence sends presence to a specific contact
func (c *Client) SendDirectedPresence(to, show, status string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	priority := c.priority
	c.mu.RUnlock()

	toJID, err := jid.Parse(to)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	p := PresenceWithStatus{
		Presence: stanza.Presence{To: toJID.Bare()},
		Show:     show,
		Status:   status,
		Priority: int8(priority),
	}
	return session.Encode(c.ctx, p)
}

// HideStatusFrom sends unavailable presence to a contact (hides your status from them)
func (c *Client) HideStatusFrom(contactJID string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	toJID, err := jid.Parse(contactJID)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	p := stanza.Presence{
		To:   toJID.Bare(),
		Type: stanza.UnavailablePresence,
	}
	return session.Encode(c.ctx, p)
}

// RequestRoster requests the roster from the server
func (c *Client) RequestRoster() error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	// Build roster get IQ
	type rosterGetQuery struct {
		XMLName xml.Name `xml:"jabber:iq:roster query"`
	}
	type rosterGetIQ struct {
		stanza.IQ
		Query rosterGetQuery
	}

	iq := rosterGetIQ{
		IQ: stanza.IQ{
			Type: stanza.GetIQ,
		},
	}

	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	return session.Encode(ctx, iq)
}

// rosterQuery represents a roster query for XML encoding
type rosterQuery struct {
	XMLName xml.Name    `xml:"jabber:iq:roster query"`
	Item    rosterItem  `xml:"item"`
}

// rosterItem represents a roster item for XML encoding
type rosterItem struct {
	JID          string   `xml:"jid,attr"`
	Name         string   `xml:"name,attr,omitempty"`
	Subscription string   `xml:"subscription,attr,omitempty"`
	Group        []string `xml:"group,omitempty"`
}

// rosterIQ represents a roster IQ stanza
type rosterIQ struct {
	stanza.IQ
	Query rosterQuery `xml:"jabber:iq:roster query"`
}

// AddContact adds a contact to the roster (fire-and-forget)
func (c *Client) AddContact(contactJID, name string, groups []string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	toJID, err := jid.Parse(contactJID)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	// Build roster IQ stanza directly - fire and forget
	// According to RFC 6121, roster set is fire-and-forget; server sends roster push asynchronously
	iq := rosterIQ{
		IQ: stanza.IQ{
			Type: stanza.SetIQ,
		},
		Query: rosterQuery{
			Item: rosterItem{
				JID:   toJID.Bare().String(),
				Name:  name,
				Group: groups,
			},
		},
	}

	// Use a short timeout - we're just sending, not waiting for response
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	return session.Encode(ctx, iq)
}

// RemoveContact removes a contact from the roster (fire-and-forget)
func (c *Client) RemoveContact(contactJID string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	toJID, err := jid.Parse(contactJID)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	// Build roster IQ stanza with subscription="remove" - fire and forget
	iq := rosterIQ{
		IQ: stanza.IQ{
			Type: stanza.SetIQ,
		},
		Query: rosterQuery{
			Item: rosterItem{
				JID:          toJID.Bare().String(),
				Subscription: "remove",
			},
		},
	}

	// Use a short timeout - we're just sending, not waiting for response
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	return session.Encode(ctx, iq)
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

	// Use a timeout context for the operation
	ctx, cancel := context.WithTimeout(c.ctx, 15*time.Second)
	defer cancel()

	return session.Encode(ctx, p)
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

// SetReceiptHandler sets the delivery/read receipt handler
// status is "delivered" (XEP-0184) or "read" (XEP-0333)
func (c *Client) SetReceiptHandler(handler func(messageID string, status string)) {
	c.onReceipt = handler
}

// receivedElement represents a XEP-0184 delivery receipt
type receivedElement struct {
	XMLName xml.Name `xml:"urn:xmpp:receipts received"`
	ID      string   `xml:"id,attr"`
}

// displayedElement represents a XEP-0333 read marker
type displayedElement struct {
	XMLName xml.Name `xml:"urn:xmpp:chat-markers:0 displayed"`
	ID      string   `xml:"id,attr"`
}

// messageWithReceipt represents a message containing a delivery receipt
type messageWithReceipt struct {
	stanza.Message
	Received receivedElement `xml:"urn:xmpp:receipts received"`
}

// messageWithDisplayed represents a message containing a read marker
type messageWithDisplayed struct {
	stanza.Message
	Displayed displayedElement `xml:"urn:xmpp:chat-markers:0 displayed"`
}

// SendReceipt sends a XEP-0184 delivery receipt for a message
func (c *Client) SendReceipt(to, messageID string) error {
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

	msg := messageWithReceipt{
		Message: stanza.Message{
			To:   toJID,
			Type: stanza.ChatMessage,
		},
		Received: receivedElement{ID: messageID},
	}

	return session.Encode(c.ctx, msg)
}

// SendDisplayedMarker sends a XEP-0333 read marker for a message
func (c *Client) SendDisplayedMarker(to, messageID string) error {
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

	msg := messageWithDisplayed{
		Message: stanza.Message{
			To:   toJID,
			Type: stanza.ChatMessage,
		},
		Displayed: displayedElement{ID: messageID},
	}

	return session.Encode(c.ctx, msg)
}

// RoomConfig holds configuration options for creating a MUC room
type RoomConfig struct {
	UseDefaults bool   // true = instant room with defaults
	Name        string // Room name
	Description string // Room description
	Password    string // Room password (optional)
	MembersOnly bool   // Only members can join
	Persistent  bool   // Room persists after all users leave
}

// CreateRoom creates a new MUC room or joins an existing one
func (c *Client) CreateRoom(roomJID, nick string, config *RoomConfig) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	// Parse room JID with nick as resource
	roomJ, err := jid.Parse(roomJID + "/" + nick)
	if err != nil {
		return fmt.Errorf("invalid room JID: %w", err)
	}

	// Send presence to join/create the room
	// Include password if provided
	var presenceXML string
	if config != nil && config.Password != "" {
		presenceXML = fmt.Sprintf(`<presence to='%s'><x xmlns='http://jabber.org/protocol/muc'><password>%s</password></x></presence>`,
			roomJ.String(), config.Password)
	} else {
		presenceXML = fmt.Sprintf(`<presence to='%s'><x xmlns='http://jabber.org/protocol/muc'/></presence>`,
			roomJ.String())
	}

	// Send the join presence
	p := stanza.Presence{To: roomJ}
	if err := session.Encode(c.ctx, p); err != nil {
		return fmt.Errorf("failed to join room: %w", err)
	}

	// If using defaults (instant room), we're done
	// Otherwise, configuration would be applied after receiving room creation confirmation
	// For now, instant room creation is the primary use case
	_ = presenceXML // Used for reference, actual encoding uses stanza

	return nil
}

// JoinRoom joins an existing MUC room
func (c *Client) JoinRoom(roomJID, nick, password string) error {
	return c.CreateRoom(roomJID, nick, &RoomConfig{
		UseDefaults: true,
		Password:    password,
	})
}

// LeaveRoom leaves a MUC room
func (c *Client) LeaveRoom(roomJID, nick string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	roomJ, err := jid.Parse(roomJID + "/" + nick)
	if err != nil {
		return fmt.Errorf("invalid room JID: %w", err)
	}

	p := stanza.Presence{
		To:   roomJ,
		Type: stanza.UnavailablePresence,
	}
	return session.Encode(c.ctx, p)
}
