package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// URL is the FoxTrack primary telemetry endpoint. Same for all users — do not expose in UI or config.
const URL = "https://vcnedcbtnhpmjgneahyk.supabase.co/functions/v1/bambu-local-relay"

// SyncURL is the FoxTrack printer registration endpoint, called on bridge startup.
const SyncURL = "https://vcnedcbtnhpmjgneahyk.supabase.co/functions/v1/bambu-local-sync"

var relayHTTPClient = &http.Client{Timeout: 10 * time.Second}

// retryItem holds a failed relay call that should be retried.
type retryItem struct {
	apiKey, webhookURL, serial, name string
	payload                          RelayPayload
	attempts                         int
	after                            time.Time
	createdAt                        time.Time
}

// retryQueue is a bounded channel for failed relay payloads awaiting retry.
// If full, new failures are dropped to prevent unbounded memory growth.
var retryQueue = make(chan retryItem, 64)

func init() {
	go retryWorker()
}

// retryWorker retries failed relay calls with exponential backoff (2s, 8s, 30s).
// Items older than 5 minutes are discarded.
func retryWorker() {
	for item := range retryQueue {
		wait := time.Until(item.after)
		if wait > 0 {
			time.Sleep(wait)
		}
		if time.Since(item.createdAt) > 5*time.Minute {
			log.Printf("[relay-retry] dropping stale payload for %s (created %s ago)", item.name, time.Since(item.createdAt).Round(time.Second))
			continue
		}
		if err := doSendRelay(item.apiKey, item.webhookURL, item.serial, item.name, item.payload); err != nil {
			backoffs := []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}
			if item.attempts < len(backoffs) {
				next := item
				next.attempts++
				next.after = time.Now().Add(backoffs[item.attempts])
				select {
				case retryQueue <- next:
				default:
					log.Printf("[relay-retry] queue full, dropping retry for %s", item.name)
				}
			} else {
				log.Printf("[relay-retry] giving up on payload for %s after %d attempts", item.name, item.attempts)
			}
		}
	}
}

// Payload is the JSON body sent to FoxTrack on every status change.
// Keep this in sync with the FoxTrack webhook endpoint schema.
type Payload struct {
	PrinterName   string  `json:"printer_name"`  // Matches the name in FoxTrack
	Serial        string  `json:"serial"`        // BambuLab serial number
	Status        string  `json:"status"`        // idle | printing | paused | finished | error | disconnected
	FileName      string  `json:"file_name"`     // Current file, empty if idle
	Progress      int     `json:"progress"`      // 0–100
	ErrorCode     string  `json:"error_code"`    // Empty string if no error
	Timestamp     int64   `json:"timestamp"`     // Unix seconds (UTC)
	NozzleTemp    float64 `json:"nozzle_temp"`   // Celsius
	NozzleTarget  float64 `json:"nozzle_target"` // Celsius
	BedTemp       float64 `json:"bed_temp"`      // Celsius
	BedTarget     float64 `json:"bed_target"`    // Celsius
	LightOn       bool    `json:"light_on"`
	TimeRemaining int     `json:"time_remaining"` // Minutes
}

// RelayPayload mirrors the Bambu relay payload shape.
type RelayPayload struct {
	Print RelayPrint `json:"print"`
}

type RelayPrint struct {
	GcodeState         string  `json:"gcode_state"`
	SubTaskName        string  `json:"subtask_name"`
	McPercent          int     `json:"mc_percent"`
	NozzleTemper       float64 `json:"nozzle_temper"`
	NozzleTargetTemper float64 `json:"nozzle_target_temper"`
	BedTemper          float64 `json:"bed_temper"`
	BedTargetTemper    float64 `json:"bed_target_temper"`
	ActiveExtruder     string  `json:"active_extruder,omitempty"`
}

// Send posts a Payload to the FoxTrack webhook URL.
// The API key is sent as a Bearer token.
func Send(apiKey, webhookURL string, p Payload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := relayHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("FoxTrack webhook returned HTTP %d", resp.StatusCode)
	}

	log.Printf("[webhook] Sent status for %s → %s (%d%%)", p.PrinterName, p.Status, p.Progress)
	return nil
}

// HistoryPayload is sent to FoxTrack when a print job completes.
type HistoryPayload struct {
	Type        string  `json:"type"` // always "print_complete"
	PrinterName string  `json:"printer_name"`
	Serial      string  `json:"serial"`
	FileName    string  `json:"file_name"`
	NozzleTemp  float64 `json:"nozzle_temp"`
	BedTemp     float64 `json:"bed_temp"`
	StartTime   int64   `json:"start_time"`
	EndTime     int64   `json:"end_time"`
	Duration    int64   `json:"duration"` // seconds
	Result      string  `json:"result"`   // "finished" | "cancelled" | "error"
	Timestamp   int64   `json:"timestamp"`
}

// SendHistory posts a HistoryPayload to the FoxTrack webhook URL.
func SendHistory(apiKey, webhookURL string, p HistoryPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := relayHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("FoxTrack history webhook returned HTTP %d", resp.StatusCode)
	}

	log.Printf("[history] Sent print_complete for %s (%s)", p.PrinterName, p.Result)
	return nil
}

// doSendRelay performs the actual HTTP POST for a relay payload.
func doSendRelay(apiKey, webhookURL, printerSerial, printerName string, p RelayPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("X-Printer-Serial", printerSerial)
	req.Header.Set("X-Printer-Name", printerName)

	resp, err := relayHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("FoxTrack relay webhook returned HTTP %d", resp.StatusCode)
	}
	log.Printf("[relay] Sent relay payload for %s (%d%%)", printerName, p.Print.McPercent)
	return nil
}

// SendRelay posts a relay payload and queues a retry with backoff on failure.
func SendRelay(apiKey, webhookURL, printerSerial, printerName string, p RelayPayload) error {
	if !strings.HasPrefix(strings.ToLower(webhookURL), "https://") {
		return fmt.Errorf("relay webhook URL must use HTTPS")
	}
	if err := doSendRelay(apiKey, webhookURL, printerSerial, printerName, p); err != nil {
		select {
		case retryQueue <- retryItem{
			apiKey:     apiKey,
			webhookURL: webhookURL,
			serial:     printerSerial,
			name:       printerName,
			payload:    p,
			attempts:   1,
			after:      time.Now().Add(2 * time.Second),
			createdAt:  time.Now(),
		}:
		default:
			log.Printf("[relay] retry queue full, dropping payload for %s", printerName)
		}
		return err
	}
	return nil
}
