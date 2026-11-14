'use strict';
// Terminal tab: local shells and SSH sessions in xterm.js tabs. Sessions live on the server, so a
// page reload re-attaches to them (recent output is replayed). Closing a tab ends the session.
// Globals: newTerm(title, init) (init = {kind:'local', cwd, cmd, shell} | {kind:'ssh', profile, cmd})
// and openSSHTerm(host, cmd). Saved hosts are managed in the SSH tab (ssh.js).

registerStrings('vi', {
  'term.shell': 'Shell',
  'term.newShell': 'Shell',
  'term.cwd': 'Thư mục làm việc (không bắt buộc)',
  'term.host': 'Máy chủ SSH',
  'term.sshClaude': 'SSH + Claude',
  'term.manageHosts': 'Quản lý máy chủ',
  'term.empty': 'Chưa mở terminal nào. Hãy mở một shell hoặc chạy Claude trong một thư mục. Terminal vẫn chạy tiếp khi bạn tải lại trang.',
  'term.noHosts': '(chưa có máy chủ)',
  'term.saved': 'Đã lưu',
  'term.config': '~/.ssh/config',
  'term.addHost': 'Hãy thêm máy chủ trước (tab SSH)',
  'term.connecting': 'Đang kết nối…',
  'term.exited': '[tiến trình đã kết thúc]',
  'term.ended': '[phiên đã kết thúc]',
  'term.reconnecting': '[mất kết nối, đang kết nối lại…]',
  'term.disconnected': '[đã ngắt kết nối]',
  'term.authTitle': 'Mật khẩu cho {host}',
  'term.authText': 'Không đăng nhập được bằng khóa hoặc ssh-agent. Nhập mật khẩu (chỉ gửi một lần, không bao giờ được lưu).',
  'term.password': 'Mật khẩu',
  'term.connect': 'Kết nối',
  'term.close': 'Đóng',
  'term.closeTitle': 'Đóng "{name}"?',
  'term.closeText': 'Phiên terminal sẽ kết thúc và mọi chương trình đang chạy trong đó sẽ bị dừng.',
  'term.cmdShell': 'Mở shell mới',
  'term.cmdShellOf': 'Mở terminal {name} mới',
  'term.cmdClaude': 'Mở terminal Claude mới',
  'term.cmdSSH': 'SSH tới {host}',
  'term.cmdSwitch': 'Chuyển tới terminal {name}',
  'term.terminal': 'Terminal',
});

let termHosts = [];
let termShells = [];
const termTabs = []; let termActive = null, termSeq = 0;
const termTheme = { background: '#0b0e14', foreground: '#e6e9f0', cursor: '#e6e9f0', selectionBackground: '#33406a' };
const termEnc = new TextEncoder();
const termFmt = (s, v) => s.replace(/\{(\w+)\}/g, (_, k) => v[k] ?? '');

// ------------------------------------------------------------------ pickers
async function termLoadHosts() {
  try { termHosts = (await api('/api/ssh/hosts')).hosts || []; } catch { termHosts = []; }
  const sel = $('#t-host'), prev = sel.value || store.get('term.host', '');
  const saved = termHosts.filter(h => h.source === 'saved'), conf = termHosts.filter(h => h.source === 'config' && !h.shadowed);
  const opt = h => `<option value="${esc(h.name)}">${esc(h.name)}</option>`;
  sel.innerHTML = !termHosts.length ? `<option value="">${esc(t('term.noHosts', '(no saved hosts)'))}</option>`
    : (saved.length ? `<optgroup label="${esc(t('term.saved', 'Saved'))}">${saved.map(opt).join('')}</optgroup>` : '')
    + (conf.length ? `<optgroup label="${esc(t('term.config', '~/.ssh/config'))}">${conf.map(opt).join('')}</optgroup>` : '');
  if ([...sel.options].some(o => o.value === prev)) sel.value = prev;
}
async function termLoadShells() {
  try { termShells = await api('/api/term/shells'); } catch { termShells = []; }
  const sel = $('#t-shell'), want = store.get('term.shell', '');
  sel.innerHTML = termShells.map(s => `<option value="${esc(s.id)}">${esc(s.name)}</option>`).join('');
  if (termShells.some(s => s.id === want)) sel.value = want;
  sel.hidden = termShells.length < 2;
  termShells.forEach(s => registerCommand({
    title: termFmt(t('term.cmdShellOf', 'New {name} terminal'), { name: s.name }), keywords: `terminal shell ${s.id}`,
    run: () => { show('term'); newTerm(s.name, { kind: 'local', cwd: $('#t-cwd').value.trim(), shell: s.id }); },
  }));
}
$('#t-shell').onchange = e => store.set('term.shell', e.target.value);
$('#t-host').onchange = e => store.set('term.host', e.target.value);
document.addEventListener('devdeck:hosts', termLoadHosts);

// ------------------------------------------------------------------ terminals
function termMakeTab(title) {
  const pane = document.createElement('div');
  pane.className = 'tpane'; $('#t-area').appendChild(pane);
  const term = new Terminal({ fontSize: 13, fontFamily: '"Cascadia Code", Consolas, ui-monospace, monospace', cursorBlink: true, theme: termTheme, scrollback: 5000 });
  const fit = new FitAddon.FitAddon(); term.loadAddon(fit); term.open(pane);
  const tt = { id: ++termSeq, title, pane, term, fit, ws: null, sid: null, init: null, exited: false, closing: false, retries: 0 };
  term.onData(d => tt.ws && tt.ws.readyState === 1 && tt.ws.send(termEnc.encode(d)));
  term.onResize(({ cols, rows }) => tt.ready && tt.ws.readyState === 1 && tt.ws.send(JSON.stringify({ resize: { cols, rows } })));
  termTabs.push(tt);
  return tt;
}
const termVisible = tt => current === 'term' && tt.pane.classList.contains('on') && tt.pane.offsetWidth > 0;
const termNote = (tt, s) => tt.term.write(`\r\n\x1b[90m${s}\x1b[0m\r\n`);

// termConnect opens a socket and either starts a session (msg = init) or attaches (msg = {attach}).
function termConnect(tt, msg) {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  const ws = new WebSocket(`${proto}://${location.host}/ws/term`); ws.binaryType = 'arraybuffer'; tt.ws = ws; tt.ready = false;
  let opened = false;
  ws.onopen = () => {
    opened = true;
    const size = termVisible(tt) ? (tt.fit.fit(), { cols: tt.term.cols, rows: tt.term.rows }) : {};
    ws.send(JSON.stringify({ ...msg, ...size })); // the first frame must be the init / attach message
    tt.ready = true;
  };
  ws.onmessage = async e => {
    if (typeof e.data !== 'string') { tt.term.write(new Uint8Array(e.data)); return; }
    let m; try { m = JSON.parse(e.data); } catch { return; }
    if (m.id) {
      tt.sid = m.id; tt.retries = 0;
      if (m.title && !tt.title) tt.title = m.title;
      if ('replay' in m) tt.term.reset(); // an attach replays the whole buffer
      termRenderTabs();
    }
    if (m.exit) { tt.exited = true; termNote(tt, t('term.exited', '[process exited]')); termRenderTabs(); }
    if (m.error) {
      tt.closing = true; // this socket is done either way
      if (m.auth && tt.init && tt.init.kind === 'ssh' && !tt.init.password) {
        const host = tt.init.profile || tt.init.host;
        termNote(tt, m.error);
        const pw = await dialog({ title: termFmt(t('term.authTitle', 'Password for {host}'), { host }), text: t('term.authText', 'Key and ssh-agent login failed. Enter the password (sent once, never saved).'), input: t('term.password', 'Password'), ok: t('term.connect', 'Connect') });
        if (pw) { tt.closing = false; tt.init = { ...tt.init, password: pw }; termNote(tt, t('term.connecting', 'Connecting…')); termConnect(tt, tt.init); tt.init = { ...tt.init, password: '' }; return; }
      }
      tt.exited = true;
      tt.term.write(`\r\n\x1b[31m${m.error}\x1b[0m\r\n`);
      if (m.gone) termNote(tt, t('term.ended', '[session ended]'));
      termRenderTabs();
    }
  };
  ws.onclose = () => {
    if (tt.ws !== ws || tt.closing || tt.exited || stopped) return;
    if (tt.sid && opened && tt.retries < 5) { // server still has the session: re-attach
      tt.retries++;
      termNote(tt, t('term.reconnecting', '[connection lost, reconnecting…]'));
      setTimeout(() => { if (!tt.closing && !tt.exited && termTabs.includes(tt)) termConnect(tt, { attach: tt.sid }); }, 500 * 2 ** tt.retries);
    } else { tt.exited = true; termNote(tt, t('term.disconnected', '[disconnected]')); termRenderTabs(); }
  };
}

function newTerm(title, init) {
  const tt = termMakeTab(title);
  tt.init = { ...init, title };
  termSelect(tt);
  if (init && init.kind === 'ssh') tt.term.write(`\x1b[90m${t('term.connecting', 'Connecting…')}\x1b[0m\r\n`);
  termConnect(tt, tt.init);
  if (tt.init.password) tt.init = { ...tt.init, password: '' }; // keep no secret around
  return tt;
}
function openSSHTerm(host, cmd = '') {
  if (!host) { toast(t('term.addHost', 'Add a host first (SSH tab)'), 'err'); return null; }
  show('term');
  return newTerm((cmd ? 'ssh claude · ' : 'ssh · ') + host, { kind: 'ssh', profile: host, cmd });
}
function termAttach(s) {
  const tt = termMakeTab(s.title);
  tt.sid = s.id; tt.init = { kind: s.kind };
  if (s.cols > 0 && s.rows > 0) tt.term.resize(s.cols, s.rows); // replay at the size the output was drawn for
  termConnect(tt, { attach: s.id });
  return tt;
}

function termSelect(tt) {
  termActive = tt;
  termTabs.forEach(x => x.pane.classList.toggle('on', x === tt));
  termRenderTabs();
  requestAnimationFrame(() => { if (termVisible(tt)) { tt.fit.fit(); tt.term.focus(); } });
}
function termDispose(tt) {
  tt.closing = true;
  try { tt.ws && tt.ws.close(); } catch {}
  tt.term.dispose(); tt.pane.remove();
  termTabs.splice(termTabs.indexOf(tt), 1);
  if (termActive === tt) termActive = termTabs[termTabs.length - 1] || null;
  termActive ? termSelect(termActive) : termRenderTabs();
}
// termKill ends the server session (explicit close), then removes the tab.
function termKill(tt) {
  if (tt.sid && !tt.exited) {
    if (tt.ws && tt.ws.readyState === 1) tt.ws.send(JSON.stringify({ close: true }));
    else fetch('/api/term/sessions/' + encodeURIComponent(tt.sid), { method: 'DELETE' }).catch(() => {});
  }
  termDispose(tt);
}
async function termClose(tt) {
  if (tt.sid && !tt.exited) {
    const ok = await dialog({ title: termFmt(t('term.closeTitle', 'Close "{name}"?'), { name: tt.title }), text: t('term.closeText', 'The terminal session ends and anything running in it is stopped.'), ok: t('term.close', 'Close'), danger: true });
    if (!ok) return;
  }
  termKill(tt);
}
function termRenderTabs() {
  const bar = $('#t-tabs'); bar.innerHTML = '';
  termTabs.forEach(tt => {
    const s = document.createElement('span'); s.className = 'ttab' + (tt === termActive ? ' on' : '') + (tt.exited ? ' term-dead' : '');
    const label = document.createElement('span'); label.textContent = tt.title; s.appendChild(label);
    s.title = tt.title; s.onclick = () => termSelect(tt);
    const x = document.createElement('a'); x.href = '#'; x.textContent = '×'; x.title = t('term.close', 'Close');
    x.onclick = e => { e.preventDefault(); e.stopPropagation(); termClose(tt); };
    s.appendChild(x); bar.appendChild(s);
  });
  $('#t-empty').hidden = termTabs.length > 0;
}
function termFitActive() { if (termActive) requestAnimationFrame(() => { if (termVisible(termActive)) termActive.fit.fit(); }); }
window.addEventListener('resize', termFitActive);
onTab('term', () => { termLoadHosts(); termFitActive(); if (termActive) requestAnimationFrame(() => termActive.term.focus()); });
termCount = () => termTabs.filter(x => !x.exited).length;
beforeQuit.push(() => termTabs.slice().forEach(termKill));

// Re-attach to sessions that are still running on the server (page reload, second window).
async function termRestore() {
  let list = [];
  try { list = await api('/api/term/sessions'); } catch { return; }
  list.forEach(s => { if (!termTabs.some(x => x.sid === s.id)) termAttach(s); });
  if (termTabs.length && !termActive) termSelect(termTabs[termTabs.length - 1]); else termRenderTabs();
}

// ------------------------------------------------------------------ toolbar + palette
const termCwd = () => $('#t-cwd').value.trim();
const termShellName = () => (termShells.find(s => s.id === $('#t-shell').value) || {}).name || 'shell';
$('#t-local').onclick = () => newTerm(termShellName(), { kind: 'local', cwd: termCwd(), shell: $('#t-shell').value });
$('#t-claude').onclick = () => newTerm('claude', { kind: 'local', cwd: termCwd(), cmd: 'claude', shell: $('#t-shell').value });
$('#t-ssh').onclick = () => openSSHTerm($('#t-host').value);
$('#t-sshclaude').onclick = () => openSSHTerm($('#t-host').value, 'claude');
$('#t-hosts').onclick = () => { show('ssh'); document.dispatchEvent(new CustomEvent('devdeck:ssh-sub', { detail: 'hosts' })); };

registerCommand({ title: t('term.cmdShell', 'New local shell'), keywords: 'terminal powershell bash zsh', run: () => { show('term'); $('#t-local').click(); } });
registerCommand({ title: t('term.cmdClaude', 'New Claude terminal'), keywords: 'claude code ai', run: () => { show('term'); $('#t-claude').click(); } });
registerCommandProvider(async q => {
  const out = [];
  if (q.startsWith('ssh')) {
    if (!termHosts.length) await termLoadHosts();
    const f = q.slice(3).trim();
    termHosts.filter(h => !h.shadowed && (!f || h.name.toLowerCase().includes(f))).forEach(h => out.push({
      title: termFmt(t('term.cmdSSH', 'SSH to {host}'), { host: h.name }), hint: `${h.user ? h.user + '@' : ''}${h.host}`, run: () => openSSHTerm(h.name),
    }));
  }
  if (q.startsWith('term') || q.startsWith('tab')) {
    termTabs.forEach(tt => out.push({ title: termFmt(t('term.cmdSwitch', 'Switch to terminal {name}'), { name: tt.title }), hint: t('term.terminal', 'Terminal'), run: () => { show('term'); termSelect(tt); } }));
  }
  return out;
});

termLoadShells();
termLoadHosts();
termRestore();
