package lan

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	configpkg "foxtrack-bridge/config"
	mqttpkg "foxtrack-bridge/mqtt"
	"foxtrack-bridge/webhook"
)

type Controller struct {
	mu        sync.RWMutex
	states    map[string]*mqttpkg.TelemetryData
	printers  map[string]configpkg.Printer
	cancelers map[string]chan struct{}
}

func NewController() *Controller {
	return &Controller{
		states:    map[string]*mqttpkg.TelemetryData{},
		printers:  map[string]configpkg.Printer{},
		cancelers: map[string]chan struct{}{},
	}
}

func (c *Controller) SyncPrinters(printers []configpkg.Printer, webhookURL, foxAPIKey string) {
	keep := make(map[string]bool)
	for _, p := range printers {
		if p.Brand == "bambu" || p.Brand == "" {
			continue
		}
		keep[p.Name] = true
		c.AddOrUpdatePrinter(p, webhookURL, foxAPIKey)
	}

	c.mu.Lock()
	for name, stop := range c.cancelers {
		if !keep[name] {
			close(stop)
			delete(c.cancelers, name)
			delete(c.printers, name)
			delete(c.states, name)
		}
	}
	c.mu.Unlock()
}

func (c *Controller) AddOrUpdatePrinter(p configpkg.Printer, webhookURL, foxAPIKey string) {
	if p.Name == "" {
		return
	}
	if p.Brand == "bambu" || p.Brand == "" {
		return
	}

	c.mu.Lock()
	if stop, ok := c.cancelers[p.Name]; ok {
		close(stop)
		delete(c.cancelers, p.Name)
	}
	stop := make(chan struct{})
	c.cancelers[p.Name] = stop
	c.printers[p.Name] = p
	c.mu.Unlock()

	go c.pollLoop(p, webhookURL, foxAPIKey, stop)
}

func (c *Controller) RemovePrinter(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if stop, ok := c.cancelers[name]; ok {
		close(stop)
		delete(c.cancelers, name)
	}
	delete(c.printers, name)
	delete(c.states, name)
}

func (c *Controller) GetStates() map[string]*mqttpkg.TelemetryData {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]*mqttpkg.TelemetryData, len(c.states))
	for k, v := range c.states {
		copyV := *v
		out[k] = &copyV
	}
	return out
}

func (c *Controller) SendCommand(name, command string, args map[string]interface{}) error {
	c.mu.RLock()
	p, ok := c.printers[name]
	state := c.states[name]
	c.mu.RUnlock()
	if !ok {
		return fmt.Errorf("printer %q not found", name)
	}

	switch p.Brand {
	case "creality":
		return c.sendCrealityCommand(p, state, command, args)
	case "prusa":
		return c.sendPrusaCommand(p, command)
	default:
		return fmt.Errorf("brand %q not supported", p.Brand)
	}
}

func (c *Controller) ProxyCamera(w http.ResponseWriter, _ *http.Request, name string) error {
	c.mu.RLock()
	p, ok := c.printers[name]
	c.mu.RUnlock()
	if !ok {
		return fmt.Errorf("printer not found")
	}

	candidates := cameraCandidates(p)
	client := &http.Client{Timeout: 8 * time.Second, Transport: insecureTransport()}
	for _, candidate := range candidates {
		req, err := http.NewRequest("GET", candidate, nil)
		if err != nil {
			continue
		}
		if p.Brand == "prusa" && p.APIKey != "" {
			req.Header.Set("X-Api-Key", p.APIKey)
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			continue
		}

		defer resp.Body.Close()
		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "image/jpeg"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, resp.Body)
		return nil
	}

	return fmt.Errorf("camera unavailable")
}

func (c *Controller) pollLoop(p configpkg.Printer, webhookURL, foxAPIKey string, stop <-chan struct{}) {
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		default:
		}

		var (
			t   mqttpkg.TelemetryData
			err error
		)
		switch p.Brand {
		case "creality":
			t, err = fetchCrealityTelemetry(p)
		case "prusa":
			t, err = fetchPrusaTelemetry(p)
		default:
			err = fmt.Errorf("unsupported brand %q", p.Brand)
		}
		if err != nil {
			t = mqttpkg.TelemetryData{Status: "disconnected", Error: err.Error()}
		}

		t.PrinterID = p.Name
		t.Timestamp = time.Now().Unix()

		c.mu.Lock()
		prev := c.states[p.Name]
		c.states[p.Name] = &t
		c.mu.Unlock()

		if webhookURL != "" && foxAPIKey != "" && shouldSendWebhook(prev, &t) {
			payload := webhook.Payload{
				PrinterName:   p.Name,
				Serial:        p.Serial,
				Status:        t.Status,
				FileName:      t.FileName,
				Progress:      t.Progress,
				ErrorCode:     t.Error,
				Timestamp:     t.Timestamp,
				NozzleTemp:    t.NozzleTemp,
				NozzleTarget:  t.NozzleTarget,
				BedTemp:       t.BedTemp,
				BedTarget:     t.BedTarget,
				LightOn:       t.LightOn,
				TimeRemaining: t.TimeRemaining,
			}
			if err := webhook.Send(foxAPIKey, webhookURL, payload); err != nil {
				log.Printf("[%s] webhook error: %v", p.Name, err)
			}
		}

		select {
		case <-stop:
			return
		case <-ticker.C:
		}
	}
}

func shouldSendWebhook(prev, curr *mqttpkg.TelemetryData) bool {
	if prev == nil {
		return true
	}
	return prev.Status != curr.Status ||
		prev.FileName != curr.FileName ||
		prev.Progress != curr.Progress ||
		prev.Error != curr.Error ||
		int(prev.NozzleTemp) != int(curr.NozzleTemp) ||
		int(prev.BedTemp) != int(curr.BedTemp) ||
		prev.LightOn != curr.LightOn ||
		prev.TimeRemaining != curr.TimeRemaining
}

func sendJSONRequest(client *http.Client, method, u string, headers map[string]string, body io.Reader) (map[string]interface{}, error) {
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func fetchCrealityTelemetry(p configpkg.Printer) (mqttpkg.TelemetryData, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	u := fmt.Sprintf("http://%s/printer/objects/query?print_stats&extruder&heater_bed&virtual_sdcard", p.IP)
	m, err := sendJSONRequest(client, "GET", u, nil, nil)
	if err != nil {
		return mqttpkg.TelemetryData{}, err
	}

	status := nestedMap(m, "result", "status")
	printStats := nestedMapAny(status, "print_stats")
	extruder := nestedMapAny(status, "extruder")
	bed := nestedMapAny(status, "heater_bed")
	virtualSD := nestedMapAny(status, "virtual_sdcard")

	stateRaw := lowerString(stringAny(anyFromMap(printStats, "state")))
	state := mapCrealityState(stateRaw)
	fileName := stringAny(anyFromMap(printStats, "filename"))
	progressF := floatAny(anyFromMap(virtualSD, "progress"))
	progress := int(progressF * 100)
	if progress > 100 {
		progress = 100
	}
	if progress < 0 {
		progress = 0
	}

	printDuration := int(floatAny(anyFromMap(printStats, "print_duration")))
	remaining := 0
	if progress > 0 && progress < 100 && printDuration > 0 {
		total := int(float64(printDuration) / (float64(progress) / 100.0))
		if total > printDuration {
			remaining = (total - printDuration) / 60
		}
	}

	return mqttpkg.TelemetryData{
		Status:        state,
		FileName:      fileName,
		Progress:      progress,
		NozzleTemp:    floatAny(anyFromMap(extruder, "temperature")),
		NozzleTarget:  floatAny(anyFromMap(extruder, "target")),
		BedTemp:       floatAny(anyFromMap(bed, "temperature")),
		BedTarget:     floatAny(anyFromMap(bed, "target")),
		TimeRemaining: remaining,
	}, nil
}

func mapCrealityState(state string) string {
	switch state {
	case "printing":
		return "printing"
	case "paused":
		return "paused"
	case "complete", "completed":
		return "finished"
	case "error":
		return "error"
	case "standby", "ready":
		return "idle"
	default:
		if state == "" {
			return "connected"
		}
		return state
	}
}

func fetchPrusaTelemetry(p configpkg.Printer) (mqttpkg.TelemetryData, error) {
	client := &http.Client{Timeout: 6 * time.Second}
	headers := map[string]string{}
	if p.APIKey != "" {
		headers["X-Api-Key"] = p.APIKey
	}

	statusURL := fmt.Sprintf("http://%s/api/v1/status", p.IP)
	statusResp, err := sendJSONRequest(client, "GET", statusURL, headers, nil)
	if err != nil {
		return mqttpkg.TelemetryData{}, err
	}

	state := strings.ToLower(stringAny(nestedAny(statusResp, "printer", "state")))
	if state == "" {
		state = strings.ToLower(stringAny(nestedAny(statusResp, "job", "state")))
	}
	state = mapPrusaState(state)

	job := nestedMap(statusResp, "job")
	fileName := stringAny(nestedAnyAny(job, "file", "display_name"))
	if fileName == "" {
		fileName = stringAny(nestedAnyAny(job, "file", "name"))
	}

	progressRaw := floatAny(anyFromMap(job, "progress"))
	progress := int(progressRaw)
	if progressRaw > 0 && progressRaw <= 1 {
		progress = int(progressRaw * 100)
	}
	if progress > 100 {
		progress = 100
	}

	nozzle := floatAny(nestedAny(statusResp, "telemetry", "temp-nozzle"))
	if nozzle == 0 {
		nozzle = floatAny(nestedAny(statusResp, "printer", "temp_nozzle"))
	}
	bed := floatAny(nestedAny(statusResp, "telemetry", "temp-bed"))
	if bed == 0 {
		bed = floatAny(nestedAny(statusResp, "printer", "temp_bed"))
	}

	remaining := int(floatAny(anyFromMap(job, "time_remaining")) / 60)
	if remaining < 0 {
		remaining = 0
	}

	return mqttpkg.TelemetryData{
		Status:        state,
		FileName:      fileName,
		Progress:      progress,
		NozzleTemp:    nozzle,
		BedTemp:       bed,
		TimeRemaining: remaining,
	}, nil
}

func mapPrusaState(state string) string {
	switch state {
	case "printing":
		return "printing"
	case "paused":
		return "paused"
	case "finished", "complete":
		return "finished"
	case "error", "stopped":
		return "error"
	case "idle", "ready", "operational":
		return "idle"
	default:
		if state == "" {
			return "connected"
		}
		return state
	}
}

func (c *Controller) sendCrealityCommand(p configpkg.Printer, state *mqttpkg.TelemetryData, command string, args map[string]interface{}) error {
	client := &http.Client{Timeout: 6 * time.Second}

	switch command {
	case "pause":
		_, err := sendJSONRequest(client, "POST", fmt.Sprintf("http://%s/printer/print/pause", p.IP), nil, nil)
		return err
	case "resume":
		_, err := sendJSONRequest(client, "POST", fmt.Sprintf("http://%s/printer/print/resume", p.IP), nil, nil)
		return err
	case "stop":
		_, err := sendJSONRequest(client, "POST", fmt.Sprintf("http://%s/printer/print/cancel", p.IP), nil, nil)
		return err
	case "start":
		filename := getArgString(args, "file_name")
		if filename == "" {
			filename = getArgString(args, "file")
		}
		if filename == "" {
			return fmt.Errorf("start requires file_name")
		}
		body := strings.NewReader(fmt.Sprintf(`{"filename":%q}`, filename))
		_, err := sendJSONRequest(client, "POST", fmt.Sprintf("http://%s/printer/print/start", p.IP), map[string]string{"Content-Type": "application/json"}, body)
		return err
	case "light", "toggle_light", "light_on", "light_off":
		desiredOn := false
		hasDesired := false
		if command == "light_on" {
			desiredOn = true
			hasDesired = true
		}
		if command == "light_off" {
			hasDesired = true
		}
		if !hasDesired {
			if v, ok := args["on"].(bool); ok {
				desiredOn = v
				hasDesired = true
			}
		}
		if !hasDesired {
			desiredOn = !(state != nil && state.LightOn)
		}
		device, err := c.findCrealityLightDevice(client, p)
		if err != nil {
			return err
		}
		action := "off"
		if desiredOn {
			action = "on"
		}
		return setCrealityLight(client, p, device, action)
	default:
		return fmt.Errorf("unsupported command for creality: %s", command)
	}
}

func (c *Controller) findCrealityLightDevice(client *http.Client, p configpkg.Printer) (string, error) {
	u := fmt.Sprintf("http://%s/machine/device_power/devices", p.IP)
	m, err := sendJSONRequest(client, "GET", u, nil, nil)
	if err != nil {
		return "", err
	}

	result, ok := m["result"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid device list")
	}
	devices, ok := result["devices"].([]interface{})
	if !ok || len(devices) == 0 {
		return "", fmt.Errorf("no controllable power devices found")
	}

	var fallback string
	for _, d := range devices {
		name := ""
		switch v := d.(type) {
		case string:
			name = v
		case map[string]interface{}:
			name = stringAny(v["device"])
			if name == "" {
				name = stringAny(v["name"])
			}
		}
		if name == "" {
			continue
		}
		if fallback == "" {
			fallback = name
		}
		lc := strings.ToLower(name)
		if strings.Contains(lc, "light") || strings.Contains(lc, "led") || strings.Contains(lc, "lamp") {
			return name, nil
		}
	}
	if fallback == "" {
		return "", fmt.Errorf("no valid power device name found")
	}
	return fallback, nil
}

func setCrealityLight(client *http.Client, p configpkg.Printer, device, action string) error {
	queries := []string{
		fmt.Sprintf("http://%s/machine/device_power/device?device=%s&action=%s", p.IP, url.QueryEscape(device), url.QueryEscape(action)),
		fmt.Sprintf("http://%s/machine/device_power/set?device=%s&action=%s", p.IP, url.QueryEscape(device), url.QueryEscape(action)),
	}
	for _, q := range queries {
		_, err := sendJSONRequest(client, "POST", q, nil, nil)
		if err == nil {
			return nil
		}
	}
	return fmt.Errorf("failed to toggle light device %q", device)
}

func (c *Controller) sendPrusaCommand(p configpkg.Printer, command string) error {
	client := &http.Client{Timeout: 6 * time.Second}
	headers := map[string]string{"Content-Type": "application/json"}
	if p.APIKey != "" {
		headers["X-Api-Key"] = p.APIKey
	}

	endpoint := ""
	switch command {
	case "pause":
		endpoint = "/api/v1/job/pause"
	case "resume":
		endpoint = "/api/v1/job/resume"
	case "stop":
		endpoint = "/api/v1/job/cancel"
	case "light", "toggle_light", "light_on", "light_off":
		return fmt.Errorf("Prusa light control is not exposed by default PrusaLink API")
	case "start":
		return fmt.Errorf("Prusa remote start requires file selection in PrusaLink and is not implemented yet")
	default:
		return fmt.Errorf("unsupported command for prusa: %s", command)
	}

	u := fmt.Sprintf("http://%s%s", p.IP, endpoint)
	_, err := sendJSONRequest(client, "POST", u, headers, strings.NewReader("{}"))
	if err != nil {
		return err
	}
	return nil
}

func cameraCandidates(p configpkg.Printer) []string {
	if p.Brand == "bambu" {
		return []string{fmt.Sprintf("https://%s:6000/mjpeg/1", p.IP)}
	}
	return []string{
		fmt.Sprintf("http://%s/webcam/?action=stream", p.IP),
		fmt.Sprintf("http://%s/webcam/?action=snapshot", p.IP),
		fmt.Sprintf("http://%s/snapshot", p.IP),
	}
}

func insecureTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
}

func getArgString(args map[string]interface{}, key string) string {
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

func nestedMap(m map[string]interface{}, keys ...string) map[string]interface{} {
	cur := m
	for _, k := range keys {
		v, ok := cur[k]
		if !ok {
			return map[string]interface{}{}
		}
		next, ok := v.(map[string]interface{})
		if !ok {
			return map[string]interface{}{}
		}
		cur = next
	}
	return cur
}

func nestedMapAny(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	v, ok := m[key]
	if !ok {
		return map[string]interface{}{}
	}
	next, ok := v.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return next
}

func nestedAny(m map[string]interface{}, keys ...string) interface{} {
	var cur interface{} = m
	for _, k := range keys {
		mm, ok := cur.(map[string]interface{})
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func nestedAnyAny(m map[string]interface{}, k1, k2 string) interface{} {
	v, ok := m[k1]
	if !ok {
		return nil
	}
	mm, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	return mm[k2]
}

func anyFromMap(m map[string]interface{}, key string) interface{} {
	if m == nil {
		return nil
	}
	return m[key]
}

func stringAny(v interface{}) string {
	s, ok := v.(string)
	if ok {
		return s
	}
	return ""
}

func lowerString(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

func floatAny(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	default:
		return 0
	}
}
