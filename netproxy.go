package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	out, err := run(ctx, 15*time.Second, filepath.Join(c.NetProxyDir, "netproxyctl"), all...)
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
	var groups []proxyGroup
	for _, item := range lists {
		if item.Group.Name == "" {
			continue
		}
		g := proxyGroup{Name: item.Group.Name, Type: "Selector", Now: "", Chain: nil}
		if item.Group.ID == st.ActiveGroupID {
			g.Now = selectedTag(st.SelectedNodeRef)
		}
		for _, node := range item.Nodes {
			g.Nodes = append(g.Nodes, proxyNode{Name: node.Tag, Type: node.Protocol, Delay: netproxyCountDelay(node)})
		}
		groups = append(groups, g)
	}
	providers := []map[string]any{}
	if b, e := s.netproxyCall(ctx, "catalog", "list"); e == nil {
		var cats []struct {
			Name      string    `json:"name"`
			Type      string    `json:"type"`
			NodeCount int       `json:"node_count"`
			UpdatedAt time.Time `json:"updated_at"`
		}
		if json.Unmarshal(b, &cats) == nil {
			for _, cat := range cats {
				providers = append(providers, map[string]any{"name": cat.Name, "kind": "proxies", "count": cat.NodeCount, "updated_at": cat.UpdatedAt})
			}
		}
	}
	mode := formatNetproxyMode(st.OutboundMode)
	writeJSON(w, http.StatusOK, map[string]any{"mode": mode, "groups": groups, "providers": providers})
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
