'use strict';
// Terminal tab: local / SSH terminals and saved SSH hosts. Exposes newTerm(title, init) for other modules.

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
const xtermTheme = { background: '#0b0e14', foreground: '#e6e9f0', cursor: '#e6e9f0', selectionBackground: '#33406a' };
function newTerm(title, init) {
  const id = ++tcount, pane = document.createElement('div');
  pane.className = 'tpane'; $('#t-area').appendChild(pane);
  const term = new Terminal({ fontSize: 13, fontFamily: '"Cascadia Code", Consolas, ui-monospace, monospace', cursorBlink: true, theme: xtermTheme, scrollback: 3000 });
  const fit = new FitAddon.FitAddon(); term.loadAddon(fit); term.open(pane);
  const tt = { id, title, pane, term, fit, ws: null };
  terms.push(tt); selectTerm(tt);
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  const ws = new WebSocket(`${proto}://${location.host}/ws/term`); ws.binaryType = 'arraybuffer'; tt.ws = ws;
  ws.onopen = () => { fit.fit(); ws.send(JSON.stringify({ ...init, cols: term.cols, rows: term.rows })); term.focus(); };
  ws.onmessage = e => term.write(typeof e.data === 'string' ? e.data : new Uint8Array(e.data));
  ws.onclose = () => term.write('\r\n\x1b[90m[disconnected]\x1b[0m\r\n');
  const enc = new TextEncoder();
  term.onData(d => ws.readyState === 1 && ws.send(enc.encode(d)));
  term.onResize(({ cols, rows }) => ws.readyState === 1 && ws.send(JSON.stringify({ resize: { cols, rows } })));
  return tt;
}
function selectTerm(tt) { activeTerm = tt; terms.forEach(x => x.pane.classList.toggle('on', x === tt)); renderTermTabs(); requestAnimationFrame(() => { tt.fit.fit(); tt.term.focus(); }); }
function closeTerm(tt) {
  tt.ws && tt.ws.close(); tt.term.dispose(); tt.pane.remove();
  terms.splice(terms.indexOf(tt), 1);
  if (activeTerm === tt) activeTerm = terms[terms.length - 1] || null;
  activeTerm ? selectTerm(activeTerm) : renderTermTabs();
}
function renderTermTabs() {
  const bar = $('#t-tabs'); bar.innerHTML = '';
  terms.forEach(tt => {
    const s = document.createElement('span'); s.className = 'ttab' + (tt === activeTerm ? ' on' : ''); s.textContent = tt.title;
    s.onclick = () => selectTerm(tt);
    const x = document.createElement('a'); x.href = '#'; x.textContent = '×'; x.title = 'Close'; x.onclick = e => { e.preventDefault(); e.stopPropagation(); closeTerm(tt); };
    s.appendChild(x); bar.appendChild(s);
  });
  $('#t-empty').hidden = terms.length > 0;
}
function fitActive() { if (activeTerm) requestAnimationFrame(() => activeTerm.fit.fit()); }
window.addEventListener('resize', fitActive);
onTab('term', () => { loadConfig(); fitActive(); });
termCount = () => terms.length;
beforeQuit.push(() => terms.slice().forEach(closeTerm));

const cwd = () => $('#t-cwd').value.trim();
$('#t-local').onclick = () => newTerm('shell', { kind: 'local', cwd: cwd() });
$('#t-claude').onclick = () => newTerm('claude', { kind: 'local', cwd: cwd(), cmd: 'claude' });
async function sshInit(cmd, profile = $('#t-host').value) {
  if (!profile) { toast('Add a host first (Manage hosts)', 'err'); return null; }
  const pw = await dialog({ title: `Connect to ${profile}`, text: 'Password (leave empty to use your SSH key). It is sent once and never saved.', input: 'Password', ok: 'Connect' });
  if (pw === null) return null;
  return { kind: 'ssh', profile, password: pw, cmd };
}
$('#t-ssh').onclick = async () => { const i = await sshInit(''); if (i) newTerm('ssh · ' + i.profile, i); };
$('#t-sshclaude').onclick = async () => { const i = await sshInit('claude'); if (i) newTerm('ssh claude · ' + i.profile, i); };

registerCommand({ title: 'New local shell', keywords: 'terminal powershell', run: () => { show('term'); $('#t-local').click(); } });
registerCommand({ title: 'New Claude terminal', keywords: 'claude code ai', run: () => { show('term'); $('#t-claude').click(); } });
registerCommandProvider(async q => {
  if (!q.startsWith('ssh')) return [];
  if (!cfg.hosts.length) await loadConfig();
  const f = q.slice(3).trim();
  return cfg.hosts.filter(h => !f || h.name.toLowerCase().includes(f)).map(h => ({
    title: `SSH to ${h.name}`, hint: `${h.user || ''}@${h.host}`, run: async () => { show('term'); const i = await sshInit('', h.name); if (i) newTerm('ssh · ' + h.name, i); },
  }));
});
