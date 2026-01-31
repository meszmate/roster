package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config represents the main application configuration
type Config struct {
	General    GeneralConfig    `toml:"general"`
	UI         UIConfig         `toml:"ui"`
	Encryption EncryptionConfig `toml:"encryption"`
	Plugins    PluginsConfig    `toml:"plugins"`
	Logging    LoggingConfig    `toml:"logging"`
	Storage    StorageConfig    `toml:"storage"`
}

// GeneralConfig contains general application settings
type GeneralConfig struct {
	DataDir     string `toml:"data_dir"`
	AutoConnect bool   `toml:"auto_connect"`
}

// UIConfig contains UI-related settings
type UIConfig struct {
	Theme          string `toml:"theme"`
	RosterPosition string `toml:"roster_position"`
	RosterWidth    int    `toml:"roster_width"`
	ShowTimestamps bool   `toml:"show_timestamps"`
	TimeFormat     string `toml:"time_format"`
	DateFormat     string `toml:"date_format"`
	Notifications  bool   `toml:"notifications"`
}

// EncryptionConfig contains encryption settings
type EncryptionConfig struct {
	Default           string `toml:"default"`
	RequireEncryption bool   `toml:"require_encryption"`
	OMEMOTOFU         bool   `toml:"omemo_tofu"`
}

// PluginsConfig contains plugin settings
type PluginsConfig struct {
	Enabled   []string `toml:"enabled"`
	PluginDir string   `toml:"plugin_dir"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level   string `toml:"level"`
	File    string `toml:"file"`
	Console bool   `toml:"console"`
}

// StorageConfig contains storage settings
type StorageConfig struct {
	// SaveMessages enables/disables message history
	SaveMessages bool `toml:"save_messages"`

	// MessageRetentionDays is the number of days to keep messages (0 = forever)
	MessageRetentionDays int `toml:"message_retention_days"`

	// SaveSessions enables/disables session persistence
	SaveSessions bool `toml:"save_sessions"`

	// SaveWindowState enables/disables window state persistence
	SaveWindowState bool `toml:"save_window_state"`

	// MaxMessageSize is the maximum size of a message to store (in bytes)
	MaxMessageSize int `toml:"max_message_size"`

	// VacuumOnStartup runs database vacuum on startup
	VacuumOnStartup bool `toml:"vacuum_on_startup"`
}

// Account represents an XMPP account configuration
type Account struct {
	JID         string `toml:"jid"`
	Password    string `toml:"password"`
	UseKeyring  bool   `toml:"use_keyring"`
	AutoConnect bool   `toml:"auto_connect"`
	OMEMO       bool   `toml:"omemo"`
	Server      string `toml:"server"`
	Port        int    `toml:"port"`
	Priority    int    `toml:"priority"`
	Resource    string `toml:"resource"`
	Session     bool   `toml:"-"` // Session-only account, not saved to disk
}

// AccountsConfig contains all account configurations
type AccountsConfig struct {
	Accounts []Account `toml:"accounts"`
}

// Paths holds the XDG-compliant paths for the application
type Paths struct {
	ConfigDir string
	DataDir   string
	CacheDir  string
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		General: GeneralConfig{
			DataDir:     "",
			AutoConnect: true,
		},
		UI: UIConfig{
			Theme:          "rainbow",
			RosterPosition: "left",
			RosterWidth:    30,
			ShowTimestamps: true,
			TimeFormat:     "15:04",
			DateFormat:     "2006-01-02",
			Notifications:  true,
		},
		Encryption: EncryptionConfig{
			Default:           "omemo",
			RequireEncryption: true,
			OMEMOTOFU:         true,
		},
		Plugins: PluginsConfig{
			Enabled:   []string{},
			PluginDir: "",
		},
		Logging: LoggingConfig{
			Level:   "info",
			File:    "",
			Console: false,
		},
		Storage: StorageConfig{
			SaveMessages:         true,
			MessageRetentionDays: 0, // Forever
			SaveSessions:         true,
			SaveWindowState:      true,
			MaxMessageSize:       1024 * 1024, // 1MB
			VacuumOnStartup:      false,
		},
	}
}

// GetPaths returns XDG-compliant paths for the application
func GetPaths() (*Paths, error) {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		configDir = filepath.Join(home, ".config")
	}
	configDir = filepath.Join(configDir, "roster")

	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		dataDir = filepath.Join(home, ".local", "share")
	}
	dataDir = filepath.Join(dataDir, "roster")

	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		cacheDir = filepath.Join(home, ".cache")
	}
	cacheDir = filepath.Join(cacheDir, "roster")

	return &Paths{
		ConfigDir: configDir,
		DataDir:   dataDir,
		CacheDir:  cacheDir,
	}, nil
}

// EnsureDirectories creates the necessary directories
func (p *Paths) EnsureDirectories() error {
	dirs := []string{p.ConfigDir, p.DataDir, p.CacheDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// Load loads the configuration from the config file
func Load() (*Config, error) {
	paths, err := GetPaths()
	if err != nil {
		return nil, err
	}

	if err := paths.EnsureDirectories(); err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	configPath := filepath.Join(paths.ConfigDir, "config.toml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Config doesn't exist, use defaults
		cfg.General.DataDir = paths.DataDir
		cfg.Plugins.PluginDir = filepath.Join(paths.DataDir, "plugins")
		cfg.Logging.File = filepath.Join(paths.DataDir, "roster.log")
		return cfg, nil
	}

	if _, err := toml.DecodeFile(configPath, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Expand paths
	if cfg.General.DataDir == "" {
		cfg.General.DataDir = paths.DataDir
	} else {
		cfg.General.DataDir = expandPath(cfg.General.DataDir)
	}

	if cfg.Plugins.PluginDir == "" {
		cfg.Plugins.PluginDir = filepath.Join(cfg.General.DataDir, "plugins")
	} else {
		cfg.Plugins.PluginDir = expandPath(cfg.Plugins.PluginDir)
	}

	if cfg.Logging.File == "" {
		cfg.Logging.File = filepath.Join(cfg.General.DataDir, "roster.log")
	} else {
		cfg.Logging.File = expandPath(cfg.Logging.File)
	}

	return cfg, nil
}

// LoadAccounts loads account configurations
func LoadAccounts() (*AccountsConfig, error) {
	paths, err := GetPaths()
	if err != nil {
		return nil, err
	}

	accountsPath := filepath.Join(paths.ConfigDir, "accounts.toml")

	if _, err := os.Stat(accountsPath); os.IsNotExist(err) {
		return &AccountsConfig{Accounts: []Account{}}, nil
	}

	var accounts AccountsConfig
	if _, err := toml.DecodeFile(accountsPath, &accounts); err != nil {
		return nil, fmt.Errorf("failed to parse accounts file: %w", err)
	}

	// Set defaults for accounts
	for i := range accounts.Accounts {
		if accounts.Accounts[i].Port == 0 {
			accounts.Accounts[i].Port = 5222
		}
		if accounts.Accounts[i].Resource == "" {
			accounts.Accounts[i].Resource = "roster"
		}
		if accounts.Accounts[i].Priority == 0 {
			accounts.Accounts[i].Priority = 0
		}
	}

	return &accounts, nil
}

// Save saves the configuration to the config file
func Save(cfg *Config) error {
	paths, err := GetPaths()
	if err != nil {
		return err
	}

	configPath := filepath.Join(paths.ConfigDir, "config.toml")
	f, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}

	return nil
}

// SaveAccounts saves account configurations
func SaveAccounts(accounts *AccountsConfig) error {
	paths, err := GetPaths()
	if err != nil {
		return err
	}

	accountsPath := filepath.Join(paths.ConfigDir, "accounts.toml")
	f, err := os.Create(accountsPath)
	if err != nil {
		return fmt.Errorf("failed to create accounts file: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(accounts); err != nil {
		return fmt.Errorf("failed to encode accounts: %w", err)
	}

	return nil
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}
