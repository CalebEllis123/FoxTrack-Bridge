package update

import "testing"

func TestParseChecksumText(t *testing.T) {
	body := "" +
		"f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0  FoxTrack-Bridge-Linux\n" +
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa *FoxTrack-Bridge-Windows.exe\n"

	got := parseChecksumText(body)
	if got["foxtrack-bridge-linux"] != "f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0" {
		t.Fatalf("missing linux checksum parse")
	}
	if got["foxtrack-bridge-windows.exe"] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("missing windows checksum parse")
	}
}

func TestPickAssetFor(t *testing.T) {
	assets := []releaseAsset{
		{Name: "FoxTrack-Bridge-Windows.exe", BrowserDownloadURL: "https://example/windows"},
		{Name: "FoxTrack-Bridge-Windows-Headless.exe", BrowserDownloadURL: "https://example/windows-headless"},
		{Name: "FoxTrack-Bridge-Linux", BrowserDownloadURL: "https://example/linux"},
		{Name: "FoxTrack-Bridge-Linux-ARM64", BrowserDownloadURL: "https://example/linux-arm64"},
		{Name: "FoxTrack-Bridge-Linux-ARM32", BrowserDownloadURL: "https://example/linux-arm32"},
		{Name: "FoxTrack-Bridge-macOS-Apple-Silicon.zip", BrowserDownloadURL: "https://example/mac-arm"},
		{Name: "FoxTrack-Bridge-macOS-Apple-Silicon-Headless", BrowserDownloadURL: "https://example/mac-arm-headless"},
		{Name: "FoxTrack-Bridge-macOS-Intel.zip", BrowserDownloadURL: "https://example/mac-intel"},
		{Name: "FoxTrack-Bridge-macOS-Intel-Headless", BrowserDownloadURL: "https://example/mac-intel-headless"},
	}

	check := func(goos, goarch, variant, wantName string) {
		t.Helper()
		a, ok := pickAssetFor(assets, goos, goarch, variant)
		if !ok || a.Name != wantName {
			t.Fatalf("pickAssetFor(%q, %q, %q): got %q ok=%v, want %q", goos, goarch, variant, a.Name, ok, wantName)
		}
	}

	check("linux", "amd64", "", "FoxTrack-Bridge-Linux")
	check("linux", "arm64", "", "FoxTrack-Bridge-Linux-ARM64")
	check("linux", "arm", "", "FoxTrack-Bridge-Linux-ARM32")
	check("windows", "amd64", "", "FoxTrack-Bridge-Windows.exe")
	check("windows", "amd64", "headless", "FoxTrack-Bridge-Windows-Headless.exe")
	check("darwin", "arm64", "", "FoxTrack-Bridge-macOS-Apple-Silicon.zip")
	check("darwin", "arm64", "headless", "FoxTrack-Bridge-macOS-Apple-Silicon-Headless")
	check("darwin", "amd64", "", "FoxTrack-Bridge-macOS-Intel.zip")
	check("darwin", "amd64", "headless", "FoxTrack-Bridge-macOS-Intel-Headless")
}
