#!/usr/bin/env bash
# Cross-compile glovebox for arm64 Android (static) and package it as a Magisk module.
#
#   ./build.sh                  -> dist/stage/ (module tree) and dist/glovebox-<version>.zip
#   VERSION=1.2.3 ./build.sh    -> explicit version (default: date-based)
#   NO_OVERLAY=1 ./build.sh     -> skip the SoftAP client-limit overlay APK
set -euo pipefail
cd "$(dirname "$0")"

ver=${VERSION:-$(date +%Y%m%d.%H%M)}
stage=dist/stage
rm -rf "$stage"
mkdir -p "$stage/bin"

unformatted=$(gofmt -l .)
if [[ -n $unformatted ]]; then
  echo "run gofmt on: $unformatted" >&2
  exit 1
fi
if command -v node >/dev/null 2>&1; then
  node --check web/app.js
else
  echo "warning: node not found; skipping JavaScript syntax check" >&2
fi
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.version=$ver" -o "$stage/bin/glovebox" .

# Rebuild the Android system-service bridge when SDK 36 is available. The
# checked-in jar remains the reproducible fallback for environments without it.
bash module/build_helper.sh
cp -R module/. "$stage/"
# Java sources and helper build output are development inputs, not Magisk
# runtime files. Keep them in the repository but never ship them to the phone.
rm -rf "$stage/.helper-build"
rm -f "$stage/SoftApConfigurator.java" "$stage/CellularConfigurator.java" "$stage/build_helper.sh" "$stage/android.jar"
# Keep reproducibility inputs in the repository, not in the installed module.
# Runtime packages retain the upstream license/notice but do not need patches,
# source manifests, or directory placeholder files.
rm -rf "$stage/netproxy/patches"
rm -f "$stage/netproxy/SOURCE.md"
find "$stage" -name .gitkeep -delete

# Runtime resource overlay that raises the SoftAP client limit. Magisk mounts the module's
# system/vendor onto /vendor, so it lands in /vendor/overlay.
if [[ -z ${NO_OVERLAY:-} ]]; then
  if [[ ! -f overlay/GloveboxWifiOverlay.apk ]] && ! overlay/build.sh; then
    echo "warning: Android SDK/JDK unavailable; skipping optional client-limit overlay" >&2
  fi
  if [[ -f overlay/GloveboxWifiOverlay.apk ]]; then
    mkdir -p "$stage/system/vendor/overlay/GloveboxWifiOverlay"
    cp overlay/GloveboxWifiOverlay.apk "$stage/system/vendor/overlay/GloveboxWifiOverlay/"
    chmod 644 "$stage/system/vendor/overlay/GloveboxWifiOverlay/GloveboxWifiOverlay.apk"
  fi
  if [[ ! -f overlay/tethering/GloveboxTetheringOverlay.apk ]] \
      && ! overlay/tethering/build.sh; then
    echo "warning: Android SDK/JDK unavailable; skipping optional USB NCM overlay" >&2
  fi
  if [[ -f overlay/tethering/GloveboxTetheringOverlay.apk ]]; then
    mkdir -p "$stage/system/product/overlay/GloveboxTetheringOverlay"
    cp overlay/tethering/GloveboxTetheringOverlay.apk \
      "$stage/system/product/overlay/GloveboxTetheringOverlay/"
    chmod 644 \
      "$stage/system/product/overlay/GloveboxTetheringOverlay/GloveboxTetheringOverlay.apk"
  fi
  if [[ ! -f overlay/ethernet/GloveboxEthernetOverlay.apk ]] \
      && ! overlay/ethernet/build.sh; then
    echo "warning: Android SDK/JDK unavailable; skipping optional Ethernet ownership overlay" >&2
  fi
  if [[ -f overlay/ethernet/GloveboxEthernetOverlay.apk ]]; then
    mkdir -p "$stage/system/product/overlay/GloveboxEthernetOverlay"
    cp overlay/ethernet/GloveboxEthernetOverlay.apk \
      "$stage/system/product/overlay/GloveboxEthernetOverlay/"
    chmod 644 \
      "$stage/system/product/overlay/GloveboxEthernetOverlay/GloveboxEthernetOverlay.apk"
  fi
fi

sed -i.bak -e "s/^version=.*/version=$ver/" -e "s/^versionCode=.*/versionCode=$(date +%y%m%d%H)/" "$stage/module.prop"
rm -f "$stage/module.prop.bak"
chmod 755 "$stage/bin/glovebox" "$stage/service.sh"

rm -f dist/glovebox-*.zip
(cd "$stage" && zip -qr -X "../glovebox-$ver.zip" .)
ls -l "$stage/bin/glovebox" "dist/glovebox-$ver.zip"
