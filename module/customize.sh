# Runs when Magisk installs the module
SKIPUNZIP=0
set_perm_recursive "$MODPATH" 0 0 0755 0644
set_perm "$MODPATH/bin/glovebox" 0 0 0755
set_perm "$MODPATH/service.sh" 0 0 0755
[ -f "$MODPATH/post-fs-data.sh" ] && set_perm "$MODPATH/post-fs-data.sh" 0 0 0755
[ -f "$MODPATH/scripts/wifi_turbo.sh" ] && set_perm "$MODPATH/scripts/wifi_turbo.sh" 0 0 0755
[ -f "$MODPATH/sepolicy.rule" ] && set_perm "$MODPATH/sepolicy.rule" 0 0 0644
[ -f "$MODPATH/bin/wifi-turbo-helper.jar" ] && set_perm "$MODPATH/bin/wifi-turbo-helper.jar" 0 0 0644
[ -d "$MODPATH/system" ] && set_perm_recursive "$MODPATH/system" 0 0 0755 0644
mkdir -p /data/adb/glovebox
chmod 700 /data/adb/glovebox
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
ui_print "- Set a console password before first use:"
ui_print "    echo <password> | /data/adb/modules/glovebox/bin/glovebox passwd"
