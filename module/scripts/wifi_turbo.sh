#!/system/bin/sh
# Xiaomi 14 SoftAP turbo controller and lifecycle guard.

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
MODDIR=$(cd "$SCRIPT_DIR/.." && pwd)
STATE_DIR="$MODDIR/state"
STATUS_FILE="$MODDIR/turbo_status"
DAEMON_PID_FILE="$STATE_DIR/daemon.pid"
DAEMON_LOCK="$STATE_DIR/daemon.lock"
WEB_PID_FILE="$STATE_DIR/web.pid"
LAST_APPLY_FILE="$STATE_DIR/last_apply"
AUTO_HOTSPOT_FILE="$STATE_DIR/auto_hotspot"
AIRPLANE_PENDING_FILE="$STATE_DIR/airplane_restore_pending"
ROUTER_BSSID_FILE="$STATE_DIR/router_bssid"
SOFTAP_BACKUP="$STATE_DIR/softap_config.backup"
SETTINGS_BACKUP="$STATE_DIR/settings.backup"
HELPER_JAR="$MODDIR/bin/wifi-turbo-helper.jar"
HOSTAPD_CLI="/vendor/bin/hostapd_cli"
HOSTAPD_DIR="/data/vendor/wifi/hostapd"
CFG_TARGET="/vendor/etc/wifi/kiwi_v2/WCNSS_qcom_cfg.ini"
CFG_OPT="$MODDIR/system/vendor/etc/wifi/kiwi_v2/WCNSS_qcom_cfg.ini"
BUSYBOX="/data/adb/magisk/busybox"
WEB_IP="192.168.88.1"
WEB_PORT="80"
TURBO_COUNTRY_CODE="US"
LOG_FILE="$STATE_DIR/daemon.log"
RUNTIME_CACHE="$STATE_DIR/runtime.status"
USB_FUNCTION_FILE="$STATE_DIR/usb_functions"
AUTO_USB_FILE="$STATE_DIR/auto_usb"

[ -x "$BUSYBOX" ] || BUSYBOX="busybox"
mkdir -p "$STATE_DIR"

log_msg() {
    printf '%s %s\n' "$(date '+%F %T')" "$*" >> "$LOG_FILE"
}

get_turbo_status() {
    [ -f "$STATUS_FILE" ] && cat "$STATUS_FILE" || echo 1
}

get_auto_hotspot() {
    [ -f "$AUTO_HOTSPOT_FILE" ] && cat "$AUTO_HOTSPOT_FILE" || echo 0
}

get_auto_usb() {
    [ -f "$AUTO_USB_FILE" ] && cat "$AUTO_USB_FILE" || echo 0
}

set_auto_usb() {
    if [ "$1" = "1" ]; then
        echo 1 > "$AUTO_USB_FILE"
    else
        echo 0 > "$AUTO_USB_FILE"
    fi
}

get_airplane_mode() {
    airplane=$(settings get global airplane_mode_on 2>/dev/null)
    [ "$airplane" = "1" ] && echo 1 || echo 0
}

usb_link_connected() {
    dumpsys usb 2>/dev/null | grep -Eiq '(^|[[:space:]])connected=true([[:space:]]|$)' && return 0
    state=$(cat /sys/class/android_usb/android0/state 2>/dev/null)
    [ "$state" = "CONNECTED" ] || [ "$state" = "CONFIGURED" ]
}

set_auto_hotspot() {
    if [ "$1" = "1" ]; then
        echo 1 > "$AUTO_HOTSPOT_FILE"
        [ "$(get_airplane_mode)" = "1" ] && echo 1 > "$AIRPLANE_PENDING_FILE"
    else
        echo 0 > "$AUTO_HOTSPOT_FILE"
        rm -f "$AIRPLANE_PENDING_FILE"
    fi
}

cache_value() {
    sed -n "s/^$1=//p" "$RUNTIME_CACHE" 2>/dev/null | head -n 1
}

refresh_runtime_cache() {
    runtime_tmp="$RUNTIME_CACHE.tmp"
    dumpsys wifi 2>/dev/null | awk '
        /^mCurrentSoftApConfiguration:/ && !config_seen {
            config_seen=1; in_config=1
            value=$0; sub(/^.*ssid = "/, "", value); sub(/" *$/, "", value)
            print "SSID=" value
            next
        }
        in_config && /^ bssid = / {
            value=$0; sub(/^ bssid = */, "", value); sub(/[ \t]+$/, "", value)
            print "CONFIG_BSSID=" value; next
        }
        in_config && /^ MaxClient = / {
            value=$0; sub(/^ MaxClient = */, "", value); sub(/[ \t]+$/, "", value)
            print "MAX_CLIENTS=" value; next
        }
        in_config && /^ Ieee80211axEnabled = / {
            value=$0; sub(/^ Ieee80211axEnabled = */, "", value); sub(/[ \t]+$/, "", value)
            print "AX=" value; next
        }
        in_config && /^ Ieee80211beEnabled = / {
            value=$0; sub(/^ Ieee80211beEnabled = */, "", value); sub(/[ \t]+$/, "", value)
            print "BE=" value; next
        }
        in_config && /^ vendorElements = / {
            print "VENDOR_EMPTY=" ($0 ~ /\[\]/ ? "true" : "false"); next
        }
        in_config && /^mCurrentSoftApCapability:/ { in_config=0; next }
        /^getConnectedClientList\(\).size\(\):/ && !clients_seen {
            clients_seen=1; value=$0; sub(/^.*: */, "", value); print "CLIENTS=" value; next
        }
        /^mCurrentSoftApInfoMap / && !info_seen {
            info_seen=1; line=$0
            if (line ~ /^mCurrentSoftApInfoMap \{\}$/) { print "AP_IFACE="; next }
            value=line; sub(/^mCurrentSoftApInfoMap \{/, "", value); sub(/=.*/, "", value)
            print "AP_IFACE=" value
            value=line; sub(/^.*bssid=/, "", value); sub(/[,}].*$/, "", value)
            print "BSSID=" value
            value=line; sub(/^.*frequency= */, "", value); sub(/[,}].*$/, "", value)
            print "FREQUENCY=" value
            value=line; sub(/^.*wifiStandard= */, "", value); sub(/[,}].*$/, "", value)
            print "WIFI_STANDARD=" value
        }
    ' > "$runtime_tmp"
    offload=$(dumpsys tethering 2>/dev/null | awk \
        '/Offload HALs started/ { found=1 } END { print found ? "true" : "false" }')
    echo "OFFLOAD=$offload" >> "$runtime_tmp"
    country=$(cmd wifi get-country-code 2>/dev/null | sed -n 's/^.*= *//p' | head -n 1)
    echo "COUNTRY_CODE=$country" >> "$runtime_tmp"
    mv -f "$runtime_tmp" "$RUNTIME_CACHE"
    chmod 644 "$RUNTIME_CACHE" 2>/dev/null
}

get_ap_iface() {
    iface=$(cache_value AP_IFACE)
    if [ -n "$iface" ] || [ -f "$RUNTIME_CACHE" ]; then
        printf '%s\n' "$iface"
    else
        dumpsys wifi 2>/dev/null \
            | sed -n 's/^mCurrentSoftApInfoMap {\([^= }]*\)=.*/\1/p' \
            | head -n 1
    fi
}

get_runtime_config() {
    printf 'mCurrentSoftApConfiguration: ssid = "%s"\n' "$(cache_value SSID)"
    printf ' bssid = %s\n' "$(cache_value CONFIG_BSSID)"
    printf ' MaxClient = %s\n' "$(cache_value MAX_CLIENTS)"
    printf ' Ieee80211axEnabled = %s\n' "$(cache_value AX)"
    printf ' Ieee80211beEnabled = %s\n' "$(cache_value BE)"
    [ "$(cache_value VENDOR_EMPTY)" = "true" ] \
        && echo ' vendorElements = []' || echo ' vendorElements = [present]'
}

get_softap_info() {
    iface=$(cache_value AP_IFACE)
    [ -n "$iface" ] || return 0
    printf 'mCurrentSoftApInfoMap {%s=SoftApInfo{frequency=%s,bssid=%s,wifiStandard=%s}}\n' \
        "$iface" "$(cache_value FREQUENCY)" "$(cache_value BSSID)" \
        "$(cache_value WIFI_STANDARD)"
}

is_mounted() {
    awk -v target="$CFG_TARGET" '$5 == target { found=1 } END { exit !found }' \
        /proc/self/mountinfo 2>/dev/null
}

mount_driver_config() {
    [ -f "$CFG_OPT" ] || return 0
    is_mounted || mount --bind "$CFG_OPT" "$CFG_TARGET" 2>/dev/null
}

unmount_driver_config() {
    while is_mounted; do
        umount "$CFG_TARGET" 2>/dev/null || return 1
    done
    return 0
}

backup_settings_once() {
    [ -f "$SETTINGS_BACKUP" ] && return 0
    {
        printf 'hotspot_80211ax_support_temp=%s\n' \
            "$(settings get system hotspot_80211ax_support_temp 2>/dev/null)"
        printf 'hotspot_vendor_specific=%s\n' \
            "$(settings get system hotspot_vendor_specific 2>/dev/null)"
        printf 'tether_offload_disabled=%s\n' \
            "$(settings get global tether_offload_disabled 2>/dev/null)"
        printf 'wifi_scan_throttle_enabled=%s\n' \
            "$(settings get global wifi_scan_throttle_enabled 2>/dev/null)"
        printf 'wifi_power_save=%s\n' \
            "$(settings get global wifi_power_save 2>/dev/null)"
        printf 'tether_force_usb_functions=%s\n' \
            "$(settings get global tether_force_usb_functions 2>/dev/null)"
    } > "$SETTINGS_BACKUP"
}

restore_setting() {
    namespace="$1"
    key="$2"
    value=$(sed -n "s/^${key}=//p" "$SETTINGS_BACKUP" | head -n 1)
    if [ -z "$value" ] || [ "$value" = "null" ]; then
        settings delete "$namespace" "$key" >/dev/null 2>&1
    else
        settings put "$namespace" "$key" "$value" >/dev/null 2>&1
    fi
}

restore_settings() {
    if [ -f "$SETTINGS_BACKUP" ]; then
        restore_setting system hotspot_80211ax_support_temp
        restore_setting system hotspot_vendor_specific
        restore_setting global tether_offload_disabled
        restore_setting global wifi_scan_throttle_enabled
        restore_setting global wifi_power_save
        restore_setting global tether_force_usb_functions
    fi
    cmd wifi force-overlay-config-value bool \
        config_wifiSoftapIeee80211axSupported disabled false >/dev/null 2>&1
    cmd wifi force-overlay-config-value bool \
        config_wifiSoftapIeee80211beSupported disabled false >/dev/null 2>&1
    cmd wifi force-country-code disabled >/dev/null 2>&1
}

derive_router_bssid() {
    if [ -s "$ROUTER_BSSID_FILE" ]; then
        cat "$ROUTER_BSSID_FILE"
        return
    fi

    base=$(cat /sys/class/net/wlan0/address 2>/dev/null)
    ap_iface=$(get_ap_iface)
    ap_mac=""
    [ -n "$ap_iface" ] && ap_mac=$(cat "/sys/class/net/$ap_iface/address" 2>/dev/null)

    case "$base" in
        ??\:??\:??\:??\:??\:??) prefix=$(echo "$base" | cut -d: -f1-3) ;;
        *) prefix="b4:05:a1" ;;
    esac
    case "$ap_mac" in
        ??\:??\:??\:??\:??\:??) suffix=$(echo "$ap_mac" | cut -d: -f4-6) ;;
        *) suffix=$(echo "$base" | cut -d: -f4-6) ;;
    esac

    bssid="$prefix:$suffix"
    printf '%s\n' "$bssid" > "$ROUTER_BSSID_FILE"
    printf '%s\n' "$bssid"
}

run_softap_helper() {
    action="$1"
    shift
    [ -f "$HELPER_JAR" ] || return 1
    helper_timeout=30
    case "$action" in
        start-usb|stop-usb) helper_timeout=8 ;;
    esac
    # A dead early-boot TetheringManager binding can otherwise block app_process
    # forever. USB leaves enough time for its gadget fallback within 35 seconds.
    CLASSPATH="$HELPER_JAR" "$BUSYBOX" timeout -k 2 "$helper_timeout" app_process /system/bin \
        com.wifiapturbo.SoftApConfigurator "$action" "$SOFTAP_BACKUP" "$@"
}

hostapd_country_ready() {
    iface=$(get_ap_iface)
    [ -n "$iface" ] || return 1
    conf="$HOSTAPD_DIR/hostapd_${iface}.conf"
    [ -f "$conf" ] || return 1
    grep -qx "country_code=$TURBO_COUNTRY_CODE" "$conf" 2>/dev/null \
        && grep -qx 'ieee80211d=1' "$conf" 2>/dev/null \
        && grep -qx 'ieee80211h=1' "$conf" 2>/dev/null
}

apply_hostapd_country() {
    iface=$(get_ap_iface)
    [ -n "$iface" ] || return 1
    conf="$HOSTAPD_DIR/hostapd_${iface}.conf"
    ctrl="$HOSTAPD_DIR/ctrl"
    sockets="$HOSTAPD_DIR/sockets"
    [ -f "$conf" ] && [ -x "$HOSTAPD_CLI" ] || return 1

    tmp="$STATE_DIR/hostapd_${iface}.conf.tmp"
    awk -v code="$TURBO_COUNTRY_CODE" '
        /^country_code=/ || /^ieee80211d=/ || /^ieee80211h=/ { next }
        { print }
        END {
            print ""
            print "country_code=" code
            print "ieee80211d=1"
            print "ieee80211h=1"
        }
    ' "$conf" > "$tmp" || return 1

    changed=0
    if ! "$BUSYBOX" cmp -s "$tmp" "$conf"; then
        "$BUSYBOX" cat "$tmp" > "$conf" || return 1
        chown wifi:wifi "$conf" 2>/dev/null
        chmod 660 "$conf" 2>/dev/null
        restorecon "$conf" 2>/dev/null
        changed=1
    fi
    rm -f "$tmp"

    cli="$HOSTAPD_CLI -p$ctrl -s$sockets -i$iface"
    $cli set country_code "$TURBO_COUNTRY_CODE" >/dev/null 2>&1 || return 1
    $cli set ieee80211d 1 >/dev/null 2>&1 || return 1
    $cli set ieee80211h 1 >/dev/null 2>&1 || return 1
    # SET updates the live configuration; UPDATE_BEACON publishes it without
    # the long AP-wide reload that Qualcomm's hostapd can time out on.
    $cli update_beacon >/dev/null 2>&1 || return 1
    log_msg "hostapd country/beacon applied: $TURBO_COUNTRY_CODE on $iface"
}

apply_framework_tuning() {
    backup_settings_once
    settings put system hotspot_80211ax_support_temp 1 >/dev/null 2>&1
    settings put global tether_offload_disabled 0 >/dev/null 2>&1
    settings put global wifi_scan_throttle_enabled 0 >/dev/null 2>&1
    settings put global wifi_power_save 0 >/dev/null 2>&1
    # 1 selects CDC-NCM as the physical function for normal USB tethering.
    # Unlike the separate TETHERING_NCM request, this path provides upstream NAT.
    settings put global tether_force_usb_functions 1 >/dev/null 2>&1
    cmd wifi force-overlay-config-value bool \
        config_wifiSoftapIeee80211axSupported enabled true >/dev/null 2>&1
    cmd wifi force-overlay-config-value bool \
        config_wifiSoftapIeee80211beSupported disabled false >/dev/null 2>&1
    cmd wifi force-country-code enabled "$TURBO_COUNTRY_CODE" >/dev/null 2>&1

    sysctl -w net.ipv4.ip_forward=1 >/dev/null 2>&1
    sysctl -w net.core.rmem_max=16777216 >/dev/null 2>&1
    sysctl -w net.core.wmem_max=16777216 >/dev/null 2>&1
    sysctl -w net.ipv4.tcp_rmem='4096 87380 16777216' >/dev/null 2>&1
    sysctl -w net.ipv4.tcp_wmem='4096 65536 16777216' >/dev/null 2>&1
}

runtime_router_ready() {
    expected=$(derive_router_bssid)
    [ -n "$(cache_value AP_IFACE)" ] || return 1
    [ "$(cache_value BSSID)" = "$expected" ] || return 1
    [ "$(cache_value MAX_CLIENTS)" = "32" ] || return 1
    [ "$(cache_value AX)" = "true" ] || return 1
    [ "$(cache_value VENDOR_EMPTY)" = "true" ] || return 1
    [ "$(cache_value COUNTRY_CODE)" = "$TURBO_COUNTRY_CODE" ] || return 1
    return 0
}

runtime_sample_complete() {
    [ -n "$(cache_value AP_IFACE)" ] || return 1
    [ -n "$(cache_value SSID)" ] || return 1
    [ -n "$(cache_value CONFIG_BSSID)" ] || return 1
    [ -n "$(cache_value MAX_CLIENTS)" ] || return 1
    [ -n "$(cache_value AX)" ] || return 1
    [ -n "$(cache_value VENDOR_EMPTY)" ] || return 1
    [ -n "$(cache_value BSSID)" ] || return 1
    [ -n "$(cache_value FREQUENCY)" ] || return 1
    [ -n "$(cache_value WIFI_STANDARD)" ] || return 1
}

allow_web_traffic() {
    iptables -w -C INPUT -d "$WEB_IP" -p tcp --dport "$WEB_PORT" -j ACCEPT \
        >/dev/null 2>&1 \
        || iptables -w -I INPUT 1 -d "$WEB_IP" -p tcp --dport "$WEB_PORT" -j ACCEPT \
        >/dev/null 2>&1
}

ensure_web_policy_rule() {
    # Android's per-network policy rules otherwise send locally generated
    # replies from 192.168.88.1 through the cellular default network.
    ip -4 rule show 2>/dev/null \
        | grep -Fq "from $WEB_IP lookup main" \
        || ip -4 rule add pref 9000 from "$WEB_IP/32" lookup main 2>/dev/null
}

remove_web_policy_rule() {
    while ip -4 rule show 2>/dev/null \
            | grep -Fq "from $WEB_IP lookup main"; do
        ip -4 rule del pref 9000 from "$WEB_IP/32" lookup main 2>/dev/null \
            || break
    done
}

remove_web_rule() {
    while iptables -w -C INPUT -d "$WEB_IP" -p tcp --dport "$WEB_PORT" -j ACCEPT \
        >/dev/null 2>&1; do
        iptables -w -D INPUT -d "$WEB_IP" -p tcp --dport "$WEB_PORT" -j ACCEPT \
            >/dev/null 2>&1 || break
    done
}

start_webui() {
	# Port 80 belongs to the embedded Glovebox server. Keep this function as a
	# compatibility no-op so the AP lifecycle guard never starts a second WebUI.
	return 0
}

stop_webui() {
    if [ -s "$WEB_PID_FILE" ]; then
        kill "$(cat "$WEB_PID_FILE")" 2>/dev/null
        rm -f "$WEB_PID_FILE"
    fi
    pkill -f "httpd -p $WEB_IP:$WEB_PORT" 2>/dev/null
    pkill -f 'httpd -p 127.0.0.1:8989' 2>/dev/null
}

setup_web_endpoint() {
    iface=$(get_ap_iface)
    [ -n "$iface" ] || return 1
    ip -4 address show dev "$iface" 2>/dev/null | grep -Fq "$WEB_IP/32" \
        || ip address add "$WEB_IP/32" dev "$iface" 2>/dev/null
    ensure_web_policy_rule
    allow_web_traffic
    start_webui
}

clear_web_endpoint() {
    for netdev in /sys/class/net/*; do
        [ -e "$netdev" ] || continue
        iface=${netdev##*/}
        ip address del "$WEB_IP/32" dev "$iface" 2>/dev/null
    done
    remove_web_policy_rule
    remove_web_rule
    stop_webui
}

start_daemon() {
	if ! mkdir "$DAEMON_LOCK" 2>/dev/null; then
		if [ -s "$DAEMON_PID_FILE" ] && kill -0 "$(cat "$DAEMON_PID_FILE")" 2>/dev/null; then
			return 0
		fi
		rmdir "$DAEMON_LOCK" 2>/dev/null || return 0
		mkdir "$DAEMON_LOCK" 2>/dev/null || return 0
	fi
    if [ -s "$DAEMON_PID_FILE" ]; then
        daemon_pid=$(cat "$DAEMON_PID_FILE")
        daemon_cmd=""
        [ -r "/proc/$daemon_pid/cmdline" ] \
            && daemon_cmd=$(tr '\000' ' ' < "/proc/$daemon_pid/cmdline")
        if kill -0 "$daemon_pid" 2>/dev/null \
                && echo "$daemon_cmd" | grep -Fq "$0 daemon"; then
            return 0
        fi
        rm -f "$DAEMON_PID_FILE"
    fi
    sh "$0" daemon >> "$LOG_FILE" 2>&1 &
    echo $! > "$DAEMON_PID_FILE"
	rmdir "$DAEMON_LOCK" 2>/dev/null || true
}

stop_daemon() {
    if [ -s "$DAEMON_PID_FILE" ]; then
        daemon_pid=$(cat "$DAEMON_PID_FILE")
        [ "$daemon_pid" != "$$" ] && kill "$daemon_pid" 2>/dev/null
        rm -f "$DAEMON_PID_FILE"
    fi
}

daemon_loop() {
    if [ -s "$DAEMON_PID_FILE" ]; then
        old_daemon_pid=$(cat "$DAEMON_PID_FILE")
        if [ "$old_daemon_pid" != "$$" ]; then
            old_daemon_cmd=""
            [ -r "/proc/$old_daemon_pid/cmdline" ] \
                && old_daemon_cmd=$(tr '\000' ' ' \
                    < "/proc/$old_daemon_pid/cmdline")
            if kill -0 "$old_daemon_pid" 2>/dev/null \
                    && echo "$old_daemon_cmd" | grep -Fq "$0 daemon"; then
                exit 0
            fi
        fi
    fi
    echo $$ > "$DAEMON_PID_FILE"
    log_msg "lifecycle guard started"
    trap '[ "$(cat "$DAEMON_PID_FILE" 2>/dev/null)" = "$$" ] && rm -f "$DAEMON_PID_FILE"; exit 0' TERM INT EXIT
    mismatch_count=0
    last_airplane=$(get_airplane_mode)
    recovery_retry_at=0
    usb_retry_at=0
    if [ "$last_airplane" = "1" ] && [ "$(get_auto_hotspot)" = "1" ]; then
        echo 1 > "$AIRPLANE_PENDING_FILE"
    fi
    while true; do
        refresh_runtime_cache
        iface=$(get_ap_iface)
        airplane=$(get_airplane_mode)
        if [ "$airplane" = "1" ]; then
            if [ "$last_airplane" != "1" ]; then
                log_msg "airplane mode enabled; waiting for Android to stop the hotspot"
            fi
            [ "$(get_auto_hotspot)" = "1" ] && echo 1 > "$AIRPLANE_PENDING_FILE"
        elif [ "$last_airplane" = "1" ]; then
            if [ "$(get_auto_hotspot)" = "1" ]; then
                echo 1 > "$AIRPLANE_PENDING_FILE"
                recovery_retry_at=0
                log_msg "airplane mode disabled; hotspot recovery scheduled"
            else
                rm -f "$AIRPLANE_PENDING_FILE"
            fi
        fi
        last_airplane="$airplane"
        if [ -n "$iface" ]; then
            setup_web_endpoint
            router_needs_apply=0
            country_needs_apply=0
            if runtime_sample_complete; then
                runtime_router_ready || router_needs_apply=1
            fi
            hostapd_country_ready || country_needs_apply=1
            if [ "$router_needs_apply" = "1" ]; then
                mismatch_count=$((mismatch_count + 1))
            else
                mismatch_count=0
            fi
            if [ "$(get_turbo_status)" = "1" ] \
                    && { [ "$mismatch_count" -ge 3 ] \
                        || [ "$country_needs_apply" = "1" ]; }; then
                now=$(date +%s)
                last=0
                [ -s "$LAST_APPLY_FILE" ] && last=$(cat "$LAST_APPLY_FILE")
                if [ $((now - last)) -ge 20 ]; then
                    echo "$now" > "$LAST_APPLY_FILE"
                    bssid=$(derive_router_bssid)
                    log_msg "AP reset detected on $iface; reapplying router/country profile $bssid"
                    apply_framework_tuning
                    if [ "$router_needs_apply" = "1" ]; then
                        run_softap_helper enable "$bssid" >> "$LOG_FILE" 2>&1
                        mismatch_count=0
                    fi
                    refresh_runtime_cache
                    apply_hostapd_country >> "$LOG_FILE" 2>&1
                    refresh_runtime_cache
                    setup_web_endpoint
                fi
            fi
        else
            stop_webui
        fi
        if [ "$airplane" = "0" ] && [ -f "$AIRPLANE_PENDING_FILE" ]; then
            if [ "$(get_auto_hotspot)" != "1" ]; then
                rm -f "$AIRPLANE_PENDING_FILE"
            elif [ -n "$iface" ]; then
                rm -f "$AIRPLANE_PENDING_FILE"
                log_msg "hotspot recovered after airplane mode on $iface"
            else
                now=$(date +%s)
                if [ $((now - recovery_retry_at)) -ge 8 ]; then
                    recovery_retry_at=$now
                    log_msg "requesting Android hotspot recovery after airplane mode"
                    run_softap_helper start-wifi "" >> "$LOG_FILE" 2>&1 || true
                fi
            fi
        fi
        # USB can be unplugged and recreated independently of the hotspot.
        # Restore only its management alias/route; never restart Wi-Fi here.
        maintain_usb_web_endpoint >/dev/null 2>&1 || true
        # HyperOS may leave the USB gadget at adb after a cable reconnect or
        # after NetworkStack rejected the first tether request.  Retry only
        # the USB path when the policy is enabled and Android reports a link.
        if [ "$(get_auto_usb)" = "1" ] && [ -z "$(get_usb_iface)" ] \
            && usb_link_connected; then
            now=$(date +%s)
            if [ $((now - usb_retry_at)) -ge 20 ]; then
                usb_retry_at=$now
                log_msg "USB link detected without tether interface; recovering USB tether"
                capture_usb_function
                run_softap_helper start-usb "" >> "$LOG_FILE" 2>&1 || true
                if ! setup_usb_web_endpoint 6; then
                    force_usb_gadget
                    setup_usb_web_endpoint 20 || true
                fi
            fi
        fi
        sleep 4
    done
}

enable_turbo() {
    echo 1 > "$STATUS_FILE"
    mount_driver_config
    apply_framework_tuning
    bssid=$(derive_router_bssid)
    echo "$(date +%s)" > "$LAST_APPLY_FILE"
    run_softap_helper enable "$bssid"
    refresh_runtime_cache
    apply_hostapd_country
    refresh_runtime_cache
    setup_web_endpoint
    start_daemon
	echo "Turbo enabled: router BSSID=$bssid, Glovebox=http://$WEB_IP/"
}

disable_turbo() {
    echo 0 > "$STATUS_FILE"
    run_softap_helper restore "" || true
    refresh_runtime_cache
    restore_settings
    unmount_driver_config
    setup_web_endpoint
    start_daemon
	echo "Stock SoftAP configuration restored; Glovebox remains at http://$WEB_IP/"
}

configure_hotspot() {
    ssid="$1"
    password="$2"
    hidden="$3"
    bssid=$(derive_router_bssid)
    apply_framework_tuning
    run_softap_helper configure "$bssid" "$ssid" "$password" "$hidden"
    refresh_runtime_cache
    apply_hostapd_country || true
    refresh_runtime_cache
}

set_hotspot() {
    enabled="$1"
    ssid="$2"
    password="$3"
    hidden="$4"
    auto_hotspot="${5:-$enabled}"
    set_auto_hotspot "$auto_hotspot"
    configure_hotspot "$ssid" "$password" "$hidden"
    if [ "$enabled" = "1" ]; then
        run_softap_helper start-wifi ""
        sleep 1
        refresh_runtime_cache
        setup_web_endpoint || true
    else
        run_softap_helper stop-wifi ""
        refresh_runtime_cache
    fi
}

set_usb_tether() {
    set_auto_usb "$1"
    set_usb_runtime "$1"
}

set_usb_runtime() {
    if [ "$1" = "1" ]; then
        capture_usb_function
        # NetworkStack normally selects the function through the Xiaomi overlay.
        # Some HyperOS builds accept the tether request but leave the gadget at
        # adb; give the native path a short window, then use the vendor-supported
        # single-function switch as a root fallback.
        run_softap_helper start-usb "" || true
        if ! setup_usb_web_endpoint 6; then
            force_usb_gadget
            setup_usb_web_endpoint 20 || true
        fi
    else
        run_softap_helper stop-usb "" || true
        restore_usb_function
    fi
}

capture_usb_function() {
    [ -s "$USB_FUNCTION_FILE" ] && return 0
    current=$(getprop sys.usb.config 2>/dev/null)
    [ -n "$current" ] && printf '%s\n' "$current" > "$USB_FUNCTION_FILE"
}

usb_gadget_function() {
    # Xiaomi's UsbService rejects the usual ncm,adb/rndis,adb combinations,
    # while accepting the single network function. The overlay's value 1 means
    # NCM; 0 is the Android RNDIS fallback.
    forced=$(settings get global tether_force_usb_functions 2>/dev/null)
    [ "$forced" = "0" ] && echo rndis || echo ncm
}

force_usb_gadget() {
    function=$(usb_gadget_function)
    log_msg "USB tether request accepted without an interface; switching gadget to $function"
    /system/bin/svc usb setFunctions "$function" >/dev/null 2>&1 || \
        svc usb setFunctions "$function" >/dev/null 2>&1 || true
}

restore_usb_function() {
    [ -s "$USB_FUNCTION_FILE" ] || return 0
    saved=$(cat "$USB_FUNCTION_FILE" 2>/dev/null)
    rm -f "$USB_FUNCTION_FILE"
    [ -n "$saved" ] || saved=adb
    log_msg "restoring USB gadget functions to $saved"
    /system/bin/svc usb setFunctions "$saved" >/dev/null 2>&1 || \
        svc usb setFunctions "$saved" >/dev/null 2>&1 || true
}

get_usb_iface() {
    dumpsys tethering 2>/dev/null | awk '
        $2 == "-" && $3 == "TetheredState" && $1 ~ /^(usb|rndis|ncm)/ {
            print $1; exit
        }'
}

setup_usb_web_endpoint() {
    attempts=0
    usb_iface=""
    max_attempts="${1:-20}"
    while [ "$attempts" -lt "$max_attempts" ]; do
        usb_iface=$(get_usb_iface)
        [ -n "$usb_iface" ] && break
        attempts=$((attempts + 1))
        sleep 1
    done
    [ -n "$usb_iface" ] || return 1

    maintain_usb_web_endpoint || return 1
    log_msg "USB management route active on $usb_iface"
}

maintain_usb_web_endpoint() {
    usb_iface=$(get_usb_iface)
    [ -n "$usb_iface" ] || return 1

    # Keep Android's native USB subnet, DHCP, gateway and NAT untouched. The
    # /32 alias is an extra stable management endpoint for fixed-address hosts.
    ip -4 address show dev "$usb_iface" 2>/dev/null | grep -Fq "$WEB_IP/32" \
        || ip address add "$WEB_IP/32" dev "$usb_iface" 2>/dev/null \
        || return 1
    ip route replace 192.168.88.0/24 dev "$usb_iface" src "$WEB_IP" \
        2>/dev/null || return 1
    ensure_web_policy_rule
    allow_web_traffic
}

boot_router() {
    ssid="$1"
    password="$2"
    hidden="$3"
    auto_hotspot="$4"
    auto_usb="$5"
    set_auto_hotspot "$auto_hotspot"
    set_auto_usb "$auto_usb"
    configure_hotspot "$ssid" "$password" "$hidden"
    [ "$auto_hotspot" = "1" ] && run_softap_helper start-wifi "" || true
    if [ "$auto_usb" = "1" ]; then
        # Propagate USB failures to Glovebox so its bounded retry loop can run.
        # Android remains the only DHCP/NAT owner.
        capture_usb_function
        run_softap_helper start-usb "" || true
        if ! setup_usb_web_endpoint 6; then
            force_usb_gadget
            setup_usb_web_endpoint 20 || true
        fi
    fi
    refresh_runtime_cache
    setup_web_endpoint || true
}

print_status() {
    refresh_runtime_cache
    status=$(get_turbo_status)
    iface=$(get_ap_iface)
    info=$(get_softap_info)
    config=$(get_runtime_config)
    echo "=== Xiaomi 14 AP Turbo ==="
	[ "$status" = "1" ] && echo "Target mode: Turbo" || echo "Target mode: Stock"
    if [ -n "$iface" ]; then
		echo "Hotspot: active ($iface)"
		echo "$info" | sed 's/^/Radio: /'
        echo "$config" | grep -E 'ssid =|bssid =|MaxClient =|Ieee80211(ax|be)Enabled|vendorElements ='
        if runtime_router_ready; then
		echo "Router profile: active"
        else
			echo "Router profile: pending"
        fi
        ip -4 address show dev "$iface" 2>/dev/null | grep -F "$WEB_IP/32" \
            && echo "WebUI: http://$WEB_IP/"
    else
		echo "Hotspot: inactive"
    fi
    [ "$(cache_value OFFLOAD)" = "true" ] \
		&& echo "Hardware offload: active" || echo "Hardware offload: inactive"
	echo "Country code: $(cache_value COUNTRY_CODE)"
}

cleanup_all() {
    echo 0 > "$STATUS_FILE"
    set_auto_hotspot 0
    set_auto_usb 0
    stop_daemon
    run_softap_helper stop-usb "" || true
    restore_usb_function
    run_softap_helper restore "" || true
    restore_settings
    unmount_driver_config
    clear_web_endpoint
}

case "$1" in
    on|enable) enable_turbo ;;
    off|disable) disable_turbo ;;
    status) print_status ;;
    webui) setup_web_endpoint && echo "WebUI: http://$WEB_IP/" ;;
    wifi) set_hotspot "$2" "$3" "$4" "$5" "$6" ;;
    usb) set_usb_tether "$2" ;;
    usb-runtime) set_usb_runtime "$2" ;;
    boot) boot_router "$2" "$3" "$4" "$5" "$6" ;;
    daemon) daemon_loop ;;
    cleanup) cleanup_all ;;
    *) echo "Usage: $0 {on|off|status|webui|wifi|usb|usb-runtime|boot|daemon|cleanup}"; exit 1 ;;
esac
