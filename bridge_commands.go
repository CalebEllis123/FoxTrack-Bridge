package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	mqttpkg "foxtrack-bridge/mqtt"
	"foxtrack-bridge/webhook"
)

type bridgeCommand struct {
	ID         string                 `json:"id"`
	ExternalID string                 `json:"external_id"`
	Command    string                 `json:"command"`
	Args       map[string]interface{} `json:"args"`
}

type bridgeCommandResult struct {
	CommandID    string `json:"command_id"`
	Status       string `json:"status"`                 // "done" or "failed"
	ErrorMessage string `json:"error_message,omitempty"` // driver error message when failed
}

// errNoMatchingPrinter is returned by executeBridgeCommand when no local printer
// matches the command's external_id. The poll loop skips the command silently —
// it may be intended for a different Bridge instance in a multi-Bridge workspace.
var errNoMatchingPrinter = errors.New("no matching printer")

var bridgeCommandsHTTPClient = &http.Client{Timeout: 8 * time.Second}

// pollBridgeCommands runs as a single long-lived goroutine. Every ~4 seconds it
// fetches pending commands from FoxTrack, executes each locally, and POSTs the
// result back. A missing or empty API key silently skips each cycle.
func pollBridgeCommands() {
	// Brief startup delay — lets the server bind and load its initial config.
	time.Sleep(3 * time.Second)

	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[bridge-commands] panic: %v", r)
				}
			}()

			configMutex.RLock()
			apiKey := ""
			if configStore != nil {
				apiKey = configStore.FoxTrack2APIKey
			}
			configMutex.RUnlock()

			if apiKey == "" {
				return
			}

			commands, err := fetchBridgeCommands(apiKey)
			if err != nil {
				log.Printf("[bridge-commands] fetch error: %v", err)
				return
			}

			for _, cmd := range commands {
				execErr := executeBridgeCommand(cmd)
				if errors.Is(execErr, errNoMatchingPrinter) {
					// Not our command — leave it pending for another Bridge instance.
					continue
				}
				result := bridgeCommandResult{CommandID: cmd.ID, Status: "done"}
				if execErr != nil {
					result.Status = "failed"
					result.ErrorMessage = execErr.Error()
					log.Printf("[bridge-commands] %s %s/%s failed: %v", cmd.ID, cmd.ExternalID, cmd.Command, execErr)
				} else {
					log.Printf("[bridge-commands] %s %s/%s done", cmd.ID, cmd.ExternalID, cmd.Command)
				}
				if err := ackBridgeCommand(apiKey, result); err != nil {
					log.Printf("[bridge-commands] ack error for %s: %v", cmd.ID, err)
				}
			}
		}()

		time.Sleep(4 * time.Second)
	}
}

func fetchBridgeCommands(apiKey string) ([]bridgeCommand, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", webhook.BridgeCommandsURLV2, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := bridgeCommandsHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result struct {
		Commands []bridgeCommand `json:"commands"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Commands, nil
}

func ackBridgeCommand(apiKey string, result bridgeCommandResult) error {
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", webhook.BridgeCommandsURLV2, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := bridgeCommandsHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// executeBridgeCommand resolves external_id to a local printer and dispatches
// the command directly to the appropriate driver — no HTTP roundtrip through
// the bridge's own server.
//
// Bambu printers: external_id matches p.Serial (not p.Name)
// Klipper printers: external_id matches p.Name
func executeBridgeCommand(cmd bridgeCommand) error {
	configMutex.RLock()
	var matchedName string
	var isBambu bool
	for _, p := range configStore.Printers {
		if isBambuPrinterConfig(p) {
			if p.Serial == cmd.ExternalID {
				matchedName = p.Name
				isBambu = true
				break
			}
		} else {
			if p.Name == cmd.ExternalID {
				matchedName = p.Name
				break
			}
		}
	}
	configMutex.RUnlock()

	if matchedName == "" {
		return errNoMatchingPrinter
	}

	if isBambu {
		return mqttpkg.SendCommandWithArgs(matchedName, cmd.Command, cmd.Args)
	}
	return lanCtrl.SendCommand(matchedName, cmd.Command, cmd.Args)
}
