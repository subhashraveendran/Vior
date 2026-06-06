'use strict';
// ── Mobile connection state machine ───────────────────────────────
//
// Single source of truth for the app's pre/post-connect lifecycle.
// Every screen subscribes via `onState(cb)` and re-renders the bits
// it owns whenever the state changes — instead of every module
// flipping classNames behind each other's backs.
//
// Why a state machine and not just a `connected` boolean?
// ────────────────────────────────────────────────────────
// Pre-state machine we had ~7 independent flags (`connected`,
// `scanning`, `selectedServer`, `transportMode`, plus DOM `.hidden`
// classes scattered across 4 files). Each screen had its own
// interpretation of "is the user connected?" → the bottom dock
// stayed visible during scanning, "Mirror/Extend" was shown
// pre-connect, etc. This module funnels every transition through
// one function so the rendering rules live in one place.
//
// States — pre-connect:
//   pre-intent      : intent picker overlay open, nothing else.
//   scanning        : discovery sweeping the LAN (or radar UI).
//   found-server    : at least one server visible in the list.
//   pairing         : pair-code modal open (server not trusted yet).
//   connecting      : WebSocket open, waiting for `ready` from server.
// Post-connect:
//   connected       : `ready` received, mode controls visible, tabs unlocked.
//   reconnecting    : connection dropped, exponential backoff retry.
//   disconnected    : transient — toast + auto-route back to scanning.

type ConnState =
  | 'pre-intent'
  | 'scanning'
  | 'found-server'
  | 'pairing'
  | 'connecting'
  | 'connected'
  | 'reconnecting'
  | 'disconnected';

interface StateData {
  state: ConnState;
  serverName?: string;
  transport?: 'wifi' | 'usb';
}

let _state: StateData = { state: 'pre-intent' };
const _subs: Array<(s: StateData) => void> = [];

function setState(next: Partial<StateData>): void {
  const merged: StateData = { ..._state, ...next };
  if (merged.state === _state.state &&
      merged.serverName === _state.serverName &&
      merged.transport === _state.transport) return;
  _state = merged;
  // Run synchronously so DOM updates land in the same paint as the
  // event that caused the transition (no flash of stale UI).
  for (let i = 0; i < _subs.length; i++) {
    try { _subs[i](_state); } catch (e) { console.error('[state] sub', e); }
  }
}

function getState(): StateData { return _state; }

function onState(cb: (s: StateData) => void): void {
  _subs.push(cb);
  // Fire immediately with the current state so subscribers don't
  // need to render twice (once on mount, once on first transition).
  try { cb(_state); } catch (e) { console.error('[state] cb init', e); }
}

// Header chip — translates state name into the plain-English label
// the user sees in the top-right corner. Centralised here so the
// chip wording stays in sync with the state machine.
function chipLabelFor(s: StateData): string {
  switch (s.state) {
    case 'pre-intent':   return 'Not connected';
    case 'scanning':     return 'Looking…';
    case 'found-server': return 'Tap a server';
    case 'pairing':      return 'Pairing…';
    case 'connecting':   return 'Connecting…';
    case 'connected':    return s.serverName ? ('Connected · ' + s.serverName) : 'Connected';
    case 'reconnecting': return 'Reconnecting…';
    case 'disconnected': return 'Disconnected';
  }
}

// Expose to globalThis so the legacy script-tag screens can call into
// the state machine without an import. Once everything is migrated to
// ES modules we'll drop this block.
(globalThis as unknown as {
  viorState: {
    set: typeof setState;
    get: typeof getState;
    on: typeof onState;
    chipLabel: typeof chipLabelFor;
  };
}).viorState = {
  set: setState,
  get: getState,
  on: onState,
  chipLabel: chipLabelFor,
};

// ── Universal UI subscriber ────────────────────────────────────────
// Centralised render of the bits that *every* state transition needs:
//   • Tab bar (Display/Files/Remote) is hidden pre-connect so a new
//     user isn't confronted with controls they can't use yet.
//   • Header chip text + dot tone reflect the current state in plain
//     English ("Looking…" / "Pairing…" / "Connected · MacBook").
//   • Disc-manual-link is suppressed once connected so it can't lead
//     the user back into a half-paired state.
onState(function (s) {
  const tabBar = document.getElementById('tab-bar');
  const isConnected = s.state === 'connected' || s.state === 'reconnecting';
  if (tabBar) tabBar.classList.toggle('hidden-pre-connect', !isConnected);

  // Pre-connect disclosure link on discovery view.
  const manualLink = document.getElementById('disc-manual-link');
  if (manualLink) manualLink.classList.toggle('hidden-pre-connect', isConnected);

  // Persistent transport toggle is visible pre-connect (so the user
  // can swap Wi-Fi ⇄ USB while still entering credentials) and hidden
  // post-connect (transport is locked at that point). We flip a body
  // class so the CSS rule (.post-connect .hidden-post-connect) takes
  // effect — keeps the styling decoupled from the JS.
  document.body.classList.toggle('post-connect', isConnected);

  // Header chip text. Dot tone is managed by setConnState (kept for
  // backwards compat with reconnect banner colour rules).
  const label = document.getElementById('conn-label');
  if (label) label.textContent = chipLabelFor(s);
});
