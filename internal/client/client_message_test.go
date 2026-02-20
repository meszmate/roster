package client

import (
	"encoding/xml"
	"testing"

	"github.com/meszmate/xmpp-go/plugins/correction"
	"github.com/meszmate/xmpp-go/stanza"
)

func TestExtensionOuterXMLPreservesAttributes(t *testing.T) {
	ext := stanza.Extension{
		XMLName: xml.Name{Space: "urn:xmpp:message-correct:0", Local: "replace"},
		Attrs: []xml.Attr{
			{Name: xml.Name{Local: "id"}, Value: "orig-1"},
		},
	}

	raw, err := extensionOuterXML(ext)
	if err != nil {
		t.Fatalf("extensionOuterXML returned error: %v", err)
	}

	var replace correction.Replace
	if err := xml.Unmarshal(raw, &replace); err != nil {
		t.Fatalf("failed to unmarshal replace extension: %v", err)
	}
	if replace.ID != "orig-1" {
		t.Fatalf("expected replace id orig-1, got %q", replace.ID)
	}
}

func TestParseForwardedMessage(t *testing.T) {
	raw := []byte(`<forwarded xmlns='urn:xmpp:forward:0'><message xmlns='jabber:client' id='m1' from='alice@example.com/phone' to='bob@example.com/roster' type='chat'><body>hello</body></message></forwarded>`)

	msg, err := parseForwardedMessage(raw)
	if err != nil {
		t.Fatalf("parseForwardedMessage returned error: %v", err)
	}

	if msg.ID != "m1" {
		t.Fatalf("expected id m1, got %q", msg.ID)
	}
	if msg.Body != "hello" {
		t.Fatalf("expected body hello, got %q", msg.Body)
	}
	if msg.From.Bare().String() != "alice@example.com" {
		t.Fatalf("unexpected from JID: %s", msg.From.String())
	}
}

func TestHandleMessageUnwrapsCarbonsForwarded(t *testing.T) {
	forwarded := []byte(`<forwarded xmlns='urn:xmpp:forward:0'><message xmlns='jabber:client' id='m2' from='alice@example.com/phone' to='bob@example.com/roster' type='chat'><body>carbon hello</body></message></forwarded>`)

	c := &Client{}
	called := false
	var got Message
	c.onMessage = func(msg Message) {
		called = true
		got = msg
	}

	outer := &stanza.Message{
		Extensions: []stanza.Extension{
			{
				XMLName: xml.Name{Space: "urn:xmpp:carbons:2", Local: "received"},
				Inner:   forwarded,
			},
		},
	}

	c.handleMessage(outer)

	if !called {
		t.Fatalf("expected onMessage to be called")
	}
	if got.ID != "m2" {
		t.Fatalf("expected id m2, got %q", got.ID)
	}
	if got.Body != "carbon hello" {
		t.Fatalf("expected forwarded body, got %q", got.Body)
	}
}

func TestHandleMessageReceiptOnlyDoesNotEmitChatMessage(t *testing.T) {
	c := &Client{}

	receiptCalled := false
	var receiptID, receiptStatus string
	c.onReceipt = func(messageID string, status string) {
		receiptCalled = true
		receiptID = messageID
		receiptStatus = status
	}

	messageCalled := false
	c.onMessage = func(msg Message) {
		messageCalled = true
	}

	msg := &stanza.Message{
		Extensions: []stanza.Extension{
			{
				XMLName: xml.Name{Space: "urn:xmpp:receipts", Local: "received"},
				Attrs: []xml.Attr{
					{Name: xml.Name{Local: "id"}, Value: "msg-123"},
				},
			},
		},
	}

	c.handleMessage(msg)

	if !receiptCalled {
		t.Fatalf("expected receipt callback to be called")
	}
	if receiptID != "msg-123" {
		t.Fatalf("expected receipt id msg-123, got %q", receiptID)
	}
	if receiptStatus != "delivered" {
		t.Fatalf("expected delivered status, got %q", receiptStatus)
	}
	if messageCalled {
		t.Fatalf("did not expect onMessage callback for receipt-only stanza")
	}
}

func TestHandleMessageParsesCorrectionIDFromAttributes(t *testing.T) {
	c := &Client{}

	var got Message
	c.onMessage = func(msg Message) {
		got = msg
	}

	msg := &stanza.Message{
		Body: "edited text",
		Extensions: []stanza.Extension{
			{
				XMLName: xml.Name{Space: "urn:xmpp:message-correct:0", Local: "replace"},
				Attrs: []xml.Attr{
					{Name: xml.Name{Local: "id"}, Value: "orig-42"},
				},
			},
		},
	}

	c.handleMessage(msg)

	if got.CorrectedID != "orig-42" {
		t.Fatalf("expected corrected id orig-42, got %q", got.CorrectedID)
	}
}
