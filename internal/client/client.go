package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	xmp "github.com/meszmate/xmpp-go"
	"github.com/meszmate/xmpp-go/dial"
	"github.com/meszmate/xmpp-go/jid"
	"github.com/meszmate/xmpp-go/plugin"
	"github.com/meszmate/xmpp-go/plugins/bookmarks"
	"github.com/meszmate/xmpp-go/plugins/caps"
	"github.com/meszmate/xmpp-go/plugins/carbons"
	"github.com/meszmate/xmpp-go/plugins/chatmarkers"
	"github.com/meszmate/xmpp-go/plugins/chatstates"
	"github.com/meszmate/xmpp-go/plugins/correction"
	"github.com/meszmate/xmpp-go/plugins/disco"
	"github.com/meszmate/xmpp-go/plugins/form"
	forwardplugin "github.com/meszmate/xmpp-go/plugins/forward"
	mamplugin "github.com/meszmate/xmpp-go/plugins/mam"
	"github.com/meszmate/xmpp-go/plugins/muc"
	omemoplugin "github.com/meszmate/xmpp-go/plugins/omemo"
	"github.com/meszmate/xmpp-go/plugins/ping"
	"github.com/meszmate/xmpp-go/plugins/presence"
	"github.com/meszmate/xmpp-go/plugins/reactions"
	"github.com/meszmate/xmpp-go/plugins/receipts"
	"github.com/meszmate/xmpp-go/plugins/roster"
	"github.com/meszmate/xmpp-go/plugins/upload"
	"github.com/meszmate/xmpp-go/stanza"
	"github.com/meszmate/xmpp-go/storage"
	"github.com/meszmate/xmpp-go/storage/memory"
	"github.com/meszmate/xmpp-go/stream"
	"github.com/meszmate/xmpp-go/transport"

	cryptoomemo "github.com/meszmate/xmpp-go/crypto/omemo"
)

type Client struct {
	mu        sync.RWMutex
	client    *xmp.Client
	session   *xmp.Session
	jid       jid.JID
	password  string
	server    string
	port      int
	resource  string
	connected bool

	plugins      *plugin.Manager
	omemoManager *cryptoomemo.Manager
	omemoStore   *OMEMOStore
	deviceID     uint32

	onMessage    func(msg Message)
	onPresence   func(p Presence)
	onRoster     func(items []RosterItem)
	onConnect    func()
	onDisconnect func(err error)
	onError      func(err error)
	onReceipt    func(messageID string, status string)

	pendingIQs map[string]chan *stanza.IQ

	ctx    context.Context
	cancel context.CancelFunc
}

const (
	nsStream = "http://etherx.jabber.org/streams"
	nsTLS    = "urn:ietf:params:xml:ns:xmpp-tls"
	nsSASL   = "urn:ietf:params:xml:ns:xmpp-sasl"
)

type streamFeatures struct {
	XMLName    xml.Name  `xml:"http://etherx.jabber.org/streams features"`
	StartTLS   *struct{} `xml:"urn:ietf:params:xml:ns:xmpp-tls starttls"`
	Mechanisms []string  `xml:"urn:ietf:params:xml:ns:xmpp-sasl mechanisms>mechanism"`
	Bind       *struct{} `xml:"urn:ietf:params:xml:ns:xmpp-bind bind"`
}

type startTLSRequest struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-tls starttls"`
}

type saslAuth struct {
	XMLName   xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-sasl auth"`
	Mechanism string   `xml:"mechanism,attr"`
	Value     string   `xml:",chardata"`
}

type saslFailure struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-sasl failure"`
	Text    string   `xml:"text"`
}

func sanitizeEmptyJIDAttrs(start *xml.StartElement) {
	if start == nil || len(start.Attr) == 0 {
		return
	}

	filtered := start.Attr[:0]
	for _, attr := range start.Attr {
		if (attr.Name.Local == "from" || attr.Name.Local == "to") && strings.TrimSpace(attr.Value) == "" {
			continue
		}
		filtered = append(filtered, attr)
	}
	start.Attr = filtered
}

func parseIQErrorDetails(resp *stanza.IQ) string {
	if resp == nil {
		return "unknown iq error"
	}

	var parts []string
	if resp.Error != nil && resp.Error.Type != "" {
		parts = append(parts, "type="+resp.Error.Type)
	}

	raw := strings.ToLower(string(resp.Query))
	conditions := []string{
		"not-authorized",
		"service-unavailable",
		"feature-not-implemented",
		"resource-constraint",
		"conflict",
		"bad-request",
		"not-acceptable",
		"jid-malformed",
	}
	for _, cond := range conditions {
		if strings.Contains(raw, cond) {
			parts = append(parts, "condition="+cond)
			break
		}
	}

	if len(parts) == 0 {
		return "unknown iq error"
	}
	return strings.Join(parts, ", ")
}

func (c *Client) getRosterPlugin() (*roster.Plugin, error) {
	if c.plugins == nil {
		return nil, fmt.Errorf("plugin manager not initialized")
	}

	p, ok := c.plugins.Get(roster.Name)
	if !ok || p == nil {
		return nil, fmt.Errorf("roster plugin not available")
	}

	rp, ok := p.(*roster.Plugin)
	if !ok || rp == nil {
		return nil, fmt.Errorf("roster plugin has unexpected type %T", p)
	}

	return rp, nil
}

func (c *Client) getOMEMOPlugin() (*omemoplugin.Plugin, error) {
	if c.plugins == nil {
		return nil, fmt.Errorf("plugin manager not initialized")
	}

	p, ok := c.plugins.Get(omemoplugin.Name)
	if !ok || p == nil {
		return nil, fmt.Errorf("omemo plugin not available")
	}

	op, ok := p.(*omemoplugin.Plugin)
	if !ok || op == nil {
		return nil, fmt.Errorf("omemo plugin has unexpected type %T", p)
	}

	return op, nil
}

func (c *Client) getMUCPlugin() (*muc.Plugin, error) {
	if c.plugins == nil {
		return nil, fmt.Errorf("plugin manager not initialized")
	}

	p, ok := c.plugins.Get(muc.Name)
	if !ok || p == nil {
		return nil, fmt.Errorf("muc plugin not available")
	}

	mp, ok := p.(*muc.Plugin)
	if !ok || mp == nil {
		return nil, fmt.Errorf("muc plugin has unexpected type %T", p)
	}

	return mp, nil
}

func (c *Client) getBookmarksPlugin() (*bookmarks.Plugin, error) {
	if c.plugins == nil {
		return nil, fmt.Errorf("plugin manager not initialized")
	}

	p, ok := c.plugins.Get(bookmarks.Name)
	if !ok || p == nil {
		return nil, fmt.Errorf("bookmarks plugin not available")
	}

	bp, ok := p.(*bookmarks.Plugin)
	if !ok || bp == nil {
		return nil, fmt.Errorf("bookmarks plugin has unexpected type %T", p)
	}

	return bp, nil
}

func (c *Client) getCarbonsPlugin() (*carbons.Plugin, error) {
	if c.plugins == nil {
		return nil, fmt.Errorf("plugin manager not initialized")
	}

	p, ok := c.plugins.Get(carbons.Name)
	if !ok || p == nil {
		return nil, fmt.Errorf("carbons plugin not available")
	}

	cp, ok := p.(*carbons.Plugin)
	if !ok || cp == nil {
		return nil, fmt.Errorf("carbons plugin has unexpected type %T", p)
	}

	return cp, nil
}

type Message struct {
	ID               string
	From             jid.JID
	To               jid.JID
	Body             string
	Type             string
	Timestamp        time.Time
	Thread           string
	Encrypted        bool
	ReceiptRequested bool
	CorrectedID      string
	Reactions        map[string][]string
}

type Presence struct {
	From     jid.JID
	To       jid.JID
	Type     string
	Show     string
	Status   string
	Priority int
}

type RosterItem struct {
	JID          jid.JID
	Name         string
	Subscription string
	Groups       []string
}

type ClientConfig struct {
	JID      string
	Password string
	Server   string
	Port     int
	Resource string
	Priority int
	DeviceID uint32
	DataDir  string
}

func NewClient(cfg ClientConfig) (*Client, error) {
	parsedJID, err := jid.Parse(cfg.JID)
	if err != nil {
		return nil, fmt.Errorf("invalid JID: %w", err)
	}

	resource := strings.TrimSpace(cfg.Resource)
	// "roster" is our historic default value; prefer server-assigned resources
	// unless the user explicitly configures another one.
	if strings.EqualFold(resource, "roster") {
		resource = ""
	}
	if resource != "" {
		parsedJID = parsedJID.WithResource(resource)
	}

	if cfg.Port == 0 {
		cfg.Port = 5222
	}

	deviceID := cfg.DeviceID
	if deviceID == 0 {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		deviceID = uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Client{
		jid:        parsedJID,
		password:   cfg.Password,
		server:     cfg.Server,
		port:       cfg.Port,
		resource:   resource,
		deviceID:   deviceID,
		pendingIQs: make(map[string]chan *stanza.IQ),
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	dialer := dial.NewDialer()
	dialer.TLSConfig = &tls.Config{
		ServerName: c.jid.Domain(),
		MinVersion: tls.VersionTLS12,
	}

	var trans *transport.TCP
	server := strings.TrimSpace(c.server)
	// Use direct host/port when explicitly configured; otherwise use SRV lookup.
	if server != "" || c.port != 5222 {
		host := c.jid.Domain()
		if server != "" {
			host = server
		}
		port := c.port
		if port == 0 {
			port = 5222
		}
		addr := net.JoinHostPort(host, strconv.Itoa(port))
		conn, err := (&net.Dialer{Timeout: 30 * time.Second}).DialContext(c.ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to dial server %s: %w", addr, err)
		}
		trans = transport.NewTCP(conn)
	} else {
		var err error
		trans, err = dialer.Dial(c.ctx, c.jid.Domain())
		if err != nil {
			return fmt.Errorf("failed to dial server: %w", err)
		}
	}

	c.omemoStore = NewOMEMOStore(c.jid.String(), c.deviceID)
	c.omemoManager = cryptoomemo.NewManager(c.omemoStore)

	if _, err := c.omemoManager.GenerateBundle(100); err != nil {
		trans.Close()
		return fmt.Errorf("failed to generate OMEMO bundle: %w", err)
	}

	c.plugins = plugin.NewManager()

	plugins := []plugin.Plugin{
		disco.New(),
		roster.New(),
		muc.New(),
		bookmarks.New(),
		carbons.New(),
		receipts.New(),
		chatstates.New(),
		correction.New(),
		reactions.New(),
		upload.New(),
		caps.New("https://github.com/meszmate/roster"),
		mamplugin.New(),
		ping.New(),
		presence.New(),
		omemoplugin.New(c.deviceID),
	}

	for _, p := range plugins {
		if err := c.plugins.Register(p); err != nil {
			trans.Close()
			return fmt.Errorf("failed to register plugin %s: %w", p.Name(), err)
		}
	}

	sessionOpts := []xmp.SessionOption{
		xmp.WithLocalAddr(c.jid),
	}

	session, err := xmp.NewSession(c.ctx, trans, sessionOpts...)
	if err != nil {
		trans.Close()
		return fmt.Errorf("failed to create session: %w", err)
	}

	c.session = session

	if err := c.negotiateClientSession(trans); err != nil {
		session.Close()
		return fmt.Errorf("xmpp negotiation failed: %w", err)
	}

	params := plugin.InitParams{
		SendRaw: func(ctx context.Context, data []byte) error {
			return c.session.SendRaw(ctx, bytes.NewReader(data))
		},
		SendElement: c.session.SendElement,
		State:       func() uint32 { return uint32(c.session.State()) },
		LocalJID:    func() string { return c.session.LocalAddr().String() },
		RemoteJID:   func() string { return c.session.RemoteAddr().String() },
		Get:         c.plugins.Get,
		Storage:     memory.New(),
	}

	if err := c.plugins.Initialize(c.ctx, params); err != nil {
		session.Close()
		return fmt.Errorf("failed to initialize plugins: %w", err)
	}

	c.client = nil
	c.connected = true

	go c.serve()
	go func() {
		_ = c.EnableCarbons()
	}()

	if c.onConnect != nil {
		c.onConnect()
	}

	return nil
}

func (c *Client) serve() {
	for {
		tok, err := c.session.Reader().Token()
		if err != nil {
			c.handleDisconnect(err)
			return
		}

		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		switch start.Name.Local {
		case "message":
			sanitizeEmptyJIDAttrs(&start)
			var msg stanza.Message
			if err := c.session.Reader().DecodeElement(&msg, &start); err != nil {
				c.handleDisconnect(err)
				return
			}
			c.handleMessage(&msg)

		case "presence":
			sanitizeEmptyJIDAttrs(&start)
			var p stanza.Presence
			if err := c.session.Reader().DecodeElement(&p, &start); err != nil {
				c.handleDisconnect(err)
				return
			}
			c.handlePresence(&p)

		case "iq":
			sanitizeEmptyJIDAttrs(&start)
			var iq stanza.IQ
			if err := c.session.Reader().DecodeElement(&iq, &start); err != nil {
				c.handleDisconnect(err)
				return
			}
			c.handleIQ(&iq)

		default:
			// Ignore stream-level elements (stream root/features/proceed/success/etc).
			if start.Name.Space == nsStream {
				if start.Name.Local == "stream" {
					continue
				}
				if err := c.session.Reader().Skip(); err != nil {
					c.handleDisconnect(err)
					return
				}
				continue
			}
			if err := c.session.Reader().Skip(); err != nil {
				c.handleDisconnect(err)
				return
			}
		}
	}
}

func (c *Client) negotiateClientSession(trans transport.Transport) error {
	if err := c.openStream(); err != nil {
		return err
	}

	features, err := c.readStreamFeatures()
	if err != nil {
		return err
	}

	if features.StartTLS != nil {
		if err := c.startTLS(trans); err != nil {
			return err
		}

		if err := c.openStream(); err != nil {
			return err
		}
		features, err = c.readStreamFeatures()
		if err != nil {
			return err
		}
	}

	if err := c.authenticatePlain(features); err != nil {
		return err
	}

	if err := c.openStream(); err != nil {
		return err
	}
	features, err = c.readStreamFeatures()
	if err != nil {
		return err
	}

	if features.Bind == nil {
		return fmt.Errorf("server did not offer resource binding")
	}

	return c.bindResource()
}

func (c *Client) openStream() error {
	to, err := jid.New("", c.jid.Domain(), "")
	if err != nil {
		return fmt.Errorf("invalid domain for stream: %w", err)
	}

	header := stream.Open(stream.Header{
		To: to,
		NS: "jabber:client",
	})

	if _, err := c.session.Writer().WriteRaw(header); err != nil {
		return fmt.Errorf("failed to send stream header: %w", err)
	}
	return nil
}

func (c *Client) readStreamFeatures() (*streamFeatures, error) {
	for {
		tok, err := c.session.Reader().Token()
		if err != nil {
			return nil, err
		}

		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		if start.Name.Space == nsStream && start.Name.Local == "stream" {
			continue
		}

		if start.Name.Space == nsStream && start.Name.Local == "features" {
			var features streamFeatures
			if err := c.session.Reader().DecodeElement(&features, &start); err != nil {
				return nil, err
			}
			return &features, nil
		}

		if err := c.session.Reader().Skip(); err != nil {
			return nil, err
		}
	}
}

func (c *Client) startTLS(trans transport.Transport) error {
	if err := c.session.SendElement(c.ctx, startTLSRequest{}); err != nil {
		return fmt.Errorf("failed to request STARTTLS: %w", err)
	}

	for {
		tok, err := c.session.Reader().Token()
		if err != nil {
			return err
		}

		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		if start.Name.Space != nsTLS {
			if err := c.session.Reader().Skip(); err != nil {
				return err
			}
			continue
		}

		switch start.Name.Local {
		case "proceed":
			if err := c.session.Reader().Skip(); err != nil {
				return err
			}
			tlsConfig := &tls.Config{
				ServerName: c.jid.Domain(),
				MinVersion: tls.VersionTLS12,
			}
			if err := trans.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("starttls handshake failed: %w", err)
			}
			return nil
		case "failure":
			_ = c.session.Reader().Skip()
			return fmt.Errorf("server rejected STARTTLS")
		default:
			if err := c.session.Reader().Skip(); err != nil {
				return err
			}
		}
	}
}

func (c *Client) authenticatePlain(features *streamFeatures) error {
	hasPlain := false
	for _, mech := range features.Mechanisms {
		if mech == "PLAIN" {
			hasPlain = true
			break
		}
	}
	if !hasPlain {
		return fmt.Errorf("server does not offer SASL PLAIN")
	}

	authcid := c.jid.Local()
	payload := "\x00" + authcid + "\x00" + c.password
	value := base64.StdEncoding.EncodeToString([]byte(payload))

	if err := c.session.SendElement(c.ctx, saslAuth{
		Mechanism: "PLAIN",
		Value:     value,
	}); err != nil {
		return fmt.Errorf("failed to send SASL auth: %w", err)
	}

	for {
		tok, err := c.session.Reader().Token()
		if err != nil {
			return err
		}

		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		if start.Name.Space != nsSASL {
			if err := c.session.Reader().Skip(); err != nil {
				return err
			}
			continue
		}

		switch start.Name.Local {
		case "success":
			if err := c.session.Reader().Skip(); err != nil {
				return err
			}
			return nil
		case "failure":
			var fail saslFailure
			if err := c.session.Reader().DecodeElement(&fail, &start); err != nil {
				return err
			}
			if fail.Text != "" {
				return fmt.Errorf("sasl authentication failed: %s", fail.Text)
			}
			return fmt.Errorf("sasl authentication failed")
		default:
			if err := c.session.Reader().Skip(); err != nil {
				return err
			}
		}
	}
}

func (c *Client) bindResource() error {
	tryBind := func(resource string) error {
		iq := stanza.NewIQ(stanza.IQSet)
		queryXML, err := xml.Marshal(xmp.BindRequest{Resource: resource})
		if err != nil {
			return err
		}
		iq.Query = queryXML

		resp, err := c.sendIQAndWaitDirect(iq, 12*time.Second)
		if err != nil {
			return err
		}
		if resp.Type != stanza.IQResult {
			return fmt.Errorf("bind iq failed: %s", parseIQErrorDetails(resp))
		}

		if len(resp.Query) > 0 {
			var bindRes xmp.BindResult
			if err := xml.Unmarshal(resp.Query, &bindRes); err == nil && bindRes.JID != "" {
				if parsed, err := jid.Parse(bindRes.JID); err == nil {
					c.jid = parsed
					c.session.SetLocalAddr(parsed)
				}
			}
		}

		return nil
	}

	resource := strings.TrimSpace(c.resource)

	// Try server-assigned resource first; some servers reject fixed/default resources.
	attempts := make([]string, 0, 2)
	attempts = append(attempts, "")
	if resource != "" {
		attempts = append(attempts, resource)
	}

	var errs []string
	seen := make(map[string]struct{}, len(attempts))
	for _, candidate := range attempts {
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}

		if err := tryBind(candidate); err == nil {
			return nil
		} else {
			label := "server-assigned"
			if candidate != "" {
				label = fmt.Sprintf("resource %q", candidate)
			}
			errs = append(errs, fmt.Sprintf("%s: %v", label, err))
		}
	}

	return fmt.Errorf("bind failed (%s)", strings.Join(errs, "; "))
}

func (c *Client) sendIQAndWaitDirect(iq *stanza.IQ, timeout time.Duration) (*stanza.IQ, error) {
	if err := c.session.Send(c.ctx, iq); err != nil {
		return nil, err
	}

	if connGetter, ok := c.session.Transport().(interface{ Conn() net.Conn }); ok {
		conn := connGetter.Conn()
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		defer func() {
			_ = conn.SetReadDeadline(time.Time{})
		}()
	}

	for {
		tok, err := c.session.Reader().Token()
		if err != nil {
			return nil, err
		}

		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		switch start.Name.Local {
		case "iq":
			sanitizeEmptyJIDAttrs(&start)
			var resp stanza.IQ
			if err := c.session.Reader().DecodeElement(&resp, &start); err != nil {
				return nil, err
			}
			if resp.ID == iq.ID {
				return &resp, nil
			}
		case "message", "presence":
			if err := c.session.Reader().Skip(); err != nil {
				return nil, err
			}
		default:
			if start.Name.Space == nsStream && start.Name.Local == "stream" {
				continue
			}
			if err := c.session.Reader().Skip(); err != nil {
				return nil, err
			}
		}
	}
}

func (c *Client) handleStanza(ctx context.Context, session *xmp.Session, st stanza.Stanza) error {
	switch s := st.(type) {
	case *stanza.Message:
		c.handleMessage(s)
	case *stanza.Presence:
		c.handlePresence(s)
	case *stanza.IQ:
		c.handleIQ(s)
	}
	return nil
}

func extensionOuterXML(ext stanza.Extension) ([]byte, error) {
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)

	start := xml.StartElement{
		Name: ext.XMLName,
		Attr: ext.Attrs,
	}
	if err := enc.EncodeToken(start); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	if len(ext.Inner) > 0 {
		if _, err := buf.Write(ext.Inner); err != nil {
			return nil, err
		}
	}
	if err := enc.EncodeToken(start.End()); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func parseForwardedMessage(raw []byte) (*stanza.Message, error) {
	var forwarded struct {
		XMLName xml.Name             `xml:"urn:xmpp:forward:0 forwarded"`
		Delay   *forwardplugin.Delay `xml:"urn:xmpp:delay delay,omitempty"`
		Message *stanza.Message      `xml:"message"`
	}

	if err := xml.Unmarshal(raw, &forwarded); err != nil {
		return nil, err
	}
	if forwarded.Message == nil {
		return nil, fmt.Errorf("forwarded stanza missing message")
	}
	return forwarded.Message, nil
}

func extensionHasNamespace(extXML []byte, ns string) bool {
	return bytes.Contains(extXML, []byte(ns))
}

func (c *Client) handleMessage(msg *stanza.Message) {
	for _, ext := range msg.Extensions {
		if ext.XMLName.Space == "urn:xmpp:mam:2" && ext.XMLName.Local == "result" {
			c.handleMAMResult(msg)
			return
		}
	}

	for _, ext := range msg.Extensions {
		if ext.XMLName.Space != "urn:xmpp:carbons:2" {
			continue
		}
		if ext.XMLName.Local != "sent" && ext.XMLName.Local != "received" {
			continue
		}

		forwardedMsg, err := parseForwardedMessage(ext.Inner)
		if err != nil || forwardedMsg == nil {
			continue
		}

		c.handleMessage(forwardedMsg)
		return
	}

	handledReceipt := false
	for _, ext := range msg.Extensions {
		extXML, err := extensionOuterXML(ext)
		if err != nil {
			continue
		}
		isReceiptsNS := ext.XMLName.Space == "urn:xmpp:receipts" || extensionHasNamespace(extXML, "urn:xmpp:receipts")
		isMarkersNS := ext.XMLName.Space == "urn:xmpp:chat-markers:0" || extensionHasNamespace(extXML, "urn:xmpp:chat-markers:0")

		switch {
		case isReceiptsNS && ext.XMLName.Local == "received":
			var received receipts.Received
			if err := xml.Unmarshal(extXML, &received); err == nil && received.ID != "" {
				if c.onReceipt != nil {
					c.onReceipt(received.ID, "delivered")
				}
				handledReceipt = true
			}
		case isMarkersNS && ext.XMLName.Local == "displayed":
			var displayed chatmarkers.Displayed
			if err := xml.Unmarshal(extXML, &displayed); err == nil && displayed.ID != "" {
				if c.onReceipt != nil {
					c.onReceipt(displayed.ID, "read")
				}
				handledReceipt = true
			}
		case isMarkersNS && ext.XMLName.Local == "received":
			var received chatmarkers.Received
			if err := xml.Unmarshal(extXML, &received); err == nil && received.ID != "" {
				if c.onReceipt != nil {
					c.onReceipt(received.ID, "delivered")
				}
				handledReceipt = true
			}
		}
	}
	if handledReceipt && strings.TrimSpace(msg.Body) == "" {
		return
	}

	if c.onMessage == nil {
		return
	}

	m := Message{
		ID:        msg.ID,
		Body:      msg.Body,
		Type:      msg.Type,
		Timestamp: time.Now(),
	}

	if !msg.From.IsZero() {
		m.From = msg.From
	}
	if !msg.To.IsZero() {
		m.To = msg.To
	}

	for _, ext := range msg.Extensions {
		extXML, err := extensionOuterXML(ext)
		if err != nil {
			continue
		}
		isReceiptsNS := ext.XMLName.Space == "urn:xmpp:receipts" || extensionHasNamespace(extXML, "urn:xmpp:receipts")

		if ext.XMLName.Local == "encrypted" {
			m.Encrypted = true
		}
		if isReceiptsNS && ext.XMLName.Local == "request" {
			m.ReceiptRequested = true
		}
		if ext.XMLName.Space == "urn:xmpp:message-correct:0" && ext.XMLName.Local == "replace" {
			var replace correction.Replace
			if err := xml.Unmarshal(extXML, &replace); err == nil {
				m.CorrectedID = replace.ID
			}
		}
		if ext.XMLName.Space == "urn:xmpp:reactions:0" && ext.XMLName.Local == "reactions" {
			var react reactions.Reactions
			if err := xml.Unmarshal(extXML, &react); err == nil {
				if m.Reactions == nil {
					m.Reactions = make(map[string][]string)
				}
				var reactionValues []string
				for _, r := range react.Items {
					reactionValues = append(reactionValues, r.Value)
				}
				m.Reactions[react.ID] = reactionValues
			}
		}
	}

	if strings.TrimSpace(m.Body) == "" && m.CorrectedID == "" && len(m.Reactions) == 0 {
		// Ignore protocol-only/empty stanzas that are not user-visible chat messages.
		return
	}

	c.onMessage(m)
}

func (c *Client) handlePresence(p *stanza.Presence) {
	if c.onPresence == nil {
		return
	}

	pr := Presence{
		Show:   p.Show,
		Status: p.Status,
	}

	if !p.From.IsZero() {
		pr.From = p.From
	}
	if !p.To.IsZero() {
		pr.To = p.To
	}

	c.onPresence(pr)
}

func (c *Client) handleIQ(iq *stanza.IQ) {
	// Handle unsolicited roster pushes (RFC 6121) directly.
	if iq.Type == stanza.IQSet {
		if handled := c.handleRosterPush(iq); handled {
			return
		}
	}

	c.mu.Lock()
	if ch, ok := c.pendingIQs[iq.ID]; ok {
		delete(c.pendingIQs, iq.ID)
		c.mu.Unlock()
		ch <- iq
		return
	}
	c.mu.Unlock()
}

func (c *Client) parseRosterQuery(iq *stanza.IQ) (roster.Query, bool) {
	if len(iq.Query) == 0 {
		return roster.Query{}, false
	}

	var query roster.Query
	if err := xml.Unmarshal(iq.Query, &query); err != nil {
		return roster.Query{}, false
	}

	if query.XMLName.Space != "jabber:iq:roster" || query.XMLName.Local != "query" {
		return roster.Query{}, false
	}

	return query, true
}

func (c *Client) applyRosterItems(query roster.Query) error {
	rp, err := c.getRosterPlugin()
	if err != nil {
		return err
	}

	for _, item := range query.Items {
		if item.Subscription == roster.SubRemove {
			if err := rp.Remove(c.ctx, item.JID); err != nil {
				return err
			}
			continue
		}

		if err := rp.Set(c.ctx, item); err != nil {
			return err
		}
	}

	if query.Ver != "" {
		_ = rp.SetVersion(c.ctx, query.Ver)
	}

	return nil
}

func (c *Client) emitRosterFromStore() {
	if c.onRoster == nil {
		return
	}

	rp, err := c.getRosterPlugin()
	if err != nil {
		return
	}

	items, err := rp.Items(c.ctx)
	if err != nil {
		return
	}

	rosterItems := make([]RosterItem, len(items))
	for i, item := range items {
		parsedJID, _ := jid.Parse(item.JID)
		rosterItems[i] = RosterItem{
			JID:          parsedJID,
			Name:         item.Name,
			Subscription: item.Subscription,
			Groups:       item.Groups,
		}
	}

	c.onRoster(rosterItems)
}

func (c *Client) handleRosterPush(iq *stanza.IQ) bool {
	query, ok := c.parseRosterQuery(iq)
	if !ok {
		return false
	}

	_ = c.applyRosterItems(query)
	c.emitRosterFromStore()

	// Ack roster push IQ set.
	_ = c.session.Send(c.ctx, iq.ResultIQ())
	return true
}

func (c *Client) sendIQAndWait(session *xmp.Session, iq *stanza.IQ, timeout time.Duration) (*stanza.IQ, error) {
	respCh := make(chan *stanza.IQ, 1)
	c.mu.Lock()
	c.pendingIQs[iq.ID] = respCh
	c.mu.Unlock()

	if err := session.Send(c.ctx, iq); err != nil {
		c.mu.Lock()
		delete(c.pendingIQs, iq.ID)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case resp := <-respCh:
		if resp == nil {
			return nil, fmt.Errorf("empty iq response")
		}
		if resp.Type == stanza.IQError {
			return nil, fmt.Errorf("iq request failed: %s", parseIQErrorDetails(resp))
		}
		return resp, nil
	case <-time.After(timeout):
		c.mu.Lock()
		delete(c.pendingIQs, iq.ID)
		c.mu.Unlock()
		return nil, fmt.Errorf("iq request timed out")
	case <-c.ctx.Done():
		c.mu.Lock()
		delete(c.pendingIQs, iq.ID)
		c.mu.Unlock()
		return nil, c.ctx.Err()
	}
}

func (c *Client) handleDisconnect(err error) {
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()

	if c.onDisconnect != nil {
		c.onDisconnect(err)
	}
}

func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.cancel()

	var firstErr error
	if c.plugins != nil {
		if err := c.plugins.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if c.session != nil {
		if err := c.session.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	c.connected = false
	c.client = nil
	c.session = nil

	if c.onDisconnect != nil {
		c.onDisconnect(nil)
	}

	return firstErr
}

func (c *Client) SendMessage(to, body string) (string, error) {
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

	id := stanza.GenerateID()

	msg := stanza.NewMessage(stanza.MessageChat)
	msg.To = toJID
	msg.ID = id
	msg.Body = body
	if reqData, err := xml.Marshal(&receipts.Request{}); err == nil {
		msg.Extensions = append(msg.Extensions, stanza.Extension{
			XMLName: xml.Name{Space: "urn:xmpp:receipts", Local: "request"},
			Inner:   reqData,
		})
	}
	if markableData, err := xml.Marshal(&chatmarkers.Markable{}); err == nil {
		msg.Extensions = append(msg.Extensions, stanza.Extension{
			XMLName: xml.Name{Space: "urn:xmpp:chat-markers:0", Local: "markable"},
			Inner:   markableData,
		})
	}

	return id, session.Send(c.ctx, msg)
}

func (c *Client) SendEncryptedMessage(to, body string) (string, error) {
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

	rp, err := c.getRosterPlugin()
	if err != nil {
		return c.SendMessage(to, body)
	}

	items, err := rp.Items(c.ctx)
	if err != nil {
		return c.SendMessage(to, body)
	}

	var devices []cryptoomemo.Address
	for _, item := range items {
		if item.JID == toJID.Bare().String() {
			op, err := c.getOMEMOPlugin()
			if err == nil {
				devs := op.GetDevices(item.JID)
				for _, d := range devs {
					devices = append(devices, cryptoomemo.Address{
						JID:      item.JID,
						DeviceID: d.ID,
					})
				}
			}
		}
	}

	if len(devices) == 0 {
		return c.SendMessage(to, body)
	}

	encMsg, err := c.omemoManager.Encrypt([]byte(body), devices...)
	if err != nil {
		return c.SendMessage(to, body)
	}

	id := stanza.GenerateID()

	msg := stanza.NewMessage(stanza.MessageChat)
	msg.To = toJID
	msg.ID = id

	enc := &omemoplugin.Encrypted{
		Header: omemoplugin.Header{
			SID: encMsg.SenderDeviceID,
			IV:  hex.EncodeToString(encMsg.IV),
		},
	}
	for _, k := range encMsg.Keys {
		enc.Header.Keys = append(enc.Header.Keys, omemoplugin.Key{
			RID:   k.DeviceID,
			Value: hex.EncodeToString(k.Data),
		})
	}
	if encMsg.Payload != nil {
		enc.Payload = &omemoplugin.Payload{
			Value: hex.EncodeToString(encMsg.Payload),
		}
	}

	encData, _ := xml.Marshal(enc)
	msg.Extensions = append(msg.Extensions, stanza.Extension{
		XMLName: xml.Name{Space: "urn:xmpp:omemo:2", Local: "encrypted"},
		Inner:   encData,
	})

	return id, session.Send(c.ctx, msg)
}

func (c *Client) SendPresence(show, status string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	p := stanza.NewPresence(stanza.PresenceAvailable)
	p.Show = show
	p.Status = status

	return session.Send(c.ctx, p)
}

func (c *Client) SendDirectedPresence(to, show, status string) error {
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

	p := stanza.NewPresence(stanza.PresenceAvailable)
	p.To = toJID.Bare()
	p.Show = show
	p.Status = status

	return session.Send(c.ctx, p)
}

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

	p := stanza.NewPresence(stanza.PresenceUnavailable)
	p.To = toJID.Bare()

	return session.Send(c.ctx, p)
}

func (c *Client) RequestRoster() error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	iq := stanza.NewIQ(stanza.IQGet)
	queryXML, err := xml.Marshal(roster.Query{})
	if err != nil {
		return fmt.Errorf("failed to marshal roster query: %w", err)
	}
	iq.Query = queryXML

	resp, err := c.sendIQAndWait(session, iq, 12*time.Second)
	if err != nil {
		return err
	}
	query, ok := c.parseRosterQuery(resp)
	if !ok {
		return fmt.Errorf("invalid roster response payload")
	}
	if err := c.applyRosterItems(query); err != nil {
		return err
	}
	c.emitRosterFromStore()
	return nil
}

func (c *Client) EnableCarbons() error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	iq := stanza.NewIQ(stanza.IQSet)
	queryXML, err := xml.Marshal(carbons.Enable{})
	if err != nil {
		return fmt.Errorf("failed to marshal carbons enable iq: %w", err)
	}
	iq.Query = queryXML

	_, err = c.sendIQAndWait(session, iq, 8*time.Second)
	if err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "feature-not-implemented") ||
			strings.Contains(lower, "service-unavailable") ||
			strings.Contains(lower, "not-authorized") {
			return nil
		}
		return fmt.Errorf("carbons enable failed: %w", err)
	}

	cp, err := c.getCarbonsPlugin()
	if err == nil {
		cp.SetEnabled(true)
	}

	return nil
}

func (c *Client) AddContact(contactJID, name string, groups []string) (err error) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("roster add contact failed: %v", r)
		}
	}()

	iq := stanza.NewIQ(stanza.IQSet)
	queryXML, err := xml.Marshal(roster.Query{
		Items: []roster.Item{
			{
				JID:    contactJID,
				Name:   name,
				Groups: groups,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal roster set request: %w", err)
	}
	iq.Query = queryXML

	_, err = c.sendIQAndWait(session, iq, 12*time.Second)
	if err != nil {
		return fmt.Errorf("roster set failed: %w", err)
	}
	return nil
}

func (c *Client) RemoveContact(contactJID string) (err error) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("roster remove contact failed: %v", r)
		}
	}()

	iq := stanza.NewIQ(stanza.IQSet)
	queryXML, err := xml.Marshal(roster.Query{
		Items: []roster.Item{
			{
				JID:          contactJID,
				Subscription: roster.SubRemove,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal roster remove request: %w", err)
	}
	iq.Query = queryXML

	_, err = c.sendIQAndWait(session, iq, 12*time.Second)
	if err != nil {
		return fmt.Errorf("roster remove failed: %w", err)
	}
	return nil
}

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

	p := stanza.NewPresence(stanza.PresenceSubscribe)
	p.To = to

	return session.Send(c.ctx, p)
}

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

	p := stanza.NewPresence(stanza.PresenceUnsubscribe)
	p.To = to

	return session.Send(c.ctx, p)
}

func (c *Client) JoinRoom(roomJID, nick, password string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	mp, err := c.getMUCPlugin()
	if err != nil {
		return err
	}

	return mp.JoinRoom(c.ctx, roomJID, nick)
}

func (c *Client) LeaveRoom(roomJID string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	mp, err := c.getMUCPlugin()
	if err != nil {
		return err
	}

	return mp.LeaveRoom(c.ctx, roomJID)
}

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

	msg := stanza.NewMessage(stanza.MessageChat)
	msg.To = toJID

	recData, _ := xml.Marshal(&receipts.Received{ID: messageID})
	msg.Extensions = append(msg.Extensions, stanza.Extension{
		XMLName: xml.Name{Space: "urn:xmpp:receipts", Local: "received"},
		Inner:   recData,
	})

	return session.Send(c.ctx, msg)
}

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

	msg := stanza.NewMessage(stanza.MessageChat)
	msg.To = toJID

	displayedData, _ := xml.Marshal(&chatmarkers.Displayed{ID: messageID})
	msg.Extensions = append(msg.Extensions, stanza.Extension{
		XMLName: xml.Name{Space: "urn:xmpp:chat-markers:0", Local: "displayed"},
		Inner:   displayedData,
	})

	return session.Send(c.ctx, msg)
}

func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *Client) JID() jid.JID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.jid
}

func (c *Client) DeviceID() uint32 {
	return c.deviceID
}

func (c *Client) OMEMOManager() *cryptoomemo.Manager {
	return c.omemoManager
}

func (c *Client) OMEMOStore() *OMEMOStore {
	return c.omemoStore
}

func (c *Client) SetMessageHandler(handler func(msg Message)) {
	c.onMessage = handler
}

func (c *Client) SetPresenceHandler(handler func(p Presence)) {
	c.onPresence = handler
}

func (c *Client) SetRosterHandler(handler func(items []RosterItem)) {
	c.onRoster = handler
}

func (c *Client) SetConnectHandler(handler func()) {
	c.onConnect = handler
}

func (c *Client) SetDisconnectHandler(handler func(err error)) {
	c.onDisconnect = handler
}

func (c *Client) SetErrorHandler(handler func(err error)) {
	c.onError = handler
}

func (c *Client) SetReceiptHandler(handler func(messageID string, status string)) {
	c.onReceipt = handler
}

func (c *Client) GetRosterItems() ([]RosterItem, error) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return nil, fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	rp, err := c.getRosterPlugin()
	if err != nil {
		return nil, err
	}

	items, err := rp.Items(c.ctx)
	if err != nil {
		return nil, err
	}

	rosterItems := make([]RosterItem, len(items))
	for i, item := range items {
		parsedJID, _ := jid.Parse(item.JID)
		rosterItems[i] = RosterItem{
			JID:          parsedJID,
			Name:         item.Name,
			Subscription: item.Subscription,
			Groups:       item.Groups,
		}
	}

	return rosterItems, nil
}

type Bookmark struct {
	RoomJID  string
	Name     string
	Nick     string
	Password string
	Autojoin bool
}

func (c *Client) GetBookmarks() ([]Bookmark, error) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return nil, fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	bp, err := c.getBookmarksPlugin()
	if err != nil {
		return nil, err
	}

	userJID := c.jid.Bare().String()
	bms, err := bp.List(c.ctx, userJID)
	if err != nil {
		return nil, err
	}

	result := make([]Bookmark, len(bms))
	for i, bm := range bms {
		result[i] = Bookmark{
			RoomJID:  bm.RoomJID,
			Name:     bm.Name,
			Nick:     bm.Nick,
			Password: bm.Password,
			Autojoin: bm.Autojoin,
		}
	}
	return result, nil
}

func (c *Client) AddBookmark(roomJID, name, nick, password string, autojoin bool) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	bp, err := c.getBookmarksPlugin()
	if err != nil {
		return err
	}

	userJID := c.jid.Bare().String()
	bm := &storage.Bookmark{
		UserJID:  userJID,
		RoomJID:  roomJID,
		Name:     name,
		Nick:     nick,
		Password: password,
		Autojoin: autojoin,
	}
	return bp.Set(c.ctx, bm)
}

func (c *Client) DeleteBookmark(roomJID string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	bp, err := c.getBookmarksPlugin()
	if err != nil {
		return err
	}

	userJID := c.jid.Bare().String()
	return bp.Delete(c.ctx, userJID, roomJID)
}

func (c *Client) CorrectMessage(to, originalID, newBody string) (string, error) {
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

	id := stanza.GenerateID()

	msg := stanza.NewMessage(stanza.MessageChat)
	msg.To = toJID
	msg.ID = id
	msg.Body = newBody

	replace := &correction.Replace{ID: originalID}
	replaceData, _ := xml.Marshal(replace)
	msg.Extensions = append(msg.Extensions, stanza.Extension{
		XMLName: xml.Name{Space: "urn:xmpp:message-correct:0", Local: "replace"},
		Inner:   replaceData,
	})

	if err := session.Send(c.ctx, msg); err != nil {
		return "", err
	}

	return id, nil
}

func (c *Client) SendReaction(to, messageID, reaction string) error {
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

	msg := stanza.NewMessage(stanza.MessageChat)
	msg.To = toJID
	msg.ID = stanza.GenerateID()

	react := &reactions.Reactions{
		ID: messageID,
		Items: []reactions.Reaction{
			{Value: reaction},
		},
	}
	reactData, _ := xml.Marshal(react)
	msg.Extensions = append(msg.Extensions, stanza.Extension{
		XMLName: xml.Name{Space: "urn:xmpp:reactions:0", Local: "reactions"},
		Inner:   reactData,
	})

	return session.Send(c.ctx, msg)
}

type UploadSlot struct {
	PutURL  string
	GetURL  string
	Headers map[string]string
}

func (c *Client) RequestUploadSlot(serviceJID, filename string, size int64, contentType string) (*UploadSlot, error) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return nil, fmt.Errorf("not connected")
	}
	session := c.session
	c.mu.RUnlock()

	service, err := jid.Parse(serviceJID)
	if err != nil {
		return nil, fmt.Errorf("invalid service JID: %w", err)
	}

	iq := stanza.NewIQ(stanza.IQGet)
	iq.ID = stanza.GenerateID()
	iq.To = service

	req := &upload.Request{
		Filename:    filename,
		Size:        size,
		ContentType: contentType,
	}
	reqData, _ := xml.Marshal(req)
	iq.Query = reqData

	respCh := make(chan *stanza.IQ, 1)
	c.mu.Lock()
	c.pendingIQs[iq.ID] = respCh
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pendingIQs, iq.ID)
		c.mu.Unlock()
	}()

	if err := session.Send(c.ctx, iq); err != nil {
		return nil, err
	}

	select {
	case resp := <-respCh:
		if resp.Type == stanza.IQError {
			return nil, fmt.Errorf("upload slot request failed")
		}

		var slot upload.Slot
		if err := xml.Unmarshal(resp.Query, &slot); err != nil {
			return nil, fmt.Errorf("failed to parse slot response: %w", err)
		}

		result := &UploadSlot{
			PutURL:  slot.Put.URL,
			GetURL:  slot.Get.URL,
			Headers: make(map[string]string),
		}
		for _, h := range slot.Put.Headers {
			result.Headers[h.Name] = h.Value
		}
		return result, nil

	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("upload slot request timed out")
	case <-c.ctx.Done():
		return nil, c.ctx.Err()
	}
}

func (c *Client) QueryMAM(jid, afterID string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	accountBare := c.jid.Bare()
	queryID := generateID()

	iq := stanza.NewIQ(stanza.IQSet)
	iq.ID = queryID
	iq.To = accountBare

	formData := &form.Form{
		Type: form.TypeSubmit,
		Fields: []form.Field{
			{
				Var:    "FORM_TYPE",
				Type:   form.FieldHidden,
				Values: []string{"urn:xmpp:mam:2"},
			},
			{
				Var:    "with",
				Type:   form.FieldJIDSingle,
				Values: []string{jid},
			},
		},
	}

	if afterID != "" {
		formData.Fields = append(formData.Fields, form.Field{
			Var:    "after-id",
			Type:   form.FieldTextSingle,
			Values: []string{afterID},
		})
	}

	formBytes, _ := xml.Marshal(formData)

	query := &mamplugin.Query{
		XMLName: xml.Name{Space: "urn:xmpp:mam:2", Local: "query"},
		QueryID: queryID,
		Form:    formBytes,
	}

	queryData, _ := xml.Marshal(query)
	iq.Query = queryData

	return c.session.SendElement(c.ctx, iq)
}

func (c *Client) handleMAMResult(msg *stanza.Message) {
	for _, ext := range msg.Extensions {
		if ext.XMLName.Space == "urn:xmpp:mam:2" && ext.XMLName.Local == "result" {
			result := &mamplugin.Result{}
			if err := xml.Unmarshal(ext.Inner, result); err != nil {
				continue
			}

			forwarded := struct {
				XMLName xml.Name `xml:"urn:xmpp:forward:0 forwarded"`
				Delay   *struct {
					XMLName xml.Name `xml:"urn:xmpp:delay delay"`
					Stamp   string   `xml:"stamp,attr"`
				} `xml:"urn:xmpp:delay delay,omitempty"`
				Inner []byte `xml:",innerxml"`
			}{}

			if err := xml.Unmarshal(result.Forwarded, &forwarded); err != nil {
				continue
			}

			var forwardedMsg stanza.Message
			if err := xml.Unmarshal(forwarded.Inner, &forwardedMsg); err != nil {
				continue
			}

			c.handleMessage(&forwardedMsg)
		}
	}
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

var _ transport.Transport = (*transport.TCP)(nil)
