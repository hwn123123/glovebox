#!/system/bin/sh
# Mount the tested Xiaomi WLAN tuning file before the driver reads it. The controller
# keeps Stock mode recoverable by unmounting it when turbo_status=0.
MODDIR=${0%/*}
STATUS=1
[ -f "$MODDIR/turbo_status" ] && STATUS=$(cat "$MODDIR/turbo_status")
TARGET="/vendor/etc/wifi/kiwi_v2/WCNSS_qcom_cfg.ini"
SOURCE="$MODDIR/system/vendor/etc/wifi/kiwi_v2/WCNSS_qcom_cfg.ini"
if [ "$STATUS" = "1" ] && [ -f "$SOURCE" ] && [ -f "$TARGET" ]; then
    mount --bind "$SOURCE" "$TARGET" 2>/dev/null
fi
