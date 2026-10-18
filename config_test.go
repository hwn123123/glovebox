package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigSavesOnlyOverrides(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	cs, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.update(func(c *Config) { c.MainGroup = "Proxy" }); err != nil {
		t.Fatal(err)
	}
	var onDisk map[string]any
	b, _ := os.ReadFile(p)
	if err := json.Unmarshal(b, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk["main_group"] != "Proxy" || onDisk["session_secret"] == nil {
		t.Fatalf("missing overrides: %v", onDisk)
	}
	if _, ok := onDisk["listen"]; ok {
		t.Fatalf("default field written: %v", onDisk)
	}
	again, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.get(); got.MainGroup != "Proxy" || got.Listen[0] != defaultConfig().Listen[0] {
		t.Fatalf("reload mismatch: %+v", got)
	}
}

func TestConfigImport(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	cs, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	before := cs.get()
	for _, bad := range []string{
		`{"password_hash": "x"}`,
		`{"session_secret": "x"}`,
		`{"hotspot_password": "not-through-import"}`,
		`{"no_such_field": 1}`,
		`{"listen": "not a list"}`,
	} {
		if err := cs.importJSON(strings.NewReader(bad)); err == nil {
			t.Errorf("import %s: want error", bad)
		}
	}
	if got := cs.get(); got.SessionSecret != before.SessionSecret || len(got.Listen) != len(before.Listen) {
		t.Fatal("failed import changed the config")
	}
	if err := cs.importJSON(strings.NewReader(`{"tailscale_proxy": "TS", "tailscale_probe": "http://100.64.0.1/"}`)); err != nil {
		t.Fatal(err)
	}
	if got := cs.get(); !got.tailscaleEnabled() || got.TailscaleProxy != "TS" {
		t.Fatalf("import not applied: %+v", got)
	}
}

func TestRedacted(t *testing.T) {
	c := Config{PasswordHash: "h", SessionSecret: "s", HotspotPassword: "wifi-secret"}.redacted()
	if c.PasswordHash == "h" || c.SessionSecret == "s" || c.HotspotPassword == "wifi-secret" {
		t.Fatal("secrets not masked")
	}
}

func TestXiaomiRouterDefaults(t *testing.T) {
	c := defaultConfig()
	if c.NetProxyDir != "/data/adb/modules/glovebox/netproxy" {
		t.Fatalf("unexpected embedded NetProxy path %q", c.NetProxyDir)
	}
	if c.HotspotSSID != "nwrt" || c.HotspotPassword != "1234567890" {
		t.Fatalf("unexpected hotspot defaults: %q / %q", c.HotspotSSID, c.HotspotPassword)
	}
	if !c.AutoStartHotspot || !c.AutoStartUSBTether {
		t.Fatal("router sharing must auto-start by default")
	}
}
