# Plugin Development Guide

This guide explains how to develop plugins for Roster.

## Overview

Roster uses HashiCorp's go-plugin library for process isolation. Plugins run as separate processes and communicate with the main application via gRPC.

## Plugin Interface

All plugins must implement the `Plugin` interface:

```go
type Plugin interface {
    // Name returns the plugin name
    Name() string

    // Version returns the plugin version
    Version() string

    // Description returns a short description
    Description() string

    // Init initializes the plugin with the API
    Init(ctx context.Context, api API) error

    // Start starts the plugin
    Start() error

    // Stop stops the plugin
    Stop() error
}
```

## Available APIs

### RosterAPI

Access roster/contact information:

```go
// Get all contacts
contacts := api.GetContacts()

// Get specific contact
contact := api.GetContact("user@example.com")

// Add contact
err := api.AddContact("user@example.com", "Name", []string{"Friends"})

// Remove contact
err := api.RemoveContact("user@example.com")

// Get presence
status := api.GetPresence("user@example.com")
```

### ChatAPI

Send and receive messages:

```go
// Send message
err := api.SendMessage("user@example.com", "Hello!")

// Get chat history
messages := api.GetHistory("user@example.com", 100)

// Get unread count
count := api.GetUnreadCount("user@example.com")
```

### UIAPI

Interact with the UI:

```go
// Show desktop notification
err := api.ShowNotification("Title", "Message body")

// Add status bar item
err := api.AddStatusBarItem("myplugin", "Status text")

// Remove status bar item
err := api.RemoveStatusBarItem("myplugin")

// Show dialog
choice, err := api.ShowDialog("Title", "Message", []string{"OK", "Cancel"})
```

### EventsAPI

Subscribe to events:

```go
// Message received
unsubscribe := api.OnMessage(func(msg plugin.Message) {
    fmt.Printf("Message from %s: %s\n", msg.From, msg.Body)
})

// Presence changed
unsubscribe := api.OnPresence(func(jid, status string) {
    fmt.Printf("%s is now %s\n", jid, status)
})

// Connected
unsubscribe := api.OnConnect(func() {
    fmt.Println("Connected!")
})

// Disconnected
unsubscribe := api.OnDisconnect(func() {
    fmt.Println("Disconnected!")
})
```

### CommandsAPI

Register custom commands:

```go
// Register command
err := api.RegisterCommand("mycommand", "Description", func(args []string) error {
    // Handle command
    return nil
})

// Unregister command
err := api.UnregisterCommand("mycommand")
```

## Example Plugin

Here's a complete example plugin:

```go
package main

import (
    "context"
    "fmt"

    "github.com/meszmate/roster/pkg/plugin"
)

type MyPlugin struct {
    api     plugin.API
    running bool
    unsub   []func()
}

func (p *MyPlugin) Name() string {
    return "myplugin"
}

func (p *MyPlugin) Version() string {
    return "1.0.0"
}

func (p *MyPlugin) Description() string {
    return "My example plugin"
}

func (p *MyPlugin) Init(ctx context.Context, api plugin.API) error {
    p.api = api
    return nil
}

func (p *MyPlugin) Start() error {
    if p.running {
        return nil
    }

    // Subscribe to messages
    unsub := p.api.OnMessage(func(msg plugin.Message) {
        if !msg.Outgoing {
            fmt.Printf("Received: %s\n", msg.Body)
        }
    })
    p.unsub = append(p.unsub, unsub)

    // Register command
    _ = p.api.RegisterCommand("greet", "Send greeting", func(args []string) error {
        if len(args) > 0 {
            return p.api.SendMessage(args[0], "Hello!")
        }
        return nil
    })

    p.running = true
    return nil
}

func (p *MyPlugin) Stop() error {
    if !p.running {
        return nil
    }

    for _, unsub := range p.unsub {
        unsub()
    }
    p.unsub = nil

    _ = p.api.UnregisterCommand("greet")

    p.running = false
    return nil
}

func main() {
    // Plugin serving code would go here
}
```

## Building Plugins

1. Create a new directory in `plugins/`:
```bash
mkdir plugins/myplugin
```

2. Create `main.go` with your plugin implementation

3. Build the plugin:
```bash
go build -o ~/.local/share/roster/plugins/myplugin plugins/myplugin/main.go
```

4. Enable in config:
```toml
[plugins]
enabled = ["myplugin"]
```

## Best Practices

1. **Handle errors gracefully**: Don't crash the plugin on errors
2. **Clean up on Stop()**: Unsubscribe from events, close connections
3. **Use goroutines carefully**: Don't block the main plugin thread
4. **Respect user privacy**: Don't log or transmit message contents
5. **Test thoroughly**: Test with various message types and edge cases

## Debugging

Enable debug logging in the main config:

```toml
[logging]
level = "debug"
console = true
```

Plugin output will be captured in the main log file.
