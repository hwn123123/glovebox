# Runs when Magisk installs the module
SKIPUNZIP=0
set_perm_recursive "$MODPATH" 0 0 0755 0644
set_perm "$MODPATH/bin/glovebox" 0 0 0755
set_perm "$MODPATH/service.sh" 0 0 0755
[ -f "$MODPATH/post-fs-data.sh" ] && set_perm "$MODPATH/post-fs-data.sh" 0 0 0755
[ -f "$MODPATH/scripts/wifi_turbo.sh" ] && set_perm "$MODPATH/scripts/wifi_turbo.sh" 0 0 0755
[ -f "$MODPATH/scripts/netproxy.sh" ] && set_perm "$MODPATH/scripts/netproxy.sh" 0 0 0755
[ -f "$MODPATH/sepolicy.rule" ] && set_perm "$MODPATH/sepolicy.rule" 0 0 0644
[ -f "$MODPATH/bin/wifi-turbo-helper.jar" ] && set_perm "$MODPATH/bin/wifi-turbo-helper.jar" 0 0 0644
[ -d "$MODPATH/system" ] && set_perm_recursive "$MODPATH/system" 0 0 0755 0644
# During a Glovebox upgrade, keep the embedded proxy's user configuration,
# catalogs, and logs. Fresh installs retain the defaults shipped in this zip.
LIVE_PROXY=/data/adb/modules/glovebox/netproxy
if [ -d "$LIVE_PROXY/config" ] && [ "$LIVE_PROXY" != "$MODPATH/netproxy" ]; then
  rm -rf "$MODPATH/netproxy/config" "$MODPATH/netproxy/data" "$MODPATH/netproxy/logs"
  cp -R "$LIVE_PROXY/config" "$MODPATH/netproxy/config"
  cp -R "$LIVE_PROXY/data" "$MODPATH/netproxy/data"
  [ ! -d "$LIVE_PROXY/logs" ] || cp -R "$LIVE_PROXY/logs" "$MODPATH/netproxy/logs"
fi
[ -f "$MODPATH/netproxy/bin/netproxyctl" ] && set_perm "$MODPATH/netproxy/bin/netproxyctl" 0 0 0755
[ -f "$MODPATH/netproxy/bin/sing-box" ] && set_perm "$MODPATH/netproxy/bin/sing-box" 0 0 0755
[ -f "$MODPATH/netproxy/bin/android-platform.dex" ] && set_perm "$MODPATH/netproxy/bin/android-platform.dex" 0 0 0644
for entry in config data runtime logs; do
  [ ! -d "$MODPATH/netproxy/$entry" ] || set_perm_recursive "$MODPATH/netproxy/$entry" 0 0 0700 0600
done
mkdir -p /data/adb/glovebox
chmod 700 /data/adb/glovebox
# Android's TETHERING_NCM API is local-only. Select NCM as the physical
# function used by the normal USB tethering path, which supplies DHCP and NAT.
settings put global tether_force_usb_functions 1 >/dev/null 2>&1 || true
# Migrate the old Xiaomi AP module without deleting its backup. Glovebox now owns the
# same 192.168.88.1:80 endpoint; the old module remains available for rollback.
OLD=/data/adb/modules/wifi_ap_turbo
if [ -d "$OLD" ]; then
  [ -f "$OLD/turbo_status" ] && cp -f "$OLD/turbo_status" "$MODPATH/turbo_status"
  mkdir -p "$MODPATH/state"
  for f in softap_config.backup settings.backup router_bssid; do
    [ -f "$OLD/state/$f" ] && cp -n "$OLD/state/$f" "$MODPATH/state/$f"
  done
  [ -f "$OLD/disable" ] || touch "$OLD/disable"
fi
ui_print "- Config and logs: /data/adb/glovebox/"
ui_print "- Xiaomi AP controls are integrated; Glovebox owns http://192.168.88.1:80"
ui_print "- NetProxy/sing-box is embedded; no separate NetProxy module or WebUI is required"
ui_print "- Set a console password before first use:"
ui_print "    echo <password> | /data/adb/modules/glovebox/bin/glovebox passwd"
