package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	APIKey          string    `json:"api_key"`
	FoxTrack2APIKey string    `json:"foxtrack2_api_key,omitempty"`
	Printers        []Printer `json:"printers"`
	AutoUpdate      bool      `json:"auto_update,omitempty"`
}

type Printer struct {
	Name         string `json:"name"`
	IP           string `json:"ip,omitempty"`
	Serial       string `json:"serial,omitempty"`
	LANCode      string `json:"lan_code,omitempty"`
	MoonrakerURL string `json:"moonraker_url,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	WebcamURL    string `json:"webcam_url,omitempty"`
}

// legacyConfigPath is the old location used by previous builds.
func legacyConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return filepath.Join("config", "config.json")
	}
	return filepath.Join(filepath.Dir(exe), "config", "config.json")
}

// configPath returns the preferred persistent user config location.
func configPath() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		return legacyConfigPath()
	}
	return filepath.Join(base, "FoxTrack-Bridge", "config.json")
}

func LoadConfig() (*Config, error) {
	preferredPath := configPath()
	data, err := os.ReadFile(preferredPath)
	if errors.Is(err, os.ErrNotExist) {
		legacyPath := legacyConfigPath()
		legacyData, legacyErr := os.ReadFile(legacyPath)
		if legacyErr != nil {
			return nil, err
		}
		data = legacyData
		err = nil
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Best-effort migration so future updates keep using the stable user config path.
	_ = SaveConfig(&cfg)

	return &cfg, nil
}

// ConfigDir returns the directory where FoxTrack Bridge stores its data files.
func ConfigDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		exe, _ := os.Executable()
		return filepath.Join(filepath.Dir(exe), "config")
	}
	return filepath.Join(base, "FoxTrack-Bridge")
}

func SaveConfig(cfg *Config) error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(p, data, 0600)
}

// WriteFileAtomic writes data to a temp file in the target directory and renames
// it over path, so a crash mid-write can never leave a truncated or partial file.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename has succeeded
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// BackupCorrupt moves a config file that failed to load aside as
// config.json.corrupt-<timestamp> so a fresh config can be saved without
// destroying the original. It checks the preferred path first, then the
// legacy path, and returns the backup location.
func BackupCorrupt() (string, error) {
	p := configPath()
	if _, err := os.Stat(p); err != nil {
		legacy := legacyConfigPath()
		if _, lerr := os.Stat(legacy); lerr != nil {
			return "", err
		}
		p = legacy
	}
	backup := p + ".corrupt-" + time.Now().Format("20060102-150405")
	if err := os.Rename(p, backup); err != nil {
		return "", err
	}
	return backup, nil
}
