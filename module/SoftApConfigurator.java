package com.wifiapturbo;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.lang.reflect.Constructor;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.lang.reflect.Proxy;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.Executor;
import java.util.concurrent.TimeUnit;

/**
 * Root/app_process helper for updating Android's in-memory SoftApConfiguration.
 *
 * The shell script used to edit WifiConfigStoreSoftAp.xml directly. WifiService
 * keeps its own in-memory copy and overwrites that XML when the hotspot changes,
 * so direct file edits are not reliable. This helper calls WifiManager instead.
 */
public final class SoftApConfigurator {
    private static final int TETHERING_WIFI = 0;
    private static final int TETHERING_USB = 1;
    private static final int TETHERING_NCM = 4;
    private static Object sContext;

    private static final class TetherResult {
        private final CountDownLatch latch = new CountDownLatch(1);
        private int error = -1;
    }

    private SoftApConfigurator() {}

    public static void main(String[] args) {
        try {
            exemptHiddenApis();
            if (args.length < 1) usage();

            final String action = args[0];
            final String backupPath = args.length > 1 ? args[1] : "";
            final String bssid = args.length > 2 ? args[2] : "";
            final Object wifiManager = getWifiManager();

            if ("enable".equals(action)) {
                enable(wifiManager, backupPath, bssid);
            } else if ("configure".equals(action)) {
                if (args.length < 6) usage();
                configure(wifiManager, backupPath, bssid, args[3], args[4],
                        Boolean.parseBoolean(args[5]));
            } else if ("restore".equals(action)) {
                restore(wifiManager, backupPath);
            } else if ("restart".equals(action)) {
                restartWifiTetheringIfActive();
            } else if ("start-wifi".equals(action)) {
                startTethering(TETHERING_WIFI);
            } else if ("stop-wifi".equals(action)) {
                stopTethering(TETHERING_WIFI);
            } else if ("start-usb".equals(action)) {
                startUsbTethering();
            } else if ("stop-usb".equals(action)) {
                stopUsbTethering();
            } else if ("status".equals(action)) {
                printStatus(getConfig(wifiManager));
            } else {
                usage();
            }
        } catch (Throwable t) {
            System.err.println("ERROR=" + rootCause(t));
            System.exit(1);
        }
    }

    private static void usage() {
        System.err.println("usage: SoftApConfigurator "
                + "enable|restore|restart|status|start-wifi|stop-wifi|start-usb|stop-usb "
                + "BACKUP [BSSID] | configure BACKUP BSSID SSID PASSWORD HIDDEN");
        System.exit(2);
    }

    private static Object getWifiManager() throws Exception {
        final Class<?> looperClass = Class.forName("android.os.Looper");
        if (looperClass.getMethod("myLooper").invoke(null) == null) {
            looperClass.getMethod("prepareMainLooper").invoke(null);
        }
        final Class<?> activityThreadClass = Class.forName("android.app.ActivityThread");
        final Object activityThread = activityThreadClass.getDeclaredMethod("systemMain").invoke(null);
        final Object context = activityThreadClass.getDeclaredMethod("getSystemContext")
                .invoke(activityThread);
        sContext = context;
        final Class<?> contextClass = Class.forName("android.content.Context");
        final Object wifi = contextClass.getMethod("getSystemService", String.class)
                .invoke(context, "wifi");
        if (wifi == null) throw new IllegalStateException("WifiManager unavailable");
        return wifi;
    }

    private static Object getConfig(Object wifiManager) throws Exception {
        return wifiManager.getClass().getMethod("getSoftApConfiguration").invoke(wifiManager);
    }

    private static void enable(Object wifiManager, String backupPath, String bssid)
            throws Exception {
        final Object original = getConfig(wifiManager);
        if (original == null) throw new IllegalStateException("SoftApConfiguration unavailable");
        if (!backupPath.isEmpty() && !new File(backupPath).exists()) {
            writeParcelable(original, backupPath);
        }

        final Class<?> configClass = Class.forName("android.net.wifi.SoftApConfiguration");
        final Class<?> builderClass = Class.forName(
                "android.net.wifi.SoftApConfiguration$Builder");
        final Constructor<?> copyCtor = builderClass.getConstructor(configClass);
        final Object builder = copyCtor.newInstance(original);

        applyRouterTuning(builder, bssid);

        applyConfiguration(wifiManager, original, builder, true);
    }

    private static void configure(Object wifiManager, String backupPath, String bssid,
            String ssid, String passphrase, boolean hidden) throws Exception {
        final Object original = getConfig(wifiManager);
        if (original == null) throw new IllegalStateException("SoftApConfiguration unavailable");
        if (!backupPath.isEmpty() && !new File(backupPath).exists()) {
            writeParcelable(original, backupPath);
        }

        final Class<?> configClass = Class.forName("android.net.wifi.SoftApConfiguration");
        final Class<?> builderClass = Class.forName(
                "android.net.wifi.SoftApConfiguration$Builder");
        final Constructor<?> copyCtor = builderClass.getConstructor(configClass);
        final Object builder = copyCtor.newInstance(original);

        invoke(builder, "setSsid", new Class<?>[]{String.class}, ssid);
        invoke(builder, "setPassphrase", new Class<?>[]{String.class, int.class},
                passphrase, 1 /* SECURITY_TYPE_WPA2_PSK */);
        invoke(builder, "setHiddenSsid", new Class<?>[]{boolean.class}, hidden);
        applyRouterTuning(builder, bssid);

        applyConfiguration(wifiManager, original, builder, true);
    }

    private static void applyRouterTuning(Object builder, String bssid) throws Exception {
        invoke(builder, "setIeee80211axEnabled", new Class<?>[]{boolean.class}, true);
        // Xiaomi 14 Pro's current SoftAP capability is Wi-Fi 6. HyperOS accepts
        // a BE=true write but normalizes it back to false, which previously made
        // every idempotency check look changed and restarted the AP forever.
        invoke(builder, "setMaxNumberOfClients", new Class<?>[]{int.class}, 32);
        invoke(builder, "setAutoShutdownEnabled", new Class<?>[]{boolean.class}, false);

        // Remove the handset vendor IE and use a stable, globally administered
        // device-unique BSSID so clients do not key on Android hotspot fingerprints.
        tryInvoke(builder, "setVendorElements", new Class<?>[]{List.class},
                Collections.emptyList());
        if (!bssid.isEmpty()) {
            final Class<?> macClass = Class.forName("android.net.MacAddress");
            final Object mac = macClass.getMethod("fromString", String.class)
                    .invoke(null, bssid);
            invoke(builder, "setMacRandomizationSetting", new Class<?>[]{int.class}, 0);
            invoke(builder, "setBssid", new Class<?>[]{macClass}, mac);
        }
    }

    private static void applyConfiguration(Object wifiManager, Object original,
            Object builder, boolean restartActive) throws Exception {
        final Class<?> configClass = Class.forName("android.net.wifi.SoftApConfiguration");
        final Class<?> builderClass = Class.forName(
                "android.net.wifi.SoftApConfiguration$Builder");
        final Object updated = builderClass.getMethod("build").invoke(builder);
        if (sameManagedFields(original, updated)) {
            System.out.println("RESULT=UNCHANGED");
            printStatus(updated);
            return;
        }

        final Object result = wifiManager.getClass()
                .getMethod("setSoftApConfiguration", configClass)
                .invoke(wifiManager, updated);
        if (result instanceof Boolean && !((Boolean) result)) {
            throw new IllegalStateException("WifiManager rejected SoftApConfiguration");
        }
        System.out.println("RESULT=UPDATED");
        if (restartActive) restartWifiTetheringIfActive();
        printStatus(getConfig(wifiManager));
    }

    private static void restore(Object wifiManager, String backupPath) throws Exception {
        if (backupPath.isEmpty() || !new File(backupPath).isFile()) {
            System.out.println("RESULT=NO_BACKUP");
            return;
        }
        final Class<?> configClass = Class.forName("android.net.wifi.SoftApConfiguration");
        final Object restored = readParcelable(configClass, backupPath);
        final Object result = wifiManager.getClass()
                .getMethod("setSoftApConfiguration", configClass)
                .invoke(wifiManager, restored);
        if (result instanceof Boolean && !((Boolean) result)) {
            throw new IllegalStateException("WifiManager rejected restored configuration");
        }
        System.out.println("RESULT=RESTORED");
        restartWifiTetheringIfActive();
        printStatus(getConfig(wifiManager));
    }

    private static void restartWifiTetheringIfActive() throws Exception {
        if (!isTethered(TETHERING_WIFI)) {
            System.out.println("TETHER_RESTART=NOT_ACTIVE");
            return;
        }
        stopTethering(TETHERING_WIFI);
        startTethering(TETHERING_WIFI);
        System.out.println("TETHER_RESTART=OK");
    }

    private static Object getTetheringManager() throws Exception {
        final Class<?> contextClass = Class.forName("android.content.Context");
        final Class<?> tetherClass = Class.forName("android.net.TetheringManager");
        final Object tether = contextClass.getMethod("getSystemService", Class.class)
                .invoke(sContext, tetherClass);
        if (tether == null) throw new IllegalStateException("TetheringManager unavailable");
        return tether;
    }

    private static void stopTethering(int type) throws Exception {
        final Object tether = getTetheringManager();
        final Class<?> tetherClass = Class.forName("android.net.TetheringManager");
        tetherClass.getMethod("stopTethering", int.class).invoke(tether, type);
        for (int i = 0; i < 30 && isTethered(type); i++) {
            Thread.sleep(200);
        }
        System.out.println("TETHER_STOP=" + (isTethered(type) ? "PENDING" : "OK"));
    }

    private static void startTethering(int type) throws Exception {
        if (isTethered(type)) {
            System.out.println("TETHER_START=ALREADY_ACTIVE");
            return;
        }
        final Object tether = getTetheringManager();
        final Class<?> tetherClass = Class.forName("android.net.TetheringManager");

        final Class<?> requestClass = Class.forName(
                "android.net.TetheringManager$TetheringRequest");
        final Class<?> requestBuilderClass = Class.forName(
                "android.net.TetheringManager$TetheringRequest$Builder");
        final Object requestBuilder = requestBuilderClass.getConstructor(int.class)
                .newInstance(type);
        tryInvoke(requestBuilder, "setExemptFromEntitlementCheck",
                new Class<?>[]{boolean.class}, true);
        final Object request = requestBuilderClass.getMethod("build").invoke(requestBuilder);

        final Class<?> callbackClass = Class.forName(
                "android.net.TetheringManager$StartTetheringCallback");
        final TetherResult result = new TetherResult();
        final Object callback = Proxy.newProxyInstance(
                callbackClass.getClassLoader(), new Class<?>[]{callbackClass},
                (proxy, method, args) -> {
                    if ("onTetheringStarted".equals(method.getName())) {
                        result.error = 0;
                        result.latch.countDown();
                    } else if ("onTetheringFailed".equals(method.getName())) {
                        result.error = args != null && args.length > 0
                                ? ((Integer) args[0]) : -2;
                        result.latch.countDown();
                    }
                    return null;
                });
        final Executor directExecutor = Runnable::run;
        tetherClass.getMethod("startTethering", requestClass, Executor.class, callbackClass)
                .invoke(tether, request, directExecutor, callback);
        if (!result.latch.await(15, TimeUnit.SECONDS)) {
            throw new IllegalStateException("Timed out starting tethering type " + type);
        }
        if (result.error != 0) {
            throw new IllegalStateException("tethering start failed for type "
                    + type + ": " + result.error);
        }
        System.out.println("TETHER_START=OK");
        Thread.sleep(type == 0 ? 1500 : 500);
    }

    private static void startUsbTethering() throws Exception {
        if (isTethered(TETHERING_USB)) {
            System.out.println("TETHER_START=ALREADY_ACTIVE");
            System.out.println("USB_TETHER=ACTIVE");
            return;
        }
        // Clear stale requests even when their gadget interface has already
        // disappeared. Otherwise NetworkStack returns DUPLICATE_REQUEST.
        stopTethering(TETHERING_NCM);
        stopTethering(TETHERING_USB);
        Thread.sleep(500);
        // NetworkStack's TETHERING_NCM request is intentionally local-only.
        // The module RRO selects NCM as the physical function for the normal
        // TETHERING_USB request, which keeps Android DHCP, upstream and NAT.
        startTethering(TETHERING_USB);
        System.out.println("USB_TETHER=NCM_INTERNET");
    }

    private static void stopUsbTethering() throws Exception {
        stopTethering(TETHERING_NCM);
        stopTethering(TETHERING_USB);
        System.out.println("USB_TETHER=OFF");
    }

    private static boolean isTethered(int type) throws Exception {
        final Object tether = getTetheringManager();
        final Class<?> tetherClass = Class.forName("android.net.TetheringManager");
        final String[] ifaces = (String[]) tetherClass.getMethod("getTetheredIfaces")
                .invoke(tether);
        if (ifaces == null) return false;
        for (String iface : ifaces) {
            if (iface == null) continue;
            if (type == TETHERING_WIFI && (iface.startsWith("wlan")
                    || iface.startsWith("softap") || iface.startsWith("ap_br_"))) {
                return true;
            }
            if (type == TETHERING_USB && (iface.startsWith("rndis")
                    || iface.startsWith("ncm") || iface.startsWith("usb"))) {
                return true;
            }
            if (type == TETHERING_NCM && (iface.startsWith("ncm") || iface.startsWith("usb"))) {
                return true;
            }
        }
        return false;
    }

    private static boolean sameManagedFields(Object a, Object b) throws Exception {
        return string(a, "getSsid").equals(string(b, "getSsid"))
                && string(a, "getPassphrase").equals(string(b, "getPassphrase"))
                && bool(a, "isHiddenSsid") == bool(b, "isHiddenSsid")
                && bool(a, "isIeee80211axEnabled") == bool(b, "isIeee80211axEnabled")
                && integer(a, "getMaxNumberOfClients") == integer(b, "getMaxNumberOfClients")
                && string(a, "getBssid").equals(string(b, "getBssid"))
                && listSize(a, "getVendorElements") == listSize(b, "getVendorElements");
    }

    private static void printStatus(Object config) throws Exception {
        System.out.println("SSID=" + string(config, "getSsid"));
        System.out.println("HIDDEN=" + bool(config, "isHiddenSsid"));
        System.out.println("AX=" + bool(config, "isIeee80211axEnabled"));
        System.out.println("BE=" + bool(config, "isIeee80211beEnabled"));
        System.out.println("MAX_CLIENTS=" + integer(config, "getMaxNumberOfClients"));
        System.out.println("BSSID=" + string(config, "getBssid"));
        System.out.println("VENDOR_IE_COUNT=" + listSize(config, "getVendorElements"));
    }

    private static boolean bool(Object target, String method) throws Exception {
        return (Boolean) target.getClass().getMethod(method).invoke(target);
    }

    private static int integer(Object target, String method) throws Exception {
        return (Integer) target.getClass().getMethod(method).invoke(target);
    }

    private static int listSize(Object target, String method) throws Exception {
        final Object value = target.getClass().getMethod(method).invoke(target);
        return value instanceof List ? ((List<?>) value).size() : -1;
    }

    private static String string(Object target, String method) throws Exception {
        final Object value = target.getClass().getMethod(method).invoke(target);
        return String.valueOf(value);
    }

    private static Object invoke(Object target, String method, Class<?>[] types,
            Object... args) throws Exception {
        return target.getClass().getMethod(method, types).invoke(target, args);
    }

    private static Object tryInvoke(Object target, String method, Class<?>[] types,
            Object... args) {
        try {
            return invoke(target, method, types, args);
        } catch (Throwable ignored) {
            return null;
        }
    }

    private static void writeParcelable(Object value, String path) throws Exception {
        final Class<?> parcelClass = Class.forName("android.os.Parcel");
        final Object parcel = parcelClass.getMethod("obtain").invoke(null);
        try {
            value.getClass().getMethod("writeToParcel", parcelClass, int.class)
                    .invoke(value, parcel, 0);
            final byte[] data = (byte[]) parcelClass.getMethod("marshall").invoke(parcel);
            final File file = new File(path);
            final File parent = file.getParentFile();
            if (parent != null) parent.mkdirs();
            try (FileOutputStream out = new FileOutputStream(file)) {
                out.write(data);
            }
        } finally {
            parcelClass.getMethod("recycle").invoke(parcel);
        }
    }

    private static Object readParcelable(Class<?> configClass, String path) throws Exception {
        final byte[] data;
        try (FileInputStream in = new FileInputStream(path)) {
            data = new byte[(int) new File(path).length()];
            int offset = 0;
            while (offset < data.length) {
                final int read = in.read(data, offset, data.length - offset);
                if (read < 0) break;
                offset += read;
            }
        }
        final Class<?> parcelClass = Class.forName("android.os.Parcel");
        final Object parcel = parcelClass.getMethod("obtain").invoke(null);
        try {
            parcelClass.getMethod("unmarshall", byte[].class, int.class, int.class)
                    .invoke(parcel, data, 0, data.length);
            parcelClass.getMethod("setDataPosition", int.class).invoke(parcel, 0);
            final Field creatorField = configClass.getField("CREATOR");
            final Object creator = creatorField.get(null);
            return creator.getClass().getMethod("createFromParcel", parcelClass)
                    .invoke(creator, parcel);
        } finally {
            parcelClass.getMethod("recycle").invoke(parcel);
        }
    }

    private static void exemptHiddenApis() {
        try {
            final Class<?> vmRuntime = Class.forName("dalvik.system.VMRuntime");
            final Method getRuntime = vmRuntime.getDeclaredMethod("getRuntime");
            getRuntime.setAccessible(true);
            final Object runtime = getRuntime.invoke(null);
            final Method exempt = vmRuntime.getDeclaredMethod(
                    "setHiddenApiExemptions", String[].class);
            exempt.setAccessible(true);
            exempt.invoke(runtime, (Object) new String[]{"L"});
        } catch (Throwable ignored) {
            // app_process launched as root/platform is normally already exempt.
        }
    }

    private static String rootCause(Throwable t) {
        Throwable cursor = t;
        while (cursor.getCause() != null) cursor = cursor.getCause();
        return cursor.getClass().getName() + ": " + cursor.getMessage();
    }
}
