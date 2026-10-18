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
    private static Object sContext;

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
            } else if ("restore".equals(action)) {
                restore(wifiManager, backupPath);
            } else if ("restart".equals(action)) {
                restartWifiTetheringIfActive();
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
        System.err.println("usage: SoftApConfigurator enable|restore|restart|status BACKUP [BSSID]");
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

        invoke(builder, "setIeee80211axEnabled", new Class<?>[]{boolean.class}, true);
        invoke(builder, "setIeee80211beEnabled", new Class<?>[]{boolean.class}, true);
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
        restartWifiTetheringIfActive();
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
        final Class<?> contextClass = Class.forName("android.content.Context");
        final Class<?> tetherClass = Class.forName("android.net.TetheringManager");
        final Object tether = contextClass.getMethod("getSystemService", Class.class)
                .invoke(sContext, tetherClass);
        if (tether == null || !isWifiTethered(tether, tetherClass)) {
            System.out.println("TETHER_RESTART=NOT_ACTIVE");
            return;
        }

        tetherClass.getMethod("stopTethering", int.class).invoke(tether, 0);
        for (int i = 0; i < 20 && isWifiTethered(tether, tetherClass); i++) {
            Thread.sleep(200);
        }

        final Class<?> requestClass = Class.forName(
                "android.net.TetheringManager$TetheringRequest");
        final Class<?> requestBuilderClass = Class.forName(
                "android.net.TetheringManager$TetheringRequest$Builder");
        final Object requestBuilder = requestBuilderClass.getConstructor(int.class)
                .newInstance(0);
        tryInvoke(requestBuilder, "setExemptFromEntitlementCheck",
                new Class<?>[]{boolean.class}, true);
        final Object request = requestBuilderClass.getMethod("build").invoke(requestBuilder);

        final Class<?> callbackClass = Class.forName(
                "android.net.TetheringManager$StartTetheringCallback");
        final CountDownLatch latch = new CountDownLatch(1);
        final int[] error = new int[]{-1};
        final Object callback = Proxy.newProxyInstance(
                callbackClass.getClassLoader(), new Class<?>[]{callbackClass},
                (proxy, method, args) -> {
                    if ("onTetheringStarted".equals(method.getName())) {
                        error[0] = 0;
                        latch.countDown();
                    } else if ("onTetheringFailed".equals(method.getName())) {
                        error[0] = args != null && args.length > 0
                                ? ((Integer) args[0]) : -2;
                        latch.countDown();
                    }
                    return null;
                });
        final Executor directExecutor = Runnable::run;
        tetherClass.getMethod("startTethering", requestClass, Executor.class, callbackClass)
                .invoke(tether, request, directExecutor, callback);
        if (!latch.await(15, TimeUnit.SECONDS)) {
            throw new IllegalStateException("Timed out restarting Wi-Fi tethering");
        }
        if (error[0] != 0) {
            throw new IllegalStateException("Wi-Fi tethering restart failed: " + error[0]);
        }
        System.out.println("TETHER_RESTART=OK");
        Thread.sleep(1500);
    }

    private static boolean isWifiTethered(Object tether, Class<?> tetherClass) throws Exception {
        final String[] ifaces = (String[]) tetherClass.getMethod("getTetheredIfaces")
                .invoke(tether);
        if (ifaces == null) return false;
        for (String iface : ifaces) {
            if (iface != null && (iface.startsWith("wlan")
                    || iface.startsWith("softap") || iface.startsWith("ap_br_"))) {
                return true;
            }
        }
        return false;
    }

    private static boolean sameManagedFields(Object a, Object b) throws Exception {
        return bool(a, "isIeee80211axEnabled") == bool(b, "isIeee80211axEnabled")
                && bool(a, "isIeee80211beEnabled") == bool(b, "isIeee80211beEnabled")
                && integer(a, "getMaxNumberOfClients") == integer(b, "getMaxNumberOfClients")
                && string(a, "getBssid").equals(string(b, "getBssid"))
                && listSize(a, "getVendorElements") == listSize(b, "getVendorElements");
    }

    private static void printStatus(Object config) throws Exception {
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
