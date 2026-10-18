package main

import (
	"context"
	"encoding/json"
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
	c := s.cfg.get()
	if c.NetProxyDir == "" {
		return nil, fmt.Errorf("netproxy directory is not configured")
	}
	all := []string{"--json", "--timeout", "10s"}
	all = append(all, args...)
	out, err := run(ctx, 15*time.Second, netproxyExecutable(c), all...)
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
		return nil, apiErr("netproxy_error", msg)
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
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	Active         bool      `json:"active"`
	NodeCount      int       `json:"node_count"`
	AutoUpdate     bool      `json:"auto_update"`
	UpdateInterval int       `json:"update_interval"`
	UpdatedAt      time.Time `json:"updated_at"`
	LastError      string    `json:"last_error"`
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
	State             string   `json:"state"`
	ConfiguredBackend string   `json:"configured_backend"`
	ActiveGroupID     string   `json:"active_group_id"`
	ActiveGroupName   string   `json:"active_group_name"`
	SelectedNodeRef   string   `json:"selected_node_ref"`
	OutboundMode      string   `json:"outbound_mode"`
	AvailableModes    []string `json:"available_outbound_modes"`
	ConnectionsIn     int      `json:"connections_in"`
	ConnectionsOut    int      `json:"connections_out"`
	UploadTotal       int64    `json:"upload_total"`
	DownloadTotal     int64    `json:"download_total"`
	MemoryBytes       int64    `json:"memory_bytes"`
	Error             string   `json:"error"`
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
	case "rule", "ruleallowads":
		return "rule"
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

func (s *server) handleNetProxyProxies(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	lists, err := s.netproxyNodes(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, proxyDown(err))
		return
	}
	st, _ := s.netproxyStatus(ctx)
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
			})
		}
	}
	mode := formatNetproxyMode(st.OutboundMode)
	running := st.State == "ready" || st.State == "started"
	writeJSON(w, http.StatusOK, map[string]any{
		"mode": mode, "groups": groups, "providers": providers,
		"running": running, "state": st.State, "backend": st.ConfiguredBackend, "error": st.Error,
	})
}

func (s *server) handleNetProxySubscription(w http.ResponseWriter, r *http.Request) {
	if s.platform != "xiaomi" {
		writeErr(w, http.StatusNotImplemented, apiErr("netproxy_unavailable", "subscription management requires integrated NetProxy"))
		return
	}
	var in struct {
		Action string `json:"action"`
		ID     string `json:"id"`
		URL    string `json:"url"`
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
		args = []string{"sub", "add", u.String()}
	case "activate", "remove":
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
		args = []string{"sub", in.Action, in.ID}
	default:
		writeErr(w, http.StatusBadRequest, apiErr("subscription_action_invalid", "action must be add, activate or remove"))
		return
	}
	data, err := s.netproxyCall(r.Context(), args...)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	log.Printf("NetProxy subscription %s", in.Action)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": json.RawMessage(data)})
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
