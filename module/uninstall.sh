# Stop the integrated AP controller and remove the fixed address/firewall rule. Config and logs
# stay in /data/adb/glovebox. The old module is re-enabled only if it was present, so rollback is
# possible after a reboot.
if [ -x /data/adb/modules/glovebox/scripts/wifi_turbo.sh ]; then
  sh /data/adb/modules/glovebox/scripts/wifi_turbo.sh cleanup >/dev/null 2>&1 || true
fi
if [ -x /data/adb/modules/glovebox/scripts/netproxy.sh ]; then
  sh /data/adb/modules/glovebox/scripts/netproxy.sh shutdown >/dev/null 2>&1 || true
fi
rm -f /data/adb/modules/wifi_ap_turbo/disable
rm -f /data/adb/glovebox/supervisor.pid
