package com.vior.mobile;

import android.Manifest;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.os.Bundle;
import android.util.Base64;
import android.util.Log;
import android.view.View;
import android.view.ViewGroup;
import android.webkit.PermissionRequest;
import android.webkit.WebChromeClient;
import android.webkit.WebView;

import androidx.core.app.ActivityCompat;
import androidx.core.content.ContextCompat;
import androidx.core.graphics.Insets;
import androidx.core.view.ViewCompat;
import androidx.core.view.WindowInsetsCompat;

import com.getcapacitor.BridgeActivity;

/**
 * Main activity — handles both normal launch and USB accessory auto-launch.
 * When USB cable is plugged and desktop runs Vior, Android auto-opens this activity.
 * Frames received over USB are passed to WebView via JavaScript bridge.
 */
public class MainActivity extends BridgeActivity {
    private static final String TAG = "ViorMain";
    private UsbAccessoryPlugin usbPlugin;
    private boolean usbConnected = false;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        usbPlugin = new UsbAccessoryPlugin(this, new UsbAccessoryPlugin.Listener() {
            @Override
            public void onConnected() {
                usbConnected = true;
                Log.i(TAG, "USB connected — starting frame stream");
                runOnUiThread(() -> {
                    // Notify web client that USB is active.
                    evaluateJs("window._viorUSBConnected && window._viorUSBConnected()");
                    // Send hello with screen dimensions.
                    sendHello();
                });
            }

            @Override
            public void onData(byte[] data, int length) {
                // Parse frame type.
                if (length < 5) return;
                byte frameType = data[0];

                if (frameType == 0x01) { // FrameVideo
                    // Extract JPEG length.
                    int jpegLen = ((data[1] & 0xFF) << 24) | ((data[2] & 0xFF) << 16) |
                                  ((data[3] & 0xFF) << 8) | (data[4] & 0xFF);
                    if (length < 5 + jpegLen) return;

                    // Base64 encode JPEG for WebView.
                    String b64 = Base64.encodeToString(data, 5, jpegLen, Base64.NO_WRAP);
                    runOnUiThread(() -> {
                        evaluateJs("window._viorUSBFrame && window._viorUSBFrame('" + b64 + "')");
                    });
                } else if (frameType == 0x04) { // FrameReady
                    int w = ((data[1] & 0xFF) << 24) | ((data[2] & 0xFF) << 16) |
                            ((data[3] & 0xFF) << 8) | (data[4] & 0xFF);
                    int h = ((data[5] & 0xFF) << 24) | ((data[6] & 0xFF) << 16) |
                            ((data[7] & 0xFF) << 8) | (data[8] & 0xFF);
                    runOnUiThread(() -> {
                        evaluateJs("window._viorUSBReady && window._viorUSBReady(" + w + "," + h + ")");
                    });
                }
            }

            @Override
            public void onDisconnected() {
                usbConnected = false;
                Log.i(TAG, "USB disconnected");
                runOnUiThread(() -> {
                    evaluateJs("window._viorUSBDisconnected && window._viorUSBDisconnected()");
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

        // Auto-grant WebView resource requests (camera/mic) — required for
        // navigator.mediaDevices.getUserMedia inside the Capacitor WebView.
        WebView wv = getBridge().getWebView();
        wv.setWebChromeClient(new WebChromeClient() {
            @Override
            public void onPermissionRequest(PermissionRequest request) {
                runOnUiThread(() -> request.grant(request.getResources()));
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
        // Send screen dimensions to desktop over USB.
        try {
            int w = getResources().getDisplayMetrics().widthPixels;
            int h = getResources().getDisplayMetrics().heightPixels;
            float dpr = getResources().getDisplayMetrics().density;

            // FrameHello: [0x03][width 4B][height 4B][dpr*100 4B]
            byte[] hello = new byte[13];
            hello[0] = 0x03;
            putInt(hello, 1, w);
            putInt(hello, 5, h);
            putInt(hello, 9, (int)(dpr * 100));
            usbPlugin.send(hello);
        } catch (Exception e) {
            Log.e(TAG, "sendHello failed: " + e.getMessage());
        }
    }

    /**
     * Send touch event over USB.
     * Called from JavaScript via: Android.sendTouch(action, x, y)
     */
    @android.webkit.JavascriptInterface
    public void sendTouch(int action, float x, float y) {
        if (!usbConnected || usbPlugin == null) return;
        try {
            // FrameTouch: [0x02][action 1B][x 4B][y 4B]
            byte[] touch = new byte[10];
            touch[0] = 0x02;
            touch[1] = (byte) action;
            putInt(touch, 2, (int) x);
            putInt(touch, 6, (int) y);
            usbPlugin.send(touch);
        } catch (Exception e) {
            Log.e(TAG, "sendTouch failed: " + e.getMessage());
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
