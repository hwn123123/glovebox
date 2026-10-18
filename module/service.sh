#!/system/bin/sh
# Runs at Magisk late_start; can also be run by hand (exits if already running)
MODDIR=${0%/*}
DATA=/data/adb/glovebox
PIDFILE=$DATA/supervisor.pid

# The previous wifi_ap_turbo module owned the same fixed address and port. Keep its
# tuning state, stop only its controller/httpd, and disable it before Glovebox binds :80.
OLD=/data/adb/modules/wifi_ap_turbo
if [ -d "$OLD" ]; then
  [ -f "$OLD/state/daemon.pid" ] && kill "$(cat "$OLD/state/daemon.pid")" 2>/dev/null || true
  [ -f "$OLD/state/web.pid" ] && kill "$(cat "$OLD/state/web.pid")" 2>/dev/null || true
  rm -f "$OLD/state/daemon.pid" "$OLD/state/web.pid"
  touch "$OLD/disable"
  pkill -f 'httpd -p 192.168.88.1:80' 2>/dev/null || true
fi

mkdir -p "$DATA"
chmod 700 "$DATA"

# a stale pid may be reused after reboot, so also check the command line
pid=$(cat "$PIDFILE" 2>/dev/null)
if [ -n "$pid" ] && grep -q glovebox/service.sh "/proc/$pid/cmdline" 2>/dev/null; then
  exit 0
fi

until [ "$(getprop sys.boot_completed)" = 1 ]; do sleep 2; done

# HyperOS publishes sys.boot_completed before the modular NetworkStack and its
# RoutingCoordinator binding are ready. Calling TetheringManager in that gap can
# crash com.android.networkstack.process and leave USB tethering unavailable for
# the rest of the boot. Give framework/mainline services a deterministic grace
# period before any WifiManager/TetheringManager helper is launched.
while :; do
  read -r uptime_s _ < /proc/uptime
  uptime_s=${uptime_s%%.*}
  [ "${uptime_s:-0}" -ge 60 ] && break
  sleep 2
done
until service check wifi 2>/dev/null | grep -q 'found'; do sleep 2; done
until service check connectivity 2>/dev/null | grep -q 'found'; do sleep 2; done

# Apply the integrated AP controller before the console starts. It is idempotent and
# preserves the user's Turbo/Stock choice in this module's state directory.
mkdir -p "$MODDIR/state"
rm -f "$MODDIR/state/daemon.pid" "$MODDIR/state/web.pid"
[ -f "$MODDIR/turbo_status" ] || echo 1 > "$MODDIR/turbo_status"
if [ "$(cat "$MODDIR/turbo_status" 2>/dev/null)" = "1" ]; then
  sh "$MODDIR/scripts/wifi_turbo.sh" on >> "$MODDIR/state/service.log" 2>&1
else
  sh "$MODDIR/scripts/wifi_turbo.sh" off >> "$MODDIR/state/service.log" 2>&1
fi

# NetProxy is a Glovebox runtime component, not a second Magisk module. Its
# boot entry starts the subscription worker and, when configured, sing-box.
sh "$MODDIR/scripts/netproxy.sh" boot >> "$MODDIR/state/service.log" 2>&1 || true

(
  while true; do
    [ -f "$MODDIR/disable" ] && break
    out=$DATA/stdout.log
    [ -f "$out" ] && [ "$(stat -c %s "$out" 2>/dev/null || echo 0)" -gt 262144 ] && mv -f "$out" "$out.1"
    "$MODDIR/bin/glovebox" -config "$DATA/config.json" >> "$out" 2>&1
    echo "$(date '+%F %T') glovebox exited ($?), restarting in 5 s" >> "$out"
    sleep 5
  done
  rm -f "$PIDFILE"
) &
echo $! > "$PIDFILE"
