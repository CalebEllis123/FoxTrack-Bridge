package mqtt

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"foxtrack-bridge/webhook"
)

type Printer struct {
	Name    string
	IP      string
	Serial  string
	LANCode string
	// Webhook credentials — injected from config at connection time
	WebhookURL string
	APIKey     string
}

// TelemetryData is the in-memory state per printer, also served by /api/status.
type TelemetryData struct {
	Status    string `json:"status"`
	FileName  string `json:"file_name"`
	Progress  int    `json:"progress"`
	Error     string `json:"error"`
	PrinterID string `json:"printer_id"`
	Timestamp int64  `json:"timestamp"`
}

// BambuReport is the top-level structure of a BambuLab MQTT report message.
type BambuReport struct {
	Print BambuPrint `json:"print"`
}

type BambuPrint struct {
	GcodeState       string `json:"gcode_state"`
	SubTaskName      string `json:"subtask_name"`
	McPercent        int    `json:"mc_percent"`
	McPrintErrorCode string `json:"mc_print_error_code"`
}

var (
	printerStates = make(map[string]*TelemetryData)
	stateMutex    sync.RWMutex
)

func GetPrinterState(name string) *TelemetryData {
	stateMutex.RLock()
	defer stateMutex.RUnlock()
	if s, ok := printerStates[name]; ok {
		return s
	}
	return &TelemetryData{Status: "disconnected", PrinterID: name}
}

func UpdatePrinterState(name string, t TelemetryData) {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	t.PrinterID = name
	t.Timestamp = time.Now().Unix()
	printerStates[name] = &t
}

func GetPrintersState() map[string]*TelemetryData {
	stateMutex.RLock()
	defer stateMutex.RUnlock()
	out := make(map[string]*TelemetryData, len(printerStates))
	for k, v := range printerStates {
		out[k] = v
	}
	return out
}

// ConnectPrinter starts a persistent background goroutine for one printer.
func ConnectPrinter(p Printer) {
	go func() {
		for {
			if err := connectAndListen(p); err != nil {
				log.Printf("[%s] disconnected: %v — retrying in 60s", p.Name, err)

				// Only push "disconnected" to FoxTrack if we haven't had a
				// real status update in the last 5 minutes. BambuLab printers
				// drop the MQTT connection frequently even while printing —
				// we don't want a brief dropout to overwrite the real status.
				state := GetPrinterState(p.Name)
				stale := time.Now().Unix()-state.Timestamp > 300
				if stale && p.WebhookURL != "" && p.APIKey != "" {
					_ = webhook.Send(p.APIKey, p.WebhookURL, webhook.Payload{
						PrinterName: p.Name,
						Serial:      p.Serial,
						Status:      "disconnected",
						Timestamp:   time.Now().Unix(),
					})
				}
			}
			time.Sleep(60 * time.Second)
		}
	}()
}

func connectAndListen(p Printer) error {
	broker := fmt.Sprintf("ssl://%s:8883", p.IP)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetUsername("bblp")
	opts.SetPassword(p.LANCode)
	opts.SetClientID(fmt.Sprintf("foxtrack-%s", p.Serial))
	opts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	opts.SetConnectTimeout(30 * time.Second)
	opts.SetAutoReconnect(false)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	log.Printf("[%s] MQTT connected", p.Name)

	topic := fmt.Sprintf("device/%s/report", p.Serial)
	subToken := client.Subscribe(topic, 0, makeHandler(p))
	if subToken.Wait() && subToken.Error() != nil {
		client.Disconnect(250)
		return subToken.Error()
	}
	log.Printf("[%s] subscribed to %s", p.Name, topic)

	// Mark as connected until first real message arrives
	UpdatePrinterState(p.Name, TelemetryData{Status: "connected", PrinterID: p.Name})

	// Push initial "connected" state to FoxTrack
	if p.WebhookURL != "" && p.APIKey != "" {
		_ = webhook.Send(p.APIKey, p.WebhookURL, webhook.Payload{
			PrinterName: p.Name,
			Serial:      p.Serial,
			Status:      "connected",
			Timestamp:   time.Now().Unix(),
		})
	}

	for client.IsConnected() {
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("connection dropped")
}

func makeHandler(p Printer) mqtt.MessageHandler {
	return func(_ mqtt.Client, msg mqtt.Message) {
		var report BambuReport
		if err := json.Unmarshal(msg.Payload(), &report); err != nil {
			log.Printf("[%s] parse error: %v", p.Name, err)
			return
		}

		pr := report.Print
		if pr.GcodeState == "" {
			return // not a status message
		}

		status := mapGcodeState(pr.GcodeState)
		t := TelemetryData{
			Status:   status,
			FileName: pr.SubTaskName,
			Progress: pr.McPercent,
			Error:    pr.McPrintErrorCode,
		}
		UpdatePrinterState(p.Name, t)

		log.Printf("[%s] %s | %s | %d%%", p.Name, status, pr.SubTaskName, pr.McPercent)

		// Push to FoxTrack webhook on every status message
		if p.WebhookURL != "" && p.APIKey != "" {
			payload := webhook.Payload{
				PrinterName: p.Name,
				Serial:      p.Serial,
				Status:      status,
				FileName:    pr.SubTaskName,
				Progress:    pr.McPercent,
				ErrorCode:   pr.McPrintErrorCode,
				Timestamp:   time.Now().Unix(),
			}
			if err := webhook.Send(p.APIKey, p.WebhookURL, payload); err != nil {
				log.Printf("[%s] webhook error: %v", p.Name, err)
			}
		} else {
			log.Printf("[%s] skipping webhook — API key or URL not configured", p.Name)
		}
	}
}

func mapGcodeState(s string) string {
	switch s {
	case "IDLE":
		return "idle"
	case "RUNNING":
		return "printing"
	case "PAUSE":
		return "paused"
	case "FINISH":
		return "finished"
	case "FAILED":
		return "error"
	default:
		return s
	}
}
