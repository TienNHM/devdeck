'use strict';
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
const mb = n => (n / 1048576).toFixed(1) + ' MB';
const ago = t => { const s = Math.max(0, Date.now() / 1000 - t) | 0; return s < 60 ? s + 's' : s < 3600 ? (s / 60 | 0) + 'm' : (s / 3600 | 0) + 'h'; };

// ------------------------------------------------------------------ tabs
let current = 'ports';
const timers = {};
function show(tab) {
  current = tab;
  document.querySelectorAll('#tabs button').forEach(b => b.classList.toggle('on', b.dataset.tab === tab));
  document.querySelectorAll('main section').forEach(s => s.classList.toggle('on', s.id === tab));
  if (tab === 'ports') loadPorts(); else if (tab === 'procs') loadProcs(); else if (tab === 'claude') loadClaude();
  else if (tab === 'term') { loadConfig(); fitActive(); }
}
$('#tabs').onclick = e => { if (e.target.dataset.tab) show(e.target.dataset.tab); };
setInterval(() => {
  if (document.hidden) return;
  if (current === 'ports') loadPorts(); else if (current === 'procs') loadProcs(); else if (current === 'claude') loadClaude();
}, 3000);

async function self() {
  try { const s = await api('/api/self'); $('#self').textContent = `devdeck ${s.version} · ${mb(s.rss)} RAM`; } catch {}
}
self(); setInterval(self, 15000);

async function kill(pid, tree, name) {
  if (!confirm(`Kill ${name} (PID ${pid})${tree ? ' and its children' : ''}?`)) return;
  try { await post('/api/kill', { pid, tree }); } catch (e) { alert(e.message); }
  current === 'ports' ? loadPorts() : loadProcs();
}

// ------------------------------------------------------------------ ports
let portsData = [];
async function loadPorts() {
  try { portsData = await api('/api/ports'); renderPorts(); } catch (e) { $('#ports-t').innerHTML = `<tr><td colspan=5>${esc(e.message)}</td></tr>`; }
}
function renderPorts() {
  const q = $('#ports-q').value.toLowerCase();
  $('#ports-t').innerHTML = portsData.filter(p => !q || `${p.port} ${p.process} ${p.pid}`.toLowerCase().includes(q)).map(p =>
    `<tr><td><a href="http://localhost:${p.port}" target="_blank" rel="noopener">${p.port}</a></td><td>${esc(p.addr)}</td><td>${p.pid}</td><td>${esc(p.process)}</td>
     <td><button class="danger" data-pid="${p.pid}" data-name="${esc(p.process)}">Kill</button></td></tr>`).join('');
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
  $('#procs-t').innerHTML = procsData.filter(p => !q || `${p.name} ${p.pid}`.toLowerCase().includes(q)).slice(0, 200).map(p =>
    `<tr><td>${p.pid}</td><td>${p.ppid}</td><td>${esc(p.name)}</td><td class="r">${mb(p.rss)}</td>
     <td><button class="danger" data-pid="${p.pid}" data-name="${esc(p.name)}">Kill</button> <button data-pid="${p.pid}" data-name="${esc(p.name)}" data-tree="1">Kill tree</button></td></tr>`).join('');
}
$('#procs-q').oninput = renderProcs;
$('#procs-r').onclick = loadProcs;
$('#procs-t').onclick = e => { const b = e.target.closest('button[data-pid]'); if (b) kill(+b.dataset.pid, !!b.dataset.tree, b.dataset.name); };

// ------------------------------------------------------------------ git graph
const COLORS = ['#0969da', '#1a7f37', '#bf8700', '#cf222e', '#8250df', '#bc4c00', '#0a7a8a', '#bf3989'];
const RH = 24, LW = 14;
let gitLimit = 200;
$('#git-repo').value = store.get('repo', '');
async function loadGit(more) {
  const repo = $('#git-repo').value.trim();
  if (!repo) return;
  store.set('repo', repo);
  gitLimit = more ? gitLimit + 300 : 200;
  $('#git-info').textContent = 'loading…';
  try {
    const g = await api(`/api/git/graph?repo=${encodeURIComponent(repo)}&limit=${gitLimit}`);
    $('#git-info').textContent = `${g.branch}${g.dirty ? ' · uncommitted changes' : ''} · ${g.rows.length} commits`;
    drawGraph(g, repo);
  } catch (e) { $('#git-info').textContent = ''; $('#git-graph').textContent = e.message; }
}
function drawGraph(g, repo) {
  const w = Math.max(1, g.lanes) * LW + 8, x = l => 6 + l * LW + LW / 2 - 6, col = l => COLORS[l % COLORS.length];
  let svg = `<svg width="${w}" height="${g.rows.length * RH}" style="position:absolute;left:0;top:0">`;
  g.rows.forEach((r, i) => {
    const y0 = i * RH, ym = y0 + RH / 2, y1 = y0 + RH;
    const curve = (xa, ya, xb, yb, c) => `<path d="M${xa} ${ya} C${xa} ${(ya + yb) / 2} ${xb} ${(ya + yb) / 2} ${xb} ${yb}" stroke="${c}" fill="none" stroke-width="2"/>`;
    (r.pass || []).forEach(l => svg += `<line x1="${x(l)}" y1="${y0}" x2="${x(l)}" y2="${y1}" stroke="${col(l)}" stroke-width="2"/>`);
    (r.conv || []).forEach(l => svg += curve(x(l), y0, x(r.col), ym, col(l)));
    (r.out || []).forEach(l => svg += curve(x(r.col), ym, x(l), y1, col(l)));
    svg += `<circle cx="${x(r.col)}" cy="${ym}" r="4.5" fill="${col(r.col)}" stroke="var(--bg)" stroke-width="1.5"/>`;
  });
  svg += '</svg>';
  const rows = g.rows.map(r => {
    const refs = (r.r || []).map(n => {
      const head = n.startsWith('HEAD'), tag = n.startsWith('tag:');
      return `<span class="ref ${head ? 'head' : tag ? 'tag' : ''}">${esc(n.replace('tag: ', ''))}</span>`;
    }).join('');
    return `<div class="grow" data-h="${r.h}" style="padding-left:${w}px">${refs}<span class="m">${esc(r.s)}</span><span class="hash">${r.h.slice(0, 7)}</span><span class="muted">${esc(r.a)} · ${new Date(r.t * 1000).toLocaleDateString()}</span></div>`;
  }).join('');
  const el = $('#git-graph');
  el.innerHTML = svg + rows + (g.truncated ? '<div style="padding:8px"><button id="git-more">Load more</button></div>' : '');
  el.dataset.repo = repo;
}
$('#git-go').onclick = () => loadGit(false);
$('#git-repo').onkeydown = e => { if (e.key === 'Enter') loadGit(false); };
$('#git-graph').onclick = async e => {
  if (e.target.id === 'git-more') return loadGit(true);
  const row = e.target.closest('.grow'); if (!row) return;
  const box = $('#git-side'), d = $('#git-detail'); box.hidden = false; d.textContent = 'loading…';
  const repo = $('#git-graph').dataset.repo, h = row.dataset.h;
  const q = extra => `/api/git/commit?repo=${encodeURIComponent(repo)}&hash=${h}${extra}`;
  const load = async patch => { try { d.textContent = (await api(q(patch ? '&patch=1' : ''))).text; } catch (err) { d.textContent = err.message; } };
  $('#git-patch').onclick = () => load(true);
  load(false);
};

// ------------------------------------------------------------------ redis
let rid = null, rcursor = '0';
async function rScan(reset) {
  if (reset) { rcursor = '0'; $('#r-keys').innerHTML = ''; }
  try {
    const r = await api(`/api/redis/scan?id=${rid}&cursor=${rcursor}&match=${encodeURIComponent($('#r-match').value || '*')}&count=500`);
    rcursor = r.cursor;
    $('#r-keys').insertAdjacentHTML('beforeend', (r.keys || []).map(k => `<div class="rk" data-k="${esc(k.key)}"><span>${esc(k.key)}</span><span class="t">${k.type}${k.ttl >= 0 ? ' · ' + k.ttl + 's' : ''}</span></div>`).join(''));
    $('#r-more').hidden = rcursor === '0';
  } catch (e) { $('#r-keys').textContent = e.message; }
}
$('#r-connect').onclick = async () => {
  $('#r-info').textContent = 'connecting…';
  try {
    if (rid) post('/api/redis/close', { id: rid });
    const r = await post('/api/redis/connect', { addr: $('#r-addr').value, db: +$('#r-db').value || 0, password: $('#r-pass').value, tls: $('#r-tls').checked });
    rid = r.id; $('#r-info').textContent = `connected · Redis ${r.version} · read-only`;
    $('#r-body').hidden = false; $('#r-pass').value = ''; rScan(true);
  } catch (e) { $('#r-info').textContent = e.message; }
};
$('#r-scan').onclick = () => rScan(true);
$('#r-match').onkeydown = e => { if (e.key === 'Enter') rScan(true); };
$('#r-more').onclick = () => rScan(false);
$('#r-keys').onclick = async e => {
  const row = e.target.closest('.rk'); if (!row) return;
  document.querySelectorAll('.rk.on').forEach(x => x.classList.remove('on')); row.classList.add('on');
  try {
    const kv = await api(`/api/redis/key?id=${rid}&key=${encodeURIComponent(row.dataset.k)}`);
    const head = `${kv.key}\ntype=${kv.type}  ttl=${kv.ttl}  size=${kv.size}${kv.truncated ? '  (truncated)' : ''}\n\n`;
    let body = kv.value;
    if (kv.type === 'string') { try { body = JSON.stringify(JSON.parse(kv.value), null, 2); } catch {} }
    else if (kv.type === 'hash') body = (kv.value || []).map(([f, v]) => `${f}\t${v}`).join('\n');
    else if (kv.type === 'zset') body = (kv.value || []).map(([m, s]) => `${s}\t${m}`).join('\n');
    else if (Array.isArray(kv.value)) body = typeof kv.value[0] === 'string' ? kv.value.join('\n') : JSON.stringify(kv.value, null, 2);
    $('#r-val').textContent = head + body;
  } catch (err) { $('#r-val').textContent = err.message; }
};

// ------------------------------------------------------------------ terminals
let cfg = { hosts: [], redis: [], repos: [] };
async function loadConfig() {
  try { cfg = await api('/api/config'); } catch {}
  cfg.hosts = cfg.hosts || [];
  $('#t-host').innerHTML = cfg.hosts.map(h => `<option>${esc(h.name)}</option>`).join('') || '<option value="">(no saved hosts)</option>';
  $('#h-list').innerHTML = cfg.hosts.map(h => `${esc(h.name)} — ${esc(h.user)}@${esc(h.host)}:${h.port || 22} <a href="#" data-del="${esc(h.name)}">remove</a>`).join('<br>');
}
$('#t-hosts').onclick = () => { const e = $('#hosts-edit'); e.hidden = !e.hidden; };
$('#h-save').onclick = async () => {
  const name = $('#h-name').value.trim(), host = $('#h-host').value.trim();
  if (!name || !host) return;
  cfg.hosts = cfg.hosts.filter(h => h.name !== name).concat({ name, host, port: +$('#h-port').value || 22, user: $('#h-user').value.trim(), keyPath: $('#h-key').value.trim() });
  await api('/api/config', { method: 'PUT', body: JSON.stringify(cfg) }); loadConfig();
};
$('#h-list').onclick = async e => {
  const n = e.target.dataset.del; if (!n) return; e.preventDefault();
  cfg.hosts = cfg.hosts.filter(h => h.name !== n);
  await api('/api/config', { method: 'PUT', body: JSON.stringify(cfg) }); loadConfig();
};

const terms = []; let activeTerm = null, tcount = 0;
const theme = { background: '#0d1117', foreground: '#e6edf3', cursor: '#e6edf3' };
function newTerm(title, init) {
  const id = ++tcount, pane = document.createElement('div');
  pane.className = 'tpane'; $('#t-area').appendChild(pane);
  const term = new Terminal({ fontSize: 13, fontFamily: 'Consolas, ui-monospace, monospace', cursorBlink: true, theme, scrollback: 3000 });
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
    const x = document.createElement('a'); x.href = '#'; x.textContent = '×'; x.onclick = e => { e.preventDefault(); e.stopPropagation(); closeTerm(t); };
    s.appendChild(x); bar.appendChild(s);
  });
}
function fitActive() { if (activeTerm) requestAnimationFrame(() => activeTerm.fit.fit()); }
window.addEventListener('resize', fitActive);

const cwd = () => $('#t-cwd').value.trim();
$('#t-local').onclick = () => newTerm('local', { kind: 'local', cwd: cwd() });
$('#t-claude').onclick = () => newTerm('claude', { kind: 'local', cwd: cwd(), cmd: 'claude' });
function sshInit(cmd) {
  const profile = $('#t-host').value; if (!profile) { alert('Add a host first (Hosts…)'); return null; }
  const pw = prompt(`Password for ${profile}? (leave empty to use your SSH key)`, '') || '';
  return { kind: 'ssh', profile, password: pw, cmd };
}
$('#t-ssh').onclick = () => { const i = sshInit(''); if (i) newTerm('ssh:' + i.profile, i); };
$('#t-sshclaude').onclick = () => { const i = sshInit('claude'); if (i) newTerm('ssh claude:' + i.profile, i); };

// ------------------------------------------------------------------ claude sessions
async function loadClaude() {
  try {
    const s = await api('/api/claude/sessions');
    $('#c-t').innerHTML = s.length ? s.map(x => `<tr><td><span class="badge ${x.state}">${x.state}</span></td><td>${esc(x.cwd)}</td><td>${esc(x.event)}</td>
      <td>${esc(x.message || x.tool || '')}</td><td>${ago(x.updated)}</td><td><button data-cwd="${esc(x.cwd)}">Open terminal here</button></td></tr>`).join('')
      : '<tr><td colspan=6 class="muted">No sessions reported yet. Install the hooks (devdeck hooks-config) and start claude.</td></tr>';
  } catch (e) { $('#c-t').innerHTML = `<tr><td colspan=6>${esc(e.message)}</td></tr>`; }
}
$('#c-t').onclick = e => {
  const b = e.target.closest('button[data-cwd]'); if (!b) return;
  show('term'); newTerm('local', { kind: 'local', cwd: b.dataset.cwd });
};

show('ports');
