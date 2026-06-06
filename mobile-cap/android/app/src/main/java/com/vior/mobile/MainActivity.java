package com.vior.mobile;

import android.Manifest;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.ActivityInfo;
import android.content.pm.PackageManager;
import android.os.Bundle;
import android.util.Base64;
import android.util.Log;
import android.view.View;
import android.view.ViewGroup;
import android.webkit.PermissionRequest;
import android.webkit.WebView;

import androidx.core.app.ActivityCompat;
import androidx.core.content.ContextCompat;
import androidx.core.graphics.Insets;
import androidx.core.view.ViewCompat;
import androidx.core.view.WindowInsetsCompat;

import com.getcapacitor.BridgeActivity;
import com.getcapacitor.BridgeWebChromeClient;

/**
 * Main activity — handles both normal launch and USB accessory auto-launch.
 * When USB cable is plugged and desktop runs Vior, Android auto-opens this activity.
 * Frames received over USB are passed to WebView via JavaScript bridge.
 */
public class MainActivity extends BridgeActivity {
    private static final String TAG = "ViorMain";
    // ── USB wire-level handshake constants ────────────────────────────
    // Magic + version mirror internal/usb/protocol.go on the desktop.
    // Both sides verify each other before honouring any subsequent
    // touch / video frames so a stray AOA accessory can't drive us.
    private static final byte[] HELLO_MAGIC = new byte[] { 'V', 'I', 'O', 'R' };
    private static final byte PROTOCOL_VERSION = 1;
    // Frame types — keep in lock-step with internal/usb/protocol.go.
    private static final byte FRAME_VIDEO = 0x01;
    private static final byte FRAME_TOUCH = 0x02;
    private static final byte FRAME_HELLO = 0x03;
    private static final byte FRAME_READY = 0x04;
    private static final byte FRAME_HELLO_ACK = 0x05;
    // Mobile waits up to this long for the desktop's hello-ack before
    // showing the "Vior desktop not responding?" recovery screen.
    private static final long HELLO_ACK_TIMEOUT_MS = 3000;

    private UsbAccessoryPlugin usbPlugin;
    private boolean usbConnected = false;
    private volatile boolean helloAckReceived = false;
    private Runnable helloAckTimeoutTask;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        // Apply persisted orientation BEFORE super so the activity comes up
        // already locked — avoids a flash + a wrong-sized hello if the user
        // connects fast after launch.
        applyPersistedOrientation();
        super.onCreate(savedInstanceState);

        usbPlugin = new UsbAccessoryPlugin(this, new UsbAccessoryPlugin.Listener() {
            @Override
            public void onConnected() {
                usbConnected = true;
                helloAckReceived = false;
                Log.i(TAG, "USB connected — sending hello (awaiting Vior ack)");
                runOnUiThread(() -> {
                    // Notify web client the cable handshake is up — the
                    // JS side shows "Verifying cable…" until the ack
                    // lands (or the timeout fires).
                    evaluateJs("window.onUsbConnected && window.onUsbConnected()");
                    sendHello();
                    // Schedule the not-responding timeout. The JS handler
                    // for onUsbHelloTimeout() switches the orb to a
                    // recovery state with a Try-Again button.
                    if (helloAckTimeoutTask != null) {
                        getBridge().getWebView().removeCallbacks(helloAckTimeoutTask);
                    }
                    helloAckTimeoutTask = () -> {
                        if (!helloAckReceived) {
                            Log.w(TAG, "usb: no hello-ack from desktop after "
                                + HELLO_ACK_TIMEOUT_MS + "ms — desktop probably not running Vior");
                            runOnUiThread(() ->
                                evaluateJs("window.onUsbHelloTimeout && window.onUsbHelloTimeout()"));
                        }
                    };
                    getBridge().getWebView().postDelayed(helloAckTimeoutTask, HELLO_ACK_TIMEOUT_MS);
                });
            }

            @Override
            public void onData(byte[] data, int length) {
                // Every well-formed frame is at least 1 byte (type) plus
                // at least 4 bytes of payload. Drop noise.
                if (length < 5) return;
                byte frameType = data[0];

                if (frameType == FRAME_VIDEO) {
                    int jpegLen = ((data[1] & 0xFF) << 24) | ((data[2] & 0xFF) << 16) |
                                  ((data[3] & 0xFF) << 8) | (data[4] & 0xFF);
                    if (length < 5 + jpegLen) return;
                    String b64 = Base64.encodeToString(data, 5, jpegLen, Base64.NO_WRAP);
                    runOnUiThread(() -> {
                        evaluateJs("window.onUsbFrame && window.onUsbFrame('" + b64 + "')");
                    });
                } else if (frameType == FRAME_READY) {
                    int w = ((data[1] & 0xFF) << 24) | ((data[2] & 0xFF) << 16) |
                            ((data[3] & 0xFF) << 8) | (data[4] & 0xFF);
                    int h = ((data[5] & 0xFF) << 24) | ((data[6] & 0xFF) << 16) |
                            ((data[7] & 0xFF) << 8) | (data[8] & 0xFF);
                    runOnUiThread(() -> {
                        evaluateJs("window.onUsbReady && window.onUsbReady(" + w + "," + h + ")");
                    });
                } else if (frameType == FRAME_HELLO_ACK) {
                    // Desktop confirms it's actually Vior. Verify magic +
                    // version before flipping the verified flag so a
                    // misaligned read can't fake an ack.
                    if (length < 6) {
                        Log.w(TAG, "usb: short hello-ack (len=" + length + ")");
                        return;
                    }
                    if (data[1] != HELLO_MAGIC[0] || data[2] != HELLO_MAGIC[1]
                            || data[3] != HELLO_MAGIC[2] || data[4] != HELLO_MAGIC[3]) {
                        Log.w(TAG, "usb: hello-ack magic mismatch — peer is not Vior");
                        // Don't flip verified. The timeout will fire and
                        // the JS recovery surface takes over.
                        return;
                    }
                    byte peerVer = data[5];
                    if (peerVer != PROTOCOL_VERSION) {
                        Log.w(TAG, "usb: hello-ack version mismatch (peer=" + peerVer
                            + " want=" + PROTOCOL_VERSION + ")");
                        return;
                    }
                    helloAckReceived = true;
                    Log.i(TAG, "usb: hello-ack verified (proto v" + peerVer + ")");
                    runOnUiThread(() ->
                        evaluateJs("window.onUsbHelloAck && window.onUsbHelloAck()"));
                }
            }

            @Override
            public void onDisconnected() {
                usbConnected = false;
                helloAckReceived = false;
                Log.i(TAG, "USB disconnected");
                runOnUiThread(() -> {
                    if (helloAckTimeoutTask != null) {
                        getBridge().getWebView().removeCallbacks(helloAckTimeoutTask);
                        helloAckTimeoutTask = null;
                    }
                    evaluateJs("window.onUsbDisconnected && window.onUsbDisconnected()");
                });
            }
        });

        // Check if launched by USB accessory intent.
        handleUsbIntent(getIntent());

        // Register JavaScript interface for touch forwarding over USB.
        getBridge().getWebView().addJavascriptInterface(this, "Android");

        // Scan for USB accessory after short delay (WebView needs time to load).
        getBridge().getWebView().postDelayed(() -> {
            if (!usbConnected && usbPlugin != null) {
                usbPlugin.scan();
            }
        }, 1000);

        // Fix Android 15 edge-to-edge: apply system bar insets as padding on
        // the WebView's root content view so CSS `env(safe-area-inset-bottom)`
        // resolves to the real value and the bottom tab bar stays clickable.
        // We use padding (not margin) and do NOT consume insets, so child
        // views and touch propagation are unaffected.
        View webView = getBridge().getWebView();
        ViewCompat.setOnApplyWindowInsetsListener(webView, (v, insets) -> {
            Insets sys = insets.getInsets(WindowInsetsCompat.Type.systemBars());
            v.setPadding(sys.left, sys.top, sys.right, sys.bottom);
            return insets;
        });

        // Request CAMERA permission proactively so the QR scanner works.
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.CAMERA)
                != PackageManager.PERMISSION_GRANTED) {
            ActivityCompat.requestPermissions(this,
                    new String[]{Manifest.permission.CAMERA}, 1001);
        }

        // Install a WebChromeClient that preserves Capacitor's full behaviour
        // (file uploads, dialogs, permission flow) but ensures camera/mic
        // resource requests are honoured. We extend BridgeWebChromeClient
        // rather than replacing it with a raw WebChromeClient — replacing it
        // breaks the Capacitor bridge in subtle ways (dialogs, geolocation,
        // file chooser) and skips the runtime CAMERA permission check, which
        // is why getUserMedia was failing with NotReadableError on Android 15
        // ("cannot open camera \"0\" without camera permission").
        WebView wv = getBridge().getWebView();
        wv.setWebChromeClient(new BridgeWebChromeClient(getBridge()) {
            @Override
            public void onPermissionRequest(final PermissionRequest request) {
                // If the CAMERA runtime permission is already granted we can
                // short-circuit and grant immediately, avoiding a second
                // prompt for the same permission inside the WebView.
                String[] res = request.getResources();
                boolean wantsCamera = false;
                for (String r : res) {
                    if (PermissionRequest.RESOURCE_VIDEO_CAPTURE.equals(r)) {
                        wantsCamera = true;
                        break;
                    }
                }
                if (wantsCamera && ContextCompat.checkSelfPermission(
                        MainActivity.this, Manifest.permission.CAMERA)
                        == PackageManager.PERMISSION_GRANTED) {
                    runOnUiThread(() -> request.grant(res));
                    return;
                }
                // Otherwise defer to Capacitor's implementation which routes
                // through the modern ActivityResultLauncher and prompts for
                // the underlying Android runtime permission as needed.
                super.onPermissionRequest(request);
            }
        });
    }

    private void handleUsbIntent(Intent intent) {
        if (intent != null && usbPlugin != null) {
            if (usbPlugin.checkIntent(intent)) {
                Log.i(TAG, "USB accessory attached — auto-connected");
            }
        }
    }

    private void sendHello() {
        // Send screen dimensions + magic + version over USB so the
        // desktop can verify we're Vior before consuming any payload.
        try {
            int w = getResources().getDisplayMetrics().widthPixels;
            int h = getResources().getDisplayMetrics().heightPixels;
            float dpr = getResources().getDisplayMetrics().density;

            // FrameHello: [0x03][magic 4B][ver 1B][w 4B][h 4B][dpr*100 4B] = 18 bytes
            byte[] hello = new byte[18];
            hello[0] = FRAME_HELLO;
            hello[1] = HELLO_MAGIC[0];
            hello[2] = HELLO_MAGIC[1];
            hello[3] = HELLO_MAGIC[2];
            hello[4] = HELLO_MAGIC[3];
            hello[5] = PROTOCOL_VERSION;
            putInt(hello, 6, w);
            putInt(hello, 10, h);
            putInt(hello, 14, (int)(dpr * 100));
            usbPlugin.send(hello);
        } catch (Exception e) {
            Log.e(TAG, "sendHello failed: " + e.getMessage());
        }
    }

    /**
     * Send touch event over USB.
     * Called from JavaScript via: Android.sendTouch(action, x, y)
     *
     * Touches are gated on the hello-ack — until we've verified the
     * peer is actually Vior we drop input rather than potentially
     * driving a stray AOA accessory's coordinate space.
     */
    @android.webkit.JavascriptInterface
    public void sendTouch(int action, float x, float y) {
        if (!usbConnected || usbPlugin == null) return;
        if (!helloAckReceived) return; // unverified peer — drop
        try {
            // FrameTouch: [0x02][action 1B][x 4B][y 4B]
            byte[] touch = new byte[10];
            touch[0] = FRAME_TOUCH;
            touch[1] = (byte) action;
            putInt(touch, 2, (int) x);
            putInt(touch, 6, (int) y);
            usbPlugin.send(touch);
        } catch (Exception e) {
            Log.e(TAG, "sendTouch failed: " + e.getMessage());
        }
    }

    /**
     * JS-callable: resend the hello and restart the ack timer. Used by
     * the "Try again" button on the recovery screen when the user has
     * (presumably) started Vior on the desktop after the cable came up.
     */
    @android.webkit.JavascriptInterface
    public void usbRetryHello() {
        if (!usbConnected || usbPlugin == null) return;
        runOnUiThread(() -> {
            helloAckReceived = false;
            sendHello();
            if (helloAckTimeoutTask != null) {
                getBridge().getWebView().removeCallbacks(helloAckTimeoutTask);
            }
            helloAckTimeoutTask = () -> {
                if (!helloAckReceived) {
                    runOnUiThread(() ->
                        evaluateJs("window.onUsbHelloTimeout && window.onUsbHelloTimeout()"));
                }
            };
            getBridge().getWebView().postDelayed(helloAckTimeoutTask, HELLO_ACK_TIMEOUT_MS);
        });
    }

    /**
     * Persist the boot-autostart flag to a SharedPreferences file the
     * BootReceiver can read. Called from JS via:
     *   Android.setBootAutostart(true);
     */
    /**
     * Lock the activity orientation. Persisted so it survives relaunch.
     * Called from JS via: Android.setOrientation("auto"|"landscape"|"portrait")
     */
    @android.webkit.JavascriptInterface
    public void setOrientation(String mode) {
        if (mode == null) mode = "auto";
        try {
            getSharedPreferences("vior_prefs", MODE_PRIVATE)
                .edit().putString("orient", mode).apply();
        } catch (Exception e) {
            Log.e(TAG, "persist orient failed: " + e.getMessage());
        }
        final String m = mode;
        runOnUiThread(() -> {
            int req;
            if ("landscape".equals(m)) {
                req = ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE;
            } else if ("portrait".equals(m)) {
                req = ActivityInfo.SCREEN_ORIENTATION_SENSOR_PORTRAIT;
            } else {
                req = ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED;
            }
            try { setRequestedOrientation(req); } catch (Exception e) {
                Log.e(TAG, "setRequestedOrientation failed: " + e.getMessage());
            }
        });
    }

    private void applyPersistedOrientation() {
        try {
            SharedPreferences sp = getSharedPreferences("vior_prefs", MODE_PRIVATE);
            String m = sp.getString("orient", "auto");
            int req;
            if ("landscape".equals(m)) {
                req = ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE;
            } else if ("portrait".equals(m)) {
                req = ActivityInfo.SCREEN_ORIENTATION_SENSOR_PORTRAIT;
            } else {
                req = ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED;
            }
            setRequestedOrientation(req);
        } catch (Exception e) {
            Log.e(TAG, "applyPersistedOrientation failed: " + e.getMessage());
        }
    }

    @android.webkit.JavascriptInterface
    public void setBootAutostart(boolean enabled) {
        try {
            getSharedPreferences("vior_prefs", MODE_PRIVATE)
                .edit().putBoolean("boot_autostart", enabled).apply();
            Log.i(TAG, "boot_autostart=" + enabled);
        } catch (Exception e) {
            Log.e(TAG, "setBootAutostart failed: " + e.getMessage());
        }
    }

    private void evaluateJs(String js) {
        WebView webView = getBridge().getWebView();
        if (webView != null) {
            webView.evaluateJavascript(js, null);
        }
    }

    private static void putInt(byte[] buf, int off, int val) {
        buf[off]     = (byte) ((val >> 24) & 0xFF);
        buf[off + 1] = (byte) ((val >> 16) & 0xFF);
        buf[off + 2] = (byte) ((val >> 8) & 0xFF);
        buf[off + 3] = (byte) (val & 0xFF);
    }
}
