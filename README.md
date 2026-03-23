<div align="center">
  <img src="https://raw.githubusercontent.com/FoxnTrain/FoxTrack-Bridge/main/assets/logo.png" alt="FoxTrack Bridge" width="80">

  # FoxTrack Bridge

  **Connect your local BambuLab printers to [FoxTrack](https://foxtrack.app) without cloud APIs.**

  [![Release](https://img.shields.io/github/v/release/FoxnTrain/FoxTrack-Bridge?style=flat-square&color=f97316)](https://github.com/FoxnTrain/FoxTrack-Bridge/releases/latest)
  [![License](https://img.shields.io/github/license/FoxnTrain/FoxTrack-Bridge?style=flat-square)](LICENSE)
  [![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-informational?style=flat-square)](https://github.com/FoxnTrain/FoxTrack-Bridge/releases/latest)

</div>

---

FoxTrack Bridge is a lightweight background app that runs on your local network and connects your BambuLab printers directly to FoxTrack using LAN mode — no BambuLab cloud, no rate limits, no account required.

It sits in your system tray, starts automatically at login, and silently relays real-time printer status (print state, file name, progress, errors) to your FoxTrack dashboard.

## Download

Get the latest version for your platform:

| Platform | Download |
|----------|----------|
| Windows | [FoxTrack-Bridge-Windows.exe](https://github.com/FoxnTrain/FoxTrack-Bridge/releases/latest/download/FoxTrack-Bridge-Windows.exe) |
| macOS (Apple Silicon) | [FoxTrack-Bridge-macOS-Apple-Silicon](https://github.com/FoxnTrain/FoxTrack-Bridge/releases/latest/download/FoxTrack-Bridge-macOS-Apple-Silicon) |
| macOS (Intel) | [FoxTrack-Bridge-macOS-Intel](https://github.com/FoxnTrain/FoxTrack-Bridge/releases/latest/download/FoxTrack-Bridge-macOS-Intel) |
| Linux | [FoxTrack-Bridge-Linux](https://github.com/FoxnTrain/FoxTrack-Bridge/releases/latest/download/FoxTrack-Bridge-Linux) |

## Setup

### 1. Prepare your printer

Your printer must be in **LAN Only Mode** to connect directly without BambuLab's cloud.

- On the printer touchscreen: **Settings → Network → LAN Only Mode**
- Or downgrade firmware to v1.08 or below

You'll need two things from your printer:
- **Serial Number** — found at Settings → Device Info on the printer screen
- **LAN Access Code** — found in the Bambu Handy app under your printer → Settings → LAN Mode

### 2. Get your FoxTrack credentials

In FoxTrack, go to **Settings → Integrations → Bambu Lab** and copy:
- Your **API Key**
- The **Webhook URL**

### 3. Run FoxTrack Bridge

**Windows:** Double-click the `.exe`. Windows may show a SmartScreen warning — click "More info" → "Run anyway". This is expected for unsigned apps.

**macOS:** Right-click the file → Open → Open. macOS will warn about an unidentified developer on first launch — this is expected for apps not distributed through the App Store.

**Linux:** Make the file executable and run it:
```bash
chmod +x FoxTrack-Bridge-Linux
./FoxTrack-Bridge-Linux
```

FoxTrack Bridge will appear in your system tray and automatically open `http://localhost:8080` in your browser.

### 4. Configure

In the browser dashboard that opens:

1. Paste your **API Key** and **Webhook URL** from FoxTrack and click **Save Settings**
2. Enter your printer's **Name**, **IP Address**, **Serial Number**, and **LAN Access Code**, then click **Add & Connect**
3. Your printer will appear in the list with a green **Connected** status

Your settings are saved automatically — you won't need to enter them again.

### 5. Enable start at login (optional)

Click the FoxTrack Bridge icon in your system tray and select **Enable: Start at Login**. The bridge will now start automatically in the background whenever you log in.

## How it works

```
BambuLab Printer  ──MQTT/LAN──►  FoxTrack Bridge  ──HTTPS──►  FoxTrack
(port 8883, TLS)                  (localhost:8080)              (Supabase)
```

- Connects to each printer's local MQTT broker on port 8883 using TLS
- Subscribes to `device/{serial}/report` for real-time telemetry
- Parses print state, file name, progress, and error codes from BambuLab's message format
- POSTs a JSON payload to your FoxTrack webhook on every status change
- Automatically reconnects if the printer goes offline

## Webhook payload

Every status change sends a POST to your FoxTrack webhook with:

```json
{
  "printer_name": "Watson",
  "serial": "01P00C591701791",
  "status": "printing",
  "file_name": "benchy.gcode",
  "progress": 47,
  "error_code": "",
  "timestamp": 1742123456
}
```

Possible `status` values: `idle` `printing` `paused` `finished` `error` `connected` `disconnected`

## Building from source

Requires Go 1.24+ and CGO.

**Linux also requires:**
```bash
sudo apt-get install gcc libgtk-3-dev libayatana-appindicator3-dev
```

**Build:**
```bash
git clone https://github.com/FoxnTrain/FoxTrack-Bridge.git
cd FoxTrack-Bridge
go mod tidy
go build -o foxtrack-bridge .
```

**Cross-compile for all platforms:**
```bash
chmod +x build-all.sh
./build-all.sh
```

## Troubleshooting

**Printer shows "Connecting…" and never connects**
- Confirm the printer is in LAN Only Mode
- Check the IP address is correct (find it on the printer screen or your router)
- Make sure the LAN access code matches what's shown in the Bambu app
- Verify the serial number is correct — it's case-sensitive

**FoxTrack isn't updating**
- Open `http://localhost:8080` and confirm the API Key and Webhook URL are saved
- Restart the bridge after saving credentials
- Test the webhook directly:
  ```bash
  curl -X POST YOUR_WEBHOOK_URL \
    -H "Authorization: Bearer YOUR_API_KEY" \
    -H "Content-Type: application/json" \
    -d '{"printer_name":"Test","serial":"123","status":"idle","file_name":"","progress":0,"error_code":"","timestamp":0}'
  ```

**macOS: "cannot be opened because the developer cannot be verified"**

Right-click the file → Open → Open. You only need to do this once.

Alternatively, remove the quarantine flag:
```bash
xattr -d com.apple.quarantine FoxTrack-Bridge-macOS-Apple-Silicon
```

## Privacy

FoxTrack Bridge runs entirely on your local network. It only communicates with:
- Your printers directly over LAN
- Your FoxTrack account via the webhook URL you provide

No data is sent to any third party. Your printer credentials (IP, serial, LAN code) are stored locally in `config/config.json` on your machine.

## License

MIT — see [LICENSE](LICENSE)

---

<div align="center">
  <sub>FoxTrack Bridge is open source. <a href="https://foxtrack.app">FoxTrack</a> is the 3D print management platform it connects to.</sub>
</div>
