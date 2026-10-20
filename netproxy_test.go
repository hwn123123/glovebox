package main

import (
	"reflect"
	"testing"
)

func TestParseNetproxyAutostart(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		want    bool
		wantErr bool
	}{
		{name: "enabled", content: `{"auto_start":true}`, want: true},
		{name: "disabled", content: `{"auto_start":false}`, want: false},
		{name: "missing", content: `{}`, wantErr: true},
		{name: "invalid", content: `{"auto_start":1}`, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseNetproxyAutostart(test.content)
			if (err != nil) != test.wantErr {
				t.Fatalf("parse error = %v, wantErr %v", err, test.wantErr)
			}
			if err == nil && got != test.want {
				t.Fatalf("auto_start = %v, want %v", got, test.want)
			}
		})
	}
}

func TestNetproxyModeIDs(t *testing.T) {
	got := netproxyModeIDs([]string{"Proxy", "RuleAllowAds", "Rule", "Direct", "Rule"})
	want := []string{"global", "ruleallowads", "rule", "direct"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mode IDs = %#v, want %#v", got, want)
	}
}

func TestValidPackageRefs(t *testing.T) {
	if !validPackageRefs([]string{"0:com.example.app", "10:org.example"}) {
		t.Fatal("valid Android package references were rejected")
	}
	for _, values := range [][]string{{"com.example.app"}, {"x:com.example.app"}, {"0:"}, {"0:com..app"}} {
		if validPackageRefs(values) {
			t.Fatalf("invalid package references accepted: %#v", values)
		}
	}
}

func TestSplitNetproxyCommand(t *testing.T) {
	got, err := splitNetproxyCommand(`'/data/adb/modules/netproxy/netproxyctl' '--json' '--timeout' '30s' 'node' 'show' 'default'`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/data/adb/modules/netproxy/netproxyctl", "--json", "--timeout", "30s", "node", "show", "default"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command words = %#v, want %#v", got, want)
	}
	if err := validateNetproxyTerminalArgs(got[1:]); err != nil {
		t.Fatalf("valid NetProxy command rejected: %v", err)
	}
}

func TestNetproxyTerminalRejectsShell(t *testing.T) {
	for _, command := range []string{
		`id`,
		`'/system/bin/sh' '-c' 'id'`,
		`'/data/adb/modules/netproxy/netproxyctl' '--bad-option' 'service' 'status'`,
		`'/data/adb/modules/netproxy/netproxyctl' '__internal' 'boot'`,
		"'/data/adb/modules/netproxy/netproxyctl' 'service' 'status'; 'id'",
	} {
		words, err := splitNetproxyCommand(command)
		if err == nil {
			err = validateNetproxyTerminalArgs(words[1:])
		}
		if err == nil {
			t.Fatalf("unsafe command accepted: %s", command)
		}
	}
}
