package disco

import (
	"sync"

	"mellium.im/xmpp/jid"
)

// Identity represents a disco identity
type Identity struct {
	Category string
	Type     string
	Name     string
	Lang     string
}

// Feature represents a disco feature
type Feature string

// Common features
const (
	FeatureDisco          Feature = "http://jabber.org/protocol/disco#info"
	FeatureDiscoItems     Feature = "http://jabber.org/protocol/disco#items"
	FeatureMUC            Feature = "http://jabber.org/protocol/muc"
	FeatureChatStates     Feature = "http://jabber.org/protocol/chatstates"
	FeatureReceipts       Feature = "urn:xmpp:receipts"
	FeatureCarbons        Feature = "urn:xmpp:carbons:2"
	FeatureMAM            Feature = "urn:xmpp:mam:2"
	FeatureHTTPUpload     Feature = "urn:xmpp:http:upload:0"
	FeatureOMEMO          Feature = "eu.siacs.conversations.axolotl.devicelist+notify"
	FeatureOMEMODevices   Feature = "urn:xmpp:omemo:2"
	FeatureCorrection     Feature = "urn:xmpp:message-correct:0"
	FeatureChatMarkers    Feature = "urn:xmpp:chat-markers:0"
	FeatureVCard4         Feature = "urn:xmpp:vcard4"
	FeatureAvatar         Feature = "urn:xmpp:avatar:metadata+notify"
	FeaturePubSub         Feature = "http://jabber.org/protocol/pubsub"
	FeatureBookmarks      Feature = "urn:xmpp:bookmarks:1"
)

// Info represents disco info response
type Info struct {
	Identities []Identity
	Features   []Feature
}

// Item represents a disco item
type Item struct {
	JID  jid.JID
	Name string
	Node string
}

// Cache caches disco information
type Cache struct {
	mu    sync.RWMutex
	info  map[string]*Info
	items map[string][]Item
}

// NewCache creates a new disco cache
func NewCache() *Cache {
	return &Cache{
		info:  make(map[string]*Info),
		items: make(map[string][]Item),
	}
}

// SetInfo sets disco info for a JID
func (c *Cache) SetInfo(j jid.JID, info *Info) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.info[j.String()] = info
}

// GetInfo gets disco info for a JID
func (c *Cache) GetInfo(j jid.JID) *Info {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.info[j.String()]
}

// SetItems sets disco items for a JID
func (c *Cache) SetItems(j jid.JID, items []Item) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[j.String()] = items
}

// GetItems gets disco items for a JID
func (c *Cache) GetItems(j jid.JID) []Item {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.items[j.String()]
}

// HasFeature checks if a JID supports a feature
func (c *Cache) HasFeature(j jid.JID, feature Feature) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	info := c.info[j.String()]
	if info == nil {
		return false
	}

	for _, f := range info.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// Clear clears the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.info = make(map[string]*Info)
	c.items = make(map[string][]Item)
}

// Remove removes entries for a JID
func (c *Cache) Remove(j jid.JID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.info, j.String())
	delete(c.items, j.String())
}
