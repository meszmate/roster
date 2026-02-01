# Roster

A modern, terminal-based XMPP/Jabber client written in Go with OMEMO encryption, vim motions, plugin system, and multiple themes.

## Features

- **Modern TUI**: Built with Bubble Tea for a reactive, fast interface
- **Vim Motions**: Full vim-style navigation and editing
- **OMEMO Encryption**: End-to-end encryption by default
- **Account Registration**: In-app XMPP account registration with comprehensive CAPTCHA support (image, audio, video, Q&A)
- **Plugin System**: Extend functionality with Go plugins
- **Themes**: Multiple built-in themes (Rainbow, Matrix, Nord, Gruvbox, Dracula) with custom theme support
- **Multi-Account**: Support for multiple XMPP accounts with easy switching
- **MUC Support**: Full multi-user chat room support with room creation
- **File Transfer**: HTTP File Upload with OMEMO encryption
- **Message History**: SQLite-backed message storage
- **20 Windows**: Quick window switching with Alt+1-0, Alt+q-p
- **Scrollable Dialogs**: Help menu and long content with vim-style scrolling

## Installation

### From Source

```bash
git clone https://github.com/meszmate/roster
cd roster
make build
```

### Install to PATH

```bash
make install
```

## Quick Start

1. Create configuration:
```bash
make init-config
```

2. Edit `~/.config/roster/accounts.toml` with your XMPP credentials

3. Run roster:
```bash
./build/roster
```

## Key Bindings

### Modes

- **Normal Mode**: Navigation and commands
- **Insert Mode**: Text input
- **Command Mode**: Execute commands with `:`
- **Search Mode**: Search with `/`

### Navigation (Normal Mode)

| Key | Action |
|-----|--------|
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `gg` | Go to top |
| `G` | Go to bottom |
| `Ctrl+u` | Half page up |
| `Ctrl+d` | Half page down |
| `Ctrl+b` | Page up |
| `Ctrl+f` | Page down |

### Mode Switching

| Key | Action |
|-----|--------|
| `i` | Enter insert mode |
| `:` | Enter command mode |
| `/` | Search forward |
| `?` | Search backward |
| `Esc` | Return to normal mode |

### Windows

| Key | Action |
|-----|--------|
| `Alt+1-0` | Switch to window 1-10 |
| `Alt+q-p` | Switch to window 11-20 |
| `Tab` | Next window |
| `Shift+Tab` | Previous window |
| `gt` | Next window |
| `gT` | Previous window |

### Actions

| Key | Action |
|-----|--------|
| `Enter` | Open chat / Send message |
| `q` | Close chat window |
| `Ctrl+r` | Toggle roster |
| `ga` | Add contact |
| `gx` | Remove contact |
| `gR` | Rename contact |
| `gj` | Join room |
| `gi` | Show contact info |
| `gs` / `S` | Settings |
| `gw` | Save windows |
| `H` | Context help popup |

### Focus

| Key | Action |
|-----|--------|
| `gr` | Focus roster |
| `gc` | Focus chat |
| `gA` | Focus accounts section |
| `gl` | Toggle full account list |

### Account Actions (in accounts section)

| Key | Action |
|-----|--------|
| `C` | Connect account |
| `D` | Disconnect account |
| `E` | Edit account |
| `X` | Remove account |
| `H` | Show account info tooltip |

### Dialog Navigation

| Key | Action |
|-----|--------|
| `Tab` | Next field |
| `Shift+Tab` | Previous field |
| `Enter` | Confirm / Toggle checkbox |
| `Esc` | Cancel dialog |
| `j` / `k` | Scroll (in help dialog) |
| `g` / `G` | Top / Bottom (in help dialog) |

## Commands

Commands are entered in command mode (press `:` first):

| Command | Description |
|---------|-------------|
| `:quit`, `:q` | Quit roster |
| `:connect <jid> <pass> [server] [port]` | Quick connect (session only) |
| `:account add` | Add saved account |
| `:register` | Register new account on a server |
| `:disconnect` | Disconnect |
| `:msg <jid> <message>` | Send message |
| `:join <room>` | Join MUC room |
| `:leave` | Leave current room |
| `:add <jid> [name]` | Add contact |
| `:remove <jid>` | Remove contact |
| `:status <status> [msg]` | Set status |
| `:away [msg]` | Set away |
| `:dnd [msg]` | Set do not disturb |
| `:online` | Set online |
| `:set theme <name>` | Change theme |
| `:omemo fingerprint` | Show OMEMO fingerprints |
| `:omemo trust <jid>` | Trust device |
| `:help [command]` | Show help |

## Configuration

Configuration files are stored in `~/.config/roster/`:

- `config.toml` - Main configuration
- `accounts.toml` - Account credentials

Data is stored in `~/.local/share/roster/`:

- `roster.db` - Message history and cache
- `plugins/` - Plugin directory
- `roster.log` - Log file

### Example Configuration

```toml
[general]
auto_connect = true

[ui]
theme = "rainbow"
roster_width = 30
show_timestamps = true

[encryption]
default = "omemo"
require_encryption = true
omemo_tofu = true

[plugins]
enabled = ["statusnotify"]
```

## Themes

### Built-in Themes

- **Rainbow**: Colorful, vibrant theme with rounded borders
- **Matrix**: Classic green-on-black terminal aesthetic
- **Nord**: Arctic blue, calm color palette
- **Gruvbox**: Retro warm terminal with earthy tones
- **Dracula**: Purple dark modern theme

### Custom Themes

Create a TOML file in `~/.local/share/roster/themes/` or `themes/`:

```toml
name = "mytheme"
description = "My custom theme"

[colors]
primary = "#FF6B6B"
secondary = "#4ECDC4"
background = "#2D3436"
foreground = "#DFE6E9"
# ... more colors

[roster]
header_fg = "#2D3436"
header_bg = "#FF6B6B"
# ... more styles
```

## Plugins

### Installing Plugins

1. Build the plugin:
```bash
cd plugins/statusnotify
go build -o ~/.local/share/roster/plugins/statusnotify
```

2. Enable in config:
```toml
[plugins]
enabled = ["statusnotify"]
```

### Available Plugins

- **statusnotify**: Desktop notifications for status changes
- **urlpreview**: Preview URLs in chat messages

### Developing Plugins

See [docs/plugins.md](docs/plugins.md) for the plugin development guide.

## Account Registration

Register new XMPP accounts directly from the client using `:register`:

1. Enter the server domain (e.g., `example.com`)
2. Fill in the registration form (username, password, etc.)
3. Complete any CAPTCHA challenge if required
4. Optionally save and connect to the new account

### CAPTCHA Support

Roster supports a wide range of CAPTCHA types commonly used by XMPP servers:

- **Image CAPTCHA**: OCR challenges with embedded or URL-based images
- **Audio CAPTCHA**: Audio recognition challenges
- **Video CAPTCHA**: Video-based verification
- **Q&A CAPTCHA**: Text-based security questions
- **Data Forms**: XEP-0004 compliant registration forms

When a CAPTCHA is required:
- Press `V` to view/open the CAPTCHA media
- Press `C` to copy the CAPTCHA URL to clipboard
- Enter your answer in the designated field

## Encryption

### OMEMO (Default)

OMEMO provides multi-device end-to-end encryption:

- Automatic key exchange
- Forward secrecy
- Multi-device support
- Trust On First Use (TOFU) option

### Fingerprint Verification

```
:omemo fingerprint user@example.com
:omemo trust user@example.com <fingerprint>
```

### Alternative Encryption

- **OTR**: Legacy encryption (optional)
- **OpenPGP**: PGP encryption (optional)

## Building

### Requirements

- Go 1.23+
- Make
- SQLite3

### Development

```bash
# Build
make build

# Run tests
make test

# Format code
make fmt

# Lint
make lint

# All checks
make check
```

## License

MIT License - see [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! Please read the contributing guidelines before submitting a PR.

## Acknowledgments

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling
- [Mellium](https://mellium.im/xmpp) - XMPP library
- [go-plugin](https://github.com/hashicorp/go-plugin) - Plugin system
