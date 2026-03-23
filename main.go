package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/getlantern/systray"

	"foxtrack-bridge/startup"
)

func main() {
	systray.Run(onReady, onExit)
}

func onReady() {
	// Set tray icon and tooltip
	systray.SetIcon(iconBytes)
	systray.SetTooltip("FoxTrack Bridge")

	// Menu items
	mOpen := systray.AddMenuItem("Open Dashboard", "Open FoxTrack Bridge in browser")
	systray.AddSeparator()

	// Run at startup toggle
	startupLabel := startupMenuLabel()
	mStartup := systray.AddMenuItem(startupLabel, "Start FoxTrack Bridge automatically at login")
	systray.AddSeparator()

	mQuit := systray.AddMenuItem("Quit FoxTrack Bridge", "Stop the bridge and exit")

	// Start the HTTP server and MQTT connections in the background
	go StartServer()

	// Wait a moment then open the browser on first run so the user
	// can enter their API key and webhook URL straight away
	go func() {
		time.Sleep(1 * time.Second)
		openBrowser("http://localhost:8080")
	}()

	// Handle menu clicks
	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openBrowser("http://localhost:8080")

			case <-mStartup.ClickedCh:
				if startup.IsEnabled() {
					_ = startup.Disable()
					mStartup.SetTitle("Enable: Start at Login")
				} else {
					if err := startup.Enable(); err != nil {
						fmt.Printf("Could not enable startup: %v\n", err)
					} else {
						mStartup.SetTitle("Disable: Start at Login")
					}
				}

			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	// Clean shutdown — nothing extra needed; MQTT clients will drop.
}

func startupMenuLabel() string {
	if startup.IsEnabled() {
		return "Disable: Start at Login"
	}
	return "Enable: Start at Login"
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
