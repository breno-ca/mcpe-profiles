# MCPE Profiles

Profile manager for **Minecraft Bedrock Edition** running through
[mcpelauncher](https://minecraft-linux.github.io) (Flatpak `io.mrarm.mcpelauncher`).

Lets you run multiple MCPE instances on the same PC, each with its own
isolated `HOME`, and **ignore specific controllers per profile** — so
several people can play locally on the same machine without their
controllers interfering with each other.

## Requirements

- Linux (x86_64)
- [`flatpak`](https://flatpak.org/) installed
- `io.mrarm.mcpelauncher` installed via Flathub:
  [flathub.org/apps/io.mrarm.mcpelauncher](https://flathub.org/apps/io.mrarm.mcpelauncher)

## Install

Download the binary from the [Releases](../../releases) page:

```bash
chmod +x mcpe-profiles-linux-amd64
./mcpe-profiles-linux-amd64
```

## Usage

- **Create profile:** `+` next to "Profiles"
- **Select:** click the name
- **Rename:** pencil icon
- **Ignore controllers:** check the ones that should **not** respond in that profile
- **Launch:** "START" button

On first run it asks whether to create an application menu shortcut.

## License

MIT
