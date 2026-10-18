package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// cellularCell is one record returned by Android's TelephonyManager. Android
// only returns cells the modem elects to expose; no neighbour is synthesized.
type cellularCell struct {
	Type       string `json:"type"`
	Registered bool   `json:"registered"`
	Connection int    `json:"connection"`
	MCC        string `json:"mcc"`
	MNC        string `json:"mnc"`
	Operator   string `json:"operator"`
	CellID     string `json:"cell_id"`
	TAC        string `json:"tac"`
	PCI        string `json:"pci"`
	ARFCN      string `json:"arfcn"`
	Bands      string `json:"bands"`
	DBM        int    `json:"dbm"`
	RSRP       int    `json:"rsrp"`
	RSRQ       int    `json:"rsrq"`
	SINR       int    `json:"sinr"`
	Source     string `json:"source"`
	LastSeen   string `json:"last_seen,omitempty"`
}

type carrierComponent struct {
	Role              string `json:"role"`
	NetworkType       string `json:"network_type"`
	Band              string `json:"band"`
	DownlinkChannel   string `json:"downlink_channel"`
	UplinkChannel     string `json:"uplink_channel"`
	PCI               string `json:"pci"`
	DownlinkBandwidth int    `json:"downlink_bandwidth_khz"`
	UplinkBandwidth   int    `json:"uplink_bandwidth_khz"`
	Source            string `json:"source"`
}

type cellularSnapshot struct {
	Time               string             `json:"time"`
	ScanTime           string             `json:"scan_time,omitempty"`
	SelectionMode      string             `json:"selection_mode"`
	RadioRegistered    bool               `json:"radio_registered"`
	RadioNetworkType   string             `json:"radio_network_type"`
	DataConnected      bool               `json:"data_connected"`
	ExactLockSupported bool               `json:"exact_lock_supported"`
	ScanSupported      bool               `json:"scan_supported"`
	Cells              []cellularCell     `json:"cells"`
	Aggregation        []carrierComponent `json:"aggregation"`
}

func parseCellHelper(raw string) (cellularSnapshot, error) {
	out := cellularSnapshot{ExactLockSupported: false, ScanSupported: true, Cells: []cellularCell{}, Aggregation: []carrierComponent{}}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "MODE=") {
			out.SelectionMode = strings.TrimSpace(strings.TrimPrefix(line, "MODE="))
			continue
		}
		if !strings.HasPrefix(line, "CELL\t") {
			continue
		}
		m := parseTabFields(line)
		out.Cells = append(out.Cells, cellularCell{
			Type: m["type"], Registered: m["registered"] == "true",
			Connection: toInt(m["connection"]), MCC: m["mcc"], MNC: m["mnc"],
			Operator: m["operator"], CellID: validCellField(m["cell_id"]), TAC: validCellField(m["tac"]),
			PCI: validCellField(m["pci"]), ARFCN: validCellField(m["arfcn"]), Bands: validCellField(m["bands"]),
			DBM: signalInt(m["dbm"], -160, -20), RSRP: signalInt(m["rsrp"], -160, -20),
			RSRQ: signalInt(m["rsrq"], -50, 20), SINR: signalInt(m["sinr"], -30, 80),
		})
	}
	if out.SelectionMode == "" {
		out.SelectionMode = "unknown"
	}
	sort.SliceStable(out.Cells, func(i, j int) bool {
		if out.Cells[i].Registered != out.Cells[j].Registered {
			return out.Cells[i].Registered
		}
		return out.Cells[i].DBM > out.Cells[j].DBM
	})
	if strings.Contains(raw, "ERROR=") && len(out.Cells) == 0 {
		return out, errors.New(strings.TrimSpace(raw))
	}
	return out, nil
}

func parseTabFields(line string) map[string]string {
	out := map[string]string{}
	for _, field := range strings.Split(line, "\t")[1:] {
		if k, v, ok := strings.Cut(field, "="); ok {
			out[k] = v
		}
	}
	return out
}

func toInt(v string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(v))
	return n
}

func validCellField(v string) string {
	switch strings.TrimSpace(v) {
	case "", "-1", "2147483647", "9223372036854775807":
		return ""
	}
	return strings.TrimSpace(v)
}

func signalInt(v string, min, max int) int {
	n := toInt(v)
	if n < min || n > max {
		return 0
	}
	return n
}

var physicalConfigRE = regexp.MustCompile(`\{([^{}]*mConnectionStatus=[^{}]*)\}`)

func parseCarrierAggregation(raw string, cells []cellularCell) []carrierComponent {
	out := make([]carrierComponent, 0)
	line := ""
	for _, candidate := range strings.Split(raw, "\n") {
		if strings.Contains(candidate, "mPhysicalChannelConfigs=") {
			line = candidate
			break
		}
	}
	for _, match := range physicalConfigRE.FindAllStringSubmatch(line, -1) {
		fields := map[string]string{}
		for _, part := range strings.Split(match[1], ",") {
			if k, v, ok := strings.Cut(strings.TrimSpace(part), "="); ok {
				fields[k] = strings.TrimSpace(v)
			}
		}
		component := carrierComponent{
			Role: fields["mConnectionStatus"], NetworkType: fields["mNetworkType"],
			Band: fields["mBand"], DownlinkChannel: fields["mDownlinkChannelNumber"],
			UplinkChannel: fields["mUplinkChannelNumber"], PCI: fields["mPhysicalCellId"],
			DownlinkBandwidth: toInt(fields["mCellBandwidthDownlinkKhz"]),
			UplinkBandwidth:   toInt(fields["mCellBandwidthUplinkKhz"]), Source: "physical_channel_config",
		}
		// HyperOS currently publishes placeholder PCC/SCC records full of zeroes.
		// Omitting them is more accurate than presenting a fake second carrier.
		networkKnown := component.NetworkType != "" && !strings.EqualFold(component.NetworkType, "UNKNOWN")
		valid := networkKnown && (component.Band != "" && component.Band != "0" ||
			component.DownlinkChannel != "" && component.DownlinkChannel != "0" ||
			component.PCI != "" && component.PCI != "0" || component.DownlinkBandwidth > 0 ||
			component.UplinkBandwidth > 0)
		if valid {
			out = append(out, component)
		}
	}
	if len(out) == 0 {
		for _, cell := range cells {
			if !cell.Registered {
				continue
			}
			out = append(out, carrierComponent{Role: "PrimaryServing", NetworkType: cell.Type,
				Band: cell.Bands, DownlinkChannel: cell.ARFCN, PCI: cell.PCI, Source: "registered_cell"})
			break
		}
	}
	return out
}

func (s *server) runCellHelper(ctx context.Context, args ...string) (string, error) {
	c := s.cfg.get()
	jar := filepath.Join(c.TurboDir, "bin/wifi-turbo-helper.jar")
	timeout := 12 * time.Second
	if len(args) > 0 && args[0] == "scan" {
		timeout = 50 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmdArgs := append([]string{"/system/bin", "com.glovebox.cellular.CellularConfigurator"}, args...)
	cmd := exec.CommandContext(ctx, "app_process", cmdArgs...)
	cmd.Env = append(os.Environ(), "CLASSPATH="+jar+":/system_ext/framework/extphonelib.jar")
	b, err := cmd.CombinedOutput()
	if err != nil {
		return string(b), fmt.Errorf("cellular helper: %w: %s", err, strings.TrimSpace(string(b)))
	}
	return string(b), nil
}

func (s *server) cellularStatus(ctx context.Context, fresh bool) (cellularSnapshot, error) {
	s.cellMu.Lock()
	defer s.cellMu.Unlock()
	if !fresh && time.Since(s.cellAt) < 1500*time.Millisecond {
		return s.cellCache, s.cellErr
	}
	if s.platform != "xiaomi" {
		return cellularSnapshot{}, apiErr("cellular_unsupported", "cellular controls are not available on this platform")
	}
	raw, err := s.runCellHelper(ctx, "cells")
	var snap cellularSnapshot
	if err == nil {
		snap, err = parseCellHelper(raw)
	}
	if err == nil {
		for i := range snap.Cells {
			snap.Cells[i].Source = "live"
			snap.Cells[i].LastSeen = time.Now().Format(time.RFC3339)
		}
		snap.Cells = mergeCellLists(snap.Cells, s.cellScan)
		if !s.cellScanAt.IsZero() {
			snap.ScanTime = s.cellScanAt.Format(time.RFC3339)
		}
		dump, _ := run(ctx, 4*time.Second, "dumpsys", "telephony.registry")
		service := parseXiaomiServiceState(dump)
		snap.RadioRegistered = service.DataRegistered
		snap.RadioNetworkType = service.NetworkType
		_, wanIface := firstWANIPv4(s.cfg.get().WANPrefixes)
		snap.DataConnected = service.DataRegistered && wanIface != ""
		snap.Aggregation = parseCarrierAggregation(dump, snap.Cells)
		snap.Time = time.Now().Format(time.RFC3339)
	}
	s.cellCache, s.cellErr, s.cellAt = snap, err, time.Now()
	return snap, err
}

func (s *server) handleCells(w http.ResponseWriter, r *http.Request) {
	snap, err := s.cellularStatus(r.Context(), r.URL.Query().Get("fresh") == "1")
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *server) handleCellScan(w http.ResponseWriter, r *http.Request) {
	if s.platform != "xiaomi" {
		writeErr(w, http.StatusNotImplemented, apiErr("cellular_unsupported", "network scan is not available on this platform"))
		return
	}
	out, err := s.runCellHelper(r.Context(), "scan")
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}
	snap, err := parseCellHelper(out)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}
	scanTime := time.Now()
	for i := range snap.Cells {
		snap.Cells[i].Source = "scan"
		snap.Cells[i].LastSeen = scanTime.Format(time.RFC3339)
	}
	// Network-scan results often redact NCI/TAC/PCI. Merge the fresh serving
	// cell identity without turning redacted neighbour data into fake values.
	if currentRaw, currentErr := s.runCellHelper(r.Context(), "cells"); currentErr == nil {
		if current, parseErr := parseCellHelper(currentRaw); parseErr == nil {
			snap.SelectionMode = current.SelectionMode
			for i := range current.Cells {
				current.Cells[i].Source = "live"
				current.Cells[i].LastSeen = time.Now().Format(time.RFC3339)
			}
			for i := range snap.Cells {
				for _, serving := range current.Cells {
					if !serving.Registered || snap.Cells[i].Type != serving.Type || cellPLMN(snap.Cells[i]) != cellPLMN(serving) {
						continue
					}
					serving.Operator = snap.Cells[i].Operator
					snap.Cells[i] = serving
					break
				}
			}
		}
	}
	dump, _ := run(r.Context(), 4*time.Second, "dumpsys", "telephony.registry")
	service := parseXiaomiServiceState(dump)
	snap.RadioRegistered = service.DataRegistered
	snap.RadioNetworkType = service.NetworkType
	_, wanIface := firstWANIPv4(s.cfg.get().WANPrefixes)
	snap.DataConnected = service.DataRegistered && wanIface != ""
	snap.Aggregation = parseCarrierAggregation(dump, snap.Cells)
	snap.Time = time.Now().Format(time.RFC3339)
	snap.ScanTime = scanTime.Format(time.RFC3339)
	s.cellMu.Lock()
	s.cellScan, s.cellScanAt = append([]cellularCell(nil), snap.Cells...), scanTime
	s.cellCache, s.cellErr, s.cellAt = snap, nil, time.Now()
	s.cellMu.Unlock()
	writeJSON(w, http.StatusOK, snap)
}

func cellPLMN(c cellularCell) string { return c.MCC + c.MNC }

func mergeCellLists(live, scanned []cellularCell) []cellularCell {
	out := append([]cellularCell(nil), live...)
	for _, old := range scanned {
		found := false
		for _, current := range live {
			sameNetwork := current.Type == old.Type && cellPLMN(current) == cellPLMN(old)
			sameIdentity := current.CellID != "" && current.CellID == old.CellID ||
				current.PCI != "" && current.PCI == old.PCI && current.ARFCN == old.ARFCN
			if sameNetwork && (sameIdentity || current.Registered || old.Registered) {
				found = true
				break
			}
		}
		if !found {
			old.Registered = false
			old.Connection = 0
			out = append(out, old)
		}
	}
	return out
}

func (s *server) handleCellSelection(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode string `json:"mode"`
		PLMN string `json:"plmn"`
	}
	if err := readBody(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var args []string
	switch strings.ToLower(strings.TrimSpace(in.Mode)) {
	case "automatic":
		args = []string{"auto"}
	case "manual":
		if ok, _ := regexp.MatchString(`^[0-9]{5,6}$`, in.PLMN); !ok {
			writeErr(w, http.StatusBadRequest, apiErr("plmn_invalid", "PLMN must contain 5 or 6 digits"))
			return
		}
		args = []string{"manual", in.PLMN}
	default:
		writeErr(w, http.StatusBadRequest, apiErr("selection_mode_invalid", "selection mode must be automatic or manual"))
		return
	}
	out, err := s.runCellHelper(r.Context(), args...)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}
	s.cellMu.Lock()
	s.cellAt = time.Time{}
	s.cellMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": strings.TrimSpace(out)})
}
