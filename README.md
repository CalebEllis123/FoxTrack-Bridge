## FoxTrack Bridge

FoxTrack Bridge runs on the same local network as your printer and sends printer data to FoxTrack.

Today the Bridge is primarily built around Bambu Lab local LAN access. Creality and Prusa are still beta and not fully implemented yet.

## What it does

- Connects local printers to FoxTrack with your FoxTrack API token
- Shows printer status, current file, progress, temperatures, light state, and time remaining
- Sends telemetry to FoxTrack through the configured webhook
- Exposes local controls for supported printers: pause, resume, stop, light toggle, camera preview, and start print
- Opens a local dashboard at `http://localhost:8080`

## Current support

- Bambu Lab: supported
- Creality: setup flow exists, full printer integration still incomplete
- Prusa: setup flow exists, full printer integration still incomplete

## Downloads

Release builds are published for these targets only:

- Windows x64
- Windows Arm
- macOS Apple Silicon
- macOS Intel
- Linux x64

Get the latest build from the GitHub releases page:

- https://github.com/CalebEllis123/FoxTrack-Bridge/releases/latest

## Setup

### 1. In FoxTrack

- Open `Settings > Integrations > 3D Printer Integration`
- Copy your API token and webhook URL

### 2. On the machine running the Bridge

- Download the correct build for your operating system
- Launch the app
- Open `http://localhost:8080` if the dashboard does not open automatically

### 3. Add your printer

For Bambu Lab:

- Put the printer in LAN Only Mode
- Enable Developer Mode
- Find the printer IP address
- Find the Serial Number
- Find the LAN Access Code
- Enter those values into the Bridge dashboard

For Creality and Prusa:

- The UI can collect basic connection details
- Full telemetry and control support is not finished yet

## Notes about controls

- Pause, resume, stop, light toggle, and camera preview are wired into the Bridge for supported Bambu printers
- Start print is available as an advanced action and currently expects a printer-accessible file path or URL
- If a start command fails, the file path or printer firmware behavior is the first thing to check

## Development

The normal app uses a system tray.

For environments that cannot compile tray dependencies, there is also a headless dev build mode:

```bash
go build -tags headless .
```

Headless builds start the local web server without the tray UI.

## Build targets

The project is configured to build only these release targets:

- Windows x64
- Windows Arm64
- macOS Apple Silicon
- macOS Intel
- Linux x64
