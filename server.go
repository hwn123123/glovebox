package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"
)

type server struct {
	cfg    *configStore
	auth   *auth
	zte    *zteClient
	mihomo *mihomoClient
	net    *netSampler
	ts     *tsProbe
	certs  *certManager // nil when HTTPS is off
	logDir string
	// platform is resolved once at startup ("zte" or "xiaomi").
	platform string
	// demo mode: in-process fake ZTE / mihomo backends, synthetic system info,
	// destructive actions are only logged
	demo bool
	sys  func() sysInfo

	zteMu    sync.Mutex
	zteCache map[string]any
	zteAt    time.Time
	zteErr   error

	sigMu   sync.Mutex
	sigHist []sigSample

	capMu  sync.Mutex
	capMax int
	capAt  time.Time

	cellMu     sync.Mutex
	cellCache  cellularSnapshot
	cellAt     time.Time
	cellErr    error
	cellScan   []cellularCell
	cellScanAt time.Time
	// cellControlMu serializes modem selection requests and their safety rollback.
	cellControlMu   sync.Mutex
	cellSelectionID uint64
}

func newServer(cs *configStore, logDir string) *server {
	c := cs.get()
	platform := detectPlatform(c.Platform)
	proxyURL, proxySecret, proxyCert, proxyKey := proxyController(c)
	mc := newMihomo(proxyURL, proxySecret, c.MihomoInsecure, proxyCert, proxyKey)
	return &server{
		cfg:      cs,
		auth:     &auth{cfg: cs},
		zte:      newZTE(c.ZTEBase, c.ZTEDB),
		mihomo:   mc,
		net:      &netSampler{wanPrefixes: c.WANPrefixes, lanDev: c.LANDev, xiaomi: platform == "xiaomi"},
		ts:       &tsProbe{m: mc, cfg: cs},
		logDir:   logDir,
		platform: platform,
		sys:      readSysInfo,
	}
}

// run starts the samplers and the HTTP / HTTPS listeners until ctx is cancelled
func (s *server) run(ctx context.Context) error {
	c := s.cfg.get()
	go s.net.loop(ctx)
	go s.sigLoop(ctx)
	go s.ts.loop(ctx)
	go ensureAlias(ctx, c.AliasAddr, c.AliasDev)
	if s.platform == "xiaomi" && !s.demo {
		go s.bootXiaomiRouter(ctx)
	}

	h := s.routes()
	newSrv := func() *http.Server {
		return &http.Server{
			Handler:           h,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       2 * time.Minute,
			ErrorLog:          log.New(quietErrors{}, "http: ", 0),
		}
	}
	// HTTP and HTTPS use separate servers: net/http configures HTTP/2 once per server, and a
	// plain Serve that runs first would leave the TLS listener advertising h2 without a handler.
	srv := newSrv()
	servers := []*http.Server{srv}
	serveRetry := func(srv *http.Server, addr string, tlsMode bool) {
		for ctx.Err() == nil {
			ln, err := listenTCP(addr)
			if err == nil {
				if tlsMode {
					log.Printf("glovebox %s listening on https://%s", version, addr)
					err = srv.ServeTLS(ln, "", "")
				} else {
					log.Printf("glovebox %s listening on http://%s", version, addr)
					err = srv.Serve(ln)
				}
			}
			if ctx.Err() != nil || err == http.ErrServerClosed {
				return
			}
			log.Printf("listener %s unavailable, retrying: %v", addr, err)
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
	for _, addr := range c.Listen {
		go serveRetry(srv, addr, false)
	}
	if s.certs != nil && len(c.TLSListen) > 0 {
		tlsSrv := newSrv()
		tlsSrv.TLSConfig = &tls.Config{GetCertificate: s.certs.getCertificate, MinVersion: tls.VersionTLS12}
		servers = append(servers, tlsSrv)
		for _, addr := range c.TLSListen {
			go serveRetry(tlsSrv, addr, true)
		}
	}
	<-ctx.Done()
	sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, sv := range servers {
		sv.Shutdown(sctx)
	}
	return nil
}

// quietErrors forwards net/http server errors to the log, except TLS handshake failures, which
// every browser that has not trusted the local CA yet produces on each visit
type quietErrors struct{}

func (quietErrors) Write(p []byte) (int, error) {
	if !bytes.Contains(p, []byte("TLS handshake error")) {
		log.Print(string(p))
	}
	return len(p), nil
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /api/session", s.handleSession)
	mux.HandleFunc("GET /ca.crt", s.handleCA)

	authed := func(pattern string, h http.HandlerFunc) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if !s.auth.valid(r) {
				writeErr(w, http.StatusUnauthorized, apiErr("unauthorized", "please log in"))
				return
			}
			h(w, r)
		})
	}
	authed("GET /api/info", s.handleInfo)
	authed("GET /api/status", s.handleStatus)
	authed("GET /api/cells", s.handleCells)
	authed("POST /api/cells/scan", s.handleCellScan)
	authed("POST /api/cells/selection", s.handleCellSelection)
	authed("GET /api/proxies", s.handleProxies)
	authed("POST /api/proxies/select", s.handleSelect)
	authed("POST /api/proxies/delay", s.handleDelay)
	authed("POST /api/proxies/mode", s.handleMode)
	authed("POST /api/proxies/update", s.handleUpdateProviders)
	authed("POST /api/proxies/close-connections", s.handleCloseConns)
	authed("POST /api/proxies/service", s.handleNetProxyService)
	authed("POST /api/proxies/autostart", s.handleNetProxyAutostart)
	authed("POST /api/proxies/settings", s.handleNetProxySettings)
	authed("POST /api/proxies/terminal", s.handleNetProxyTerminal)
	authed("GET /api/connections", s.handleConnections)
	authed("POST /api/tailscale/check", s.handleTSCheck)
	authed("GET /api/clients", s.handleClients)
	authed("GET /api/wifi", s.handleWiFi)
	authed("POST /api/wifi", s.handleSetWiFi)
	authed("POST /api/wifi/turbo", s.handleSetTurbo)
	authed("POST /api/usb", s.handleSetUSB)
	authed("POST /api/proxies/subscriptions", s.handleNetProxySubscription)
	authed("POST /api/box/restart", s.handleBoxRestart)
	authed("POST /api/device/reboot", s.handleReboot)
	authed("POST /api/device/time", s.handleSetTime)
	authed("POST /api/console/password", s.handlePassword)
	authed("GET /api/logs", s.handleLogs)

	mux.Handle("GET /", staticHandler())
	return s.guard(mux)
}

const csp = "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; " +
	"connect-src 'self'; worker-src 'self'; manifest-src 'self'; frame-src 'self'; frame-ancestors 'self'; base-uri 'none'; form-action 'self'"

// guard only lets the hotspot subnet and localhost in. Non-GET requests must carry the
// X-Glovebox header, which cross-site pages cannot set without a CORS preflight (CSRF protection).
func (s *server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := s.cfg.get()
		allowed := allowedRemote(r.RemoteAddr, c.AllowCIDRs) ||
			allowedOnInterface(r.RemoteAddr, c.LANDev)
		if s.platform == "xiaomi" {
			allowed = allowed || allowedOnXiaomiDownstream(r.RemoteAddr, c.LANDev)
		}
		if !allowed {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-Glovebox") != "1" {
			http.Error(w, "missing X-Glovebox header", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// allowedOnXiaomiDownstream admits clients from the address range Android assigned to any
// active Wi-Fi or USB tethering interface. This keeps DHCP fully owned by NetworkStack while
// letting USB clients reach the same Glovebox endpoint as hotspot clients.
func allowedOnXiaomiDownstream(remote, wifiDev string) bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, ifc := range ifaces {
		if xiaomiDownstream(ifc.Name, wifiDev) && allowedOnInterface(remote, ifc.Name) {
			return true
		}
	}
	return false
}

// allowedOnInterface admits only peers in one of the LAN interface's directly connected
// prefixes. Android chooses the tether subnet dynamically, so a static CIDR is not reliable.
func allowedOnInterface(remote, dev string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil || dev == "" {
		return false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	ifc, err := net.InterfaceByName(dev)
	if err != nil {
		return false
	}
	addrs, err := ifc.Addrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		if p, err := netip.ParsePrefix(addr.String()); err == nil && p.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}

func allowedRemote(remote string, cidrs []string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	for _, c := range cidrs {
		if p, err := netip.ParsePrefix(c); err == nil && p.Contains(ip) {
			return true
		}
	}
	return false
}
