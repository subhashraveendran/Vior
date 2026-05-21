package com.vior.mobile;

import android.app.PendingIntent;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.hardware.usb.UsbAccessory;
import android.hardware.usb.UsbManager;
import android.os.ParcelFileDescriptor;
import android.util.Log;

import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;

/**
 * USB Accessory plugin for direct communication with Vior desktop.
 * When desktop switches phone to AOA mode, this receives the connection.
 * No ADB, no developer mode required.
 */
public class UsbAccessoryPlugin {
    private static final String TAG = "ViorUSB";
    private static final String ACTION_USB_PERMISSION = "com.vior.mobile.USB_PERMISSION";

    private UsbManager usbManager;
    private UsbAccessory accessory;
    private ParcelFileDescriptor fileDescriptor;
    private FileInputStream inputStream;
    private FileOutputStream outputStream;
    private boolean connected = false;

    public interface Listener {
        void onConnected();
        void onData(byte[] data, int length);
        void onDisconnected();
    }

    private Listener listener;
    private Context context;

    public UsbAccessoryPlugin(Context ctx, Listener listener) {
        this.context = ctx;
        this.listener = listener;
        this.usbManager = (UsbManager) ctx.getSystemService(Context.USB_SERVICE);
    }

    /**
     * Check if an accessory is already connected (app launched by USB plug).
     */
    public boolean checkIntent(Intent intent) {
        if (UsbManager.ACTION_USB_ACCESSORY_ATTACHED.equals(intent.getAction())) {
            UsbAccessory acc = intent.getParcelableExtra(UsbManager.EXTRA_ACCESSORY);
            if (acc != null) {
                openAccessory(acc);
                return true;
            }
        }
        return false;
    }

    /**
     * Scan for connected accessories.
     */
    public void scan() {
        UsbAccessory[] accessories = usbManager.getAccessoryList();
        if (accessories != null && accessories.length > 0) {
            UsbAccessory acc = accessories[0];
            if (usbManager.hasPermission(acc)) {
                openAccessory(acc);
            } else {
                PendingIntent pi = PendingIntent.getBroadcast(context, 0,
                    new Intent(ACTION_USB_PERMISSION), PendingIntent.FLAG_IMMUTABLE);
                usbManager.requestPermission(acc, pi);
            }
        }
    }

    private void openAccessory(UsbAccessory acc) {
        fileDescriptor = usbManager.openAccessory(acc);
        if (fileDescriptor == null) {
            Log.e(TAG, "Failed to open accessory");
            return;
        }
        accessory = acc;
        inputStream = new FileInputStream(fileDescriptor.getFileDescriptor());
        outputStream = new FileOutputStream(fileDescriptor.getFileDescriptor());
        connected = true;
        Log.i(TAG, "USB Accessory connected: " + acc.getManufacturer() + " " + acc.getModel());

        if (listener != null) listener.onConnected();

        // Start reading in background.
        new Thread(this::readLoop).start();
    }

    private void readLoop() {
        byte[] buffer = new byte[65536];
        while (connected) {
            try {
                int n = inputStream.read(buffer);
                if (n > 0 && listener != null) {
                    byte[] data = new byte[n];
                    System.arraycopy(buffer, 0, data, 0, n);
                    listener.onData(data, n);
                }
            } catch (IOException e) {
                Log.e(TAG, "Read error: " + e.getMessage());
                break;
            }
        }
        disconnect();
    }

    /**
     * Send data to desktop.
     */
    public void send(byte[] data) throws IOException {
        if (outputStream != null && connected) {
            outputStream.write(data);
            outputStream.flush();
        }
    }

    public boolean isConnected() {
        return connected;
    }

    public void disconnect() {
        connected = false;
        try { if (inputStream != null) inputStream.close(); } catch (IOException ignored) {}
        try { if (outputStream != null) outputStream.close(); } catch (IOException ignored) {}
        try { if (fileDescriptor != null) fileDescriptor.close(); } catch (IOException ignored) {}
        inputStream = null;
        outputStream = null;
        fileDescriptor = null;
        accessory = null;
        if (listener != null) listener.onDisconnected();
        Log.i(TAG, "USB Accessory disconnected");
    }
}
