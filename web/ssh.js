'use strict';
// SSH tab: saved hosts (+ read-only ~/.ssh/config hosts), SSH tunnels and an SFTP file browser.
// Renders into <section id="ssh">. Uses openSSHTerm() from term.js; tells it about host changes
// through the 'devdeck:hosts' event.

registerStrings('vi', {
  'ssh.hosts': 'Máy chủ', 'ssh.tunnels': 'Đường hầm', 'ssh.files': 'Tệp',
  'ssh.savedHosts': 'Máy chủ đã lưu',
  'ssh.hostsHint': 'Mật khẩu không bao giờ được lưu. Nếu không có đường dẫn khóa, devdeck thử ssh-agent và các khóa trong ~/.ssh.',
  'ssh.name': 'Tên', 'ssh.address': 'Địa chỉ', 'ssh.port': 'Cổng', 'ssh.user': 'Người dùng',
  'ssh.keyPath': 'Đường dẫn khóa riêng', 'ssh.jump': 'Máy chủ trung gian (ProxyJump)', 'ssh.jumpPh': 'bastion, user@10.0.0.1:2222',
  'ssh.saveHost': 'Lưu máy chủ', 'ssh.updateHost': 'Cập nhật máy chủ', 'ssh.cancelEdit': 'Hủy sửa', 'ssh.editing': 'Đang sửa "{name}"',
  'ssh.configHosts': 'Từ ~/.ssh/config', 'ssh.configHint': 'Chỉ đọc. Sửa tệp này để thay đổi.',
  'ssh.noSaved': 'Chưa có máy chủ nào được lưu. Điền biểu mẫu ở trên rồi lưu.',
  'ssh.noConfig': 'Không tìm thấy máy chủ nào trong ~/.ssh/config.',
  'ssh.overridden': 'bị máy chủ đã lưu cùng tên ghi đè',
  'ssh.defaultKeys': 'ssh-agent / khóa mặc định',
  'ssh.terminal': 'Terminal', 'ssh.browse': 'Tệp', 'ssh.edit': 'Sửa', 'ssh.remove': 'Xóa', 'ssh.copy': 'Lưu bản sao',
  'ssh.nameHostRequired': 'Cần nhập tên và địa chỉ', 'ssh.badPort': 'Cổng phải từ 1 đến 65535',
  'ssh.replaceTitle': 'Thay thế "{name}"?', 'ssh.replaceText': 'Đã có máy chủ trùng tên, máy chủ đó sẽ bị ghi đè.', 'ssh.replace': 'Thay thế',
  'ssh.saved': 'Đã lưu {name}', 'ssh.updated': 'Đã cập nhật {name}', 'ssh.removed': 'Đã xóa {name}',
  'ssh.removeTitle': 'Xóa "{name}"?', 'ssh.removeText': 'Chỉ xóa mục đã lưu. Máy chủ thật không bị ảnh hưởng.',
  'ssh.agentOk': 'ssh-agent: {n} khóa', 'ssh.agentOff': 'không có ssh-agent',
  'ssh.addTunnel': 'Thêm đường hầm', 'ssh.newTunnel': 'Đường hầm mới', 'ssh.editTunnel': 'Sửa "{name}"',
  'ssh.tunnelHint': 'Chuyển tiếp cổng cục bộ chỉ lắng nghe trên 127.0.0.1. Đường hầm không tự khởi động.',
  'ssh.tunnelNamePh': 'postgres trên prod', 'ssh.via': 'Qua máy chủ', 'ssh.viaShort': 'qua', 'ssh.localPort': 'Cổng cục bộ', 'ssh.remotePort': 'Cổng từ xa',
  'ssh.remoteHost': 'Máy đích (nhìn từ máy chủ SSH)', 'ssh.autoReconnect': 'Tự kết nối lại khi kết nối SSH bị ngắt',
  'ssh.saveTunnel': 'Lưu đường hầm', 'ssh.local': 'Cục bộ', 'ssh.remote': 'Từ xa', 'ssh.status': 'Trạng thái',
  'ssh.conns': 'Kết nối', 'ssh.traffic': 'Lưu lượng',
  'ssh.noTunnels': 'Chưa có đường hầm nào. Ví dụ: 127.0.0.1:15432 → cơ sở dữ liệu trên máy chủ.',
  'ssh.start': 'Bật', 'ssh.stop': 'Tắt', 'ssh.started': 'Đã bật {name}', 'ssh.stopped': 'Đã tắt {name}',
  'ssh.st.running': 'đang chạy', 'ssh.st.idle': 'chờ kết nối lại', 'ssh.st.connecting': 'đang kết nối', 'ssh.st.error': 'lỗi', 'ssh.st.stopped': 'đã tắt',
  'ssh.deleteTunnelTitle': 'Xóa đường hầm "{name}"?', 'ssh.deleteTunnelText': 'Đường hầm sẽ bị dừng và xóa khỏi danh sách.',
  'ssh.badTunnel': 'Hãy chọn máy chủ và nhập cổng hợp lệ (1-65535)',
  'ssh.pwTitle': 'Mật khẩu cho {host}', 'ssh.pwText': 'Không đăng nhập được bằng khóa hoặc ssh-agent. Nhập mật khẩu (chỉ giữ trong bộ nhớ, không bao giờ được lưu).',
  'ssh.password': 'Mật khẩu', 'ssh.connect': 'Kết nối', 'ssh.host': 'Máy chủ',
  'ssh.up': 'Thư mục cha', 'ssh.refresh': 'Làm mới', 'ssh.newFolder': 'Thư mục mới', 'ssh.upload': 'Tải lên…',
  'ssh.size': 'Kích thước', 'ssh.modified': 'Sửa đổi', 'ssh.mode': 'Quyền',
  'ssh.pickHost': 'Chọn máy chủ rồi kết nối để duyệt tệp qua SFTP.', 'ssh.emptyDir': 'Thư mục trống.',
  'ssh.truncated': 'Chỉ hiển thị 5000 mục đầu tiên.', 'ssh.close': 'Đóng',
  'ssh.download': 'Tải về', 'ssh.rename': 'Đổi tên', 'ssh.delete': 'Xóa',
  'ssh.folderName': 'Tên thư mục mới', 'ssh.create': 'Tạo', 'ssh.newName': 'Tên mới', 'ssh.renameTitle': 'Đổi tên "{name}"',
  'ssh.deleteTitle': 'Xóa "{name}"?', 'ssh.deleteFile': 'Tệp sẽ bị xóa vĩnh viễn trên máy chủ.', 'ssh.deleteDir': 'Thư mục (phải trống) sẽ bị xóa trên máy chủ.',
  'ssh.overwriteTitle': 'Ghi đè "{name}"?', 'ssh.overwriteText': 'Tệp này đã tồn tại trên máy chủ và sẽ bị thay thế.', 'ssh.overwrite': 'Ghi đè',
  'ssh.uploading': 'Đang tải lên {name}…', 'ssh.uploaded': 'Đã tải lên {name}', 'ssh.done': 'Xong',
  'ssh.binary': 'Tệp nhị phân, không xem trước được. Hãy tải về.', 'ssh.previewCut': '… (đã cắt bớt, tệp có {size})',
  'ssh.connected': 'Đã kết nối {host}',
  'ssh.cmdTunnels': 'Đường hầm SSH', 'ssh.cmdFiles': 'Trình duyệt tệp SFTP', 'ssh.cmdHosts': 'Máy chủ SSH',
  'ssh.cmdStart': 'Bật đường hầm {name}', 'ssh.cmdStop': 'Tắt đường hầm {name}', 'ssh.cmdBrowse': 'Duyệt tệp trên {host}', 'ssh.tunnel': 'Đường hầm',
});

(() => {
const S = { sub: 'hosts', hosts: [], tunnels: [], editingHost: null, editingTunnel: null, fs: null, path: '', entries: [] };
const F = (s, v) => s.replace(/\{(\w+)\}/g, (_, k) => v[k] ?? '');
const FILE_ICON = '<svg class="i" viewBox="0 0 24 24"><path d="M6 3h8l4 4v14H6zM14 3v4h4"/></svg>';
const bytes = n => n < 1024 ? n + ' B' : n < 1048576 ? (n / 1024).toFixed(1) + ' KB' : n < 1073741824 ? (n / 1048576).toFixed(1) + ' MB' : (n / 1073741824).toFixed(2) + ' GB';
const icon = (id, label, attrs, cls = 'btn sm') => `<button class="${cls}" ${attrs}>${id ? `<svg class="i"><use href="#i-${id}"/></svg>` : ''}${esc(label)}</button>`;

// call is api() that keeps the server's auth/gone flags on the error.
async function call(method, path, body) {
  const r = await fetch(path, { method, headers: body !== undefined ? { 'Content-Type': 'application/json' } : {}, body: body !== undefined ? JSON.stringify(body) : undefined });
  const j = await r.json().catch(() => ({}));
  if (!r.ok) { const e = new Error(j.error || r.statusText); e.auth = !!j.auth; e.gone = r.status === 410; e.status = r.status; throw e; }
  return j;
}
// withPassword runs fn(password); if key/agent auth fails it asks for a password once per attempt.
async function withPassword(host, fn) {
  try { return await fn(''); } catch (e) {
    if (!e.auth) throw e;
    const pw = await dialog({ title: F(t('ssh.pwTitle', 'Password for {host}'), { host }), text: t('ssh.pwText', 'Key and ssh-agent login failed. Enter the password (kept in memory only, never saved).'), input: t('ssh.password', 'Password'), ok: t('ssh.connect', 'Connect') });
    if (!pw) return null;
    return await fn(pw);
  }
}

// ------------------------------------------------------------------ sub tabs
function sub(name) {
  S.sub = name;
  document.querySelectorAll('#ssh .ssh-sub [data-sub]').forEach(b => b.classList.toggle('on', b.dataset.sub === name));
  document.querySelectorAll('#ssh .ssh-pane').forEach(p => p.classList.toggle('on', p.dataset.pane === name));
  store.set('ssh.sub', name);
  if (name === 'tunnels') loadTunnels();
  if (name === 'files') fillHostSelect('#ssh-f-host', store.get('ssh.fhost', ''));
}
$('#ssh .ssh-sub').onclick = e => { const b = e.target.closest('[data-sub]'); if (b) sub(b.dataset.sub); };
document.addEventListener('devdeck:ssh-sub', e => sub(e.detail));

// ------------------------------------------------------------------ hosts
async function loadHosts(withAgent) {
  try {
    const r = await api('/api/ssh/hosts' + (withAgent ? '?agent=1' : ''));
    S.hosts = r.hosts || [];
    if (r.agent) {
      const a = $('#ssh-agent');
      a.className = 'pill ssh-agent ' + (r.agent.ok ? 'ok' : 'warn');
      a.textContent = r.agent.ok ? F(t('ssh.agentOk', 'ssh-agent: {n} key(s)'), { n: r.agent.keys }) : t('ssh.agentOff', 'no ssh-agent');
      a.title = r.agent.ok ? (r.agent.comments || []).join('\n') : r.agent.error || '';
    }
  } catch (e) { toast(e.message, 'err'); }
  renderHosts();
  fillHostSelect('#ssh-t-host', $('#ssh-t-host').value);
  fillHostSelect('#ssh-f-host', $('#ssh-f-host').value || store.get('ssh.fhost', ''));
}
const usable = () => S.hosts.filter(h => !h.shadowed);
function fillHostSelect(sel, keep) {
  const el = $(sel);
  el.innerHTML = usable().map(h => `<option value="${esc(h.name)}">${esc(h.name)}${h.source === 'config' ? ' (config)' : ''}</option>`).join('');
  if (keep && [...el.options].some(o => o.value === keep)) el.value = keep;
}
const addr = h => `${h.user ? esc(h.user) + '@' : ''}${esc(h.host)}${h.port && h.port !== 22 ? ':' + h.port : ''}`;
function hostActions(h) {
  return icon('term', t('ssh.terminal', 'Terminal'), `data-term="${esc(h.name)}"`) + icon('projects', t('ssh.browse', 'Files'), `data-files="${esc(h.name)}"`);
}
function renderHosts() {
  const saved = S.hosts.filter(h => h.source === 'saved'), conf = S.hosts.filter(h => h.source === 'config');
  $('#ssh-h-list').innerHTML = saved.length ? saved.map(h => `<tr>
    <td><b>${esc(h.name)}</b></td><td class="mono">${addr(h)}</td>
    <td class="muted ssh-ell" title="${esc(h.keyPath)}">${esc(h.keyPath || t('ssh.defaultKeys', 'ssh-agent / default keys'))}</td>
    <td class="muted">${h.jump ? '↪ ' + esc(h.jump) : ''}</td>
    <td class="act">${hostActions(h)}${icon('edit', t('ssh.edit', 'Edit'), `data-edit="${esc(h.name)}"`)}${icon('trash', t('ssh.remove', 'Remove'), `data-del="${esc(h.name)}"`, 'btn sm danger')}</td></tr>`).join('')
    : `<tr><td class="muted">${esc(t('ssh.noSaved', 'No saved hosts yet. Fill the form above and save.'))}</td></tr>`;
  $('#ssh-c-list').innerHTML = conf.length ? conf.map(h => `<tr${h.shadowed ? ' class="ssh-shadowed"' : ''}>
    <td><b>${esc(h.name)}</b>${h.shadowed ? ` <span class="pill warn">${esc(t('ssh.overridden', 'overridden by a saved host'))}</span>` : ''}</td><td class="mono">${addr(h)}</td>
    <td class="muted ssh-ell" title="${esc((h.keyFiles || []).join('\n'))}">${esc((h.keyFiles || []).join(', ') || t('ssh.defaultKeys', 'ssh-agent / default keys'))}</td>
    <td class="muted">${h.jump ? '↪ ' + esc(h.jump) : ''}</td>
    <td class="act">${h.shadowed ? '' : hostActions(h)}${icon('plus', t('ssh.copy', 'Save copy'), `data-copy="${esc(h.name)}"`)}</td></tr>`).join('')
    : `<tr><td class="muted">${esc(t('ssh.noConfig', 'No hosts found in ~/.ssh/config.'))}</td></tr>`;
}
const hostFields = ['name', 'host', 'port', 'user', 'key', 'jump'];
function fillForm(h, editing) {
  S.editingHost = editing ? h.name : null;
  $('#ssh-h-name').value = h.name || ''; $('#ssh-h-host').value = h.host || ''; $('#ssh-h-port').value = h.port || '';
  $('#ssh-h-user').value = h.user || ''; $('#ssh-h-key').value = h.keyPath || (h.keyFiles || [])[0] || ''; $('#ssh-h-jump').value = h.jump || '';
  $('#ssh-h-title').textContent = editing ? F(t('ssh.editing', 'Editing "{name}"'), { name: h.name }) : t('ssh.savedHosts', 'Saved hosts');
  $('#ssh-h-save').textContent = editing ? t('ssh.updateHost', 'Update host') : t('ssh.saveHost', 'Save host');
  $('#ssh-h-cancel').hidden = !editing;
  $('#ssh-h-name').focus();
}
function resetForm() { hostFields.forEach(f => $('#ssh-h-' + f).value = ''); fillForm({}, false); $('#ssh-h-name').blur(); }
// Writes the hosts list back into config.json, re-reading it first so other sections are kept.
async function saveHosts(hosts) {
  const fresh = await api('/api/config');
  fresh.hosts = hosts;
  await put('/api/config', fresh);
}
$('#ssh-h-cancel').onclick = resetForm;
$('#ssh-h-save').onclick = async () => {
  const name = $('#ssh-h-name').value.trim(), host = $('#ssh-h-host').value.trim(), jump = $('#ssh-h-jump').value.trim();
  const port = $('#ssh-h-port').value.trim() === '' ? 22 : +$('#ssh-h-port').value;
  if (!name || !host) return toast(t('ssh.nameHostRequired', 'Name and host are required'), 'err');
  if (!Number.isInteger(port) || port < 1 || port > 65535) return toast(t('ssh.badPort', 'Port must be 1-65535'), 'err');
  const savedHosts = S.hosts.filter(h => h.source === 'saved');
  if (name !== S.editingHost && savedHosts.some(h => h.name === name)) {
    const ok = await dialog({ title: F(t('ssh.replaceTitle', 'Replace "{name}"?'), { name }), text: t('ssh.replaceText', 'A host with this name already exists and will be overwritten.'), ok: t('ssh.replace', 'Replace'), danger: true });
    if (!ok) return;
  }
  const entry = { name, host, port, user: $('#ssh-h-user').value.trim(), keyPath: $('#ssh-h-key').value.trim() };
  const hosts = savedHosts.filter(h => h.name !== S.editingHost && h.name !== name).map(h => ({ name: h.name, host: h.host, port: h.port, user: h.user, keyPath: h.keyPath })).concat(entry);
  try {
    await call('PUT', '/api/ssh/extra', { name, jump }); // validates the jump list before the host is saved
    await saveHosts(hosts);
    if (S.editingHost && S.editingHost !== name) await call('PUT', '/api/ssh/extra', { name: S.editingHost, jump: '' });
    toast(F(S.editingHost ? t('ssh.updated', 'Updated {name}') : t('ssh.saved', 'Saved {name}'), { name }));
    resetForm(); await loadHosts(); document.dispatchEvent(new Event('devdeck:hosts'));
  } catch (e) { toast(e.message, 'err'); }
};
async function onHostAction(e) {
  const b = e.target.closest('button'); if (!b) return;
  const d = b.dataset;
  if (d.term) openSSHTerm(d.term);
  else if (d.files) { sub('files'); $('#ssh-f-host').value = d.files; connectFiles(); }
  else if (d.edit) { const h = S.hosts.find(x => x.source === 'saved' && x.name === d.edit); if (h) fillForm(h, true); }
  else if (d.copy) { const h = S.hosts.find(x => x.source === 'config' && x.name === d.copy); if (h) { fillForm(h, false); $('#ssh .ssh-pane.on').scrollTop = 0; } }
  else if (d.del) {
    const n = d.del;
    const ok = await dialog({ title: F(t('ssh.removeTitle', 'Remove "{name}"?'), { name: n }), text: t('ssh.removeText', 'Only the saved entry is removed. The server itself is not touched.'), ok: t('ssh.remove', 'Remove'), danger: true });
    if (!ok) return;
    try {
      await saveHosts(S.hosts.filter(h => h.source === 'saved' && h.name !== n).map(h => ({ name: h.name, host: h.host, port: h.port, user: h.user, keyPath: h.keyPath })));
      await call('PUT', '/api/ssh/extra', { name: n, jump: '' });
      if (S.editingHost === n) resetForm();
      toast(F(t('ssh.removed', 'Removed {name}'), { name: n }));
      await loadHosts(); document.dispatchEvent(new Event('devdeck:hosts'));
    } catch (err) { toast(err.message, 'err'); }
  }
}
$('#ssh-h-list').onclick = onHostAction;
$('#ssh-c-list').onclick = onHostAction;

// ------------------------------------------------------------------ tunnels
async function loadTunnels() {
  try { S.tunnels = await api('/api/ssh/tunnels'); } catch (e) { $('#ssh-t-list').innerHTML = `<tr><td colspan="7">${esc(e.message)}</td></tr>`; return; }
  renderTunnels();
}
const stateCls = { running: 'ok', idle: 'warn', connecting: 'warn', error: 'bad', stopped: '' };
function renderTunnels() {
  $('#ssh-t-list').innerHTML = S.tunnels.length ? S.tunnels.map(x => {
    const live = x.state === 'running' || x.state === 'idle' || x.state === 'connecting';
    return `<tr>
    <td><b>${esc(x.name)}</b>${x.autoReconnect ? ' <span class="muted" title="auto-reconnect">↻</span>' : ''}</td>
    <td class="mono">${live ? `<a href="http://127.0.0.1:${x.localPort}" target="_blank" rel="noopener">127.0.0.1:${x.localPort}</a>` : `127.0.0.1:${x.localPort}`}</td>
    <td class="mono">${esc(x.remoteHost)}:${x.remotePort} <span class="muted">${esc(t('ssh.viaShort', 'via'))} ${esc(x.host)}</span></td>
    <td><span class="pill ssh-st ${stateCls[x.state] || ''}" title="${esc(x.error || '')}">${esc(t('ssh.st.' + x.state, x.state))}</span>${x.error ? ` <span class="muted ssh-ell ssh-err" title="${esc(x.error)}">${esc(x.error)}</span>` : ''}</td>
    <td>${live ? `${x.active} / ${x.total}` : ''}</td>
    <td class="muted">${live ? `↓ ${bytes(x.bytesIn)} · ↑ ${bytes(x.bytesOut)}` : ''}</td>
    <td class="act">${live ? icon('stop', t('ssh.stop', 'Stop'), `data-stop="${x.id}"`) : icon('play', t('ssh.start', 'Start'), `data-start="${x.id}"`, 'btn sm primary')}
      ${icon('edit', t('ssh.edit', 'Edit'), `data-tedit="${x.id}"`)}${icon('trash', t('ssh.delete', 'Delete'), `data-tdel="${x.id}"`, 'btn sm danger')}</td></tr>`;
  }).join('') : `<tr><td colspan="7" class="empty">${esc(t('ssh.noTunnels', 'No tunnels yet. Example: 127.0.0.1:15432 → a database on the server.'))}</td></tr>`;
}
every(2000, 'ssh', () => { if (S.sub === 'tunnels' && S.tunnels.some(x => x.state !== 'stopped' && x.state !== 'error')) loadTunnels(); });

function tunnelForm(x) {
  S.editingTunnel = x ? x.id : null;
  fillHostSelect('#ssh-t-host', x ? x.host : $('#ssh-t-host').value);
  $('#ssh-t-name').value = x ? x.name : ''; $('#ssh-t-lport').value = x ? x.localPort : ''; $('#ssh-t-rport').value = x ? x.remotePort : '';
  $('#ssh-t-rhost').value = x ? x.remoteHost : ''; $('#ssh-t-auto').checked = x ? x.autoReconnect : false;
  $('#ssh-t-title').textContent = x ? F(t('ssh.editTunnel', 'Edit "{name}"'), { name: x.name }) : t('ssh.newTunnel', 'New tunnel');
  $('#ssh-t-form').hidden = false; $('#ssh-t-name').focus();
}
$('#ssh-t-add').onclick = () => tunnelForm(null);
$('#ssh-t-cancel').onclick = () => { $('#ssh-t-form').hidden = true; S.editingTunnel = null; };
$('#ssh-t-save').onclick = async () => {
  const d = { id: S.editingTunnel || '', name: $('#ssh-t-name').value.trim(), host: $('#ssh-t-host').value, localPort: +$('#ssh-t-lport').value, remoteHost: $('#ssh-t-rhost').value.trim(), remotePort: +$('#ssh-t-rport').value, autoReconnect: $('#ssh-t-auto').checked };
  const okPort = p => Number.isInteger(p) && p >= 1 && p <= 65535;
  if (!d.host || !okPort(d.localPort) || !okPort(d.remotePort)) return toast(t('ssh.badTunnel', 'Pick a host and enter valid ports (1-65535)'), 'err');
  try { const r = await call('POST', '/api/ssh/tunnels', d); toast(F(t('ssh.saved', 'Saved {name}'), { name: r.name })); $('#ssh-t-form').hidden = true; S.editingTunnel = null; loadTunnels(); }
  catch (e) { toast(e.message, 'err'); }
};
async function startTunnel(x) {
  try {
    const r = await withPassword(x.host, pw => call('POST', `/api/ssh/tunnels/${encodeURIComponent(x.id)}/start`, { password: pw }));
    if (r) toast(F(t('ssh.started', 'Started {name}'), { name: x.name }));
  } catch (e) { toast(e.message, 'err'); }
  loadTunnels();
}
async function stopTunnel(x) {
  try { await call('POST', `/api/ssh/tunnels/${encodeURIComponent(x.id)}/stop`, {}); toast(F(t('ssh.stopped', 'Stopped {name}'), { name: x.name })); } catch (e) { toast(e.message, 'err'); }
  loadTunnels();
}
$('#ssh-t-list').onclick = async e => {
  const b = e.target.closest('button'); if (!b) return;
  const id = b.dataset.start || b.dataset.stop || b.dataset.tedit || b.dataset.tdel;
  const x = S.tunnels.find(y => y.id === id); if (!x) return;
  if (b.dataset.start) startTunnel(x);
  else if (b.dataset.stop) stopTunnel(x);
  else if (b.dataset.tedit) tunnelForm(x);
  else if (b.dataset.tdel) {
    const ok = await dialog({ title: F(t('ssh.deleteTunnelTitle', 'Delete tunnel "{name}"?'), { name: x.name }), text: t('ssh.deleteTunnelText', 'The tunnel is stopped and removed from the list.'), ok: t('ssh.delete', 'Delete'), danger: true });
    if (!ok) return;
    try { await call('DELETE', '/api/ssh/tunnels/' + encodeURIComponent(x.id)); } catch (err) { toast(err.message, 'err'); }
    loadTunnels();
  }
};

// ------------------------------------------------------------------ SFTP file browser
const fsBtns = ['#ssh-f-up', '#ssh-f-path', '#ssh-f-refresh', '#ssh-f-mkdir', '#ssh-f-upload'];
const parentOf = p => p === '/' ? '/' : (p.replace(/\/+$/, '').replace(/\/[^/]*$/, '') || '/');
const joinPath = (dir, name) => (dir.endsWith('/') ? dir : dir + '/') + name;
function fsEnable(on) { fsBtns.forEach(s => $(s).disabled = !on); }
async function connectFiles() {
  const host = $('#ssh-f-host').value; if (!host) return;
  store.set('ssh.fhost', host);
  if (S.fs) { call('POST', '/api/sftp/close', { id: S.fs.id }).catch(() => {}); S.fs = null; }
  fsEnable(false); $('#ssh-f-preview').hidden = true;
  $('#ssh-f-list').innerHTML = `<tr><td colspan="5" class="empty">${esc(t('term.connecting', 'Connecting…'))}</td></tr>`;
  try {
    const r = await withPassword(host, pw => call('POST', '/api/sftp/open', { host, password: pw }));
    if (!r) { $('#ssh-f-list').innerHTML = ''; return; }
    S.fs = { id: r.id, host, home: r.home };
    fsEnable(true); toast(F(t('ssh.connected', 'Connected to {host}'), { host }));
    await list(r.home);
  } catch (e) { $('#ssh-f-list').innerHTML = `<tr><td colspan="5">${esc(e.message)}</td></tr>`; }
}
// fsCall runs an SFTP request; an expired session is reopened once (silently if keys work).
async function fsCall(fn) {
  try { return await fn(S.fs.id); } catch (e) {
    if (!e.gone || !S.fs) throw e;
    const host = S.fs.host;
    const r = await withPassword(host, pw => call('POST', '/api/sftp/open', { host, password: pw }));
    if (!r) throw e;
    S.fs = { id: r.id, host, home: r.home };
    return await fn(S.fs.id);
  }
}
async function list(path) {
  if (!S.fs) return;
  try {
    const r = await fsCall(id => call('GET', `/api/sftp/list?id=${encodeURIComponent(id)}&path=${encodeURIComponent(path || '')}`));
    S.path = r.path; S.entries = r.entries || [];
    $('#ssh-f-path').value = r.path;
    renderFiles(r.truncated);
  } catch (e) { toast(e.message, 'err'); }
}
function renderFiles(truncated) {
  const dl = e => `/api/sftp/download?id=${encodeURIComponent(S.fs.id)}&path=${encodeURIComponent(joinPath(S.path, e.name))}`;
  const rows = S.entries.map((e, i) => `<tr data-i="${i}" class="${e.dir ? 'ssh-dir' : 'ssh-file'}">
    <td class="ssh-fname">${e.dir ? '<svg class="i"><use href="#i-projects"/></svg>' : FILE_ICON}${esc(e.name)}${e.link ? ' <span class="muted">↪</span>' : ''}</td>
    <td class="muted">${e.dir ? '' : bytes(e.size)}</td>
    <td class="muted">${new Date(e.mtime * 1000).toLocaleString()}</td>
    <td class="mono muted">${esc(e.mode)}</td>
    <td class="act">${e.dir ? '' : `<a class="btn sm" href="${dl(e)}" download="${esc(e.name)}">${esc(t('ssh.download', 'Download'))}</a>`}
      ${icon('', t('ssh.rename', 'Rename'), `data-ren="${i}"`)}${icon('trash', t('ssh.delete', 'Delete'), `data-fdel="${i}"`, 'btn sm danger')}</td></tr>`);
  $('#ssh-f-list').innerHTML = (rows.join('') || `<tr><td colspan="5" class="empty">${esc(t('ssh.emptyDir', 'This folder is empty.'))}</td></tr>`)
    + (truncated ? `<tr><td colspan="5" class="muted">${esc(t('ssh.truncated', 'Only the first 5000 entries are shown.'))}</td></tr>` : '');
}
$('#ssh-f-connect').onclick = connectFiles;
$('#ssh-f-host').onchange = () => store.set('ssh.fhost', $('#ssh-f-host').value);
$('#ssh-f-refresh').onclick = () => list(S.path);
$('#ssh-f-up').onclick = () => list(parentOf(S.path));
$('#ssh-f-path').onkeydown = e => { if (e.key === 'Enter') list($('#ssh-f-path').value.trim()); };
$('#ssh-f-pclose').onclick = () => { $('#ssh-f-preview').hidden = true; };
async function preview(e) {
  const p = joinPath(S.path, e.name);
  try {
    const r = await fsCall(id => call('GET', `/api/sftp/preview?id=${encodeURIComponent(id)}&path=${encodeURIComponent(p)}`));
    $('#ssh-f-pname').textContent = p;
    $('#ssh-f-ptext').textContent = r.binary ? t('ssh.binary', 'Binary file, no preview. Download it instead.')
      : r.text + (r.truncated ? '\n' + F(t('ssh.previewCut', '… (truncated, the file is {size})'), { size: bytes(r.size) }) : '');
    $('#ssh-f-preview').hidden = false;
  } catch (err) { toast(err.message, 'err'); }
}
$('#ssh-f-list').onclick = async ev => {
  const b = ev.target.closest('button'), row = ev.target.closest('tr[data-i]');
  if (ev.target.closest('a')) return; // download link
  if (!row) return;
  const e = S.entries[+row.dataset.i]; if (!e) return;
  const p = joinPath(S.path, e.name);
  if (b && b.dataset.ren !== undefined) {
    const n = await dialog({ title: F(t('ssh.renameTitle', 'Rename "{name}"'), { name: e.name }), input: t('ssh.newName', 'New name'), ok: t('ssh.rename', 'Rename'), password: false });
    if (!n || n === e.name) return;
    if (n.includes('/')) return toast('/', 'err');
    try { await fsCall(id => call('POST', '/api/sftp/rename', { id, path: p, to: joinPath(S.path, n) })); list(S.path); } catch (err) { toast(err.message, 'err'); }
  } else if (b && b.dataset.fdel !== undefined) {
    const ok = await dialog({ title: F(t('ssh.deleteTitle', 'Delete "{name}"?'), { name: e.name }), text: e.dir ? t('ssh.deleteDir', 'The folder (it must be empty) is removed from the server.') : t('ssh.deleteFile', 'The file is permanently deleted on the server.'), ok: t('ssh.delete', 'Delete'), danger: true });
    if (!ok) return;
    try { await fsCall(id => call('POST', '/api/sftp/delete', { id, path: p })); list(S.path); } catch (err) { toast(err.message, 'err'); }
  } else if (!b) {
    if (e.dir) list(p); else preview(e);
  }
};
$('#ssh-f-mkdir').onclick = async () => {
  const n = await dialog({ title: t('ssh.newFolder', 'New folder'), input: t('ssh.folderName', 'Folder name'), ok: t('ssh.create', 'Create'), password: false });
  if (!n) return;
  if (n.includes('/')) return toast('/', 'err');
  try { await fsCall(id => call('POST', '/api/sftp/mkdir', { id, path: joinPath(S.path, n) })); list(S.path); } catch (err) { toast(err.message, 'err'); }
};
$('#ssh-f-upload').onclick = () => $('#ssh-f-file').click();
$('#ssh-f-file').onchange = async () => {
  const files = [...$('#ssh-f-file').files]; $('#ssh-f-file').value = '';
  for (const f of files) {
    let overwrite = false;
    if (S.entries.some(e => e.name === f.name)) {
      const ok = await dialog({ title: F(t('ssh.overwriteTitle', 'Overwrite "{name}"?'), { name: f.name }), text: t('ssh.overwriteText', 'This file already exists on the server and will be replaced.'), ok: t('ssh.overwrite', 'Overwrite'), danger: true });
      if (!ok) continue;
      overwrite = true;
    }
    toast(F(t('ssh.uploading', 'Uploading {name}…'), { name: f.name }));
    try {
      await fsCall(async id => {
        const r = await fetch(`/api/sftp/upload?id=${encodeURIComponent(id)}&dir=${encodeURIComponent(S.path)}&name=${encodeURIComponent(f.name)}&overwrite=${overwrite ? 1 : 0}`, { method: 'POST', body: f });
        const j = await r.json().catch(() => ({}));
        if (!r.ok) { const e = new Error(j.error || r.statusText); e.gone = r.status === 410; throw e; }
        return j;
      });
      toast(F(t('ssh.uploaded', 'Uploaded {name}'), { name: f.name }));
    } catch (err) { toast(`${f.name}: ${err.message}`, 'err'); }
  }
  list(S.path);
};

// ------------------------------------------------------------------ palette + init
registerCommand({ title: t('ssh.cmdTunnels', 'SSH tunnels'), keywords: 'ssh tunnel port forward', run: () => { show('ssh'); sub('tunnels'); } });
registerCommand({ title: t('ssh.cmdFiles', 'SFTP file browser'), keywords: 'ssh sftp files browse upload download', run: () => { show('ssh'); sub('files'); } });
registerCommand({ title: t('ssh.cmdHosts', 'SSH hosts'), keywords: 'ssh hosts config agent', run: () => { show('ssh'); sub('hosts'); } });
registerCommandProvider(async q => {
  const out = [];
  if (q.startsWith('tun')) {
    if (!S.tunnels.length) await loadTunnels();
    const f = q.replace(/^tun\w*/, '').trim();
    S.tunnels.filter(x => !f || x.name.toLowerCase().includes(f)).forEach(x => {
      const live = x.state !== 'stopped' && x.state !== 'error';
      out.push({ title: F(live ? t('ssh.cmdStop', 'Stop tunnel {name}') : t('ssh.cmdStart', 'Start tunnel {name}'), { name: x.name }), hint: `127.0.0.1:${x.localPort} → ${x.remoteHost}:${x.remotePort}`, run: () => { show('ssh'); sub('tunnels'); return live ? stopTunnel(x) : startTunnel(x); } });
    });
  }
  if (q.startsWith('files') || q.startsWith('sftp') || q.startsWith('browse')) {
    if (!S.hosts.length) await loadHosts();
    const f = q.replace(/^(files|sftp|browse)/, '').trim();
    usable().filter(h => !f || h.name.toLowerCase().includes(f)).forEach(h => out.push({
      title: F(t('ssh.cmdBrowse', 'Browse files on {host}'), { host: h.name }), hint: 'SFTP',
      run: () => { show('ssh'); sub('files'); $('#ssh-f-host').value = h.name; connectFiles(); },
    }));
  }
  return out;
});
onTab('ssh', () => { loadHosts(true); if (S.sub === 'tunnels') loadTunnels(); });
sub(['hosts', 'tunnels', 'files'].includes(store.get('ssh.sub', 'hosts')) ? store.get('ssh.sub', 'hosts') : 'hosts');
})();
