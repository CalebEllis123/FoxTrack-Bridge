package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"foxtrack-bridge/config"
	mqttpkg "foxtrack-bridge/mqtt"
)

var (
	configStore *config.Config
	configMutex sync.RWMutex
)

func StartServer() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("No config found (%v) — starting fresh", err)
		cfg = &config.Config{Printers: []config.Printer{}}
	}

	configMutex.Lock()
	configStore = cfg
	configMutex.Unlock()

	for _, p := range cfg.Printers {
		if p.Brand == "bambu" || p.Brand == "" {
			mqttpkg.ConnectPrinter(mqttPrinter(p, cfg))
		}
	}

	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/logo.png", handleLogo)
	http.HandleFunc("/api/config", handleConfig)
	http.HandleFunc("/api/printers", handlePrinters)
	http.HandleFunc("/api/status", handleStatus)
	http.HandleFunc("/api/test", handleTest)
	http.HandleFunc("/api/control/", handleControl) // /api/control/{name}/{command}
	http.HandleFunc("/api/camera/", handleCamera)   // /api/camera/{name}

	fmt.Println("FoxTrack Bridge running at http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Printf("Server error: %v", err)
	}
}

func mqttPrinter(p config.Printer, cfg *config.Config) mqttpkg.Printer {
	return mqttpkg.Printer{
		Name:       p.Name,
		IP:         p.IP,
		Serial:     p.Serial,
		LANCode:    p.LANCode,
		WebhookURL: cfg.WebhookURL,
		APIKey:     cfg.APIKey,
	}
}

func cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(uiHTML)
}

func handleLogo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Write(logoPNG)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	cors(w)
	json.NewEncoder(w).Encode(mqttpkg.GetPrintersState())
}

// handleControl handles printer control commands.
// URL: /api/control/{printerName}/{command}
// Commands: pause, resume, stop, light_on, light_off
func handleControl(w http.ResponseWriter, r *http.Request) {
	cors(w)
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse /api/control/{name}/{command}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/control/"), "/")
	if len(parts) != 2 {
		http.Error(w, "usage: /api/control/{printer_name}/{command}", http.StatusBadRequest)
		return
	}
	printerName := parts[0]
	command := parts[1]

	if err := mqttpkg.SendCommand(printerName, command); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "command": command})
}

// handleCamera proxies the BambuLab MJPEG camera stream.
// URL: /api/camera/{printerName}
// BambuLab streams MJPEG on port 6000 with basic auth (bblp:lancode).
func handleCamera(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		return
	}

	printerName := strings.TrimPrefix(r.URL.Path, "/api/camera/")
	printerName = strings.TrimSuffix(printerName, "/")

	// Find the printer config
	configMutex.RLock()
	var found *config.Printer
	for i := range configStore.Printers {
		if configStore.Printers[i].Name == printerName {
			found = &configStore.Printers[i]
			break
		}
	}
	configMutex.RUnlock()

	if found == nil {
		http.Error(w, "printer not found", http.StatusNotFound)
		return
	}

	// BambuLab MJPEG stream: http://IP:6000/mjpeg/1 with basic auth bblp:lancode
	streamURL := fmt.Sprintf("https://%s:6000/mjpeg/1", found.IP)

	req, err := http.NewRequest("GET", streamURL, nil)
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}
	req.SetBasicAuth("bblp", found.LANCode)

	// Use a transport that tolerates the printer's self-signed cert situation
	transport := &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		DisableKeepAlives:   false,
		IdleConnTimeout:     0,
	}
	client := &http.Client{Transport: transport, Timeout: 0} // no timeout — stream

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[camera/%s] failed to connect: %v", printerName, err)
		http.Error(w, "camera unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Forward the content type (should be multipart/x-mixed-replace for MJPEG)
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	// Stream the body directly — this blocks until client disconnects
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				break // client disconnected
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("[camera/%s] stream error: %v", printerName, err)
			break
		}
	}
}

func handleTest(w http.ResponseWriter, r *http.Request) {
	cors(w)
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		IP    string `json:"ip"`
		Brand string `json:"brand"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	port := "8883"
	if req.Brand == "creality" || req.Brand == "prusa" {
		port = "80"
	}

	address := net.JoinHostPort(req.IP, port)
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	reachable := err == nil
	if conn != nil {
		conn.Close()
	}

	json.NewEncoder(w).Encode(map[string]bool{"reachable": reachable})
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	cors(w)
	switch r.Method {
	case "OPTIONS":
		return
	case "GET":
		configMutex.RLock()
		defer configMutex.RUnlock()
		json.NewEncoder(w).Encode(configStore)
	case "POST":
		var newCfg config.Config
		if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		configMutex.Lock()
		configStore = &newCfg
		configMutex.Unlock()
		if err := config.SaveConfig(&newCfg); err != nil {
			log.Printf("Warning: failed to save config: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handlePrinters(w http.ResponseWriter, r *http.Request) {
	cors(w)
	switch r.Method {
	case "OPTIONS":
		return
	case "GET":
		configMutex.RLock()
		defer configMutex.RUnlock()
		json.NewEncoder(w).Encode(configStore.Printers)
	case "POST":
		var p config.Printer
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		configMutex.Lock()
		configStore.Printers = append(configStore.Printers, p)
		cfg := configStore
		configMutex.Unlock()

		if err := config.SaveConfig(cfg); err != nil {
			log.Printf("Warning: failed to save config: %v", err)
		}
		if p.Brand == "bambu" || p.Brand == "" {
			mqttpkg.ConnectPrinter(mqttPrinter(p, cfg))
		} else {
			log.Printf("[%s] Brand '%s' connected (support coming soon)", p.Name, p.Brand)
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
