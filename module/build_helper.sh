#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
sdk=${ANDROID_SDK_ROOT:-}
d8=${D8:-${sdk:+$sdk/build-tools/36.0.0/d8}}
r8_jar=${R8_JAR:-}
android_jar=${ANDROID_JAR:-${sdk:+$sdk/platforms/android-36/android.jar}}
javac_bin=${JAVAC:-$(command -v javac 2>/dev/null || true)}
jdk_home=$(cd "$(dirname "$javac_bin")/.." && pwd)
build=module/.helper-build

[[ -n $javac_bin && -x $javac_bin && -n $android_jar && -f $android_jar ]] \
  && { [[ -n $d8 && -x $d8 ]] || [[ -n $r8_jar && -f $r8_jar ]]; } || {
  echo "Android SDK 36 not found; keeping the bundled helper jar" >&2
  exit 0
}

rm -rf "$build"
mkdir -p "$build/classes" "$build/dex"
javac_args=(-source 8 -target 8 -d "$build/classes")
[[ -n $android_jar && -f $android_jar ]] && javac_args+=(-cp "$android_jar")
"$javac_bin" "${javac_args[@]}" module/SoftApConfigurator.java module/CellularConfigurator.java
if [[ -n $d8 && -x $d8 ]]; then
  JAVA_HOME="$jdk_home" PATH="$jdk_home/bin:$PATH" "$d8" --min-api 31 \
    --output "$build/dex" \
    "$build"/classes/com/wifiapturbo/SoftApConfigurator*.class \
    "$build"/classes/com/glovebox/cellular/CellularConfigurator*.class
else
  JAVA_HOME="$jdk_home" PATH="$jdk_home/bin:$PATH" java -cp "$r8_jar" \
    com.android.tools.r8.D8 --min-api 31 --output "$build/dex" \
    "$build"/classes/com/wifiapturbo/SoftApConfigurator*.class \
    "$build"/classes/com/glovebox/cellular/CellularConfigurator*.class
fi
(
  cd "$build/dex"
  zip -q -9 ../../bin/wifi-turbo-helper.jar classes.dex
)
echo "Built module/bin/wifi-turbo-helper.jar"
