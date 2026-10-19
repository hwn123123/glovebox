#!/system/bin/sh
# Magisk/KernelSU action menu for the persistent Android USB tether policy.

MODDIR=${0%/*}
CONTROL="$MODDIR/scripts/wifi_turbo.sh"
AUTO_FILE="$MODDIR/state/auto_usb"
BUSYBOX=/data/adb/magisk/busybox
[ -x "$BUSYBOX" ] || BUSYBOX=busybox

usb_iface() {
    dumpsys tethering 2>/dev/null | awk '
        $2 == "-" && $3 == "TetheredState" && $1 ~ /^(usb|rndis|ncm)/ {
            print $1; exit
        }'
}

print_status() {
    auto=0
    [ -s "$AUTO_FILE" ] && auto=$(cat "$AUTO_FILE")
    iface=$(usb_iface)
    [ "$auto" = "1" ] && auto_text=ON || auto_text=OFF
    [ -n "$iface" ] && runtime="ON ($iface)" || runtime=OFF
    echo "USB auto-start: $auto_text"
    echo "USB tether: $runtime"
}

run_action() {
    case "$1" in
        start)
            echo "Enabling USB tether and boot auto-start..."
            sh "$CONTROL" usb 1
            ;;
        stop)
            echo "Stopping USB tether and disabling boot auto-start..."
            sh "$CONTROL" usb 0
            ;;
        restart)
            echo "Restarting USB tether and enabling boot auto-start..."
            sh "$CONTROL" usb-restart
            ;;
        status) print_status; return ;;
        *) echo "Unknown action: $1"; return 2 ;;
    esac
    # Reload the console's in-memory configuration after the standalone action
    # command changed config.json. Its supervisor restarts it automatically.
    for pid in $(pidof glovebox 2>/dev/null); do kill "$pid" 2>/dev/null || true; done
    sleep 1
    print_status
}

if [ -n "$1" ]; then
    run_action "$1"
    exit $?
fi

choice=1
while :; do
    echo ""
    echo "Glovebox USB controls"
    print_status
    echo ""
    echo "Volume Up: next    Volume Down: select"
    case "$choice" in
        1) echo "> Enable + boot auto-start" ;;
        2) echo "> Stop + disable boot auto-start" ;;
        3) echo "> Restart tether" ;;
        4) echo "> Exit" ;;
    esac
    event=$($BUSYBOX timeout 30 /system/bin/getevent -qlc 8 2>/dev/null)
    case "$event" in
        *KEY_VOLUMEUP*DOWN*) choice=$((choice % 4 + 1)) ;;
        *KEY_VOLUMEDOWN*DOWN*)
            case "$choice" in
                1) run_action start ;;
                2) run_action stop ;;
                3) run_action restart ;;
                4) echo "Exit" ;;
            esac
            exit $?
            ;;
        *) echo "No key received; exit without changes"; exit 1 ;;
    esac
done
