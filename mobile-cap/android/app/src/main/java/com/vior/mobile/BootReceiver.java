package com.vior.mobile;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.util.Log;

/**
 * Auto-launch Vior when the device finishes booting. Gated by a
 * SharedPreferences flag the WebView toggles in Settings → Startup →
 * "Auto-launch on boot" (key: vior_boot_autostart=1). User can flip
 * it off at any time. No flag → no launch.
 */
public class BootReceiver extends BroadcastReceiver {
    private static final String TAG = "ViorBoot";

    @Override
    public void onReceive(Context ctx, Intent intent) {
        if (intent == null) return;
        String action = intent.getAction();
        if (action == null) return;
        if (!action.equals(Intent.ACTION_BOOT_COMPLETED)
            && !action.equals("android.intent.action.QUICKBOOT_POWERON")
            && !action.equals("com.htc.intent.action.QUICKBOOT_POWERON")) {
            return;
        }

        // Capacitor stores localStorage under WebView's preferences;
        // since we don't have direct access from here we keep a tiny
        // mirror SharedPreferences file the WebView writes to via the
        // JS bridge below (see core.ts startup sync).
        SharedPreferences sp = ctx.getSharedPreferences("vior_prefs", Context.MODE_PRIVATE);
        if (!sp.getBoolean("boot_autostart", false)) {
            Log.i(TAG, "boot_autostart disabled — skipping");
            return;
        }

        try {
            Intent launch = ctx.getPackageManager()
                .getLaunchIntentForPackage(ctx.getPackageName());
            if (launch != null) {
                launch.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
                ctx.startActivity(launch);
                Log.i(TAG, "Vior launched on boot");
            }
        } catch (Exception e) {
            Log.e(TAG, "boot launch failed: " + e.getMessage());
        }
    }
}
