'use strict';
// ── File transfer ──
const CHUNK_SIZE = 48 * 1024;

interface FileTransfer {
  id: string;
  name: string;
  size: number;
  mimeType: string;
  preview: string;
  transferred: number;
  complete: boolean;
  direction: 'in' | 'out';
  status: 'sending' | 'receiving' | 'done' | 'received' | 'failed' | 'incoming';
  data?: Uint8Array;
  chunks?: string[];
  pending?: boolean;
  progress?: number;
  blobUrl?: string;
}

interface FileMessage {
  type: 'file-offer' | 'file-accept' | 'file-reject' | 'file-chunk' | 'file-complete';
  data: {
    id: string;
    name?: string;
    size?: number;
    mimeType?: string;
    preview?: string;
    offset?: number;
    data?: string;
    hash?: string;
    reason?: string;
  };
}

const fileTransfers: Record<string, FileTransfer> = {};

($('send-file-btn') as HTMLElement).addEventListener('click', function (): void { ($('file-input') as HTMLElement).click(); });
($('send-photo-btn') as HTMLElement).addEventListener('click', function (): void { ($('photo-input') as HTMLElement).click(); });
($('file-input') as HTMLInputElement).addEventListener('change', function (e: Event): void { const tgt = e.target as HTMLInputElement; if (tgt.files && tgt.files[0]) sendFile(tgt.files[0]); tgt.value = ''; });
($('photo-input') as HTMLInputElement).addEventListener('change', function (e: Event): void { const tgt = e.target as HTMLInputElement; if (tgt.files && tgt.files[0]) sendFile(tgt.files[0]); tgt.value = ''; });

function genID(): string { const a = new Uint8Array(8); crypto.getRandomValues(a); return Array.from(a, function (b: number): string { return ('0' + b.toString(16)).slice(-2); }).join(''); }
function fmtSize(b: number): string { if (b < 1024) return b + ' B'; if (b < 1048576) return (b / 1024).toFixed(1) + ' KB'; return (b / 1048576).toFixed(1) + ' MB'; }

function sendFile(file: File): void {
  // USB transport doesn't support file transfer (AOA carries video+touch only).
  const transport = (window as unknown as { viorTransport?: () => string }).viorTransport;
  if (typeof transport === 'function' && transport() === 'usb') {
    toast('warning', 'Unavailable over USB', 'File transfer needs a Wi-Fi connection.');
    return;
  }
  const id = genID();
  const reader = new FileReader();
  reader.onload = function (): void {
    const data = new Uint8Array(reader.result as ArrayBuffer);
    const t: FileTransfer = { id: id, name: file.name, size: file.size, mimeType: file.type || 'application/octet-stream', preview: '', transferred: 0, complete: false, data: data, direction: 'out', status: 'sending' };
    fileTransfers[id] = t;
    if (file.type && file.type.indexOf('image/') === 0) {
      const pr = new FileReader();
      pr.onload = function (): void { t.preview = pr.result as string; sendOffer(t); };
      pr.readAsDataURL(file);
    } else { sendOffer(t); }
  };
  reader.readAsArrayBuffer(file);
}
function sendOffer(t: FileTransfer): void {
  if (!ws || ws.readyState !== 1) return;
  ws.send(JSON.stringify({ type: 'file-offer', data: { id: t.id, name: t.name, size: t.size, mimeType: t.mimeType, preview: t.preview } }));
  renderTransfers();
  toast('info', 'Offering', t.name);
}
function sendChunks(t: FileTransfer): void {
  let offset = 0;
  function next(): void {
    if (!t.data) return;
    if (offset >= t.data.length) {
      t.complete = true; t.status = 'done';
      ws!.send(JSON.stringify({ type: 'file-complete', data: { id: t.id, hash: '' } }));
      renderTransfers();
      toast('success', 'Sent', t.name);
      return;
    }
    const end = Math.min(offset + CHUNK_SIZE, t.data.length);
    const chunk = t.data.slice(offset, end);
    let s = ''; for (let i = 0; i < chunk.length; i++) s += String.fromCharCode(chunk[i]);
    ws!.send(JSON.stringify({ type: 'file-chunk', data: { id: t.id, offset: offset, data: btoa(s) } }));
    offset = end; t.transferred = offset;
    t.progress = Math.round(offset / t.data.length * 100);
    renderTransfers();
    setTimeout(next, 5);
  }
  next();
}
// A file id is a server-supplied opaque string used as both an object
// key on fileTransfers and as text interpolated into HTML onclick
// handlers. Without this strict filter, ids like '__proto__' polluted
// the object prototype, and ids containing quote characters broke out
// of the onclick attribute and executed arbitrary JS. Hex-only and
// 8-64 chars covers every legitimate desktop-generated id format.
const VALID_ID = /^[a-f0-9]{8,64}$/;
function validId(id: unknown): id is string {
  return typeof id === 'string' && VALID_ID.test(id);
}

// MAX_TRANSFER_BYTES mirrors the desktop's MaxDownloadSize (2 GiB). A phone
// has far less headroom than that — chunks are accumulated in memory as
// base64 strings before being assembled into a Blob — so the declared size is
// the ceiling, and the running total is enforced against it below.
const MAX_TRANSFER_BYTES = 2 * 1024 * 1024 * 1024;

function handleFileMessage(msg: FileMessage): void {
  const d = msg.data;
  if (!validId(d.id)) {
    return;
  }
  if (msg.type === 'file-offer') {
    // A declared size outside sane bounds is refused before a single
    // chunk is buffered. Without this the peer set the budget that the
    // chunk handler below is meant to enforce.
    const size = d.size || 0;
    if (size <= 0 || size > MAX_TRANSFER_BYTES) {
      ws!.send(JSON.stringify({ type: 'file-reject', data: { id: d.id, reason: 'invalid size' } }));
      toast('warning', 'Declined', 'That file is too large to receive.');
      return;
    }
    fileTransfers[d.id] = { id: d.id, name: d.name || '', size: size, mimeType: d.mimeType || '', preview: d.preview || '', transferred: 0, complete: false, chunks: [], direction: 'in', pending: true, status: 'incoming' };
    renderIncoming();
    toast('info', 'Incoming', d.name || '');
    switchTab('files');
  } else if (msg.type === 'file-accept') {
    const t = fileTransfers[d.id]; if (t && t.direction === 'out') sendChunks(t);
  } else if (msg.type === 'file-reject') {
    const t2 = fileTransfers[d.id]; if (t2) { delete fileTransfers[d.id]; renderTransfers(); toast('warning', 'Declined', t2.name); }
  } else if (msg.type === 'file-chunk') {
    const t3 = fileTransfers[d.id]; if (t3 && t3.direction === 'in' && d.data) {
      // Enforce the size declared in the offer. Chunks were previously
      // accumulated without any cumulative check, so a peer could
      // announce a 1 MB file and then stream indefinitely — every chunk
      // retained in memory until the phone was killed by the OS.
      const chunkLen = atob(d.data).length;
      if (t3.transferred + chunkLen > t3.size) {
        ws!.send(JSON.stringify({ type: 'file-reject', data: { id: d.id, reason: 'exceeded declared size' } }));
        delete fileTransfers[d.id];
        renderIncoming(); renderTransfers();
        toast('error', 'Transfer aborted', 'The sender exceeded the size it declared.');
        return;
      }
      t3.chunks!.push(d.data); t3.transferred += chunkLen;
      t3.progress = Math.round(t3.transferred / t3.size * 100); t3.status = 'receiving';
      renderTransfers();
    }
  } else if (msg.type === 'file-complete') {
    const t4 = fileTransfers[d.id];
    if (t4 && t4.direction === 'in' && t4.chunks) {
      t4.complete = true; t4.status = 'received';
      const parts: Uint8Array[] = []; for (let c = 0; c < t4.chunks.length; c++) { const raw = atob(t4.chunks[c]); const arr = new Uint8Array(raw.length); for (let j = 0; j < raw.length; j++) arr[j] = raw.charCodeAt(j); parts.push(arr); }
      t4.blobUrl = URL.createObjectURL(new Blob(parts as BlobPart[], { type: t4.mimeType }));
      t4.chunks = [];
      renderTransfers(); renderIncoming();
      toast('success', 'Received', t4.name);
    }
  }
}
(window as unknown as { _acceptFile: (id: string) => void })._acceptFile = function (id: string): void {
  const t = fileTransfers[id]; if (!t) return;
  t.pending = false; t.status = 'receiving';
  ws!.send(JSON.stringify({ type: 'file-accept', data: { id: id } }));
  renderIncoming(); renderTransfers();
};
(window as unknown as { _rejectFile: (id: string) => void })._rejectFile = function (id: string): void {
  ws!.send(JSON.stringify({ type: 'file-reject', data: { id: id, reason: 'rejected' } }));
  delete fileTransfers[id]; renderIncoming(); renderTransfers();
};
(window as unknown as { _saveFile: (id: string) => void })._saveFile = function (id: string): void {
  const t = fileTransfers[id]; if (!t || !t.blobUrl) return;
  const a = document.createElement('a'); a.href = t.blobUrl; a.download = t.name; document.body.appendChild(a); a.click(); document.body.removeChild(a);
  // Revoke after save to prevent unbounded blob memory growth.
  URL.revokeObjectURL(t.blobUrl); t.blobUrl = undefined;
  toast('success', 'Saved', t.name);
};
function statusMeta(t: FileTransfer): { color: string; text: string } {
  if (t.status === 'failed') return { color: 'var(--err)', text: 'Failed' };
  if (t.status === 'done') return { color: 'var(--ok)', text: 'Sent' };
  if (t.status === 'received') return { color: 'var(--ok)', text: 'Received' };
  if (t.status === 'receiving') return { color: 'var(--warn)', text: 'Receiving · ' + (t.progress || 0) + '%' };
  return { color: 'var(--accent)', text: 'Sending · ' + (t.progress || 0) + '%' };
}
function fileIconSvg(): string { return '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M6 3h7l5 5v13a0 0 0 0 1 0 0H6a0 0 0 0 1 0 0z"/><path d="M13 3v5h5"/></svg>'; }
function photoIconSvg(): string { return '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="18" height="14" rx="2"/><circle cx="8.5" cy="10" r="1.6"/><path d="M5 17l4.5-4 3 2.6L16 12l3 3.2"/></svg>'; }

const BASE64_RE = /^[A-Za-z0-9+/]+={0,2}$/;
// Raster formats only. SVG is deliberately excluded: it is a document format
// with its own parser, and nothing in this product ever produces an SVG
// thumbnail. (Script inside an SVG does not run when it is loaded through an
// <img>, so this is depth rather than the primary defence.)
const DATA_IMAGE_RE = /^data:image\/(?:png|jpeg|jpg|gif|webp);base64,[A-Za-z0-9+/]+={0,2}$/;

// previewSrc builds the thumbnail data: URI from a peer-supplied preview, or
// returns '' when the value is not a base64 image. The caller falls back to
// the extension badge.
//
// The peer's mimeType is deliberately NOT used. Previews are always JPEG —
// the desktop re-encodes every thumbnail with jpeg.Encode regardless of the
// source file's type, and the mobile never generates previews at all. The
// mimeType field describes the *file*, not its thumbnail, so interpolating it
// here was both wrong (JPEG bytes labelled image/png, saved only by browser
// sniffing) and the injection vector: a hostile desktop could send
// `image/jpeg";onerror="alert(1)" x="` and break straight out of the src
// attribute. Hardcoding the type removes the vector at its root instead of
// validating around it.
//
// The value lands in a URI, so this validates rather than escapes — escaping
// would prevent a breakout but still allow a hostile scheme inside the
// attribute. An allowlist answers the question that matters: is this actually
// a base64 image?
function previewSrc(preview: string): string {
  if (!preview) return '';
  // Some senders may pass a complete data: URI — accept only a well-formed
  // raster one.
  if (preview.indexOf('data:') === 0) {
    return DATA_IMAGE_RE.test(preview) ? preview : '';
  }
  if (!BASE64_RE.test(preview)) return '';
  return 'data:image/jpeg;base64,' + preview;
}

function renderIncoming(): void {
  const wrap = $('incoming-wrap') as HTMLElement, list = $('incoming-list') as HTMLElement;
  let html = ''; let has = false;
  Object.keys(fileTransfers).forEach(function (id: string): void {
    const t = fileTransfers[id];
    if (t.direction !== 'in' || !t.pending) return;
    has = true;
    // Preview: real thumbnail if server sent one (images), big ext badge otherwise.
    let thumb: string;
    const src = previewSrc(t.preview || '');
    if (src) {
      // esc() as well as the allowlist: the validation already rules out a
      // breakout, and this keeps the attribute safe if the pattern is ever
      // loosened.
      thumb = '<img src="' + esc(src) + '" alt="" style="width:56px;height:56px;border-radius:10px;object-fit:cover;border:1px solid var(--border);">';
    } else {
      const ext = (t.name.split('.').pop() || 'FILE').slice(0, 4).toUpperCase();
      thumb = '<div style="width:56px;height:56px;border-radius:10px;display:flex;align-items:center;justify-content:center;background:var(--accent-weak);color:var(--accent);font:700 13px/1 var(--font-mono);letter-spacing:0.03em;border:1px solid var(--accent-line);">' + esc(ext) + '</div>';
    }
    html +=
      '<div class="incoming-card">' +
        '<div class="incoming-head">' +
          '<span class="incoming-icon">' + thumb + '</span>' +
          '<div style="flex:1;min-width:0;">' +
            '<div class="incoming-name">' + esc(t.name) + '</div>' +
            '<div class="incoming-meta">' + fmtSize(t.size) + ' · from ' + esc(serverName) + '</div>' +
          '</div>' +
        '</div>' +
        '<div class="incoming-buttons">' +
          '<button class="btn btn-ghost btn-block" onclick="window._rejectFile(\'' + esc(id) + '\')">Decline</button>' +
          '<button class="btn btn-primary btn-block" onclick="window._acceptFile(\'' + esc(id) + '\')">' +
            '<svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M12 4v11M7 11l5 5 5-5"/><path d="M5 20h14"/></svg>' +
            'Accept' +
          '</button>' +
        '</div>' +
      '</div>';
  });
  list.innerHTML = html;
  wrap.classList.toggle('hidden', !has);
}
// ── Desktop → Mobile HTTP-download path ─────────────────────────────
//
// The desktop pushes {type:"incoming-file", data:{id,name,size,mime,url}}
// over the WS. We register the transfer in the same `fileTransfers`
// map as the WS-chunked path so the Files UI just works, then either
// auto-accept (trusted/known server) or render an Accept/Reject card.
// On accept we GET `frameBaseUrl + url` and store the body as a blob URL.
interface IncomingFilePayload { id: string; name: string; size: number; mime?: string; url?: string; preview?: string }

function handleIncomingFile(msg: { type: 'incoming-file'; data: unknown }): void {
  const d = (msg.data || {}) as IncomingFilePayload;
  if (!d.id || !d.url) return;
  const t: FileTransfer = {
    id: d.id, name: d.name || 'file', size: d.size || 0,
    mimeType: d.mime || 'application/octet-stream', preview: d.preview || '',
    transferred: 0, complete: false, direction: 'in', pending: true,
    status: 'incoming',
  };
  // Stash the URL on the transfer object so accept can fetch later.
  (t as unknown as { url: string }).url = d.url;
  fileTransfers[d.id] = t;

  // Trust mirror: if this server is "known" client-side (we previously
  // paired with it and never forgot the pair) auto-accept silently —
  // same UX as the upload path's trusted device auto-accept.
  let auto = false;
  try {
    if (selectedServer) {
      const key = selectedServer.host + ':' + selectedServer.port;
      auto = localStorage.getItem('vior_known_' + key) === '1';
    }
  } catch (_) { /* localStorage blocked */ }

  if (auto) {
    toast('info', 'Receiving', d.name || '');
    fetchDownload(d.id);
  } else {
    renderIncoming();
    toast('info', 'Incoming', d.name || '');
    switchTab('files');
  }
}

async function fetchDownload(id: string): Promise<void> {
  const t = fileTransfers[id];
  if (!t) return;
  // Clamp to MaxDownloadSize so a malicious server offering a huge file
  // can't OOM the phone. The desktop enforces this too — this is belt-
  // and-suspenders on the receiving side.
  if (t.size > 2 * 1024 * 1024 * 1024 || t.size < 0) {
    t.status = 'failed';
    toast('error', 'File too large', fmtSize(t.size));
    if (ws && ws.readyState === 1) {
      try { ws.send(JSON.stringify({ type: 'download-reject', data: { id: id, reason: 'file too large' } })); }
      catch (_) {}
    }
    delete fileTransfers[id];
    renderTransfers(); renderIncoming();
    return;
  }
  const url = (t as unknown as { url?: string }).url;
  if (!url || !frameBaseUrl) {
    t.status = 'failed';
    renderTransfers(); renderIncoming();
    if (ws && ws.readyState === 1) {
      ws.send(JSON.stringify({ type: 'download-reject', data: { id: id, reason: 'no url or transport' } }));
    }
    return;
  }
  t.pending = false; t.status = 'receiving';
  renderIncoming(); renderTransfers();
  // Tell the desktop we're starting — purely advisory so the desktop UI
  // can show "Sending" instead of "Offered".
  try {
    if (ws && ws.readyState === 1) {
      ws.send(JSON.stringify({ type: 'download-accept', data: { id: id } }));
    }
  } catch (_) { /* best-effort notify */ }

  try {
    const resp = await fetch(frameBaseUrl + url);
    if (!resp.ok) throw new Error('HTTP ' + resp.status);
    // Stream chunks so the progress bar reflects bytes received instead
    // of jumping from 0 → 100. Falls back to blob() when ReadableStream
    // isn't available (very old WebViews).
    const total = t.size || parseInt(resp.headers.get('Content-Length') || '0', 10) || 0;
    let blob: Blob;
    if (resp.body && (resp.body as ReadableStream<Uint8Array>).getReader) {
      const reader = (resp.body as ReadableStream<Uint8Array>).getReader();
      const chunks: BlobPart[] = [];
      let got = 0;
      // eslint-disable-next-line no-constant-condition
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        if (value) {
          chunks.push(value as BlobPart);
          got += value.byteLength;
          t.transferred = got;
          t.progress = total > 0 ? Math.round(got / total * 100) : 0;
          renderTransfers();
        }
      }
      blob = new Blob(chunks, { type: t.mimeType });
    } else {
      blob = await resp.blob();
      t.transferred = blob.size;
      t.progress = 100;
    }
    t.blobUrl = URL.createObjectURL(blob);
    t.complete = true; t.status = 'received';
    renderTransfers();
    toast('success', 'Received', t.name);
    try {
      if (ws && ws.readyState === 1) {
        ws.send(JSON.stringify({ type: 'download-complete', data: { id: id } }));
      }
    } catch (_) { /* best-effort notify */ }
  } catch (e) {
    console.error('download failed', e);
    t.status = 'failed';
    renderTransfers();
    toast('error', 'Download failed', String(e));
    try {
      if (ws && ws.readyState === 1) {
        ws.send(JSON.stringify({ type: 'download-reject', data: { id: id, reason: String(e) } }));
      }
    } catch (_) { /* best-effort notify */ }
  }
}

// Extend Accept/Reject so the existing UI buttons drive the HTTP path
// when the transfer was created via incoming-file (it has a stashed
// `url`), otherwise fall back to the original WS chunked accept/reject.
const _origAccept = (window as unknown as { _acceptFile: (id: string) => void })._acceptFile;
(window as unknown as { _acceptFile: (id: string) => void })._acceptFile = function (id: string): void {
  const t = fileTransfers[id];
  if (t && (t as unknown as { url?: string }).url) { fetchDownload(id); return; }
  _origAccept(id);
};
const _origReject = (window as unknown as { _rejectFile: (id: string) => void })._rejectFile;
(window as unknown as { _rejectFile: (id: string) => void })._rejectFile = function (id: string): void {
  const t = fileTransfers[id];
  if (t && (t as unknown as { url?: string }).url) {
    try {
      if (ws && ws.readyState === 1) {
        ws.send(JSON.stringify({ type: 'download-reject', data: { id: id, reason: 'rejected' } }));
      }
    } catch (_) { /* best-effort notify */ }
    delete fileTransfers[id]; renderIncoming(); renderTransfers();
    return;
  }
  _origReject(id);
};

function renderTransfers(): void {
  const list = $('transfer-list') as HTMLElement, empty = $('transfer-empty') as HTMLElement;
  let html = ''; let count = 0;
  Object.keys(fileTransfers).forEach(function (id: string): void {
    const t = fileTransfers[id]; if (t.pending) return; count++;
    const m = statusMeta(t);
    const active = t.status === 'sending' || t.status === 'receiving';
    const icon = t.mimeType && t.mimeType.indexOf('image/') === 0 ? photoIconSvg() : fileIconSvg();
    html +=
      '<div class="card transfer-row">' +
        '<div class="transfer-head">' +
          '<span class="transfer-icon" style="color:' + m.color + ';">' + icon + '</span>' +
          '<div style="flex:1;min-width:0;">' +
            '<div class="transfer-name">' + esc(t.name) + '</div>' +
            '<div class="transfer-meta">' +
              '<span class="transfer-status" style="color:' + m.color + ';">' + m.text + '</span>' +
              '<span class="transfer-size">· ' + fmtSize(t.size) + '</span>' +
            '</div>' +
          '</div>' +
          (t.status === 'received'
            ? '<button class="btn btn-primary btn-sm" onclick="window._saveFile(\'' + id + '\')"><svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M12 4v11M7 11l5 5 5-5"/><path d="M5 20h14"/></svg>Save</button>'
            : (t.status === 'done' ? '<span style="color: var(--ok);"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M4.5 12.5l4.5 4.5L19.5 6.5"/></svg></span>' : '')) +
        '</div>' +
        (active ? '<div class="bar-inner"><i style="width:' + (t.progress || 0) + '%;"></i></div>' : '') +
      '</div>';
  });
  list.innerHTML = html;
  empty.classList.toggle('hidden', count > 0);
}

// Called from connect.ts on WS disconnect — clears stale transfers
// whose chunks will never arrive, and revokes any blob URLs to prevent
// unbounded memory growth across session cycles.
function clearFileTransfers(): void {
  Object.keys(fileTransfers).forEach(function (id) {
    const t = fileTransfers[id];
    if (t?.blobUrl) URL.revokeObjectURL(t.blobUrl);
  });
  for (const k in fileTransfers) delete fileTransfers[k];
  renderTransfers(); renderIncoming();
}
(window as unknown as { clearFileTransfers: () => void }).clearFileTransfers = clearFileTransfers;
