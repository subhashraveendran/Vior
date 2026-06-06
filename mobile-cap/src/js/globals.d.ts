// Ambient global declarations for the script-tag-based mobile build.
// No imports/exports → TypeScript treats every type and var here as
// available to every other .ts file in the project without an import.
// Once we move to Vite ES-modules these collapse into real exports.

type ToastTone = 'success' | 'warning' | 'error' | 'info';
type Mode = 'extend' | 'mirror';
type ConnectionState = 'offline' | 'connecting' | 'reconnecting' | 'online' | 'error';
type TabName = 'display' | 'files' | 'remote' | 'qr' | 'settings';

interface ServerInfo {
  name: string;
  host: string;
  port: number;
  platform?: string;
  resolution?: string;
  [k: string]: unknown;
}

// Discriminated WS envelope.
type WSMessage =
  | { type: 'ready'; data: { resolution: string; streamUrl?: string } }
  | { type: 'error'; data?: { code?: string; message?: string } }
  | { type: 'status'; data?: { fps?: number; uptime?: number } }
  | { type: string; data?: unknown };

// Native QR detector (Chromium only).
interface BarcodeDetectorLike {
  detect(src: CanvasImageSource): Promise<{ rawValue: string }[]>;
}

// USB bridge callbacks installed on `window` by the Android side.
// `onUsbHelloAck` fires once the desktop has verified the magic +
// version we sent in FrameHello; `onUsbHelloTimeout` fires after a 3s
// silence (peer is not actually Vior, or the desktop app is down).
interface UsbBridge {
  onUsbFrame: (b64: string) => void;
  onUsbConnected: () => void;
  onUsbDisconnected: () => void;
  onUsbReady: (w: number, h: number) => void;
  onUsbHelloAck: () => void;
  onUsbHelloTimeout: () => void;
}

interface Window extends Partial<UsbBridge> {
  jsQR?: typeof globalThis.jsQR;
}

// ── Cross-file globals — declared on globalThis at runtime ──
declare var $: <T extends HTMLElement = HTMLElement>(id: string) => T;

declare var ws: WebSocket | null;
declare var connected: boolean;
declare var reconnectAttempts: number;
declare var maxReconnect: number;

declare var displayW: number;
declare var displayH: number;
declare var selectedServer: ServerInfo | null;
declare var selectedMode: Mode;
declare var serverName: string;
declare var serverPlatform: string;
declare var serverRes: string;

declare var streamImg: HTMLImageElement;
declare var framePolling: boolean;
declare var frameBaseUrl: string;
declare var blobUrl: string | null;
declare var streamVisible: boolean;
declare var fps: number;
declare var frameCount: number;
declare var fpsTimer: ReturnType<typeof setInterval> | null;
declare var overlayTimer: ReturnType<typeof setTimeout> | null;

declare var currentTab: TabName;
declare var toastId: number;

declare var jsQR: ((d: Uint8ClampedArray, w: number, h: number, opts?: { inversionAttempts?: string }) => { data: string } | null) | undefined;

// ── Connection state machine (core/state.ts) ──
type ViorConnState =
  | 'pre-intent'
  | 'scanning'
  | 'found-server'
  | 'pairing'
  | 'connecting'
  | 'connected'
  | 'reconnecting'
  | 'disconnected';

interface ViorStateData {
  state: ViorConnState;
  serverName?: string;
  transport?: 'wifi' | 'usb';
}

declare const viorState: {
  set: (next: Partial<ViorStateData>) => void;
  get: () => ViorStateData;
  on: (cb: (s: ViorStateData) => void) => void;
  chipLabel: (s: ViorStateData) => string;
};

// ── Cross-file functions defined in screen modules ──
declare function startDiscovery(): void;
declare function stopQRScan(): void;
declare function openStream(): void;
declare function hideStream(): void;
declare function toast(tone: ToastTone, title: string, msg?: string | null): void;
declare function esc(s: unknown): string;
declare function setConnState(state: ConnectionState): void;
declare function switchTab(name: TabName): void;
declare function handleFileMessage(msg: { type: string; data?: unknown }): void;
declare function handleIncomingFile(msg: { type: 'incoming-file'; data: unknown }): void;
declare function selectServer(host: string, port: number, name: string, platform: string): void;
declare function doConnect(): void;
declare function initiateConnect(): void;
declare function promptPair(): void;
declare function showView(name: string): void;
declare function sendInput(action: string, x: number, y: number): void;
declare function wsSend(obj: unknown): void;
