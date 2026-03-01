<h1 align="center"> crictui </h1>

<p align="center">
Beautiful, minimal TUI cricket scorecard viewer
</p>

---

## Features

- **Live Cricket Scores:** Real-time updates from Cricbuzz
- **Match Details:** Team scores, current batsmen, bowler figures
- **Complete Scorecards:** Detailed batting and bowling statistics
- **Innings Navigation:** Browse through all innings with ease
- **Multi-Match Support:** Switch between multiple live matches
- **Clean Interface:** Minimal, terminal-friendly design

## Installation

### Nix

```bash
nix profile install nixpkgs#crictui
```

### Docker

```bash
docker build -t crictui .
docker run --rm -it crictui
```

### `go install`

```bash
go install github.com/12345nikhilkumars/crictui@latest
```

### From Source

```bash
git clone https://github.com/12345nikhilkumars/crictui.git
cd crictui
go build
sudo mv crictui /usr/local/bin/
crictui -h
```

## Usage

```bash
# View all live matches
crictui

# View a specific match
crictui --match-id 118928

# Set refresh rate to 30 seconds
crictui --tick-rate 30000

# Show help
crictui --help
```

> [!TIP]
> To use the `--match-id` flag, open the specific match page on [Cricbuzz](https://www.cricbuzz.com), and extract the match ID from the URL <br>
`https://www.cricbuzz.com/live-cricket-scorecard/<id>/...`

### Controls

| Key | Action |
|-----|--------|
| **`←`** **`→`** | Switch between matches |
| **`↑`** **`↓`** | Navigate innings |
| **`b`** | Toggle batting/bowling view |
| **`q`** | Quit application |

## Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Styling and layout
- [Cobra](https://github.com/spf13/cobra) - CLI framework

## Acknowledgments

- [Cricbuzz](https://www.cricbuzz.com) for providing cricket data
- [Charm](https://charm.sh/) for the excellent TUI libraries

## Contributing

Contributions are always welcome! Feel free to submit a Pull Request.

## Contributors

<a href="https://github.com/12345nikhilkumars/crictui/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=12345nikhilkumars/crictui" />
</a>

<br><br>

<p align="center">
	<img src="https://raw.githubusercontent.com/catppuccin/catppuccin/main/assets/footers/gray0_ctp_on_line.svg?sanitize=true" />
</p>

<p align="center">
        <i><code>&copy 2025-present <a href="https://github.com/12345nikhilkumars">12345nikhilkumars</a></code></i>
</p>

<div align="center">
<a href="https://github.com/12345nikhilkumars/crictui/blob/main/LICENSE"><img src="https://img.shields.io/github/license/12345nikhilkumars/crictui?style=for-the-badge&color=CBA6F7&logoColor=cdd6f4&labelColor=302D41" alt="LICENSE"></a>&nbsp;&nbsp;
</div>
