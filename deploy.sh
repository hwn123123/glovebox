#!/usr/bin/env bash
# Install dist/stage into /data/adb/modules/glovebox on a device connected over adb and
# (re)start the console without rebooting. Requires root (su) on the device.
#
#   GLOVEBOX_PASSWORD_FILE=path   set the console password from this file on first install
#   ADB_SERIAL=serial             pick a device when several are connected
set -euo pipefail
cd "$(dirname "$0")"

stage=dist/stage
[[ -x $stage/bin/glovebox ]] || { echo "run ./build.sh first" >&2; exit 1; }
adb=(adb)
[[ -n ${ADB_SERIAL:-} ]] && adb+=(-s "$ADB_SERIAL")
pwfile=${GLOVEBOX_PASSWORD_FILE:-}
tmp=/data/local/tmp/glovebox-stage

"${adb[@]}" shell "rm -rf $tmp" >/dev/null
"${adb[@]}" push "$stage" "$tmp" >/dev/null
if [[ -n $pwfile ]]; then
  "${adb[@]}" push "$pwfile" "$tmp/initial-password" >/dev/null
fi

"${adb[@]}" shell "su -c 'sh -s'" <<SH
set -e
MOD=/data/adb/modules/glovebox
DATA=/data/adb/glovebox
mkdir -p \$MOD \$DATA
chmod 700 \$DATA
cp -f $tmp/module.prop $tmp/service.sh $tmp/uninstall.sh $tmp/customize.sh \$MOD/
# Copy the integrated Xiaomi AP controller and its runtime resources. The config/state
# directory is deliberately kept outside the module so upgrades cannot erase it.
mkdir -p \$MOD/bin \$MOD/scripts \$MOD/state
cp -f $tmp/bin/wifi-turbo-helper.jar \$MOD/bin/wifi-turbo-helper.jar
cp -f $tmp/scripts/wifi_turbo.sh \$MOD/scripts/wifi_turbo.sh
cp -f $tmp/scripts/netproxy.sh \$MOD/scripts/netproxy.sh
cp -f $tmp/post-fs-data.sh \$MOD/post-fs-data.sh
cp -f $tmp/sepolicy.rule \$MOD/sepolicy.rule
chmod 755 \$MOD/scripts/wifi_turbo.sh \$MOD/scripts/netproxy.sh \$MOD/post-fs-data.sh
chmod 644 \$MOD/bin/wifi-turbo-helper.jar \$MOD/sepolicy.rule
# Install the self-contained proxy runtime. Upgrades replace programs and
# bundled remote rules while preserving user configuration and catalogs.
if [ ! -f \$MOD/netproxy/config/module.json ]; then
  cp -R $tmp/netproxy \$MOD/netproxy
else
  mkdir -p \$MOD/netproxy/bin \$MOD/netproxy/config/singbox/rules/remote
  cp -f $tmp/netproxy/bin/netproxyctl $tmp/netproxy/bin/sing-box $tmp/netproxy/bin/android-platform.dex \$MOD/netproxy/bin/
  cp -f $tmp/netproxy/LICENSE $tmp/netproxy/NOTICE $tmp/netproxy/SOURCE.md \$MOD/netproxy/
  cp -R $tmp/netproxy/config/singbox/rules/remote/. \$MOD/netproxy/config/singbox/rules/remote/
fi
chmod 755 \$MOD/netproxy/bin/netproxyctl \$MOD/netproxy/bin/sing-box
chmod 644 \$MOD/netproxy/bin/android-platform.dex
mkdir -p \$MOD/netproxy/logs \$MOD/netproxy/runtime \$MOD/netproxy/data/catalog/staging
chmod 700 \$MOD/netproxy/config \$MOD/netproxy/data \$MOD/netproxy/logs \$MOD/netproxy/runtime
# files under system/ are mounted by Magisk at boot, so changes need a reboot
if [ -d $tmp/system ]; then
  if ! diff -r $tmp/system \$MOD/system >/dev/null 2>&1; then
    rm -rf \$MOD/system && cp -r $tmp/system \$MOD/system
    echo "note: system/ changed, reboot the device to apply"
  fi
fi
# Keep the critical NetworkStack RRO explicit. Some Android toybox/diff builds
# have treated the nested product overlay tree inconsistently during upgrades.
if [ -f $tmp/system/product/overlay/GloveboxTetheringOverlay/GloveboxTetheringOverlay.apk ]; then
  mkdir -p \$MOD/system/product/overlay/GloveboxTetheringOverlay
  cp -f $tmp/system/product/overlay/GloveboxTetheringOverlay/GloveboxTetheringOverlay.apk \
    \$MOD/system/product/overlay/GloveboxTetheringOverlay/GloveboxTetheringOverlay.apk
  chmod 644 \$MOD/system/product/overlay/GloveboxTetheringOverlay/GloveboxTetheringOverlay.apk
fi
if [ -f $tmp/system/product/overlay/GloveboxEthernetOverlay/GloveboxEthernetOverlay.apk ]; then
  mkdir -p \$MOD/system/product/overlay/GloveboxEthernetOverlay
  cp -f $tmp/system/product/overlay/GloveboxEthernetOverlay/GloveboxEthernetOverlay.apk \
    \$MOD/system/product/overlay/GloveboxEthernetOverlay/GloveboxEthernetOverlay.apk
  chmod 644 \$MOD/system/product/overlay/GloveboxEthernetOverlay/GloveboxEthernetOverlay.apk
fi
# copy then rename so a running binary is never overwritten in place
cp -f $tmp/bin/glovebox \$MOD/bin/glovebox.new
chmod 755 \$MOD/bin/glovebox.new \$MOD/service.sh
mv -f \$MOD/bin/glovebox.new \$MOD/bin/glovebox
chown -R 0:0 \$MOD
# Reload the lifecycle guard after replacing its shell script.  An existing
# daemon process keeps the old script in memory, so leaving it alive would
# postpone USB reconnect recovery until the next phone reboot.
if [ -f \$MOD/state/daemon.pid ]; then
  old_daemon_pid=\$(cat \$MOD/state/daemon.pid 2>/dev/null || true)
  [ -n "\$old_daemon_pid" ] && kill "\$old_daemon_pid" 2>/dev/null || true
  rm -f \$MOD/state/daemon.pid
fi
# Migrate the prior AP Turbo module in-place. Keep its backups and selected mode, but
# let the integrated controller own the fixed address and port 80.
OLD=/data/adb/modules/wifi_ap_turbo
if [ -d \$OLD ]; then
  [ -f \$OLD/turbo_status ] && cp -f \$OLD/turbo_status \$MOD/turbo_status
  for f in softap_config.backup settings.backup router_bssid; do
    [ -f \$OLD/state/\$f ] && [ -f \$MOD/state/\$f ] || [ ! -f \$OLD/state/\$f ] || cp -f \$OLD/state/\$f \$MOD/state/\$f
  done
  [ -f \$OLD/state/daemon.pid ] && kill \$(cat \$OLD/state/daemon.pid) 2>/dev/null || true
  [ -f \$OLD/state/web.pid ] && kill \$(cat \$OLD/state/web.pid) 2>/dev/null || true
  rm -f \$OLD/state/daemon.pid \$OLD/state/web.pid
  touch \$OLD/disable
  pkill -f 'httpd -p 192.168.88.1:80' 2>/dev/null || true
fi
# Apply the AP profile immediately, preserving Stock/Turbo selection across upgrades.
[ -f \$MOD/turbo_status ] || echo 1 > \$MOD/turbo_status
if [ "\$(cat \$MOD/turbo_status 2>/dev/null)" = 1 ]; then
  sh \$MOD/scripts/wifi_turbo.sh on >/dev/null 2>&1 || true
else
  sh \$MOD/scripts/wifi_turbo.sh off >/dev/null 2>&1 || true
fi
# Stop and archive the former standalone module, then start the integrated
# worker/core from Glovebox's module tree.
sh \$MOD/scripts/netproxy.sh boot >> \$MOD/state/service.log 2>&1 || true
if ! grep -q pbkdf2 \$DATA/config.json 2>/dev/null; then
  if [ -f $tmp/initial-password ]; then
    \$MOD/bin/glovebox -config \$DATA/config.json passwd < $tmp/initial-password
  else
    echo "warning: no console password set; run: echo <password> | \$MOD/bin/glovebox passwd"
  fi
fi
rm -rf $tmp
# if the supervisor is running, kill the binary and let it restart the new one
pid=\$(cat \$DATA/supervisor.pid 2>/dev/null || true)
if [ -n "\$pid" ] && grep -q glovebox/service.sh /proc/\$pid/cmdline 2>/dev/null; then
  pkill -f "\$MOD/bin/glovebox" || true
  echo "restarted glovebox (supervisor \$pid)"
else
  setsid sh \$MOD/service.sh </dev/null >/dev/null 2>&1 &
  echo "started supervisor"
fi
SH
