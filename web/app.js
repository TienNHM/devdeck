'use strict';
// Shared helpers (global: also used by git.js)
const $ = (s, r = document) => r.querySelector(s);
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const store = { get: (k, d) => { try { return localStorage.getItem(k) ?? d; } catch { return d; } }, set: (k, v) => { try { localStorage.setItem(k, v); } catch {} } };

async function api(path, opts) {
  const r = await fetch(path, opts);
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error || r.statusText);
  return j;
}
const post = (p, body) => api(p, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
const put = (p, body) => api(p, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
const mb = n => (n / 1048576).toFixed(1) + ' MB';
const ago = t => { const s = Math.max(0, Date.now() / 1000 - t) | 0; return s < 60 ? s + 's' : s < 3600 ? (s / 60 | 0) + 'm' : (s / 3600 | 0) + 'h'; };

function toast(msg, kind) {
  const t = document.createElement('div'); t.className = 'toast' + (kind === 'err' ? ' err' : ''); t.textContent = msg;
  $('#toasts').appendChild(t); setTimeout(() => t.remove(), kind === 'err' ? 6000 : 2800);
}

// Modal dialog. Resolves null when cancelled, true when confirmed, or the typed string when `input` is set.
function dialog({ title, text = '', input = '', ok = 'OK', danger = false }) {
  return new Promise(res => {
    const d = $('#dlg'), inp = $('#dlg-in'), okb = $('#dlg-ok');
    $('#dlg-t').textContent = title; $('#dlg-p').textContent = text;
    inp.hidden = !input; inp.value = ''; inp.placeholder = input;
    okb.textContent = ok; okb.className = 'btn ' + (danger ? 'danger-solid' : 'primary');
    d.returnValue = '';
    d.onclose = () => res(d.returnValue === 'ok' ? (input ? inp.value : true) : null);
    d.showModal(); (input ? inp : okb).focus();
  });
}

let stopped = false;

// ------------------------------------------------------------------ theme
const root = document.documentElement;
$('#theme').onclick = () => { const t = root.dataset.theme === 'dark' ? 'light' : 'dark'; root.dataset.theme = t; store.set('theme', t); };

// ------------------------------------------------------------------ tabs
let current = 'ports';
function show(tab) {
  current = tab;
  document.querySelectorAll('#tabs button').forEach(b => b.classList.toggle('on', b.dataset.tab === tab));
  document.querySelectorAll('main section').forEach(s => s.classList.toggle('on', s.id === tab));
  if (tab === 'ports') loadPorts(); else if (tab === 'procs') loadProcs(); else if (tab === 'claude') loadClaude();
  else if (tab === 'term') { loadConfig(); fitActive(); }
  document.dispatchEvent(new CustomEvent('devdeck:tab', { detail: tab }));
}
$('#tabs').onclick = e => { const b = e.target.closest('button[data-tab]'); if (b) { show(b.dataset.tab); store.set('tab', b.dataset.tab); } };
setInterval(() => {
  if (document.hidden || stopped) return;
  if (current === 'ports') loadPorts(); else if (current === 'procs') loadProcs(); else if (current === 'claude') loadClaude();
}, 3000);

async function refreshSelf() {
  if (stopped) return;
  try { const s = await api('/api/self'); $('#self').textContent = `${mb(s.rss)} RAM`; } catch {}
}
refreshSelf(); setInterval(refreshSelf, 15000);

async function kill(pid, tree, name) {
  const ok = await dialog({ title: `Kill ${name}?`, text: `PID ${pid}${tree ? ' and its child processes' : ''} will be terminated. Unsaved work in it is lost.`, ok: 'Kill', danger: true });
  if (!ok) return;
  try { await post('/api/kill', { pid, tree }); toast(`Killed ${name} (${pid})`); } catch (e) { toast(e.message, 'err'); }
  current === 'ports' ? loadPorts() : loadProcs();
}

// ------------------------------------------------------------------ ports
let portsData = [];
const isLoopback = a => a === '127.0.0.1' || a === '::1';
async function loadPorts() {
  try { portsData = await api('/api/ports'); renderPorts(); } catch (e) { $('#ports-t').innerHTML = `<tr><td colspan=5>${esc(e.message)}</td></tr>`; }
}
function renderPorts() {
  const q = $('#ports-q').value.toLowerCase();
  const rows = portsData.filter(p => !q || `${p.port} ${p.process} ${p.pid}`.toLowerCase().includes(q));
  $('#ports-count').textContent = `${rows.length} listening`;
  $('#ports-t').innerHTML = rows.length ? rows.map(p => `<tr>
    <td><a class="port" href="http://localhost:${p.port}" target="_blank" rel="noopener" title="Open in browser">${p.port}</a></td>
    <td><span class="mono">${esc(p.addr)}</span> ${isLoopback(p.addr) ? '<span class="pill ok">local</span>' : '<span class="pill warn" title="Reachable from other machines">exposed</span>'}</td>
    <td>${esc(p.process) || '<span class="muted">(access denied)</span>'}</td><td class="pid">${p.pid}</td>
    <td class="act"><button class="btn sm danger" data-pid="${p.pid}" data-name="${esc(p.process || 'process')}">Kill</button></td></tr>`).join('')
    : '<tr><td colspan=5 class="empty">Nothing matches.</td></tr>';
}
$('#ports-q').oninput = renderPorts;
$('#ports-r').onclick = loadPorts;
$('#ports-t').onclick = e => { const b = e.target.closest('button[data-pid]'); if (b) kill(+b.dataset.pid, true, b.dataset.name); };

// ------------------------------------------------------------------ processes
let procsData = [];
async function loadProcs() {
  try { procsData = await api('/api/processes?limit=400'); renderProcs(); } catch (e) { $('#procs-t').innerHTML = `<tr><td colspan=5>${esc(e.message)}</td></tr>`; }
}
function renderProcs() {
  const q = $('#procs-q').value.toLowerCase();
  const rows = procsData.filter(p => !q || `${p.name} ${p.pid}`.toLowerCase().includes(q)).slice(0, 200);
  const max = Math.max(1, ...rows.map(p => p.rss));
  $('#procs-count').textContent = `${rows.length} shown`;
  $('#procs-t').innerHTML = rows.length ? rows.map(p => `<tr><td>${esc(p.name)}</td><td class="pid">${p.pid}</td><td class="pid">${p.ppid}</td>
    <td><div class="memcell"><span>${mb(p.rss)}</span><i style="width:${Math.max(2, p.rss / max * 100)}px"></i></div></td>
    <td class="act"><button class="btn sm danger" data-pid="${p.pid}" data-name="${esc(p.name)}">Kill</button><button class="btn sm" data-pid="${p.pid}" data-name="${esc(p.name)}" data-tree="1">Kill tree</button></td></tr>`).join('')
    : '<tr><td colspan=5 class="empty">Nothing matches.</td></tr>';
}
$('#procs-q').oninput = renderProcs;
$('#procs-r').onclick = loadProcs;
$('#procs-t').onclick = e => { const b = e.target.closest('button[data-pid]'); if (b) kill(+b.dataset.pid, !!b.dataset.tree, b.dataset.name); };

// ------------------------------------------------------------------ redis
let rid = null, rcursor = '0';
const ttlText = t => t === -1 ? 'no expiry' : t === -2 ? 'missing' : t < 90 ? t + 's' : t < 5400 ? (t / 60 | 0) + 'm' : t < 172800 ? (t / 3600 | 0) + 'h' : (t / 86400 | 0) + 'd';
async function rScan(reset) {
  if (reset) { rcursor = '0'; $('#r-keys').innerHTML = ''; }
  try {
    const r = await api(`/api/redis/scan?id=${rid}&cursor=${rcursor}&match=${encodeURIComponent($('#r-match').value || '*')}&count=500`);
    rcursor = r.cursor;
    $('#r-keys').insertAdjacentHTML('beforeend', (r.keys || []).map(k => `<div class="rk" data-k="${esc(k.key)}"><span class="tp ${esc(k.type)}">${esc(k.type)}</span><span class="k mono">${esc(k.key)}</span>${k.ttl >= 0 ? `<span class="ttl">${ttlText(k.ttl)}</span>` : ''}</div>`).join(''));
    if (reset && !(r.keys || []).length && rcursor === '0') $('#r-keys').innerHTML = '<div class="empty">No keys match.</div>';
    $('#r-more').hidden = rcursor === '0';
  } catch (e) { $('#r-keys').textContent = e.message; }
}
$('#r-connect').onclick = async () => {
  $('#r-info').textContent = 'Connecting…';
  try {
    if (rid) post('/api/redis/close', { id: rid });
    const r = await post('/api/redis/connect', { addr: $('#r-addr').value, db: +$('#r-db').value || 0, password: $('#r-pass').value, tls: $('#r-tls').checked });
    rid = r.id; $('#r-info').textContent = `Connected · Redis ${r.version} · read-only`;
    $('#r-body').hidden = false; $('#r-empty').hidden = true; $('#r-pass').value = ''; rScan(true);
  } catch (e) { $('#r-info').textContent = ''; toast(e.message, 'err'); }
};
$('#r-scan').onclick = () => rScan(true);
$('#r-match').onkeydown = e => { if (e.key === 'Enter') rScan(true); };
$('#r-more').onclick = () => rScan(false);
$('#r-keys').onclick = async e => {
  const row = e.target.closest('.rk'); if (!row) return;
  document.querySelectorAll('.rk.on').forEach(x => x.classList.remove('on')); row.classList.add('on');
  try {
    const kv = await api(`/api/redis/key?id=${rid}&key=${encodeURIComponent(row.dataset.k)}`);
    const head = `${kv.key}\ntype=${kv.type}   ttl=${ttlText(kv.ttl)}   size=${kv.size}${kv.truncated ? '   (truncated)' : ''}\n\n`;
    let body = kv.value;
    if (kv.type === 'string') { try { body = JSON.stringify(JSON.parse(kv.value), null, 2); } catch {} }
    else if (kv.type === 'hash') body = (kv.value || []).map(([f, v]) => `${f}\t${v}`).join('\n');
    else if (kv.type === 'zset') body = (kv.value || []).map(([m, s]) => `${s}\t${m}`).join('\n');
    else if (Array.isArray(kv.value)) body = typeof kv.value[0] === 'string' ? kv.value.join('\n') : JSON.stringify(kv.value, null, 2);
    $('#r-val').textContent = head + body;
  } catch (err) { $('#r-val').textContent = err.message; }
};

// ------------------------------------------------------------------ SSH hosts (add / edit / remove)
let cfg = { hosts: [], redis: [], repos: [] };
let editingHost = null;

function renderHosts() {
  $('#t-host').innerHTML = cfg.hosts.map(h => `<option>${esc(h.name)}</option>`).join('') || '<option value="">(no saved hosts)</option>';
  $('#h-list').innerHTML = cfg.hosts.length ? cfg.hosts.map(h => `<tr>
    <td><b>${esc(h.name)}</b></td><td class="mono">${esc(h.user || '')}${h.user ? '@' : ''}${esc(h.host)}:${h.port || 22}</td>
    <td class="muted">${esc(h.keyPath || 'default keys in ~/.ssh')}</td>
    <td class="act"><button class="btn sm" data-edit="${esc(h.name)}"><svg class="i"><use href="#i-edit"/></svg>Edit</button>
    <button class="btn sm danger" data-del="${esc(h.name)}"><svg class="i"><use href="#i-trash"/></svg>Remove</button></td></tr>`).join('')
    : '<tr><td class="muted">No saved hosts yet. Fill the form above and save.</td></tr>';
}
async function loadConfig() {
  try { cfg = await api('/api/config'); } catch {}
  cfg.hosts = cfg.hosts || [];
  renderHosts();
}
// Re-read the config right before writing so changes made elsewhere (e.g. saved repos) are not overwritten.
async function saveHosts(hosts) {
  const fresh = await api('/api/config');
  fresh.hosts = hosts;
  cfg = await put('/api/config', fresh);
  cfg.hosts = cfg.hosts || [];
  renderHosts();
}
function resetHostForm() {
  editingHost = null;
  ['#h-name', '#h-host', '#h-port', '#h-user', '#h-key'].forEach(s => $(s).value = '');
  $('#h-title').textContent = 'SSH hosts'; $('#h-save').textContent = 'Save host'; $('#h-cancel').hidden = true;
}
$('#t-hosts').onclick = () => { const e = $('#hosts-edit'); e.hidden = !e.hidden; if (!e.hidden) loadConfig(); };
$('#h-cancel').onclick = resetHostForm;
$('#h-save').onclick = async () => {
  const name = $('#h-name').value.trim(), host = $('#h-host').value.trim();
  const port = $('#h-port').value.trim() === '' ? 22 : +$('#h-port').value;
  if (!name || !host) return toast('Name and host are required', 'err');
  if (!Number.isInteger(port) || port < 1 || port > 65535) return toast('Port must be 1-65535', 'err');
  if (name !== editingHost && cfg.hosts.some(h => h.name === name)) {
    const ok = await dialog({ title: `Replace "${name}"?`, text: 'A host with this name already exists and will be overwritten.', ok: 'Replace', danger: true });
    if (!ok) return;
  }
  const entry = { name, host, port, user: $('#h-user').value.trim(), keyPath: $('#h-key').value.trim() };
  const hosts = cfg.hosts.filter(h => h.name !== editingHost && h.name !== name).concat(entry);
  try { await saveHosts(hosts); toast(editingHost ? `Updated ${name}` : `Saved ${name}`); resetHostForm(); } catch (e) { toast(e.message, 'err'); }
};
$('#h-list').onclick = async e => {
  const ed = e.target.closest('[data-edit]'), del = e.target.closest('[data-del]');
  if (ed) {
    const h = cfg.hosts.find(x => x.name === ed.dataset.edit); if (!h) return;
    editingHost = h.name;
    $('#h-name').value = h.name; $('#h-host').value = h.host; $('#h-port').value = h.port || 22; $('#h-user').value = h.user || ''; $('#h-key').value = h.keyPath || '';
    $('#h-title').textContent = `Editing "${h.name}"`; $('#h-save').textContent = 'Update host'; $('#h-cancel').hidden = false;
    $('#h-name').focus();
  } else if (del) {
    const n = del.dataset.del;
    const ok = await dialog({ title: `Remove "${n}"?`, text: 'Only the saved entry is removed. The server itself is not touched.', ok: 'Remove', danger: true });
    if (!ok) return;
    try { await saveHosts(cfg.hosts.filter(h => h.name !== n)); if (editingHost === n) resetHostForm(); toast(`Removed ${n}`); } catch (err) { toast(err.message, 'err'); }
  }
};

// ------------------------------------------------------------------ terminals
const terms = []; let activeTerm = null, tcount = 0;
const theme = { background: '#0b0e14', foreground: '#e6e9f0', cursor: '#e6e9f0', selectionBackground: '#33406a' };
function newTerm(title, init) {
  const id = ++tcount, pane = document.createElement('div');
  pane.className = 'tpane'; $('#t-area').appendChild(pane);
  const term = new Terminal({ fontSize: 13, fontFamily: '"Cascadia Code", Consolas, ui-monospace, monospace', cursorBlink: true, theme, scrollback: 3000 });
  const fit = new FitAddon.FitAddon(); term.loadAddon(fit); term.open(pane);
  const t = { id, title, pane, term, fit, ws: null };
  terms.push(t); selectTerm(t);
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  const ws = new WebSocket(`${proto}://${location.host}/ws/term`); ws.binaryType = 'arraybuffer'; t.ws = ws;
  ws.onopen = () => { fit.fit(); ws.send(JSON.stringify({ ...init, cols: term.cols, rows: term.rows })); term.focus(); };
  ws.onmessage = e => term.write(typeof e.data === 'string' ? e.data : new Uint8Array(e.data));
  ws.onclose = () => term.write('\r\n\x1b[90m[disconnected]\x1b[0m\r\n');
  const enc = new TextEncoder();
  term.onData(d => ws.readyState === 1 && ws.send(enc.encode(d)));
  term.onResize(({ cols, rows }) => ws.readyState === 1 && ws.send(JSON.stringify({ resize: { cols, rows } })));
  return t;
}
function selectTerm(t) { activeTerm = t; terms.forEach(x => x.pane.classList.toggle('on', x === t)); renderTermTabs(); requestAnimationFrame(() => { t.fit.fit(); t.term.focus(); }); }
function closeTerm(t) {
  t.ws && t.ws.close(); t.term.dispose(); t.pane.remove();
  terms.splice(terms.indexOf(t), 1);
  if (activeTerm === t) activeTerm = terms[terms.length - 1] || null;
  activeTerm ? selectTerm(activeTerm) : renderTermTabs();
}
function renderTermTabs() {
  const bar = $('#t-tabs'); bar.innerHTML = '';
  terms.forEach(t => {
    const s = document.createElement('span'); s.className = 'ttab' + (t === activeTerm ? ' on' : ''); s.textContent = t.title;
    s.onclick = () => selectTerm(t);
    const x = document.createElement('a'); x.href = '#'; x.textContent = '×'; x.title = 'Close'; x.onclick = e => { e.preventDefault(); e.stopPropagation(); closeTerm(t); };
    s.appendChild(x); bar.appendChild(s);
  });
  $('#t-empty').hidden = terms.length > 0;
}
function fitActive() { if (activeTerm) requestAnimationFrame(() => activeTerm.fit.fit()); }
window.addEventListener('resize', fitActive);

const cwd = () => $('#t-cwd').value.trim();
$('#t-local').onclick = () => newTerm('shell', { kind: 'local', cwd: cwd() });
$('#t-claude').onclick = () => newTerm('claude', { kind: 'local', cwd: cwd(), cmd: 'claude' });
async function sshInit(cmd) {
  const profile = $('#t-host').value;
  if (!profile) { toast('Add a host first (Manage hosts)', 'err'); return null; }
  const pw = await dialog({ title: `Connect to ${profile}`, text: 'Password (leave empty to use your SSH key). It is sent once and never saved.', input: 'Password', ok: 'Connect' });
  if (pw === null) return null;
  return { kind: 'ssh', profile, password: pw, cmd };
}
$('#t-ssh').onclick = async () => { const i = await sshInit(''); if (i) newTerm('ssh · ' + i.profile, i); };
$('#t-sshclaude').onclick = async () => { const i = await sshInit('claude'); if (i) newTerm('ssh claude · ' + i.profile, i); };

// ------------------------------------------------------------------ claude sessions
async function loadClaude() {
  try {
    const s = await api('/api/claude/sessions');
    $('#c-t').innerHTML = s.length ? s.map(x => `<tr><td><span class="badge ${esc(x.state)}">${esc(x.state)}</span></td><td class="mono">${esc(x.cwd)}</td><td>${esc(x.event)}</td>
      <td class="muted">${esc(x.message || x.tool || '')}</td><td class="muted">${ago(x.updated)} ago</td>
      <td class="act"><button class="btn sm" data-cwd="${esc(x.cwd)}">Open terminal here</button></td></tr>`).join('')
      : '<tr><td colspan=6 class="empty">No sessions reported yet. Install the hooks (devdeck hooks-config) and start claude.</td></tr>';
  } catch (e) { $('#c-t').innerHTML = `<tr><td colspan=6>${esc(e.message)}</td></tr>`; }
}
$('#c-t').onclick = e => {
  const b = e.target.closest('button[data-cwd]'); if (!b) return;
  show('term'); newTerm('shell', { kind: 'local', cwd: b.dataset.cwd });
};

// ------------------------------------------------------------------ quit
$('#quit').onclick = async () => {
  const n = terms.length;
  const ok = await dialog({
    title: 'Quit devdeck?',
    text: (n ? `${n} open terminal${n > 1 ? 's' : ''} will be closed. ` : '') + 'The server stops, and Claude hooks are ignored until you start devdeck again.',
    ok: 'Quit', danger: true,
  });
  if (!ok) return;
  stopped = true;
  try { await post('/api/quit', {}); } catch {}
  terms.slice().forEach(closeTerm);
  document.body.innerHTML = '<div class="bye"><div><h2>devdeck stopped</h2><p>Start it again with <code>devdeck.exe --open</code>, then close this tab.</p></div></div>';
};

show(['ports', 'procs', 'git', 'redis', 'term', 'claude'].includes(store.get('tab', 'ports')) ? store.get('tab', 'ports') : 'ports');
