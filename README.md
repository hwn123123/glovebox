# Glovebox

[![CI](https://github.com/Cluas/glovebox/actions/workflows/ci.yml/badge.svg)](https://github.com/Cluas/glovebox/actions/workflows/ci.yml)

**A LuCI-style web console for rooted Android mobile routers.** Glovebox runs as root on the
stock firmware of a pocket 5G/4G Wi-Fi router and gives you one place to watch the cellular link,
manage the hotspot, drive a [mihomo](https://github.com/MetaCubeX/mihomo) or NetProxy sing-box proxy and read logs —
without flashing OpenWrt.

English · [简体中文](README.zh-CN.md)

![Overview](docs/screenshots/en/overview.png)

- **One self-contained Magisk module** with the Glovebox web UI, Android system bridge and embedded
  NetProxy/sing-box runtime; no second management module or web interface is required.
- **Works with the stock firmware**: supports the ZTE goform stack and Xiaomi 14 Pro / HyperOS
  through Android hotspot APIs, NetProxy's root CLI and `/proc`, `/sys` — the vendor UI and apps keep working.
- **Installable app (PWA)** with a responsive layout for desktop, tablets, phones (portrait and
  landscape) and in-vehicle / embedded browsers. HTTPS via a generated, name-constrained local CA.
- **Internationalized** (English, 简体中文); adding a language is adding one JSON file.
- **Demo mode** runs the whole console on any computer with fake backends.

## Features

| Page | What you get |
| --- | --- |
| Overview | Live WAN throughput chart, signal (RSRP/SINR/band) with history, egress node and proxy chain, Tailscale reachability probe, monthly data usage, battery/CPU/memory/temperature gauges, connected clients |
| Cellular | Live registration and data state, modem-reported serving/candidate cells, on-demand network scans, carrier components, automatic/manual PLMN selection with offline safety rollback |
| Proxy | mihomo or embedded NetProxy groups and nodes, complete start/stop/restart and boot policy, subscriptions with scheduling/history, node selection and batch latency tests, rule/ad-block/global/direct modes, TUN/eBPF and app/root policy, live connections and traffic totals |
| Device | Android-managed hotspot and USB tethering with separate runtime/boot controls, AP Turbo status, fixed `192.168.88.1` management address, SSID/password and Wi-Fi 6/32-client tuning, connected devices, HTTPS & app install guide, reboot, clock sync, console password, theme/language |
| Logs | mihomo, Box for Root, Glovebox and Android logcat — level filter, grep with highlight, follow/pause, wrap |

Keyboard: `1`–`5` switch pages, `/` focuses the filter.

<p>
<img src="docs/screenshots/en/proxy.png" width="49%" alt="Proxy">
<img src="docs/screenshots/en/logs.png" width="49%" alt="Logs">
</p>
<p>
<img src="docs/screenshots/en/device.png" width="66%" alt="Device">
<img src="docs/screenshots/en/phone.png" width="22%" alt="Phone layout">
</p>

## Supported devices

Glovebox supports the **ZTE F50** (Unisoc platform, Android 13, firmware B09) and the tested
**Xiaomi 14 Pro / shennong** HyperOS Android 16 integration. Hardware access is kept behind small
adapters (`zte.go`, `xiaomi.go`, `netproxy.go`, `mihomo.go`, `sys.go`), so other Android routers can
be added — contributions are welcome.

Requirements on the device:

- Root with [Magisk](https://github.com/topjohnwu/Magisk)
- Optional on ZTE: [Box for Root](https://github.com/taamarin/box_for_magisk) running mihomo with the
  external controller on `127.0.0.1:9091`
- Xiaomi 14 Pro: Glovebox includes NetProxy 8.6.1 and sing-box. A previously installed standalone
  NetProxy module is stopped and archived outside Magisk's active module directory on first start.
  The embedded converter carries a documented compatibility patch for pinned-certificate Hysteria2
  subscriptions when the bundled sing-box core uses ChromeParrot.

## Install

1. Download `glovebox-<version>.zip` from the [releases page](https://github.com/Cluas/glovebox/releases) (or [build it](#build)) and install it
   in the Magisk app, then reboot.
2. Set the console password (once):

   ```sh
   adb shell su -c "'echo YOUR-PASSWORD | /data/adb/modules/glovebox/bin/glovebox passwd'"
   ```

3. Join the hotspot and open **http://192.168.88.1:80**. On Xiaomi 14 Pro, the integrated AP
   controller starts `nwrt` / `1234567890`, keeps the fixed address on the hotspot interface and
   enables Android USB tethering. Glovebox is the only management WebUI.

For a friendly name, resolve `glovebox.lan` to `192.168.88.1`, e.g. with mihomo:

```yaml
hosts:
  glovebox.lan: 192.168.88.1
```

### HTTPS and installing as an app

Browsers only install PWAs from secure origins. On first start Glovebox creates a local root CA in
`/data/adb/glovebox/tls/` and serves HTTPS on `192.168.88.1:443`. The CA is **name-constrained**
to the configured hostnames and listen IPs, so it cannot sign certificates for any other site even
if its key leaks. The Device page has a download button and step-by-step instructions for iOS,
Android, macOS and Windows; compare the SHA-256 fingerprint shown there before trusting it.

Prefer your own certificate? Set `tls_cert` / `tls_key`.

### Hotspot client limit

On Xiaomi 14 Pro the integrated SoftApConfiguration helper requests 32 clients and the bundled
Qualcomm driver profile enables the matching peer and IPA limits; the Device page reports the
actual running value. The optional static resource overlay is retained for other Android routers.
Build with `NO_OVERLAY=1` when the Android SDK/JDK is unavailable or to leave vendor overlays alone.
If the hotspot misbehaves, delete `/data/adb/modules/glovebox/system` and reboot.

## Configuration

`/data/adb/glovebox/config.json` stores only values that differ from the defaults. Inspect the
effective configuration with `glovebox config`, merge changes with `glovebox config import`, then
restart (`su -c 'pkill -f bin/glovebox'`; the supervisor starts it again):

```sh
echo '{"tailscale_proxy":"Tailscale","tailscale_probe":"http://100.64.0.1/"}' \
  | su -c '/data/adb/modules/glovebox/bin/glovebox config import'
```

| Key | Default | Description |
| --- | --- | --- |
| `platform` | `auto` | `xiaomi` or `zte`; auto detects `shennong` |
| `listen` | `["192.168.88.1:80"]` | HTTP listen addresses |
| `tls_listen` | `["192.168.88.1:443"]` | HTTPS listen addresses; empty disables HTTPS |
| `tls_cert`, `tls_key` | | Custom certificate and key (PEM) instead of the local CA |
| `hostnames` | `["glovebox.lan"]` | Names in issued certificates; the first is used for the HTTPS link |
| `alias_addr`, `alias_dev` | `192.168.88.1/32`, `wlan2` | Address Glovebox adds to the hotspot interface; empty disables it |
| `allow_cidrs` | loopback plus the live `lan_dev` prefix | Source networks allowed to connect |
| `zte_base`, `zte_db` | `http://192.168.0.1:8080`, vendor DB path | Vendor web API and the database holding its login hash |
| `mihomo_api`, `mihomo_secret`, `mihomo_insecure` | `https://127.0.0.1:9091` | mihomo controller (legacy ZTE; Xiaomi uses NetProxy CLI) |
| `main_group` | first visible selector | Proxy group shown on the overview |
| `box_dir` | `/data/adb/box` | Box for Root directory (logs, restart script) |
| `netproxy_dir`, `turbo_dir` | `/data/adb/modules/glovebox/netproxy`, `/data/adb/modules/glovebox` | Embedded NetProxy and integrated AP controller paths |
| `hotspot_ssid`, `hotspot_password`, `hotspot_hidden` | `nwrt`, `1234567890`, `false` | Android system hotspot profile managed from the Device page |
| `auto_start_hotspot`, `auto_start_usb_tether` | `true`, `true` | Restore Android hotspot and USB tethering after boot; USB runtime and boot policy can be controlled independently, while Android NetworkStack remains the only DHCP/NAT owner |
| `tailscale_proxy`, `tailscale_probe` | | mihomo outbound and a tailnet URL to probe through it; both enable the Tailnet card |
| `delay_url` | `https://www.gstatic.com/generate_204` | URL for latency tests |
| `wan_prefixes`, `lan_dev` | `sipa_eth`, `rmnet`, `ccmni` / `br0` | Interfaces used for traffic rates |

Files in `/data/adb/glovebox/`: `config.json`, `glovebox.log` (rotated at 1 MiB),
`stdout.log` (output of the supervisor in `service.sh`) and `tls/` (the local CA). They survive
module upgrades and uninstalls.

## How it works

```
 browser ──HTTP/HTTPS──▶ glovebox (root, 192.168.88.1:80)
                           ├─ Xiaomi Android/AP adapter + AP Turbo (fixed address, Wi-Fi tuning, clients)
                           ├─ embedded NetProxy/sing-box (groups, nodes, subscriptions, mode, traffic)
                           ├─ vendor web API   192.168.0.1:8080  (ZTE signal, SIM, usage, clients, Wi-Fi)
                           ├─ mihomo REST API   127.0.0.1:9091   (legacy ZTE groups, nodes, providers, connections)
                           ├─ /proc, /sys                       (rates, CPU, memory, battery, thermal)
                           └─ shell                              (box.service, logcat, reboot, date)
```

The vendor login hash is read from the vendor database instead of storing the admin password; a
hash that failed once is never retried, to avoid tripping the vendor lockout. Writes carry the
vendor's request signature (`AD`). The vendor firewall only admits 192.168.0.0/24 to port 8080,
which is why Glovebox talks to 192.168.0.1 rather than loopback.

## Security

- The console only accepts connections from `allow_cidrs` (the hotspot LAN by default).
- Passwords are stored as PBKDF2-SHA256; sessions are HMAC-signed cookies that become invalid
  when the password changes. Failed logins lock out with exponential back-off.
- State-changing requests require an `X-Glovebox: 1` header (CSRF protection); responses carry a
  strict Content-Security-Policy.
- Glovebox runs as root and can reboot the device and change Wi-Fi settings — treat the console
  password accordingly. See [SECURITY.md](SECURITY.md) to report issues.

## Build

Requires Go 1.24+. Rebuilding the Android bridge and optional overlay additionally needs Android
SDK 36 build-tools and `android.jar` (`ANDROID_SDK_ROOT`), plus a Java 17 `javac`. Checked-in build
artifacts are used when these tools are absent.

```sh
./build.sh                    # dist/stage/ and dist/glovebox-<version>.zip
NO_OVERLAY=1 ./build.sh       # without the client-limit overlay
VERSION=1.0.0 ./build.sh      # explicit version
GLOVEBOX_PASSWORD_FILE=pw.txt ./deploy.sh   # push to a device over adb and restart, no reboot
```

### Demo mode

```sh
go run . demo                 # http://127.0.0.1:8080, password "glovebox"
```

Demo mode serves the real UI against in-process fakes of the vendor API and mihomo with synthetic
traffic, which is handy for UI work, screenshots and tests.

### Tests

```sh
go test ./...
```

Besides unit and API tests, the suite checks that every locale has every key and no unused ones,
that source files stay English-only, and that README translations match this file's structure.

## Translations

UI strings live in [`web/i18n`](web/i18n). To add a language, copy `en.json` to `<code>.json`,
translate the values, and add the code with its native name to `index.json`. API errors are
translated through `err.<code>` keys; unknown codes fall back to the English message. The UI
picks the browser language and can be switched on the Device page.

This README is also available as `README.<code>.md`; translations keep the same sections,
tables, code blocks and screenshots (`docs/screenshots/<code>/`), which the tests check.

## Acknowledgements

Design inspired by OpenWrt LuCI, [zashboard](https://github.com/Zephyruso/zashboard),
[metacubexd](https://github.com/MetaCubeX/metacubexd), [WebTUI](https://github.com/webtui/webtui)
and futuristic UI work collected by [daisyUI](https://trends.daisyui.com/futuristic-speculative-ui/).
Bundled font: [JetBrains Mono](https://www.jetbrains.com/lp/mono/) (SIL Open Font License 1.1,
see `web/fonts/OFL.txt`).

## License

[MIT](LICENSE). Not affiliated with ZTE, Unisoc, the mihomo project or Tailscale.
