//go:build headless

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"foxtrack-bridge/config"
)

func twoPrinters() []config.Printer {
	return []config.Printer{
		{Name: "Monsieur", IP: "192.168.87.22", Serial: "01P00C580801716", LANCode: "81f1aafd"},
		{Name: "Sherlock", IP: "192.168.87.24", Serial: "01P00C580800360", LANCode: "f849c383"},
	}
}

// Settings save with empty local printer state must NOT clear printers:
// the new client omits the "printers" key entirely -> existing list is kept.
func TestResolveConfigUpdate_OmittedPrintersPreservesExisting(t *testing.T) {
	old := &config.Config{APIKey: "k", Printers: twoPrinters()}
	got, err := resolveConfigUpdate(old, []byte(`{"api_key":"newkey","auto_update":true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Printers) != 2 {
		t.Fatalf("printers = %d, want 2 (settings save must not clear)", len(got.Printers))
	}
	if got.APIKey != "newkey" || !got.AutoUpdate {
		t.Errorf("api_key=%q auto_update=%v, want newkey/true", got.APIKey, got.AutoUpdate)
	}
}

// Legacy/old-shape client sending printers:[] against a non-empty list is refused.
func TestResolveConfigUpdate_EmptyPrintersRefusedWithoutConfirm(t *testing.T) {
	old := &config.Config{Printers: twoPrinters()}
	if _, err := resolveConfigUpdate(old, []byte(`{"printers":[]}`)); !errors.Is(err, errRefuseClearPrinters) {
		t.Fatalf("err = %v, want errRefuseClearPrinters", err)
	}
}

// A deliberate clear (confirmation present) succeeds even against a non-empty list.
func TestResolveConfigUpdate_EmptyPrintersAllowedWithConfirm(t *testing.T) {
	old := &config.Config{Printers: twoPrinters()}
	got, err := resolveConfigUpdate(old, []byte(`{"printers":[],"confirm_clear_printers":true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Printers) != 0 {
		t.Fatalf("printers = %d, want 0 (confirmed clear must succeed)", len(got.Printers))
	}
}

// No false positive: clearing an already-empty list needs no confirmation.
func TestResolveConfigUpdate_EmptyPrintersOKWhenExistingEmpty(t *testing.T) {
	if _, err := resolveConfigUpdate(&config.Config{}, []byte(`{"printers":[]}`)); err != nil {
		t.Fatalf("unexpected error clearing already-empty list: %v", err)
	}
}

// Backward compat: old-shape payload with a non-empty array replaces as before.
func TestResolveConfigUpdate_NonEmptyReplaceBackwardCompat(t *testing.T) {
	old := &config.Config{Printers: twoPrinters()}
	got, err := resolveConfigUpdate(old, []byte(`{"printers":[{"name":"Watson","ip":"192.168.87.26","serial":"01P00C591701791","lan_code":"f065d489"}]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Printers) != 1 || got.Printers[0].Name != "Watson" {
		t.Fatalf("printers = %+v, want single Watson", got.Printers)
	}
}

// Blank top-level secrets and untouched printers keep their stored values.
func TestResolveConfigUpdate_PreservesSecretsOnPartial(t *testing.T) {
	old := &config.Config{APIKey: "stored", Printers: twoPrinters()}
	got, err := resolveConfigUpdate(old, []byte(`{"auto_update":true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.APIKey != "stored" {
		t.Errorf("api_key = %q, want stored (blank keeps existing)", got.APIKey)
	}
	if len(got.Printers) != 2 || got.Printers[0].LANCode != "81f1aafd" {
		t.Errorf("printer secrets not preserved: %+v", got.Printers)
	}
}

// The real UI delete path (DELETE /api/printers/{name}) still removes the last
// printer. Klipper printer => no MQTT; XDG sandbox => SaveConfig writes to temp.
func TestDeleteLastPrinter(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	configMutex.Lock()
	configStore = &config.Config{Printers: []config.Printer{{Name: "Only", MoonrakerURL: "http://127.0.0.1:1/"}}}
	configMutex.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/printers/Only", nil)
	handlePrinterByName(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	configMutex.RLock()
	n := len(configStore.Printers)
	configMutex.RUnlock()
	if n != 0 {
		t.Fatalf("printers = %d, want 0 after deleting the last one", n)
	}
}
