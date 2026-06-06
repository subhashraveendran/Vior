// Shared types contract.
//
// Every other screen module imports from this file. Keep this file
// dependency-free (no DOM-mutating side effects, no runtime exports
// other than the small helper at the bottom).
//
// The original mobile JS was a series of plain <script defer> files
// that all shared a window-level namespace. To preserve that
// behaviour during the TS migration without forcing every screen
// agent to rewrite their lookups, the cross-file variables live on
// `globalThis` and are reachable as bare identifiers thanks to the
// `declare global { var X }` block below. Once Vite bundling lands
// we can collapse this into proper ES module imports.

export type ToastTone = 'success' | 'warning' | 'error' | 'info';

export type Mode = 'extend' | 'mirror';

export type ConnectionState =
  | 'offline'
  | 'connecting'
  | 'reconnecting'
  | 'online'
  | 'error';

export type TabName = 'display' | 'files' | 'remote' | 'qr' | 'settings';

export interface ServerInfo {
  name: string;
  host: string;
  port: number;
  platform?: string;
  resolution?: string;
  // Some discovery payloads include an addr field; keep loose for now.
  [k: string]: unknown;
}

// USB bridge callbacks installed on `window` by the Android side.
export interface UsbBridge {
  onUsbFrame: (b64: string) => void;
  onUsbConnected: () => void;
  onUsbDisconnected: () => void;
  onUsbReady: (w: number, h: number) => void;
}

// ── Globals shared across all screen modules ──
// Keeping these typed in one place means screen agents can write
// `ws = new WebSocket(...)` or `connected = true` and still get full
// TypeScript intellisense + checking without an explicit import.
declare global {
  // DOM helper used pervasively in the legacy code.
  // eslint-disable-next-line no-var
  var $: (id: string) => HTMLElement | null;

  // Connection / WebSocket state
  // eslint-disable-next-line no-var
  var ws: WebSocket | null;
  // eslint-disable-next-line no-var
  var connected: boolean;
  // eslint-disable-next-line no-var
  var reconnectAttempts: number;
  // eslint-disable-next-line no-var
  var maxReconnect: number;

  // Display / server selection state
  // eslint-disable-next-line no-var
  var displayW: number;
  // eslint-disable-next-line no-var
  var displayH: number;
  // eslint-disable-next-line no-var
  var selectedServer: ServerInfo | null;
  // eslint-disable-next-line no-var
  var selectedMode: Mode;
  // eslint-disable-next-line no-var
  var serverName: string;
  // eslint-disable-next-line no-var
  var serverPlatform: string;
  // eslint-disable-next-line no-var
  var serverRes: string;

  // Streaming state
  // eslint-disable-next-line no-var
  var streamImg: HTMLImageElement | null;
  // eslint-disable-next-line no-var
  var framePolling: boolean;
  // eslint-disable-next-line no-var
  var frameBaseUrl: string;
  // eslint-disable-next-line no-var
  var blobUrl: string | null;
  // eslint-disable-next-line no-var
  var streamVisible: boolean;
  // eslint-disable-next-line no-var
  var fps: number;
  // eslint-disable-next-line no-var
  var frameCount: number;
  // eslint-disable-next-line no-var
  var fpsTimer: ReturnType<typeof setInterval> | null;
  // eslint-disable-next-line no-var
  var overlayTimer: ReturnType<typeof setTimeout> | null;

  // Misc UI state
  // eslint-disable-next-line no-var
  var currentTab: TabName;
  // eslint-disable-next-line no-var
  var toastId: number;

  // Cross-module functions exposed by screen agents. Declared as
  // optional so callers must narrow with `typeof X === 'function'`
  // — matching the defensive checks already in the legacy code.
  // eslint-disable-next-line no-var
  var startDiscovery: () => void;
  // eslint-disable-next-line no-var
  var stopQRScan: (() => void) | undefined;
  // eslint-disable-next-line no-var
  var openStream: () => void;
  // eslint-disable-next-line no-var
  var hideStream: () => void;

  // Toast + connection-state helpers (defined in core.ts, used everywhere).
  // eslint-disable-next-line no-var
  var toast: (tone: ToastTone, title: string, msg?: string) => void;
  // eslint-disable-next-line no-var
  var esc: (s: unknown) => string;
  // eslint-disable-next-line no-var
  var setConnState: (state: ConnectionState) => void;
  // eslint-disable-next-line no-var
  var switchTab: (name: TabName) => void;

  // USB bridge — these are written by the Android WebView host.
  interface Window extends Partial<UsbBridge> {}
}

export {};
