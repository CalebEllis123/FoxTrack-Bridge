package mqtt

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"foxtrack-bridge/history"
	"foxtrack-bridge/webhook"
)

type Printer struct {
	Name    string
	IP      string
	Serial  string
	LANCode string
	APIKey  string
}

// AmsSlot holds the state of a single AMS filament tray.
type AmsSlot struct {
	Slot     int    `json:"slot"`      // 0-indexed tray position
	Color    string `json:"color"`     // 6-char hex, e.g. "FF0000"
	Material string `json:"material"`  // e.g. "PLA", "PETG"
	Active   bool   `json:"active"`    // currently printing from this slot
}

type TelemetryData struct {
	Status        string    `json:"status"`
	FileName      string    `json:"file_name"`
	Progress      int       `json:"progress"`
	Error         string    `json:"error"`
	PrinterID     string    `json:"printer_id"`
	Timestamp     int64     `json:"timestamp"`
	NozzleTemp    float64   `json:"nozzle_temp"`
	NozzleTarget  float64   `json:"nozzle_target"`
	BedTemp       float64   `json:"bed_temp"`
	BedTarget     float64   `json:"bed_target"`
	LightOn       bool      `json:"light_on"`
	TimeRemaining int       `json:"time_remaining"` // minutes
	SpeedLevel    int       `json:"speed_level"`    // 1=Silent 2=Standard 3=Sport 4=Ludicrous
	AMS           []AmsSlot `json:"ams,omitempty"`  // nil when no AMS
}

type BambuReport struct {
	Print BambuPrint `json:"print"`
}

type BambuPrint struct {
	GcodeState         string  `json:"gcode_state"`
	SubTaskName        string  `json:"subtask_name"`
	McPercent          int     `json:"mc_percent"`
	McPrintErrorCode   string  `json:"mc_print_error_code"`
	NozzleTemper       float64 `json:"nozzle_temper"`
	NozzleTargetTemper float64 `json:"nozzle_target_temper"`
	BedTemper          float64 `json:"bed_temper"`
	BedTargetTemper    float64 `json:"bed_target_temper"`
	Lights             []struct {
		Node string `json:"node"`
		Mode string `json:"mode"`
	} `json:"lights_report"`
	McRemainingTime int    `json:"mc_remaining_time"`
	SpdLvl          int    `json:"spd_lvl"`
	AmsStatus       int    `json:"ams_status"`
	Ams             *struct {
		AMS []struct {
			Tray []struct {
				ID       string `json:"id"`
				Color    string `json:"tray_color"` // 8-char RRGGBBAA
				Material string `json:"tray_type"`
			} `json:"tray"`
		} `json:"ams"`
		TrayNow string `json:"tray_now"` // currently loaded tray ("0"-"3"), "255" = none/external
	} `json:"ams"`
}

var (
	printerStates  = make(map[string]*TelemetryData)
	printerClients = make(map[string]mqtt.Client) // for sending control commands
	stateMutex     sync.RWMutex
	clientMutex    sync.RWMutex
	sequenceID     uint64
	sequenceMu     sync.Mutex

	// managedPrinters tracks which printer names have an active connection goroutine.
	// Prevents duplicate goroutines when ConnectPrinter is called redundantly.
	managedPrinters   = make(map[string]bool)
	managedPrintersMu sync.Mutex
)

func nextSequenceID() string {
	sequenceMu.Lock()
	defer sequenceMu.Unlock()
	sequenceID++
	return fmt.Sprintf("%d", sequenceID)
}

type printSession struct {
	StartTime  int64
	FileName   string
	NozzleTemp float64
	BedTemp    float64
}

var (
	activeSessions = make(map[string]*printSession)
	sessionMu      sync.Mutex
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

// SendCommand sends a control command to a named printer via MQTT.
// command is one of: "pause", "resume", "stop", "light_on", "light_off", "start", "toggle_light"
func SendCommand(printerName, command string) error {
	return SendCommandWithArgs(printerName, command, nil)
}

// SendCommandWithArgs sends a command with optional arguments.
// For start prints, accepted args are:
// - file_name or file: printer-local path or URL
// - url: explicit URL
// - start_command: optional override for printer command name (default: project_file)
func SendCommandWithArgs(printerName, command string, args map[string]interface{}) error {
	clientMutex.RLock()
	client, ok := printerClients[printerName]
	clientMutex.RUnlock()

	if !ok {
		return fmt.Errorf("printer %q is not connected (no active MQTT session)", printerName)
	}
	if !client.IsConnected() {
		return fmt.Errorf("printer %q MQTT session dropped — reconnecting, try again in a moment", printerName)
	}

	// Find serial for the topic
	stateMutex.RLock()
	state, hasState := printerStates[printerName]
	stateMutex.RUnlock()
	_ = hasState

	// We store serial in PrinterID only if set — look it up from connected printer map
	serial := getSerial(printerName)
	if serial == "" {
		return fmt.Errorf("serial not found for %q", printerName)
	}
	_ = state

	topic := fmt.Sprintf("device/%s/request", serial)
	var payload string

	// ledPayload builds the ledctrl JSON for a single node with a fresh sequence_id.
	ledPayload := func(node, mode string) string {
		b, _ := json.Marshal(map[string]interface{}{
			"system": map[string]interface{}{
				"sequence_id":   nextSequenceID(),
				"command":       "ledctrl",
				"led_node":      node,
				"led_mode":      mode,
				"led_on_time":   500,
				"led_off_time":  500,
				"loop_times":    0,
				"interval_time": 0,
			},
		})
		return string(b)
	}

	// sendLights publishes ledctrl to both chamber_light and chamber_light2.
	// QoS 0: BambuLab's embedded broker does not reliably PUBACK QoS-1 publishes
	// on the request topic, so QoS 1 always times out. Fire-and-forget is safe on LAN.
	sendLights := func(mode string) error {
		for _, node := range []string{"chamber_light", "chamber_light2"} {
			p := ledPayload(node, mode)
			tok := client.Publish(topic, 0, false, p)
			if err := tok.Error(); err != nil {
				return fmt.Errorf("light command failed for %q: %w", printerName, err)
			}
		}
		return nil
	}

	// applyLightResult applies optimistic state and requests a fresh pushall.
	applyLightResult := func(wantOn bool) {
		stateMutex.Lock()
		if s, ok := printerStates[printerName]; ok {
			s.LightOn = wantOn
		}
		stateMutex.Unlock()
		go sendPushall(client, printerName, topic)
	}

	switch command {
	case "pause":
		payload = `{"print":{"sequence_id":"0","command":"pause"}}`
	case "resume":
		payload = `{"print":{"sequence_id":"0","command":"resume"}}`
	case "stop":
		payload = `{"print":{"sequence_id":"0","command":"stop"}}`

	case "light_on":
		if err := sendLights("on"); err != nil {
			return err
		}
		log.Printf("[%s] sent command: light_on", printerName)
		applyLightResult(true)
		return nil

	case "light_off":
		if err := sendLights("off"); err != nil {
			return err
		}
		log.Printf("[%s] sent command: light_off", printerName)
		applyLightResult(false)
		return nil

	case "light":
		on := getBoolArg(args, "on")
		if on == nil {
			onVal := !(state != nil && state.LightOn)
			on = &onVal
		}
		mode := "off"
		if *on {
			mode = "on"
		}
		if err := sendLights(mode); err != nil {
			return err
		}
		log.Printf("[%s] sent command: light (%s)", printerName, mode)
		applyLightResult(*on)
		return nil

	case "toggle_light":
		wantOn := !(state != nil && state.LightOn)
		mode := "off"
		if wantOn {
			mode = "on"
		}
		if err := sendLights(mode); err != nil {
			return err
		}
		log.Printf("[%s] sent command: toggle_light (%s)", printerName, mode)
		applyLightResult(wantOn)
		return nil

	case "print_speed":
		level := getStringArg(args, "level")
		if level == "" {
			level = "2"
		}
		b, err := json.Marshal(map[string]interface{}{
			"print": map[string]interface{}{
				"sequence_id": "0",
				"command":     "print_speed",
				"param":       level,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to build print_speed payload: %w", err)
		}
		payload = string(b)

	case "gcode":
		line := getStringArg(args, "line")
		if line == "" {
			return fmt.Errorf("gcode command requires args[\"line\"]")
		}
		b, err := json.Marshal(map[string]interface{}{
			"print": map[string]interface{}{
				"sequence_id": "0",
				"command":     "gcode_line",
				"param":       line,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to build gcode payload: %w", err)
		}
		payload = string(b)

	case "start":
		target := getStringArg(args, "url")
		if target == "" {
			target = getStringArg(args, "file_name")
		}
		if target == "" {
			target = getStringArg(args, "file")
		}
		if target == "" {
			return fmt.Errorf("start command requires one of: url, file_name, or file")
		}
		if !strings.Contains(target, "://") {
			target = "file:///" + strings.TrimPrefix(target, "/")
		}
		startCommand := getStringArg(args, "start_command")
		if startCommand == "" {
			startCommand = "project_file"
		}
		b, err := json.Marshal(map[string]interface{}{
			"print": map[string]interface{}{
				"sequence_id": "0",
				"command":     startCommand,
				"url":         target,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to build start payload: %w", err)
		}
		payload = string(b)

	case "ams_load":
		amsId, _ := strconv.Atoi(getStringArg(args, "ams"))
		trayId, _ := strconv.Atoi(getStringArg(args, "tray"))
		// Use actual nozzle temp so the printer knows whether it needs to heat up first.
		currTemp := 25
		if st := GetPrinterState(printerName); st != nil && st.NozzleTemp > 0 {
			currTemp = int(st.NozzleTemp)
		}
		b, err := json.Marshal(map[string]interface{}{
			"print": map[string]interface{}{
				"sequence_id": nextSequenceID(),
				"command":     "ams_change_filament",
				"tar_ams":     amsId,
				"tar_tray":    trayId,
				"curr_temp":   currTemp,
				"tar_temp":    220,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to build ams_load payload: %w", err)
		}
		payload = string(b)

	case "ams_unload":
		// "param" key is required — BambuLab broker silently drops the message without it.
		b, err := json.Marshal(map[string]interface{}{
			"print": map[string]interface{}{
				"sequence_id": nextSequenceID(),
				"command":     "ams_unload_filament",
				"param":       "",
			},
		})
		if err != nil {
			return fmt.Errorf("failed to build ams_unload payload: %w", err)
		}
		payload = string(b)

	case "ams_filament_setting":
		amsId, _ := strconv.Atoi(getStringArg(args, "ams"))
		trayId, _ := strconv.Atoi(getStringArg(args, "tray"))
		color := getStringArg(args, "color")
		if len(color) == 6 {
			color = color + "FF"
		} else if len(color) != 8 {
			color = "FFFFFFFF"
		}
		material := strings.ToUpper(getStringArg(args, "material"))
		if material == "" {
			material = "PLA"
		}
		b, err := json.Marshal(map[string]interface{}{
			"print": map[string]interface{}{
				"sequence_id":     nextSequenceID(),
				"command":         "ams_filament_setting",
				"ams_id":          amsId,
				"tray_id":         trayId,
				"tray_color":      strings.ToUpper(color),
				"nozzle_temp_min": amsTempMin(material),
				"nozzle_temp_max": amsTempMax(material),
				"tray_type":       material,
				"setting_id":      "",
				"ctype":           0,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to build ams_filament_setting payload: %w", err)
		}
		payload = string(b)

	default:
		return fmt.Errorf("unknown command: %q", command)
	}

	// QoS 0: same reasoning as sendLights above — BambuLab broker drops QoS-1 PUBACKs.
	token := client.Publish(topic, 0, false, payload)
	if err := token.Error(); err != nil {
		return fmt.Errorf("command %q failed for printer %q: %w", command, printerName, err)
	}
	log.Printf("[%s] sent command: %s", printerName, command)

	// After filament settings change, the printer won't broadcast an update automatically.
	// Send a pushall after a short delay so the UI reflects the new colors/material.
	if command == "ams_filament_setting" || command == "ams_unload" || command == "ams_load" {
		go func() {
			time.Sleep(400 * time.Millisecond)
			sendPushall(client, printerName, topic)
		}()
	}

	return nil
}

func amsTempMin(material string) int {
	switch material {
	case "PETG", "TPU":
		return 220
	case "ABS", "ASA":
		return 240
	case "PA", "PC":
		return 260
	default:
		return 190
	}
}

func amsTempMax(material string) int {
	switch material {
	case "PETG", "TPU":
		return 250
	case "ABS", "ASA":
		return 270
	case "PA", "PC":
		return 290
	default:
		return 230
	}
}

func getStringArg(args map[string]interface{}, key string) string {
	if args == nil {
		return ""
	}
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func getBoolArg(args map[string]interface{}, key string) *bool {
	if args == nil {
		return nil
	}
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	b, ok := v.(bool)
	if !ok {
		return nil
	}
	return &b
}

// printerSerials maps name → serial for control commands
var (
	printerSerials = make(map[string]string)
	serialMutex    sync.RWMutex
)

func setSerial(name, serial string) {
	serialMutex.Lock()
	defer serialMutex.Unlock()
	printerSerials[name] = serial
}

func getSerial(name string) string {
	serialMutex.RLock()
	defer serialMutex.RUnlock()
	return printerSerials[name]
}

// DisconnectPrinter stops the management goroutine for the named printer so that
// a subsequent ConnectPrinter call can start a fresh one (e.g. after IP/LAN code change).
func DisconnectPrinter(name string) {
	clientMutex.Lock()
	if c, ok := printerClients[name]; ok {
		c.Disconnect(250)
		delete(printerClients, name)
	}
	clientMutex.Unlock()

	managedPrintersMu.Lock()
	delete(managedPrinters, name)
	managedPrintersMu.Unlock()
}

func ConnectPrinter(p Printer) {
	setSerial(p.Name, p.Serial)

	// Guard against duplicate goroutines. If a management goroutine is already
	// running for this printer (e.g. syncPrinterConnections called redundantly on
	// settings save), do nothing — the existing goroutine handles reconnects.
	managedPrintersMu.Lock()
	if managedPrinters[p.Name] {
		managedPrintersMu.Unlock()
		return
	}
	managedPrinters[p.Name] = true
	managedPrintersMu.Unlock()

	go func() {
		defer func() {
			managedPrintersMu.Lock()
			delete(managedPrinters, p.Name)
			managedPrintersMu.Unlock()
		}()
		for {
			err := connectAndListen(p)
			if err != nil {
				log.Printf("[%s] disconnected: %v — retrying in 15s", p.Name, err)
			}

			// Remove client immediately so commands fail fast rather than
			// hitting a disconnected client.
			clientMutex.Lock()
			delete(printerClients, p.Name)
			clientMutex.Unlock()

			// Always update status to disconnected right away so the UI
			// disables controls instead of showing stale "idle/printing" state.
			UpdatePrinterState(p.Name, TelemetryData{
				Status:    "disconnected",
				PrinterID: p.Name,
				Timestamp: time.Now().Unix(),
			})

			time.Sleep(15 * time.Second)
		}
	}()
}

func connectAndListen(p Printer) error {
	broker := fmt.Sprintf("ssl://%s:8883", p.IP)
	done := make(chan struct{})

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetUsername("bblp")
	opts.SetPassword(p.LANCode)
	opts.SetClientID(fmt.Sprintf("foxtrack-%s-%d", p.Serial, time.Now().UnixNano()))
	opts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	opts.SetConnectTimeout(10 * time.Second)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetAutoReconnect(false)
	opts.SetCleanSession(true)

	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		log.Printf("[%s] connection lost: %v", p.Name, err)
		close(done)
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if token.WaitTimeout(15*time.Second) && token.Error() != nil {
		return token.Error()
	}
	if !client.IsConnected() {
		return fmt.Errorf("connect timed out")
	}
	log.Printf("[%s] MQTT connected", p.Name)

	// Store client for control commands
	clientMutex.Lock()
	printerClients[p.Name] = client
	clientMutex.Unlock()

	topic := fmt.Sprintf("device/%s/report", p.Serial)
	subToken := client.Subscribe(topic, 0, makeHandler(p))
	if subToken.WaitTimeout(10*time.Second) && subToken.Error() != nil {
		client.Disconnect(250)
		return subToken.Error()
	}
	log.Printf("[%s] subscribed to %s", p.Name, topic)

	UpdatePrinterState(p.Name, TelemetryData{
		Status:    "connected",
		PrinterID: p.Name,
		Timestamp: time.Now().Unix(),
	})

	requestTopic := fmt.Sprintf("device/%s/request", p.Serial)
	sendPushall(client, p.Name, requestTopic)

	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if client.IsConnected() {
					sendPushall(client, p.Name, requestTopic)
				}
			}
		}
	}()

	<-done
	client.Disconnect(250)
	return fmt.Errorf("connection lost")
}

func sendPushall(client mqtt.Client, printerName, requestTopic string) {
	payload := `{"pushing": {"sequence_id": "0", "command": "pushall"}}`
	client.Publish(requestTopic, 0, false, payload)
	log.Printf("[%s] sent pushall", printerName)
}

func makeHandler(p Printer) mqtt.MessageHandler {
	return func(_ mqtt.Client, msg mqtt.Message) {
		var report BambuReport
		if err := json.Unmarshal(msg.Payload(), &report); err != nil {
			return
		}

		pr := report.Print

		// Ignore messages that carry no print or system data at all.
		hasData := pr.GcodeState != "" || pr.NozzleTemper != 0 || pr.BedTemper != 0 || len(pr.Lights) > 0 || pr.Ams != nil || pr.SpdLvl != 0
		if !hasData {
			return
		}

		// Preserve previous state for fields absent in incremental updates.
		prev := GetPrinterState(p.Name)

		status := prev.Status
		if pr.GcodeState != "" {
			status = mapGcodeState(pr.GcodeState)
		}

		// Preserve file name across partial updates — BambuLab omits subtask_name
		// in incremental messages even when a print is running.
		fileName := prev.FileName
		if pr.SubTaskName != "" {
			fileName = pr.SubTaskName
		}
		// Clear file name when the printer is not printing.
		if status != "printing" && status != "paused" {
			fileName = ""
		}

		// Preserve temps when they are absent (reported as 0) in partial updates.
		nozzleTemp := prev.NozzleTemp
		if pr.NozzleTemper != 0 {
			nozzleTemp = pr.NozzleTemper
		}
		nozzleTarget := prev.NozzleTarget
		if pr.NozzleTargetTemper != 0 {
			nozzleTarget = pr.NozzleTargetTemper
		}
		bedTemp := prev.BedTemp
		if pr.BedTemper != 0 {
			bedTemp = pr.BedTemper
		}
		bedTarget := prev.BedTarget
		if pr.BedTargetTemper != 0 {
			bedTarget = pr.BedTargetTemper
		}
		progress := prev.Progress
		if pr.McPercent != 0 {
			progress = pr.McPercent
		}
		timeRemaining := prev.TimeRemaining
		if pr.McRemainingTime != 0 {
			timeRemaining = pr.McRemainingTime
		}
		// Clear transient print fields when the printer is no longer actively printing.
		if status != "printing" && status != "paused" {
			timeRemaining = 0
			progress = 0
		}

		// Parse light state; preserve previous value when lights_report is absent.
		// Printers report either "chamber_light" or "work_light" depending on model.
		lightOn := prev.LightOn
		if len(pr.Lights) > 0 {
			lightOn = false
			for _, l := range pr.Lights {
				if (l.Node == "chamber_light" || l.Node == "work_light") && l.Mode == "on" {
					lightOn = true
				}
			}
		}

		// Speed level: preserve previous when absent (spd_lvl==0 means not reported).
		speedLevel := prev.SpeedLevel
		if pr.SpdLvl != 0 {
			speedLevel = pr.SpdLvl
		}

		// AMS: parse slot data; preserve previous when ams field is absent.
		amsSlots := prev.AMS
		if pr.Ams != nil {
			amsSlots = nil
			// tray_now is the globally loaded tray index ("0"-"3"), or "255"/"254" when none.
			trayNow := -1
			if n, err := strconv.Atoi(pr.Ams.TrayNow); err == nil && n < 254 {
				trayNow = n
			}
			for amsIdx, unit := range pr.Ams.AMS {
				for i, tray := range unit.Tray {
					globalSlot := amsIdx*4 + i
					color := tray.Color
					if len(color) >= 6 {
						color = color[:6] // strip alpha channel
					}
					amsSlots = append(amsSlots, AmsSlot{
						Slot:     globalSlot,
						Color:    color,
						Material: tray.Material,
						Active:   globalSlot == trayNow,
					})
				}
			}
		}

		t := TelemetryData{
			Status:        status,
			FileName:      fileName,
			Progress:      progress,
			Error:         pr.McPrintErrorCode,
			NozzleTemp:    nozzleTemp,
			NozzleTarget:  nozzleTarget,
			BedTemp:       bedTemp,
			BedTarget:     bedTarget,
			LightOn:       lightOn,
			TimeRemaining: timeRemaining,
			SpeedLevel:    speedLevel,
			AMS:           amsSlots,
		}

		// Track print sessions for history.
		sessionMu.Lock()
		sess := activeSessions[p.Name]
		if prev.Status != "printing" && status == "printing" {
			activeSessions[p.Name] = &printSession{
				StartTime:  time.Now().Unix(),
				FileName:   pr.SubTaskName,
				NozzleTemp: pr.NozzleTemper,
				BedTemp:    pr.BedTemper,
			}
		} else if prev.Status == "printing" && status != "printing" && sess != nil {
			now := time.Now().Unix()
			result := status
			if result != "finished" && result != "error" {
				result = "cancelled"
			}
			rec := history.Record{
				PrinterName: p.Name,
				FileName:    sess.FileName,
				NozzleTemp:  sess.NozzleTemp,
				BedTemp:     sess.BedTemp,
				StartTime:   sess.StartTime,
				EndTime:     now,
				Duration:    now - sess.StartTime,
				Result:      result,
			}
			delete(activeSessions, p.Name)
			sessionMu.Unlock()
			go func() {
				if err := history.Append(rec); err != nil {
					log.Printf("[%s] history write error: %v", p.Name, err)
				}
			}()
		} else {
			sessionMu.Unlock()
		}

		UpdatePrinterState(p.Name, t)

		log.Printf("[%s] %s | %q | %d%% | nozzle:%.0f°C bed:%.0f°C",
			p.Name, status, fileName, progress,
			nozzleTemp, bedTemp)

		if p.APIKey == "" {
			log.Printf("[%s] skipping webhook — API key not configured", p.Name)
			return
		}

		go func() {
			relayPayload := webhook.RelayPayload{
				Print: webhook.RelayPrint{
					GcodeState:         pr.GcodeState,
					SubTaskName:        fileName,
					McPercent:          progress,
					NozzleTemper:       nozzleTemp,
					NozzleTargetTemper: nozzleTarget,
					BedTemper:          bedTemp,
					BedTargetTemper:    bedTarget,
				},
			}
			if err := webhook.SendRelay(p.APIKey, webhook.URL, p.Serial, p.Name, relayPayload); err != nil {
				log.Printf("[%s] webhook error: %v", p.Name, err)
			}
		}()
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
