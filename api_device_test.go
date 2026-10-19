package main

import "testing"

func boolPtr(v bool) *bool { return &v }

func TestNormalizeUSBRequestPersistsRuntimeChoice(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      usbRequest
		active  bool
		auto    bool
		restart bool
	}{
		{name: "start", in: usbRequest{Action: "start"}, active: true, auto: true},
		{name: "stop", in: usbRequest{Action: "stop"}, active: false, auto: false},
		{name: "restart", in: usbRequest{Action: "restart"}, active: true, auto: true, restart: true},
		{name: "legacy active", in: usbRequest{Active: boolPtr(false)}, active: false, auto: false},
		{name: "explicit split policy", in: usbRequest{Active: boolPtr(false), AutoStart: boolPtr(true)}, active: false, auto: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restart, err := normalizeUSBRequest(&tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if tc.in.Active == nil || *tc.in.Active != tc.active || tc.in.AutoStart == nil || *tc.in.AutoStart != tc.auto || restart != tc.restart {
				t.Fatalf("normalized request = active %v, auto %v, restart %v", tc.in.Active, tc.in.AutoStart, restart)
			}
		})
	}

	if _, err := normalizeUSBRequest(&usbRequest{Action: "invalid"}); err == nil {
		t.Fatal("invalid USB action was accepted")
	}
}
