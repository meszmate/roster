package client

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/xml"
	"fmt"
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

type Message struct {
	ID          string
	From        jid.JID
	To          jid.JID
	Body        string
	Type        string
	Timestamp   time.Time
	Thread      string
	Encrypted   bool
	CorrectedID string
	Reactions   map[string][]string
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

	if cfg.Resource != "" {
		parsedJID = parsedJID.WithResource(cfg.Resource)
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
		resource:   cfg.Resource,
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

	trans, err := dialer.Dial(c.ctx, c.jid.Domain())
	if err != nil {
		return fmt.Errorf("failed to dial server: %w", err)
	}

	c.omemoStore = NewOMEMOStore(c.jid.String(), c.deviceID)
	c.omemoManager = cryptoomemo.NewManager(c.omemoStore)

	_, err = c.omemoManager.GenerateBundle(100)
	if err != nil {
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

	client, err := xmp.NewClient(c.jid, c.password,
		xmp.WithPlugins(plugins...),
		xmp.WithHandler(xmp.HandlerFunc(c.handleStanza)),
	)
	if err != nil {
		trans.Close()
		return fmt.Errorf("failed to create client: %w", err)
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

	params := plugin.InitParams{
		SendRaw: func(ctx context.Context, data []byte) error {
			return c.session.SendRaw(ctx, nil)
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

	c.client = client
	c.connected = true

	go c.serve()

	if c.onConnect != nil {
		c.onConnect()
	}

	return nil
}

func (c *Client) serve() {
	if err := c.session.Serve(nil); err != nil {
		c.handleDisconnect(err)
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

func (c *Client) handleMessage(msg *stanza.Message) {
	for _, ext := range msg.Extensions {
		if ext.XMLName.Space == "urn:xmpp:mam:2" && ext.XMLName.Local == "result" {
			c.handleMAMResult(msg)
			return
		}
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
		if ext.XMLName.Local == "encrypted" {
			m.Encrypted = true
		}
		if ext.XMLName.Space == "urn:xmpp:message-correct:0" && ext.XMLName.Local == "replace" {
			var replace correction.Replace
			if err := xml.Unmarshal(ext.Inner, &replace); err == nil {
				m.CorrectedID = replace.ID
			}
		}
		if ext.XMLName.Space == "urn:xmpp:reactions:0" && ext.XMLName.Local == "reactions" {
			var react reactions.Reactions
			if err := xml.Unmarshal(ext.Inner, &react); err == nil {
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
	c.mu.Lock()
	if ch, ok := c.pendingIQs[iq.ID]; ok {
		delete(c.pendingIQs, iq.ID)
		c.mu.Unlock()
		ch <- iq
		return
	}
	c.mu.Unlock()
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
	c.mu.RUnlock()

	rp, err := c.getRosterPlugin()
	if err != nil {
		return err
	}

	items, err := rp.Items(c.ctx)
	if err != nil {
		return err
	}

	if c.onRoster != nil {
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

	return nil
}

func (c *Client) AddContact(contactJID, name string, groups []string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	rp, err := c.getRosterPlugin()
	if err != nil {
		return err
	}

	return rp.Set(c.ctx, roster.Item{
		JID:    contactJID,
		Name:   name,
		Groups: groups,
	})
}

func (c *Client) RemoveContact(contactJID string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	rp, err := c.getRosterPlugin()
	if err != nil {
		return err
	}

	return rp.Remove(c.ctx, contactJID)
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
