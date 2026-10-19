package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type netproxyReply struct {
	OK      bool            `json:"ok"`
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (s *server) netproxyCall(ctx context.Context, args ...string) (json.RawMessage, error) {
	return s.netproxyCallWithTimeout(ctx, 15*time.Second, "10s", args...)
}

func (s *server) netproxyCallWithTimeout(ctx context.Context, commandTimeout time.Duration, cliTimeout string, args ...string) (json.RawMessage, error) {
	c := s.cfg.get()
	if c.NetProxyDir == "" {
		return nil, fmt.Errorf("netproxy directory is not configured")
	}
	all := []string{"--json", "--timeout", cliTimeout}
	all = append(all, args...)
	out, err := run(ctx, commandTimeout, netproxyExecutable(c), all...)
	if err != nil && strings.TrimSpace(out) == "" {
		return nil, err
	}
	var reply netproxyReply
	if err := json.Unmarshal([]byte(out), &reply); err != nil {
		return nil, fmt.Errorf("netproxy response: %w", err)
	}
	if !reply.OK {
		msg := reply.Message
		if msg == "" {
			msg = reply.Code
		}
		code := reply.Code
		if code == "" {
			code = "netproxy_error"
		}
		return nil, apiErr(code, msg)
	}
	return reply.Data, nil
}

func netproxyExecutable(c Config) string {
	p := filepath.Join(c.NetProxyDir, "bin/netproxyctl")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return filepath.Join(c.NetProxyDir, "netproxyctl")
}

type netproxyConfigSnapshot struct {
	Target   string `json:"target"`
	Content  string `json:"content"`
	Revision string `json:"revision"`
}

func (s *server) netproxyReadConfig(ctx context.Context, target string) (netproxyConfigSnapshot, error) {
	b, err := s.netproxyCall(ctx, "config", "read", target)
	if err != nil {
		return netproxyConfigSnapshot{}, err
	}
	var snapshot netproxyConfigSnapshot
	if err := json.Unmarshal(b, &snapshot); err != nil {
		return snapshot, fmt.Errorf("decode NetProxy config %s: %w", target, err)
	}
	if snapshot.Revision == "" {
		return snapshot, fmt.Errorf("NetProxy config %s has no revision", target)
	}
	return snapshot, nil
}

func parseNetproxyAutostart(content string) (bool, error) {
	var value struct {
		AutoStart *bool `json:"auto_start"`
	}
	if err := json.Unmarshal([]byte(content), &value); err != nil {
		return false, err
	}
	if value.AutoStart == nil {
		return false, errors.New("NetProxy auto_start is missing")
	}
	return *value.AutoStart, nil
}

func netproxyAutostartFromFile(c Config) (bool, error) {
	b, err := os.ReadFile(filepath.Join(c.NetProxyDir, "config", "module.json"))
	if err != nil {
		return false, err
	}
	return parseNetproxyAutostart(string(b))
}

func (s *server) netproxyApplyConfig(ctx context.Context, target string, content []byte) error {
	c := s.cfg.get()
	if c.NetProxyDir == "" {
		return fmt.Errorf("netproxy directory is not configured")
	}
	runtimeDir := filepath.Join(c.NetProxyDir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		return fmt.Errorf("prepare NetProxy runtime: %w", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		snapshot, err := s.netproxyReadConfig(ctx, target)
		if err != nil {
			return err
		}
		file, err := os.CreateTemp(runtimeDir, "glovebox-config-*.json")
		if err != nil {
			return fmt.Errorf("create NetProxy config candidate: %w", err)
		}
		path := file.Name()
		_ = file.Chmod(0600)
		_, writeErr := file.Write(content)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(path)
			if writeErr != nil {
				return writeErr
			}
			return closeErr
		}
		_, validateErr := s.netproxyCallWithTimeout(ctx, 40*time.Second, "35s", "config", "validate", "--revision", snapshot.Revision, target, path)
		if validateErr == nil {
			_, err = s.netproxyCallWithTimeout(ctx, 55*time.Second, "50s", "config", "apply", "--revision", snapshot.Revision, target, path)
		} else {
			err = validateErr
		}
		_ = os.Remove(path)
		var api *apiError
		if errors.As(err, &api) && api.Code == "config.conflict" && attempt == 0 {
			continue
		}
		return err
	}
	return apiErr("config.conflict", "NetProxy configuration changed concurrently")
}

func (s *server) setNetproxyAutostart(ctx context.Context, enabled bool) error {
	content, _ := json.Marshal(map[string]bool{"auto_start": enabled})
	return s.netproxyApplyConfig(ctx, "module/auto_start", content)
}

func (s *server) startNetproxyAction(ctx context.Context, action string) (netproxyServiceStatus, error) {
	// Starting and stopping are user intent and therefore also set the next-boot
	// policy. A restart is only a runtime operation and must preserve that policy.
	previousAutoStart, previousErr := netproxyAutostartFromFile(s.cfg.get())
	policyChanged := action == "start" || action == "stop"
	if policyChanged {
		enabled := action == "start"
		if err := s.setNetproxyAutostart(ctx, enabled); err != nil {
			return netproxyServiceStatus{}, err
		}
	}
	timeout := 45 * time.Second
	cliTimeout := "40s"
	if action == "stop" {
		timeout = 25 * time.Second
		cliTimeout = "20s"
	}
	b, err := s.netproxyCallWithTimeout(ctx, timeout, cliTimeout, "service", action)
	if err != nil {
		// Do not persist a boot policy that the runtime action failed to reach.
		if policyChanged && previousErr == nil {
			if rollbackErr := s.setNetproxyAutostart(context.Background(), previousAutoStart); rollbackErr != nil {
				log.Printf("NetProxy %s failed and boot-policy rollback failed: %v", action, rollbackErr)
			}
		}
		return netproxyServiceStatus{}, err
	}
	var result struct {
		Status netproxyServiceStatus `json:"status"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return netproxyServiceStatus{}, fmt.Errorf("decode NetProxy service result: %w", err)
	}
	return result.Status, nil
}

type netproxyNode struct {
	Tag      string `json:"tag"`
	Protocol string `json:"protocol"`
	Delay    int    `json:"url_test_delay"`
}

type netproxyGroup struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Runtime   string `json:"runtime_tag"`
	NodeCount int    `json:"node_count"`
	Nodes     []netproxyNode
}

type netproxyNodeList struct {
	Group netproxyGroup  `json:"group"`
	Nodes []netproxyNode `json:"nodes"`
}

type netproxySubscription struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Type           string          `json:"type"`
	Active         bool            `json:"active"`
	NodeCount      int             `json:"node_count"`
	AutoUpdate     bool            `json:"auto_update"`
	UpdateInterval int64           `json:"update_interval"`
	UpdateViaProxy string          `json:"update_via_proxy"`
	Usage          json.RawMessage `json:"usage"`
	LastAttemptAt  string          `json:"last_attempt_at"`
	LastSuccessAt  string          `json:"last_success_at"`
	NextUpdateAt   string          `json:"next_update_at"`
	UpdatedAt      string          `json:"updated_at"`
	LastError      string          `json:"last_error"`
	Progress       json.RawMessage `json:"progress"`
}

func (s *server) netproxySubscriptions(ctx context.Context) ([]netproxySubscription, error) {
	b, err := s.netproxyCall(ctx, "sub", "list")
	if err != nil {
		return nil, err
	}
	var out []netproxySubscription
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *server) netproxyNodes(ctx context.Context) ([]netproxyNodeList, error) {
	b, err := s.netproxyCall(ctx, "node", "list")
	if err != nil {
		return nil, err
	}
	var out []netproxyNodeList
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

type netproxyServiceStatus struct {
	State                  string   `json:"state"`
	ConfiguredBackend      string   `json:"configured_backend"`
	ActiveBackend          *string  `json:"active_backend"`
	PID                    *int     `json:"pid"`
	StartedAt              int64    `json:"started_at"`
	ReadyAt                int64    `json:"ready_at"`
	UptimeSeconds          int64    `json:"uptime_seconds"`
	ActiveGroupID          string   `json:"active_group_id"`
	ActiveGroupName        string   `json:"active_group_name"`
	SelectedNodeRef        string   `json:"selected_node_ref"`
	OutboundMode           string   `json:"outbound_mode"`
	ConfiguredOutboundMode string   `json:"configured_outbound_mode"`
	AvailableModes         []string `json:"available_outbound_modes"`
	ConnectionsIn          int      `json:"connections_in"`
	ConnectionsOut         int      `json:"connections_out"`
	UploadTotal            int64    `json:"upload_total"`
	DownloadTotal          int64    `json:"download_total"`
	MemoryBytes            int64    `json:"memory_bytes"`
	WorkerState            string   `json:"worker_state"`
	WorkerPID              *int     `json:"worker_pid"`
	Error                  string   `json:"error"`
}

func (s *server) netproxyStatus(ctx context.Context) (netproxyServiceStatus, error) {
	b, err := s.netproxyCall(ctx, "service", "status")
	if err != nil {
		return netproxyServiceStatus{}, err
	}
	var st netproxyServiceStatus
	if err := json.Unmarshal(b, &st); err != nil {
		return st, err
	}
	return st, nil
}

func netproxyMode(mode string) string {
	switch strings.ToLower(mode) {
	case "rule":
		return "Rule"
	case "ruleallowads":
		return "RuleAllowAds"
	case "global":
		return "Proxy"
	case "direct":
		return "Direct"
	default:
		return mode
	}
}

func gloveboxMode(mode string) string {
	switch strings.ToLower(mode) {
	case "rule":
		return "rule"
	case "ruleallowads":
		return "ruleallowads"
	case "proxy", "global":
		return "global"
	case "direct":
		return "direct"
	default:
		return strings.ToLower(mode)
	}
}

func netproxyRef(group netproxyGroup, node string) string {
	return group.ID + "/" + node
}

func parseNetproxyDelay(b json.RawMessage) (int, string) {
	var v struct {
		Target string `json:"target"`
		Groups []struct {
			Items []struct {
				Tag   string `json:"tag"`
				Delay int    `json:"url_test_delay"`
			} `json:"items"`
		} `json:"groups"`
	}
	if json.Unmarshal(b, &v) != nil {
		return -1, ""
	}
	for _, g := range v.Groups {
		for _, item := range g.Items {
			return item.Delay, item.Tag
		}
	}
	return -1, v.Target
}

// parseNetproxyDelays converts the batch node-delay response into the names the
// web console uses. NetProxy prefixes item tags with the runtime group name
// ("group/node"), while the UI displays just the node tag.
func parseNetproxyDelays(b json.RawMessage) map[string]int {
	var v struct {
		Groups []struct {
			Items []struct {
				Tag   string `json:"tag"`
				Delay int    `json:"url_test_delay"`
			} `json:"items"`
		} `json:"groups"`
	}
	out := map[string]int{}
	if json.Unmarshal(b, &v) != nil {
		return out
	}
	for _, group := range v.Groups {
		for _, item := range group.Items {
			name := selectedTag(item.Tag)
			// A node without url_test_delay is unreachable. Keep it in the
			// response with zero so the frontend renders the normal timeout state.
			out[name] = item.Delay
		}
	}
	return out
}

func parseNetproxyProviderGroups(b json.RawMessage) ([]netproxyGroup, error) {
	var groups []netproxyGroup
	if err := json.Unmarshal(b, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func netproxyCountDelay(n netproxyNode) int {
	if n.Delay <= 0 {
		return -1
	}
	return n.Delay
}

func formatNetproxyMode(mode string) string {
	if mode == "" {
		return "rule"
	}
	return gloveboxMode(mode)
}

func parseIntString(v string) int {
	n, _ := strconv.Atoi(v)
	return n
}

func selectedTag(ref string) string {
	if i := strings.IndexByte(ref, '/'); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

type netproxyAppPolicy struct {
	Enabled    bool     `json:"enabled"`
	Mode       string   `json:"mode"`
	ProxyApps  []string `json:"proxy_apps"`
	BypassApps []string `json:"bypass_apps"`
}

type netproxyInboundSettings struct {
	Backend    string            `json:"backend"`
	RootPolicy string            `json:"root_policy"`
	App        netproxyAppPolicy `json:"app"`
	EBPF       json.RawMessage   `json:"ebpf"`
	TUN        json.RawMessage   `json:"tun"`
}

func (s *server) netproxyInbound(ctx context.Context) (netproxyInboundSettings, netproxyConfigSnapshot, error) {
	snapshot, err := s.netproxyReadConfig(ctx, "inbound")
	if err != nil {
		return netproxyInboundSettings{}, snapshot, err
	}
	var settings netproxyInboundSettings
	if err := json.Unmarshal([]byte(snapshot.Content), &settings); err != nil {
		return settings, snapshot, fmt.Errorf("decode NetProxy inbound settings: %w", err)
	}
	return settings, snapshot, nil
}

func netproxyModeIDs(modes []string) []string {
	out := make([]string, 0, len(modes))
	for _, mode := range modes {
		id := gloveboxMode(mode)
		if id == "" {
			continue
		}
		seen := false
		for _, existing := range out {
			if existing == id {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, id)
		}
	}
	return out
}

func (s *server) handleNetProxyProxies(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	st, err := s.netproxyStatus(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, proxyDown(err))
		return
	}
	running := st.State == "ready" || st.State == "started"
	var lists []netproxyNodeList
	if running {
		lists, err = s.netproxyNodes(ctx)
		if err != nil {
			writeErr(w, http.StatusBadGateway, proxyDown(err))
			return
		}
	}
	groups := make([]proxyGroup, 0, len(lists))
	for _, item := range lists {
		if item.Group.Name == "" {
			continue
		}
		g := proxyGroup{Name: item.Group.Name, Type: "Selector", Now: "", Chain: []string{}, Nodes: []proxyNode{}}
		if item.Group.ID == st.ActiveGroupID {
			g.Now = selectedTag(st.SelectedNodeRef)
		}
		for _, node := range item.Nodes {
			g.Nodes = append(g.Nodes, proxyNode{Name: node.Tag, Type: node.Protocol, Delay: netproxyCountDelay(node)})
		}
		groups = append(groups, g)
	}
	providers := []map[string]any{}
	if subs, e := s.netproxySubscriptions(ctx); e == nil {
		for _, sub := range subs {
			providers = append(providers, map[string]any{
				"id": sub.ID, "name": sub.Name, "kind": "proxies", "count": sub.NodeCount,
				"updated_at": sub.UpdatedAt, "active": sub.Active, "error": sub.LastError,
				"auto_update": sub.AutoUpdate, "update_interval": sub.UpdateInterval,
				"update_via_proxy": sub.UpdateViaProxy, "next_update_at": sub.NextUpdateAt,
				"last_success_at": sub.LastSuccessAt, "last_attempt_at": sub.LastAttemptAt,
				"usage": json.RawMessage(sub.Usage), "progress": json.RawMessage(sub.Progress),
			})
		}
	}
	autoStart, autoStartErr := netproxyAutostartFromFile(s.cfg.get())
	settings, _, settingsErr := s.netproxyInbound(ctx)
	mode := formatNetproxyMode(st.OutboundMode)
	out := map[string]any{
		"mode": mode, "groups": groups, "providers": providers,
		"available_modes": netproxyModeIDs(st.AvailableModes),
		"running":         running, "state": st.State, "backend": st.ConfiguredBackend,
		"active_backend": st.ActiveBackend, "pid": st.PID, "uptime_seconds": st.UptimeSeconds,
		"worker_state": st.WorkerState, "worker_pid": st.WorkerPID, "error": st.Error,
		"auto_start": autoStart, "auto_start_known": autoStartErr == nil,
	}
	if settingsErr == nil {
		out["settings"] = map[string]any{
			"backend": settings.Backend, "root_policy": settings.RootPolicy, "app": settings.App,
		}
	}
	if autoStartErr != nil {
		out["settings_error"] = autoStartErr.Error()
	} else if settingsErr != nil {
		out["settings_error"] = settingsErr.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) handleNetProxyService(w http.ResponseWriter, r *http.Request) {
	if s.platform != "xiaomi" {
		writeErr(w, http.StatusNotImplemented, apiErr("netproxy_unavailable", "service controls require integrated NetProxy"))
		return
	}
	var in struct {
		Action string `json:"action"`
	}
	if err := readBody(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	action := strings.ToLower(strings.TrimSpace(in.Action))
	switch action {
	case "start", "stop", "restart":
	default:
		writeErr(w, http.StatusBadRequest, apiErr("service_action_invalid", "action must be start, stop or restart"))
		return
	}
	if s.demo {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": action})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	status, err := s.startNetproxyAction(ctx, action)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	log.Printf("NetProxy service -> %s", action)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": action, "status": status})
}

func (s *server) handleNetProxyAutostart(w http.ResponseWriter, r *http.Request) {
	if s.platform != "xiaomi" {
		writeErr(w, http.StatusNotImplemented, apiErr("netproxy_unavailable", "autostart is available only with integrated NetProxy"))
		return
	}
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if err := readBody(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if in.Enabled == nil {
		writeErr(w, http.StatusBadRequest, apiErr("bad_request", "enabled is required"))
		return
	}
	if s.demo {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": *in.Enabled})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := s.setNetproxyAutostart(ctx, *in.Enabled); err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	log.Printf("NetProxy autostart -> %v", *in.Enabled)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": *in.Enabled})
}

func (s *server) handleNetProxySubscription(w http.ResponseWriter, r *http.Request) {
	if s.platform != "xiaomi" {
		writeErr(w, http.StatusNotImplemented, apiErr("netproxy_unavailable", "subscription management requires integrated NetProxy"))
		return
	}
	var in struct {
		Action         string `json:"action"`
		ID             string `json:"id"`
		URL            string `json:"url"`
		Name           string `json:"name"`
		AutoUpdate     *bool  `json:"auto_update"`
		UpdateInterval string `json:"update_interval"`
		UpdateViaProxy string `json:"update_via_proxy"`
	}
	if err := readBody(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var args []string
	switch in.Action {
	case "add":
		u, err := url.ParseRequestURI(strings.TrimSpace(in.URL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			writeErr(w, http.StatusBadRequest, apiErr("subscription_url_invalid", "subscription URL must be HTTP or HTTPS"))
			return
		}
		args = []string{"sub", "add"}
		if name := strings.TrimSpace(in.Name); name != "" {
			args = append(args, "--name", name)
		}
		if in.AutoUpdate != nil {
			args = append(args, "--auto-update="+strconv.FormatBool(*in.AutoUpdate))
		}
		if interval := strings.TrimSpace(in.UpdateInterval); interval != "" {
			if duration, err := time.ParseDuration(interval); err != nil || duration < 15*time.Minute {
				writeErr(w, http.StatusBadRequest, apiErr("subscription_interval_invalid", "update interval must be at least 15 minutes"))
				return
			}
			args = append(args, "--interval", interval)
		}
		if via := strings.TrimSpace(in.UpdateViaProxy); via != "" {
			if via != "auto" && via != "direct" && via != "proxy" {
				writeErr(w, http.StatusBadRequest, apiErr("subscription_proxy_mode_invalid", "update_via_proxy must be auto, direct or proxy"))
				return
			}
			args = append(args, "--via-proxy", via)
		}
		args = append(args, u.String())
	case "activate", "remove", "details", "history", "cancel", "edit":
		known := false
		if subs, err := s.netproxySubscriptions(r.Context()); err == nil {
			for _, sub := range subs {
				if sub.ID == in.ID {
					known = true
					break
				}
			}
		}
		if !known {
			writeErr(w, http.StatusBadRequest, apiErr("subscription_unknown", "subscription does not exist"))
			return
		}
		if in.Action == "details" {
			args = []string{"sub", "show", "--private", in.ID}
		} else if in.Action == "edit" {
			args = []string{"sub", "edit"}
			if name := strings.TrimSpace(in.Name); name != "" {
				args = append(args, "--name", name)
			}
			if in.AutoUpdate != nil {
				args = append(args, "--auto-update="+strconv.FormatBool(*in.AutoUpdate))
			}
			if rawURL := strings.TrimSpace(in.URL); rawURL != "" {
				u, err := url.ParseRequestURI(rawURL)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
					writeErr(w, http.StatusBadRequest, apiErr("subscription_url_invalid", "subscription URL must be HTTP or HTTPS"))
					return
				}
				args = append(args, "--url", u.String())
			}
			if interval := strings.TrimSpace(in.UpdateInterval); interval != "" {
				if duration, err := time.ParseDuration(interval); err != nil || duration < 15*time.Minute {
					writeErr(w, http.StatusBadRequest, apiErr("subscription_interval_invalid", "update interval must be at least 15 minutes"))
					return
				}
				args = append(args, "--interval", interval)
			}
			if via := strings.TrimSpace(in.UpdateViaProxy); via != "" {
				if via != "auto" && via != "direct" && via != "proxy" {
					writeErr(w, http.StatusBadRequest, apiErr("subscription_proxy_mode_invalid", "update_via_proxy must be auto, direct or proxy"))
					return
				}
				args = append(args, "--via-proxy", via)
			}
			args = append(args, in.ID)
		} else {
			args = []string{"sub", in.Action, in.ID}
		}
	default:
		writeErr(w, http.StatusBadRequest, apiErr("subscription_action_invalid", "unsupported subscription action"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 100*time.Second)
	defer cancel()
	data, err := s.netproxyCallWithTimeout(ctx, 95*time.Second, "90s", args...)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	log.Printf("NetProxy subscription %s", in.Action)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": json.RawMessage(data)})
}

func validPackageRefs(values []string) bool {
	for _, value := range values {
		parts := strings.SplitN(value, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return false
		}
		if _, err := strconv.Atoi(parts[0]); err != nil {
			return false
		}
		for _, part := range strings.Split(parts[1], ".") {
			if part == "" {
				return false
			}
		}
	}
	return true
}

func (s *server) handleNetProxySettings(w http.ResponseWriter, r *http.Request) {
	if s.platform != "xiaomi" {
		writeErr(w, http.StatusNotImplemented, apiErr("netproxy_unavailable", "settings require integrated NetProxy"))
		return
	}
	var in struct {
		Action     string             `json:"action"`
		Backend    string             `json:"backend"`
		RootPolicy string             `json:"root_policy"`
		App        *netproxyAppPolicy `json:"app"`
	}
	if err := readBody(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 70*time.Second)
	defer cancel()
	switch in.Action {
	case "check", "reload":
		if s.demo {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": in.Action})
			return
		}
		b, err := s.netproxyCallWithTimeout(ctx, 55*time.Second, "50s", "service", in.Action)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": in.Action, "data": json.RawMessage(b)})
		return
	case "save":
	default:
		writeErr(w, http.StatusBadRequest, apiErr("settings_action_invalid", "action must be save, check or reload"))
		return
	}
	if in.Backend != "tun" && in.Backend != "ebpf" {
		writeErr(w, http.StatusBadRequest, apiErr("backend_invalid", "backend must be tun or ebpf"))
		return
	}
	if in.RootPolicy != "default" && in.RootPolicy != "include" && in.RootPolicy != "exclude" {
		writeErr(w, http.StatusBadRequest, apiErr("root_policy_invalid", "root policy must be default, include or exclude"))
		return
	}
	if in.App == nil || (in.App.Mode != "blacklist" && in.App.Mode != "whitelist") {
		writeErr(w, http.StatusBadRequest, apiErr("app_policy_invalid", "app policy must use blacklist or whitelist mode"))
		return
	}
	if !validPackageRefs(in.App.ProxyApps) || !validPackageRefs(in.App.BypassApps) {
		writeErr(w, http.StatusBadRequest, apiErr("app_package_invalid", "app entries must use Android user ID and package name, for example 0:com.example.app"))
		return
	}
	if s.demo {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	settings, _, err := s.netproxyInbound(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	settings.Backend = in.Backend
	settings.RootPolicy = in.RootPolicy
	settings.App = *in.App
	content, err := json.Marshal(settings)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.netproxyApplyConfig(ctx, "inbound", content); err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	log.Printf("NetProxy settings saved: backend=%s root=%s app=%v/%s", in.Backend, in.RootPolicy, in.App.Enabled, in.App.Mode)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *server) handleNetProxySelect(w http.ResponseWriter, r *http.Request) {
	var in struct{ Group, Name string }
	if err := readBody(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	lists, err := s.netproxyNodes(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	for _, item := range lists {
		if item.Group.Name != in.Group {
			continue
		}
		for _, node := range item.Nodes {
			if node.Tag == in.Name {
				if _, err := s.netproxyCall(r.Context(), "node", "use", netproxyRef(item.Group, node.Tag)); err != nil {
					writeErr(w, http.StatusBadGateway, err)
					return
				}
				log.Printf("select %s -> %s", in.Group, in.Name)
				writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
				return
			}
		}
	}
	writeErr(w, http.StatusBadRequest, apiErr("node_unknown", "node does not exist"))
}

func (s *server) handleNetProxyDelay(w http.ResponseWriter, r *http.Request) {
	var in struct{ Group, Name string }
	if err := readBody(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	lists, err := s.netproxyNodes(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	// A group-level test is a batch operation. Passing an empty node ref to
	// NetProxy asks it to test every node in the active group; trying to derive
	// a node from selected_node_ref is wrong because URL-test groups may have no
	// manual selection at all.
	if in.Group != "" && in.Name == "" {
		st, e := s.netproxyStatus(r.Context())
		if e != nil {
			writeJSON(w, http.StatusOK, map[string]any{"delays": map[string]int{}, "error": e.Error()})
			return
		}
		groupID := ""
		for _, item := range lists {
			if item.Group.Name == in.Group {
				groupID = item.Group.ID
				if item.Group.ID != st.ActiveGroupID {
					writeJSON(w, http.StatusOK, map[string]any{"delays": map[string]int{}, "error": "only the active node group can be tested"})
					return
				}
				break
			}
		}
		if groupID == "" {
			writeJSON(w, http.StatusOK, map[string]any{"delays": map[string]int{}, "error": "node group does not exist"})
			return
		}
		// NetProxy's empty delay target means only the currently selected node.
		// "auto <catalog-id>" is its public contract for testing every node in
		// a catalog group, including timeout entries for unreachable nodes.
		b, e := s.netproxyCallWithTimeout(r.Context(), 25*time.Second, "20s", "node", "delay", "auto", groupID)
		if e != nil {
			writeJSON(w, http.StatusOK, map[string]any{"delays": map[string]int{}, "error": e.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"delays": parseNetproxyDelays(b)})
		return
	}
	for _, item := range lists {
		if in.Group != "" && item.Group.Name != in.Group {
			continue
		}
		name := in.Name
		if name == "" {
			st, e := s.netproxyStatus(r.Context())
			if e == nil && st.ActiveGroupID == item.Group.ID {
				name = selectedTag(st.SelectedNodeRef)
			}
		}
		if name == "" {
			continue
		}
		for _, node := range item.Nodes {
			if node.Tag != name {
				continue
			}
			b, e := s.netproxyCall(r.Context(), "node", "delay", netproxyRef(item.Group, node.Tag))
			if e != nil {
				writeJSON(w, http.StatusOK, map[string]any{"delays": map[string]int{}, "error": e.Error()})
				return
			}
			d, _ := parseNetproxyDelay(b)
			writeJSON(w, http.StatusOK, map[string]any{"delays": map[string]int{name: d}})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"delays": map[string]int{}, "error": "node does not exist"})
}
