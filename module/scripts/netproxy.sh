#!/system/bin/sh
# Own the NetProxy runtime embedded under the Glovebox module. The former
# standalone Magisk module is stopped and moved out of the active module tree;
# it is retained as a rollback copy and is never started by this script.

MODDIR=$(cd "${0%/*}/.." && pwd)
PROXY="$MODDIR/netproxy"
CONTROL="$PROXY/bin/netproxyctl"
OLD=/data/adb/modules/netproxy
BACKUP_ROOT=/data/adb/glovebox

prepare_runtime() {
    mkdir -p "$PROXY/logs" "$PROXY/runtime" "$PROXY/data/catalog/staging"
    chmod 700 "$PROXY/config" "$PROXY/data" "$PROXY/runtime" "$PROXY/logs" 2>/dev/null || true
    chmod 755 "$PROXY/bin/netproxyctl" "$PROXY/bin/sing-box"
    chmod 644 "$PROXY/bin/android-platform.dex"
}

stop_instance() {
    root="$1"
    ctl="$root/bin/netproxyctl"
    [ -x "$ctl" ] || return 0
    "$ctl" --json --timeout 15s service stop >/dev/null 2>&1 || true
    "$ctl" __internal worker stop --module-dir "$root" >/dev/null 2>&1 || true
}

migrate_legacy() {
    [ -d "$OLD" ] || return 0
    [ "$OLD" != "$PROXY" ] || return 0

    stop_instance "$OLD"
    touch "$OLD/disable" 2>/dev/null || true
    mkdir -p "$BACKUP_ROOT"
    target="$BACKUP_ROOT/netproxy-legacy-module"
    if [ -e "$target" ]; then
        target="$target-$(date +%Y%m%d-%H%M%S)"
    fi
    if mv "$OLD" "$target"; then
        printf '%s\n' "$target" > "$BACKUP_ROOT/netproxy-legacy-location"
    fi
}

boot_proxy() {
    migrate_legacy
    prepare_runtime
    "$CONTROL" __internal boot --module-dir "$PROXY"
}

case "${1:-boot}" in
    boot|start)
        boot_proxy
        ;;
    stop)
        stop_instance "$PROXY"
        ;;
    restart)
        prepare_runtime
        "$CONTROL" --json --timeout 45s service restart
        ;;
    status)
        "$CONTROL" --json --timeout 10s service status
        ;;
    migrate)
        migrate_legacy
        ;;
    *)
        echo "usage: $0 boot|start|stop|restart|status|migrate" >&2
        exit 2
        ;;
esac
