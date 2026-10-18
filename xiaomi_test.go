package main

import "testing"

func TestXiaomiSignalParsingSkipsInvalidPlaceholders(t *testing.T) {
	raw := `CellSignalStrengthNr:{ csiRsrp = 2147483647 csiRsrq = 2147483647 csiSinr = 2147483647 ssRsrp = -72 ssRsrq = -11 ssSinr = 18 level=3 }`
	if got := firstValidMatch(nrRSRPRe, raw, -160, -30); got != "-72" {
		t.Fatalf("RSRP = %q, want -72", got)
	}
	if got := firstValidMatch(nrRSRQRe, raw, -50, 20); got != "-11" {
		t.Fatalf("RSRQ = %q, want -11", got)
	}
	if got := firstValidMatch(nrSINRRe, raw, -30, 80); got != "18" {
		t.Fatalf("SINR = %q, want 18", got)
	}
}

func TestXiaomiSIMSelectsLoadedSlot(t *testing.T) {
	state, slot := xiaomiSIM("ABSENT,LOADED")
	if state != "modem_init_complete" || slot != 2 {
		t.Fatalf("xiaomiSIM = %q, %d; want ready slot 2", state, slot)
	}
	state, slot = xiaomiSIM("LOADED,ABSENT")
	if state != "modem_init_complete" || slot != 1 {
		t.Fatalf("xiaomiSIM = %q, %d; want ready slot 1", state, slot)
	}
}

func TestXiaomiPropertyAndChannelHelpers(t *testing.T) {
	if got := firstActiveProp("Unknown, \u4e2d\u56fd\u79fb\u52a8,"); got != "\u4e2d\u56fd\u79fb\u52a8" {
		t.Fatalf("firstActiveProp = %q", got)
	}
	for freq, want := range map[int]int{2412: 1, 2484: 14, 5745: 149, 5955: 1} {
		if got := wifiChannel(freq); got != want {
			t.Errorf("wifiChannel(%d) = %d, want %d", freq, got, want)
		}
	}
}

func TestXiaomiNetSamplerCountsUSBAndWiFiDownstreams(t *testing.T) {
	n := netSampler{lanDev: "wlan2", xiaomi: true}
	for _, dev := range []string{"wlan2", "usb0", "rndis0", "ncm0", "ap_br_wlan2"} {
		if !n.isLAN(dev) {
			t.Errorf("%s should be a Xiaomi downstream", dev)
		}
	}
	if n.isLAN("rmnet_data1") {
		t.Fatal("WAN interface counted as downstream")
	}
}
