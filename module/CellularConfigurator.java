package com.glovebox.cellular;

import android.telephony.CellInfo;
import android.telephony.NetworkScan;
import android.telephony.NetworkScanRequest;
import android.telephony.RadioAccessSpecifier;
import android.telephony.TelephonyManager;
import android.telephony.TelephonyScanManager;
import java.lang.reflect.Array;
import java.lang.reflect.Method;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.Executor;
import java.util.concurrent.TimeUnit;

/** Root/app_process bridge for reading Android telephony state as line records. */
public final class CellularConfigurator {
    private CellularConfigurator() {}

    public static void main(String[] args) {
        try {
            exemptHiddenApis();
            initializeTelephonyFramework();
            Object context = systemContext();
            Object telephony = context.getClass().getMethod("getSystemService", String.class)
                    .invoke(context, "phone");
            if (telephony == null) throw new IllegalStateException("TelephonyManager unavailable");
            if (args.length == 0 || "cells".equals(args[0])) {
                printCells(telephony);
            } else if ("service".equals(args[0])) {
                printService(telephony);
            } else if ("scan".equals(args[0])) {
                startNetworkScan(telephony);
            } else if ("manual".equals(args[0]) && args.length == 2) {
                setManualNetwork(telephony, args[1]);
            } else if ("auto".equals(args[0])) {
                setAutomaticNetwork(telephony);
            } else if ("mode".equals(args[0])) {
                printSelectionMode(telephony);
            } else {
                throw new IllegalArgumentException(
                        "usage: CellularConfigurator cells|service|scan|manual PLMN|auto|mode");
            }
            System.exit(0);
        } catch (Throwable t) {
            System.err.println("ERROR=" + rootCause(t));
            System.exit(1);
        }
    }

    private static Object systemContext() throws Exception {
        Class<?> looper = Class.forName("android.os.Looper");
        if (looper.getMethod("myLooper").invoke(null) == null)
            looper.getMethod("prepareMainLooper").invoke(null);
        Class<?> activityThread = Class.forName("android.app.ActivityThread");
        Object thread = activityThread.getDeclaredMethod("systemMain").invoke(null);
        return activityThread.getDeclaredMethod("getSystemContext").invoke(thread);
    }

    // A standalone app_process is not forked from zygote, so the framework's
    // telephony service registerers have not been installed yet.
    private static void initializeTelephonyFramework() {
        try {
            Class<?> initializer = Class.forName("android.telephony.TelephonyFrameworkInitializer");
            Method getter = initializer.getDeclaredMethod("getTelephonyServiceManager");
            getter.setAccessible(true);
            if (getter.invoke(null) != null) return;
            Class<?> managerClass = Class.forName("android.os.TelephonyServiceManager");
            java.lang.reflect.Constructor<?> ctor = managerClass.getDeclaredConstructor();
            ctor.setAccessible(true);
            Object manager = ctor.newInstance();
            Method setter = initializer.getDeclaredMethod(
                    "setTelephonyServiceManager", managerClass);
            setter.setAccessible(true);
            setter.invoke(null, manager);
        } catch (Throwable ignored) {}
    }

    private static void printCells(Object tm) throws Exception {
        final TelephonyManager manager = (TelephonyManager) tm;
        printSelectionMode(tm);
        final CountDownLatch latch = new CountDownLatch(1);
        final List<?>[] result = new List<?>[]{null};
        final Throwable[] error = new Throwable[]{null};
        final Executor direct = Runnable::run;
        manager.requestCellInfoUpdate(direct, new TelephonyManager.CellInfoCallback() {
            @Override public void onCellInfo(List<CellInfo> info) {
                result[0] = info;
                latch.countDown();
            }
            @Override public void onError(int code, Throwable detail) {
                error[0] = detail != null ? detail
                        : new IllegalStateException("cell info error " + code);
                latch.countDown();
            }
        });
        latch.await(8, TimeUnit.SECONDS);
        List<?> cells = result[0];
        if (cells == null) {
            Object value = tm.getClass().getMethod("getAllCellInfo").invoke(tm);
            cells = value instanceof List ? (List<?>) value : Collections.emptyList();
        }
        if (error[0] != null) System.err.println("WARNING=" + rootCause(error[0]));
        printCellRecords(cells);
    }

    private static void printCellRecords(List<?> cells) {
        System.out.println("COUNT=" + cells.size());
        for (Object cell : cells) {
            Object id = invoke(cell, "getCellIdentity");
            Object signal = invoke(cell, "getCellSignalStrength");
            String type = cell.getClass().getSimpleName().replace("CellInfo", "").toUpperCase();
            String channel = first(id, "getNrarfcn", "getEarfcn", "getUarfcn", "getArfcn");
            String cellID = first(id, "getNci", "getCi", "getCid", "getLac");
            String bands = array(firstObject(id, "getBands"));
            System.out.println("CELL"
                    + "\ttype=" + clean(type)
                    + "\tregistered=" + clean(first(cell, "isRegistered"))
                    + "\tconnection=" + clean(first(cell, "getCellConnectionStatus"))
                    + "\tmcc=" + clean(first(id, "getMccString", "getMcc"))
                    + "\tmnc=" + clean(first(id, "getMncString", "getMnc"))
                    + "\toperator=" + clean(first(id, "getOperatorAlphaLong", "getOperatorAlphaShort"))
                    + "\tcell_id=" + clean(cellID)
                    + "\ttac=" + clean(first(id, "getTac", "getLac"))
                    + "\tpci=" + clean(first(id, "getPci", "getPsc", "getBsic"))
                    + "\tarfcn=" + clean(channel)
                    + "\tbands=" + clean(bands)
                    + "\tdbm=" + clean(first(signal, "getDbm"))
                    + "\trsrp=" + clean(first(signal, "getSsRsrp", "getRsrp"))
                    + "\trsrq=" + clean(first(signal, "getSsRsrq", "getRsrq"))
                    + "\tsinr=" + clean(first(signal, "getSsSinr", "getRssnr")));
        }
    }

    /**
     * Run Android's privileged one-shot network scan for LTE and NR. Root can
     * use the system telephony service, while Qualcomm's private scan service
     * is deliberately not exported to standalone processes.
     */
    private static void startNetworkScan(Object tm) throws Exception {
        final TelephonyManager manager = (TelephonyManager) tm;
        final CountDownLatch latch = new CountDownLatch(1);
        final List<CellInfo>[] result = new List[]{null};
        final int[] error = new int[]{0};
        RadioAccessSpecifier[] radios = new RadioAccessSpecifier[]{
                new RadioAccessSpecifier(3, null, null), // EUTRAN
                new RadioAccessSpecifier(6, null, null), // NGRAN
        };
        NetworkScanRequest request = new NetworkScanRequest(
                0, radios, 5, 60, true, 5, new ArrayList<String>());
        NetworkScan scan = manager.requestNetworkScan(request, Runnable::run,
                new TelephonyScanManager.NetworkScanCallback() {
                    @Override public void onResults(List<CellInfo> cells) {
                        result[0] = cells;
                        latch.countDown();
                    }
                    @Override public void onComplete() { latch.countDown(); }
                    @Override public void onError(int code) { error[0] = code; latch.countDown(); }
                });
        latch.await(40, TimeUnit.SECONDS);
        if (scan != null) scan.stopScan();
        if (error[0] != 0) throw new IllegalStateException("network scan error " + error[0]);
        if (result[0] == null) throw new IllegalStateException("network scan timed out without results");
        System.out.println("MODE=scan");
        printCellRecords(result[0]);
    }

    /** Manual selection is PLMN selection, not a PCI/ARFCN cell lock. */
    private static void setManualNetwork(Object tm, String plmn) throws Exception {
        if (!plmn.matches("[0-9]{5,6}")) throw new IllegalArgumentException("invalid PLMN");
        Object result = tm.getClass()
                .getMethod("setNetworkSelectionModeManual", String.class, boolean.class)
                .invoke(tm, plmn, true);
        if (result instanceof Boolean && !Boolean.TRUE.equals(result))
            throw new IllegalStateException("manual network selection rejected");
        System.out.println("OK=manual\tPLMN=" + clean(plmn));
    }

    private static void setAutomaticNetwork(Object tm) throws Exception {
        tm.getClass().getMethod("setNetworkSelectionModeAutomatic").invoke(tm);
        System.out.println("OK=automatic");
    }

    private static void printSelectionMode(Object tm) throws Exception {
        Object mode = tm.getClass().getMethod("getNetworkSelectionMode").invoke(tm);
        System.out.println("MODE=" + (String.valueOf(mode).equals("2") ? "manual" : "automatic"));
    }

    private static void printService(Object tm) throws Exception {
        Object service = tm.getClass().getMethod("getServiceState").invoke(tm);
        System.out.println("SERVICE=" + clean(String.valueOf(service)));
    }

    private static Object invoke(Object target, String method) {
        if (target == null) return null;
        try { return target.getClass().getMethod(method).invoke(target); }
        catch (Throwable ignored) { return null; }
    }

    private static Object firstObject(Object target, String... methods) {
        for (String method : methods) {
            Object value = invoke(target, method);
            if (value != null) return value;
        }
        return null;
    }

    private static String first(Object target, String... methods) {
        Object value = firstObject(target, methods);
        return value == null ? "" : String.valueOf(value);
    }

    private static String array(Object value) {
        if (value == null || !value.getClass().isArray()) return value == null ? "" : String.valueOf(value);
        StringBuilder out = new StringBuilder();
        for (int i = 0; i < Array.getLength(value); i++) {
            if (i > 0) out.append(',');
            out.append(Array.get(value, i));
        }
        return out.toString();
    }

    private static String clean(String value) {
        if (value == null) return "";
        return value.replace('\t', ' ').replace('\n', ' ').replace('\r', ' ');
    }

    private static void exemptHiddenApis() {
        try {
            Class<?> vmRuntime = Class.forName("dalvik.system.VMRuntime");
            Object runtime = vmRuntime.getDeclaredMethod("getRuntime").invoke(null);
            vmRuntime.getDeclaredMethod("setHiddenApiExemptions", String[].class)
                    .invoke(runtime, (Object) new String[]{"L"});
        } catch (Throwable ignored) {}
    }

    private static Throwable rootCause(Throwable t) {
        while (t.getCause() != null) t = t.getCause();
        return t;
    }
}
