# FoxTrack Bridge

![GitHub release](https://img.shields.io/github/v/release/FoxesRCool1/FoxTrack-Bridge)
![Platforms](https://img.shields.io/badge/platforms-Windows%20%7C%20macOS%20%7C%20Linux-blue)
![License](https://img.shields.io/github/license/FoxesRCool1/FoxTrack-Bridge)

FoxTrack Bridge is a local 3D printer dashboard and integration server. It runs on any machine on the same network as your printers and gives you a real-time web dashboard for monitoring status, live camera feeds, print history, and basic printer controls. It also connects to [FoxTrack](https://foxtrack.studio/) so you can monitor your printers from anywhere.

### A note on AI

Among many other tools, AI was used to develop this program. If you have a problem with that, you may look for alternative programs made by people with more time or contribute human-made code.

---

## Features

- Live status for Bambu Lab and Klipper/Moonraker printers
- Print progress, temperatures, elapsed time, and ETA
- Live camera feed from printers & light toggle
- AMS filament slot display with slot colors and material types (Bambu Lab only)
- Print speed selector: Silent / Standard / Sport / Ludicrous (Bambu Lab only)
- Browser push notifications when prints finish, fail, or are paused
- Print history locally stored and available via the History view
- Automatic updates with staged installation
- Headless mode for servers, NAS devices, and Raspberry Pi

---

## Supported Platforms

| Platform | Notes |
|---|---|
| Windows x64 | 
| macOS Apple Silicon |
| macOS Intel |
| Linux x64 |
| Linux ARM (Headless) | Open `http://<ip>:8080` from any device on the network |

Download the latest builds from the [GitHub releases page](https://github.com/FoxesRCool1/FoxTrack-Bridge/releases/latest).

---

## Quick Start

**1. Run the Bridge**

Download the binary for your platform and launch it. The system tray icon appears on Windows, macOS, and Linux x64. On the Linux ARM build, run the binary from a terminal — the startup output lists the local addresses where the dashboard is available.

**2. Open the dashboard**

Navigate to [http://localhost:8080](http://localhost:8080) in your browser. From other devices on the same network, use the IP address shown in the startup output (e.g. `http://192.168.1.100:8080`).

**3. Connect to FoxTrack (optional)**

In Settings, paste your FoxTrack API key and click Save. Get your key from [FoxTrack Settings > Integrations](https://foxtrack.studio/settings).

**4. Add a printer**

In Printers, click "Add Printer", select the printer type, fill in the required fields, and click Connect.

---

## Adding Printers

### Bambu Lab

1. On the printer touchscreen, enable **LAN Only Mode** under Network settings.
2. Enable **Developer Mode** under About.
3. Note the **IP address**, **Serial Number**, and **LAN Access Code** (shown on the screen after enabling LAN Only Mode).
4. In the Bridge dashboard under Settings, select **Bambu Lab**, enter those values, and click Connect.

### Klipper / Moonraker

1. Find your Moonraker URL — usually `http://192.168.x.x:7125`.
2. In the Bridge dashboard under Settings, select **Klipper / Moonraker**, enter the URL, and click Connect.
3. If Moonraker has authentication enabled, also provide the Moonraker API key.

---

## Raspberry Pi

The Linux ARM build is designed for single-board computers — no display or desktop environment is required.

**Quick install:**

```bash
chmod +x FoxTrack-Bridge-Linux-ARM
./FoxTrack-Bridge-Linux-ARM
```

The startup output lists the local IP addresses where the dashboard is available from other devices.

**Run as a background service:**

A systemd unit file is included in the repository at `linux/foxtrack-bridge.service`. To install it:

```bash
# Copy the binary to /usr/local/bin
sudo cp FoxTrack-Bridge-Linux-ARM /usr/local/bin/foxtrack-bridge

# Copy the service file
sudo cp linux/foxtrack-bridge.service /etc/systemd/system/

# Enable and start the service
sudo systemctl daemon-reload
sudo systemctl enable --now foxtrack-bridge
```

The service restarts automatically on failure and starts on boot. The dashboard is available at `http://<raspberry-pi-ip>:8080` from any device on the same network.

---

## Building from Source

Requires Go 1.24 or later.

**Standard build (Windows, macOS, Linux x64):**

```bash
go build -ldflags="-s -w" -o foxtrack-bridge .
```

**Linux ARM (headless, no systray):**

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags headless -ldflags="-s -w" -o foxtrack-bridge-arm .
```

**Run tests:**

```bash
go test ./...
```

---

## License

MIT License. See [LICENSE](LICENSE) for details.
