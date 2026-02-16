package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/meszmate/roster/pkg/plugin"
)

// StatusNotifyPlugin notifies on status changes
type StatusNotifyPlugin struct {
	api     plugin.API
	running bool
	unsub   []func()
}

// Name returns the plugin name
func (p *StatusNotifyPlugin) Name() string {
	return "statusnotify"
}

// Version returns the plugin version
func (p *StatusNotifyPlugin) Version() string {
	return "1.0.0"
}

// Description returns a short description
func (p *StatusNotifyPlugin) Description() string {
	return "Desktop notifications for status changes"
}

// Init initializes the plugin
func (p *StatusNotifyPlugin) Init(ctx context.Context, api plugin.API) error {
	p.api = api
	return nil
}

// Start starts the plugin
func (p *StatusNotifyPlugin) Start() error {
	if p.running {
		return nil
	}

	// Subscribe to presence changes
	unsubPresence := p.api.OnPresence(func(jid, status string) {
		contact := p.api.GetContact(jid)
		name := jid
		if contact != nil && contact.Name != "" {
			name = contact.Name
		}

		var message string
		switch status {
		case "online":
			message = fmt.Sprintf("%s is now online", name)
		case "away":
			message = fmt.Sprintf("%s is away", name)
		case "dnd":
			message = fmt.Sprintf("%s is busy", name)
		case "offline":
			message = fmt.Sprintf("%s went offline", name)
		default:
			return
		}

		_ = sendNotification("Roster", message)
	})
	p.unsub = append(p.unsub, unsubPresence)

	// Subscribe to messages
	unsubMessage := p.api.OnMessage(func(msg plugin.Message) {
		if msg.Outgoing {
			return
		}

		contact := p.api.GetContact(msg.From)
		name := msg.From
		if contact != nil && contact.Name != "" {
			name = contact.Name
		}

		_ = sendNotification(name, msg.Body)
	})
	p.unsub = append(p.unsub, unsubMessage)

	p.running = true
	return nil
}

// Stop stops the plugin
func (p *StatusNotifyPlugin) Stop() error {
	if !p.running {
		return nil
	}

	for _, unsub := range p.unsub {
		unsub()
	}
	p.unsub = nil

	p.running = false
	return nil
}

// sendNotification sends a desktop notification
func sendNotification(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(`display notification "%s" with title "%s"`, body, title)
		return exec.Command("osascript", "-e", script).Run()

	case "linux":
		return exec.Command("notify-send", title, body).Run()

	case "windows":
		// Windows Toast notifications require more complex implementation
		return nil

	default:
		return nil
	}
}

func main() {
	// This would use go-plugin to serve the plugin
	// Simplified for example purposes
}
