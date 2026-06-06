'use strict';
// ── File transfer ──
var CHUNK_SIZE = 48 * 1024;
var fileTransfers = {};

$('send-file-btn').addEventListener('click', function () { $('file-input').click(); });
$('send-photo-btn').addEventListener('click', function () { $('photo-input').click(); });
$('file-input').addEventListener('change', function (e) { if (e.target.files[0]) sendFile(e.target.files[0]); e.target.value = ''; });
$('photo-input').addEventListener('change', function (e) { if (e.target.files[0]) sendFile(e.target.files[0]); e.target.value = ''; });

function genID() { var a = new Uint8Array(8); crypto.getRandomValues(a); return Array.from(a, function (b) { return ('0' + b.toString(16)).slice(-2); }).join(''); }
function fmtSize(b) { if (b < 1024) return b + ' B'; if (b < 1048576) return (b / 1024).toFixed(1) + ' KB'; return (b / 1048576).toFixed(1) + ' MB'; }

function sendFile(file) {
  var id = genID();
  var reader = new FileReader();
  reader.onload = function () {
    var data = new Uint8Array(reader.result);
    var t = { id: id, name: file.name, size: file.size, mimeType: file.type || 'application/octet-stream', preview: '', transferred: 0, complete: false, data: data, direction: 'out', status: 'sending' };
    fileTransfers[id] = t;
    if (file.type && file.type.indexOf('image/') === 0) {
      var pr = new FileReader();
      pr.onload = function () { t.preview = pr.result; sendOffer(t); };
      pr.readAsDataURL(file);
    } else { sendOffer(t); }
  };
  reader.readAsArrayBuffer(file);
}
function sendOffer(t) {
  if (!ws || ws.readyState !== 1) return;
  ws.send(JSON.stringify({ type: 'file-offer', data: { id: t.id, name: t.name, size: t.size, mimeType: t.mimeType, preview: t.preview } }));
  renderTransfers();
  toast('info', 'Offering', t.name);
}
function sendChunks(t) {
  var offset = 0;
  function next() {
    if (offset >= t.data.length) {
      t.complete = true; t.status = 'done';
      ws.send(JSON.stringify({ type: 'file-complete', data: { id: t.id, hash: '' } }));
      renderTransfers();
      toast('success', 'Sent', t.name);
      return;
    }
    var end = Math.min(offset + CHUNK_SIZE, t.data.length);
    var chunk = t.data.slice(offset, end);
    var s = ''; for (var i = 0; i < chunk.length; i++) s += String.fromCharCode(chunk[i]);
    ws.send(JSON.stringify({ type: 'file-chunk', data: { id: t.id, offset: offset, data: btoa(s) } }));
    offset = end; t.transferred = offset;
    t.progress = Math.round(offset / t.data.length * 100);
    renderTransfers();
    setTimeout(next, 5);
  }
  next();
}
function handleFileMessage(msg) {
  var d = msg.data;
  if (msg.type === 'file-offer') {
    fileTransfers[d.id] = { id: d.id, name: d.name, size: d.size, mimeType: d.mimeType, preview: d.preview || '', transferred: 0, complete: false, chunks: [], direction: 'in', pending: true, status: 'incoming' };
    renderIncoming();
    toast('info', 'Incoming', d.name);
    switchTab('files');
  } else if (msg.type === 'file-accept') {
    var t = fileTransfers[d.id]; if (t && t.direction === 'out') sendChunks(t);
  } else if (msg.type === 'file-reject') {
    var t2 = fileTransfers[d.id]; if (t2) { delete fileTransfers[d.id]; renderTransfers(); toast('warning', 'Declined', t2.name); }
  } else if (msg.type === 'file-chunk') {
    var t3 = fileTransfers[d.id]; if (t3 && t3.direction === 'in') {
      t3.chunks.push(d.data); t3.transferred += atob(d.data).length;
      t3.progress = Math.round(t3.transferred / t3.size * 100); t3.status = 'receiving';
      renderTransfers();
    }
  } else if (msg.type === 'file-complete') {
    var t4 = fileTransfers[d.id];
    if (t4 && t4.direction === 'in') {
      t4.complete = true; t4.status = 'received';
      var parts = []; for (var c = 0; c < t4.chunks.length; c++) { var raw = atob(t4.chunks[c]); var arr = new Uint8Array(raw.length); for (var j = 0; j < raw.length; j++) arr[j] = raw.charCodeAt(j); parts.push(arr); }
      t4.blobUrl = URL.createObjectURL(new Blob(parts, { type: t4.mimeType }));
      t4.chunks = [];
      renderTransfers(); renderIncoming();
      toast('success', 'Received', t4.name);
    }
  }
}
window._acceptFile = function (id) {
  var t = fileTransfers[id]; if (!t) return;
  t.pending = false; t.status = 'receiving';
  ws.send(JSON.stringify({ type: 'file-accept', data: { id: id } }));
  renderIncoming(); renderTransfers();
};
window._rejectFile = function (id) {
  ws.send(JSON.stringify({ type: 'file-reject', data: { id: id, reason: 'rejected' } }));
  delete fileTransfers[id]; renderIncoming(); renderTransfers();
};
window._saveFile = function (id) {
  var t = fileTransfers[id]; if (!t || !t.blobUrl) return;
  var a = document.createElement('a'); a.href = t.blobUrl; a.download = t.name; document.body.appendChild(a); a.click(); document.body.removeChild(a);
  toast('success', 'Saved', t.name);
};
function statusMeta(t) {
  if (t.status === 'failed') return { color: 'var(--err)', text: 'Failed' };
  if (t.status === 'done') return { color: 'var(--ok)', text: 'Sent' };
  if (t.status === 'received') return { color: 'var(--ok)', text: 'Received' };
  if (t.status === 'receiving') return { color: 'var(--warn)', text: 'Receiving · ' + (t.progress || 0) + '%' };
  return { color: 'var(--accent)', text: 'Sending · ' + (t.progress || 0) + '%' };
}
function fileIconSvg() { return '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M6 3h7l5 5v13a0 0 0 0 1 0 0H6a0 0 0 0 1 0 0z"/><path d="M13 3v5h5"/></svg>'; }
function photoIconSvg() { return '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="18" height="14" rx="2"/><circle cx="8.5" cy="10" r="1.6"/><path d="M5 17l4.5-4 3 2.6L16 12l3 3.2"/></svg>'; }

function renderIncoming() {
  var wrap = $('incoming-wrap'), list = $('incoming-list');
  var html = ''; var has = false;
  Object.keys(fileTransfers).forEach(function (id) {
    var t = fileTransfers[id];
    if (t.direction !== 'in' || !t.pending) return;
    has = true;
    var icon = t.mimeType && t.mimeType.indexOf('image/') === 0 ? photoIconSvg() : fileIconSvg();
    html +=
      '<div class="incoming-card">' +
        '<div class="incoming-head">' +
          '<span class="incoming-icon">' + icon + '</span>' +
          '<div style="flex:1;min-width:0;">' +
            '<div class="incoming-name">' + esc(t.name) + '</div>' +
            '<div class="incoming-meta">' + fmtSize(t.size) + ' · from ' + esc(serverName) + '</div>' +
          '</div>' +
        '</div>' +
        '<div class="incoming-buttons">' +
          '<button class="btn btn-ghost btn-block" onclick="window._rejectFile(\'' + id + '\')">Decline</button>' +
          '<button class="btn btn-primary btn-block" onclick="window._acceptFile(\'' + id + '\')">' +
            '<svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M12 4v11M7 11l5 5 5-5"/><path d="M5 20h14"/></svg>' +
            'Accept' +
          '</button>' +
        '</div>' +
      '</div>';
  });
  list.innerHTML = html;
  wrap.classList.toggle('hidden', !has);
}
function renderTransfers() {
  var list = $('transfer-list'), empty = $('transfer-empty');
  var html = ''; var count = 0;
  Object.keys(fileTransfers).forEach(function (id) {
    var t = fileTransfers[id]; if (t.pending) return; count++;
    var m = statusMeta(t);
    var active = t.status === 'sending' || t.status === 'receiving';
    var icon = t.mimeType && t.mimeType.indexOf('image/') === 0 ? photoIconSvg() : fileIconSvg();
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

