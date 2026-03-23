package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	APIKey     string    `json:"api_key"`
	WebhookURL string    `json:"webhook_url"`
	Printers   []Printer `json:"printers"`
}

type Printer struct {
	Name    string `json:"name"`
	IP      string `json:"ip"`
	Serial  string `json:"serial"`
	LANCode string `json:"lan_code,omitempty"`
}

// LoadConfig reads config from config/config.json
func LoadConfig() (*Config, error) {
	data, err := os.ReadFile("config/config.json")
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveConfig writes config back to config/config.json
func SaveConfig(cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("config/config.json", data, 0644)
}
