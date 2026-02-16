package plugin

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// Host manages plugin lifecycle
type Host struct {
	mu        sync.RWMutex
	plugins   map[string]*LoadedPlugin
	pluginDir string
	api       API
}

// LoadedPlugin represents a loaded plugin
type LoadedPlugin struct {
	Name    string
	Version string
	Plugin  Plugin
	Client  *plugin.Client
	Running bool
}

// Handshake is the plugin handshake config
var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "ROSTER_PLUGIN",
	MagicCookieValue: "roster",
}

// PluginMap is the plugin type map
var PluginMap = map[string]plugin.Plugin{
	"plugin": &GRPCPlugin{},
}

// NewHost creates a new plugin host
func NewHost(pluginDir string, api API) *Host {
	return &Host{
		plugins:   make(map[string]*LoadedPlugin),
		pluginDir: pluginDir,
		api:       api,
	}
}

// LoadAll loads all plugins from the plugin directory
func (h *Host) LoadAll() error {
	if h.pluginDir == "" {
		return nil
	}

	entries, err := os.ReadDir(h.pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(h.pluginDir, entry.Name())
		if err := h.Load(path); err != nil {
			log.Printf("Failed to load plugin %s: %v", entry.Name(), err)
		}
	}

	return nil
}

// Load loads a single plugin
func (h *Host) Load(path string) error {
	// Create the plugin client
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: Handshake,
		Plugins:         PluginMap,
		Cmd:             exec.Command(path),
		AllowedProtocols: []plugin.Protocol{
			plugin.ProtocolGRPC,
		},
	})

	// Connect via RPC
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to connect to plugin: %w", err)
	}

	// Request the plugin
	raw, err := rpcClient.Dispense("plugin")
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to dispense plugin: %w", err)
	}

	p := raw.(Plugin)

	// Initialize the plugin
	ctx := context.Background()
	if err := p.Init(ctx, h.api); err != nil {
		client.Kill()
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}

	h.mu.Lock()
	h.plugins[p.Name()] = &LoadedPlugin{
		Name:    p.Name(),
		Version: p.Version(),
		Plugin:  p,
		Client:  client,
	}
	h.mu.Unlock()

	return nil
}

// Start starts a loaded plugin
func (h *Host) Start(name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	lp := h.plugins[name]
	if lp == nil {
		return fmt.Errorf("plugin not found: %s", name)
	}

	if lp.Running {
		return nil
	}

	if err := lp.Plugin.Start(); err != nil {
		return fmt.Errorf("failed to start plugin: %w", err)
	}

	lp.Running = true
	return nil
}

// Stop stops a running plugin
func (h *Host) Stop(name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	lp := h.plugins[name]
	if lp == nil {
		return fmt.Errorf("plugin not found: %s", name)
	}

	if !lp.Running {
		return nil
	}

	if err := lp.Plugin.Stop(); err != nil {
		return fmt.Errorf("failed to stop plugin: %w", err)
	}

	lp.Running = false
	return nil
}

// Unload unloads a plugin
func (h *Host) Unload(name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	lp := h.plugins[name]
	if lp == nil {
		return nil
	}

	if lp.Running {
		_ = lp.Plugin.Stop()
	}

	lp.Client.Kill()
	delete(h.plugins, name)

	return nil
}

// UnloadAll unloads all plugins
func (h *Host) UnloadAll() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for name, lp := range h.plugins {
		if lp.Running {
			_ = lp.Plugin.Stop()
		}
		lp.Client.Kill()
		delete(h.plugins, name)
	}
}

// List returns all loaded plugins
func (h *Host) List() []*LoadedPlugin {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]*LoadedPlugin, 0, len(h.plugins))
	for _, lp := range h.plugins {
		result = append(result, lp)
	}
	return result
}

// Get returns a specific plugin
func (h *Host) Get(name string) *LoadedPlugin {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.plugins[name]
}

// GRPCPlugin is the gRPC plugin implementation
type GRPCPlugin struct {
	plugin.Plugin
	Impl Plugin
}

// GRPCServer returns the gRPC server
func (p *GRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	// Would register the gRPC service here
	return nil
}

// GRPCClient returns the gRPC client
func (p *GRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	// Would create gRPC client here
	return nil, nil
}
