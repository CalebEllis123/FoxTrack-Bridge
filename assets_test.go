//go:build headless

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestAssetsEmbedded(t *testing.T) {
	if len(tailwindCSS) == 0 {
		t.Error("tailwindCSS is empty — web/tailwind.css not embedded")
	}
	if len(iconsCSS) == 0 {
		t.Error("iconsCSS is empty — web/icons.css not embedded")
	}
	if len(fontsCSS) == 0 {
		t.Error("fontsCSS is empty — web/fonts.css not embedded")
	}
}

func TestCSSHandlerServesCSS(t *testing.T) {
	cases := map[string][]byte{
		"/tailwind.css": tailwindCSS,
		"/icons.css":    iconsCSS,
		"/fonts.css":    fontsCSS,
	}
	for path, body := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		cssHandler(body)(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
			t.Errorf("%s: Content-Type = %q, want text/css…", path, ct)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("%s: empty body", path)
		}
	}
}

func TestNoCDNReferences(t *testing.T) {
	banned := []string{
		"cdn.tailwindcss.com",
		"fonts.googleapis.com",
		"fonts.gstatic.com",
		"cdnjs.cloudflare.com",
	}
	for _, b := range banned {
		if bytes.Contains(uiHTML, []byte(b)) {
			t.Errorf("ui.html still references CDN host %q", b)
		}
	}
}

func TestUIReferencesLocalAssets(t *testing.T) {
	for _, want := range []string{`href="/tailwind.css"`, `href="/icons.css"`, `href="/fonts.css"`} {
		if !bytes.Contains(uiHTML, []byte(want)) {
			t.Errorf("ui.html missing local asset link %q", want)
		}
	}
}

func TestFontsEmbedded(t *testing.T) {
	data, err := fontsFS.ReadFile("web/fonts/inter-latin.woff2")
	if err != nil {
		t.Fatalf("embedded font web/fonts/inter-latin.woff2 missing: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("wOF2")) {
		t.Error("inter-latin.woff2 is not a woff2 file (bad magic)")
	}
	if len(data) < 1000 {
		t.Errorf("inter-latin.woff2 suspiciously small: %d bytes", len(data))
	}
}

func TestFontHandlerServesWoff2(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fonts/inter-latin.woff2", nil)
	fontHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "font/woff2" {
		t.Errorf("Content-Type = %q, want font/woff2", ct)
	}
	if rec.Body.Len() == 0 {
		t.Error("empty body")
	}
}

func TestFontHandlerRejectsUnknown(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fonts/nope.woff2", nil)
	fontHandler(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestAllUsedIconsDefined(t *testing.T) {
	re := regexp.MustCompile(`fa-[a-z0-9-]+`)
	ignore := map[string]bool{"fa-spin": true} // animation utility, not a glyph
	seen := map[string]bool{}
	for _, m := range re.FindAll(uiHTML, -1) {
		icon := string(m)
		if ignore[icon] || seen[icon] {
			continue
		}
		seen[icon] = true
		if !bytes.Contains(iconsCSS, []byte("."+icon)) {
			t.Errorf("icon %q used in ui.html but not defined in icons.css", icon)
		}
	}
}
