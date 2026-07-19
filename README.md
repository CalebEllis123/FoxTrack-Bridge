# FoxTrack Bridge

![GitHub release](https://img.shields.io/github/v/release/FoxesRCool1/FoxTrack-Bridge)
![Platforms](https://img.shields.io/badge/platforms-Windows%20%7C%20macOS%20%7C%20Linux-blue)

FoxTrack Bridge is a local dashboard and integration server for 3D printers. Run it on any machine on the same network as your printers to get a web dashboard for status, live camera feeds, print history, and basic controls. It can also connect to [FoxTrack](https://foxtrack.studio/) so you can monitor your printers remotely.

The dashboard runs fully offline. Its CSS, fonts, and icons are embedded in the binary, so no external network or CDN is needed to load the UI.

## A note on AI

Among many other tools, AI was used to develop this program. If you have a problem with that, you may look for alternative programs made by people with more time or contribute human-made code.

<img width="2519" height="1255" alt="screenshot-2026-05-21_16-45-12" src="https://github.com/user-attachments/assets/3b25ea5a-f462-459b-b5b3-6c7a5244bb86" />

---

## Features

- Live status for Bambu Lab and Klipper/Moonraker printers
- Print progress, temperatures, elapsed time, and ETA
- Live camera feed and light toggle
- AMS filament slot display with colors and material types (Bambu Lab only)
- Print speed selector: Silent, Standard, Sport, Ludicrous (Bambu Lab only)
- Browser push notifications when prints finish, fail, or pause
- Print history stored locally, shown in the History view
- Automatic updates with staged installation
- Headless mode for servers, NAS devices, and Raspberry Pi

---

## Supported platforms

| Platform |
|---|
| Windows x64 |
| macOS Apple Silicon |
| macOS Intel |
| Linux x64 |
| Linux ARM (headless) |

Download the latest builds from the [releases page](https://github.com/FoxesRCool1/FoxTrack-Bridge/releases/latest).

---

## Support the project

If you want to support development, sign up to [FoxTrack](https://foxtrack.studio/). Join the [Discord](https://discord.com/invite/3hd96AFYBf) to leave feedback.

---

## Quick start

1. Run the Bridge. Download the binary for your platform and launch it. A system tray icon appears on Windows, macOS, and Linux x64. On the Linux ARM build, run the binary from a terminal; the startup output lists the local addresses where the dashboard is available.
2. Open the dashboard at [http://localhost:8080](http://localhost:8080). From other devices on the same network, use the IP address shown in the startup output (for example `http://192.168.x.x:8080`). You may need to disable WiFi AP isolation on your router.
3. Connect to FoxTrack (optional). In Settings, paste your FoxTrack API key and click Save. Get your key from [FoxTrack Settings > Integrations](https://foxtrack.studio/settings).
4. Add a printer. In Printers, click "Add Printer", select the printer type, fill in the required fields, and click Connect.

---

## Adding printers

### Bambu Lab

1. On the printer touchscreen, enable **LAN Only Mode** under Network settings.
2. Enable **Developer Mode** under About.
3. Note the **IP address**, **Serial Number**, and **LAN Access Code** (shown on the screen after enabling LAN Only Mode).
4. In the dashboard under Settings, select **Bambu Lab**, enter those values, and click Connect.

### Klipper / Moonraker

1. Find your Moonraker URL, usually `http://192.168.x.x:7125`.
2. In the dashboard under Settings, select **Klipper / Moonraker**, enter the URL, and click Connect.
3. If Moonraker has authentication enabled, also provide the Moonraker API key.

---

## Raspberry Pi

The Linux ARM build runs on single-board computers with no display or desktop environment.

Quick install:

```bash
chmod +x FoxTrack-Bridge-Linux-ARM
./FoxTrack-Bridge-Linux-ARM
```

The startup output lists the local IP addresses where the dashboard is available from other devices.

Run as a background service:

A systemd unit file is included at `linux/foxtrack-bridge.service`. To install it:

```bash
# Copy the binary to /usr/local/bin
sudo cp FoxTrack-Bridge-Linux-ARM /usr/local/bin/foxtrack-bridge

# Copy the service file
sudo cp linux/foxtrack-bridge.service /etc/systemd/system/

# Enable and start the service
sudo systemctl daemon-reload
sudo systemctl enable --now foxtrack-bridge
```

The service restarts on failure and starts on boot. The dashboard is available at `http://<raspberry-pi-ip>:8080` from any device on the same network.

---

## Building from source

Requires Go 1.24 or later.

Standard build (Windows, macOS, Linux x64):

```bash
go build -ldflags="-s -w" -o foxtrack-bridge .
```

Linux ARM (headless, no systray):

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags headless -ldflags="-s -w" -o foxtrack-bridge-arm .
```

Docker:

```bash
docker build --build-arg APP_VERSION=v2.2.0 -t foxtrack-bridge .
docker run -d --name foxtrack-bridge -p 8080:8080 -v foxtrack-bridge-data:/data --restart unless-stopped foxtrack-bridge
```

See [Docker documentation](docs/docker.md) for fork, branch, and server deployment examples.

Run tests:

```bash
go test -tags headless ./...
```

The `headless` tag runs the suite without the system tray dependency and includes the embedded-asset tests.

---

## Regenerating dashboard assets

The dashboard loads no external resources. Tailwind CSS, the Inter font, and the icons are committed under `web/` and embedded in the binary. Attribution is in [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md).

Tailwind: after changing classes in `web/ui.html`, regenerate `web/tailwind.css` with the standalone CLI (no Node required):

```bash
# one-time: download the pinned CLI (git-ignored)
curl -sL -o web/tailwindcss \
  https://github.com/tailwindlabs/tailwindcss/releases/download/v3.4.17/tailwindcss-linux-x64
chmod +x web/tailwindcss
./web/tailwindcss -c web/tailwind.config.js -i web/tailwind.input.css -o web/tailwind.css --minify
```

Fonts: `web/fonts/inter-latin.woff2` is a latin subset of Inter (variable, weights 400 to 600), served through `web/fonts.css`. Replace the file to update it.

Icons: `web/icons.css` holds the Font Awesome Free 6.4.0 icons used by `web/ui.html`, encoded as inline SVG masks. To add an icon, add a `.fa-<name>` rule with the glyph's SVG path (see the file header). `go test -tags headless ./...` fails if `web/ui.html` uses an icon that `web/icons.css` does not define.

The generated files are committed. No asset tooling runs during `go build` or the Docker build.

---

## Contributing

Contributions are welcome. If you find a bug, want a feature, or want to improve the docs, open an issue or a pull request. Small fixes and additions are appreciated, including edits to this README.

---

## License

MIT. Free to use, modify, and redistribute with attribution.
