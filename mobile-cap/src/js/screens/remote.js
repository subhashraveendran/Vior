// ── Remote trackpad ──
var trackpad = $('trackpad'), trackpadHint = $('trackpad-hint');
var tpLastX = 0, tpLastY = 0, tpFingers = 0, tpMoved = false, tpStartT = 0;
function wsSend(obj) { if (ws && ws.readyState === 1) ws.send(JSON.stringify(obj)); }
function flash(msg) {
  var p = $('flash-pill');
  p.textContent = msg + ' sent';
  p.classList.remove('hidden');
  clearTimeout(flash._t);
  flash._t = setTimeout(function () { p.classList.add('hidden'); }, 900);
}
trackpad.addEventListener('touchstart', function (e) {
  e.preventDefault();
  tpFingers = e.touches.length; tpMoved = false; tpStartT = Date.now();
  var t = e.touches[0]; tpLastX = t.clientX; tpLastY = t.clientY;
  trackpadHint.style.display = 'none';
}, { passive: false });
trackpad.addEventListener('touchmove', function (e) {
  e.preventDefault();
  var t = e.touches[0];
  var dx = t.clientX - tpLastX, dy = t.clientY - tpLastY;
  tpLastX = t.clientX; tpLastY = t.clientY;
  if (Math.abs(dx) + Math.abs(dy) > 2) tpMoved = true;
  if (e.touches.length >= 2) wsSend({ type: 'input', data: { event: 'scroll', dx: Math.round(dx / 4), dy: Math.round(dy / 4) } });
  else wsSend({ type: 'input', data: { event: 'mouse', action: 'move', dx: dx * 2, dy: dy * 2 } });
}, { passive: false });
trackpad.addEventListener('touchend', function (e) {
  e.preventDefault();
  var dur = Date.now() - tpStartT;
  if (!tpMoved && dur < 300) {
    var action = tpFingers >= 2 ? 'rightclick' : 'click';
    wsSend({ type: 'input', data: { event: 'mouse', action: action } });
    flash(action === 'rightclick' ? 'Right click' : 'Click');
  }
  tpFingers = 0;
  trackpadHint.style.display = '';
}, { passive: false });

// Scroll strip
var ss = $('scroll-strip'), ssDown = false, ssY = 0;
ss.addEventListener('touchstart', function (e) { e.preventDefault(); ssDown = true; ssY = e.touches[0].clientY; ss.classList.add('active'); }, { passive: false });
ss.addEventListener('touchmove', function (e) {
  e.preventDefault();
  if (!ssDown) return;
  var y = e.touches[0].clientY, dy = y - ssY;
  if (Math.abs(dy) > 24) { ssY = y; wsSend({ type: 'input', data: { event: 'scroll', dx: 0, dy: dy > 0 ? 3 : -3 } }); flash('Scroll'); }
}, { passive: false });
ss.addEventListener('touchend', function (e) { e.preventDefault(); ssDown = false; ss.classList.remove('active'); }, { passive: false });

$('click-btn').addEventListener('click', function () { wsSend({ type: 'input', data: { event: 'mouse', action: 'click' } }); flash('Click'); });
$('rclick-btn').addEventListener('click', function () { wsSend({ type: 'input', data: { event: 'mouse', action: 'rightclick' } }); flash('Right click'); });

// Remote view toggle
$('remote-view-trackpad').addEventListener('click', function () {
  $('remote-view-trackpad').classList.add('active'); $('remote-view-keys').classList.remove('active');
  $('remote-trackpad-body').classList.remove('hidden'); $('remote-keys-body').classList.add('hidden');
});
$('remote-view-keys').addEventListener('click', function () {
  $('remote-view-trackpad').classList.remove('active'); $('remote-view-keys').classList.add('active');
  $('remote-trackpad-body').classList.add('hidden'); $('remote-keys-body').classList.remove('hidden');
});

// Shortcuts grid
var SHORTCUTS = [
  ['Cmd+c', 'Copy', '⌘C', 'copy'], ['Cmd+v', 'Paste', '⌘V', 'paste'],
  ['Cmd+x', 'Cut', '⌘X', 'cut'], ['Cmd+z', 'Undo', '⌘Z', 'undo'],
  ['Cmd+Shift+z', 'Redo', '⇧⌘Z', 'redo'], ['Cmd+a', 'Select All', '⌘A', 'layers'],
  ['Cmd+s', 'Save', '⌘S', 'save'], ['Cmd+f', 'Find', '⌘F', 'search'],
  ['Cmd+Tab', 'App ⇆', '⌘⇥', 'swap'], ['Cmd+`', 'Window ⇆', '⌘`', 'window'],
  ['Cmd+Space', 'Spotlight', '⌘Sp', 'spotlight'], ['Cmd+q', 'Quit', '⌘Q', 'quit'],
];
var ICONS = {
  copy: '<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/>',
  paste: '<rect x="5" y="5" width="14" height="16" rx="2"/><rect x="9" y="2.5" width="6" height="3.5" rx="1.2"/>',
  cut: '<circle cx="6.5" cy="17" r="2.5"/><circle cx="6.5" cy="7" r="2.5"/><path d="M8.7 8.5L20 17M8.7 15.5L20 7"/>',
  undo: '<path d="M9 7L4.5 11.5 9 16"/><path d="M4.5 11.5H14a5 5 0 0 1 0 10h-1.5"/>',
  redo: '<path d="M15 7l4.5 4.5L15 16"/><path d="M19.5 11.5H10a5 5 0 0 0 0 10h1.5"/>',
  layers: '<path d="M12 3l9 5-9 5-9-5z"/><path d="M3 13l9 5 9-5"/>',
  save: '<path d="M5 4h11l3 3v13H5z"/><path d="M8 4v5h7V4M9 13h6v7H9z"/>',
  search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="M15.5 15.5L21 21"/>',
  swap: '<path d="M4 8h13M14 5l3 3-3 3"/><path d="M20 16H7M10 13l-3 3 3 3"/>',
  window: '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 9h18"/>',
  spotlight: '<circle cx="11" cy="11" r="6.5"/><path d="M15.6 15.6L21 21"/><circle cx="11" cy="11" r="2.3"/>',
  quit: '<path d="M14 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3"/><path d="M10 12H3M6 8.5L2.5 12 6 15.5"/>',
};
var grid = $('shortcut-grid');
SHORTCUTS.forEach(function (s) {
  var b = document.createElement('button');
  b.className = 'keycap';
  b.dataset.key = s[0];
  b.innerHTML =
    '<svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">' + (ICONS[s[3]] || '') + '</svg>' +
    '<span class="keycap-label keycap-label-icon">' + s[1] + '</span>' +
    '<span class="keycap-sub">' + s[2] + '</span>';
  grid.appendChild(b);
});

// F-keys
var fkeyGrid = $('fkey-grid');
['F1', 'F2', 'F3', 'F4', 'F5', 'F6', 'F11', 'F12'].forEach(function (f) {
  var b = document.createElement('button');
  b.className = 'keycap'; b.dataset.key = f;
  b.innerHTML = '<span class="keycap-label">' + f + '</span>';
  fkeyGrid.appendChild(b);
});

// Wire all keycap clicks
document.querySelectorAll('.keycap').forEach(function (k) {
  var fired = false;
  function send() {
    var key = k.dataset.key; if (!key) return;
    wsSend({ type: 'input', data: { event: 'key', key: key } });
    var label = k.querySelector('.keycap-label'); flash(label ? label.textContent : key);
  }
  k.addEventListener('touchend', function (e) { e.preventDefault(); fired = true; send(); setTimeout(function () { fired = false; }, 400); }, { passive: false });
  k.addEventListener('click', function () { if (!fired) send(); });
});

// Soft keyboard
var kbInput = $('kb-input');
$('kb-btn').addEventListener('click', function () {
  kbInput.value = ''; kbInput.focus();
  toast('info', 'Keyboard ready', 'Type to forward keys.');
});
kbInput.addEventListener('input', function (e) {
  var data = e.data;
  if (data) { for (var i = 0; i < data.length; i++) wsSend({ type: 'input', data: { event: 'key', key: data[i] } }); }
  kbInput.value = '';
});
kbInput.addEventListener('keydown', function (e) {
  var map = { 'Backspace': 'BackSpace', 'Enter': 'Return', 'Tab': 'Tab', 'ArrowUp': 'Up', 'ArrowDown': 'Down', 'ArrowLeft': 'Left', 'ArrowRight': 'Right', 'Escape': 'Escape' };
  var k = map[e.key];
  if (k) { e.preventDefault(); wsSend({ type: 'input', data: { event: 'key', key: k } }); }
});

