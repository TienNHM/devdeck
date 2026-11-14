'use strict';
// Redis tab: saved profiles, key tree + flat list, JSON viewer, server info, optional write mode.
// Everything lives inside this closure; nothing leaks into the shared global scope.
(() => {
registerStrings('vi', {
  'redis.profiles': 'Hồ sơ kết nối đã lưu',
  'redis.dbIndex': 'Chỉ số database',
  'redis.user': 'Tên người dùng (tùy chọn)',
  'redis.pass': 'Mật khẩu (chỉ giữ trong bộ nhớ)',
  'redis.writeTip': 'Cho phép thay đổi dữ liệu trên kết nối này',
  'redis.writeMode': 'Chế độ ghi',
  'redis.connect': 'Kết nối',
  'redis.connecting': 'Đang kết nối…',
  'redis.saveProfile': 'Lưu hồ sơ',
  'redis.deleteProfile': 'Xóa hồ sơ',
  'redis.disconnect': 'Ngắt kết nối',
  'redis.keys': 'Khóa',
  'redis.server': 'Thông tin máy chủ',
  'redis.match': 'Mẫu tìm khóa',
  'redis.scan': 'Quét',
  'redis.tree': 'Cây',
  'redis.list': 'Danh sách',
  'redis.selectAll': 'Chọn tất cả khóa đã tải',
  'redis.newKey': 'Khóa mới',
  'redis.more': 'Tải thêm',
  'redis.pick': 'Chọn một khóa để xem chi tiết.',
  'redis.empty': 'Kết nối tới máy chủ Redis. Mặc định chỉ đọc, trừ khi bạn bật chế độ ghi.',
  'redis.newConn': '— Kết nối mới —',
  'redis.saveTitle': 'Lưu hồ sơ kết nối',
  'redis.saveText': 'Mật khẩu không bao giờ được lưu. Để trống tên để ghi đè hồ sơ đang chọn.',
  'redis.profileName': 'Tên hồ sơ',
  'redis.save': 'Lưu',
  'redis.saved': 'Đã lưu hồ sơ',
  'redis.needName': 'Hồ sơ cần có tên',
  'redis.delProfTitle': 'Xóa hồ sơ này?',
  'redis.delProfText': 'Chỉ xóa hồ sơ đã lưu, dữ liệu Redis không bị ảnh hưởng.',
  'redis.delete': 'Xóa',
  'redis.writeWarnTitle': 'Bật chế độ ghi?',
  'redis.writeWarnText': 'Chế độ ghi cho phép sửa, đổi tên và xóa dữ liệu trên máy chủ này. Mỗi thay đổi đều phải xác nhận, nhưng không thể hoàn tác. Hãy cẩn thận với máy chủ production.',
  'redis.enable': 'Bật chế độ ghi',
  'redis.readonly': 'chỉ đọc',
  'redis.writeOn': 'CHẾ ĐỘ GHI',
  'redis.loaded': 'khóa đã tải',
  'redis.moreAvail': 'còn nữa',
  'redis.noKeys': 'Không có khóa nào khớp.',
  'redis.moreRows': 'mục nữa (dùng mẫu tìm để thu hẹp)',
  'redis.noExpiry': 'không hết hạn',
  'redis.missing': 'không tồn tại',
  'redis.size': 'kích thước',
  'redis.truncated': 'đã cắt bớt',
  'redis.binary': 'nhị phân',
  'redis.reload': 'Tải lại',
  'redis.copyName': 'Sao chép tên',
  'redis.rename': 'Đổi tên',
  'redis.copied': 'Đã sao chép',
  'redis.gone': 'Khóa này không còn tồn tại.',
  'redis.formatted': 'Định dạng',
  'redis.raw': 'Thô',
  'redis.expandAll': 'Mở hết',
  'redis.collapseAll': 'Thu gọn hết',
  'redis.edit': 'Sửa',
  'redis.cancel': 'Hủy',
  'redis.noEdit': 'Không thể sửa giá trị bị cắt bớt hoặc nhị phân',
  'redis.field': 'Trường',
  'redis.value': 'Giá trị',
  'redis.member': 'Phần tử',
  'redis.score': 'Điểm',
  'redis.index': 'Vị trí',
  'redis.setField': 'Đặt trường',
  'redis.pushLeft': 'Thêm vào đầu',
  'redis.pushRight': 'Thêm vào cuối',
  'redis.add': 'Thêm',
  'redis.remove': 'Xóa',
  'redis.clickValue': 'Bấm vào một giá trị để xem đầy đủ.',
  'redis.confirm': 'Xác nhận',
  'redis.apply': 'Áp dụng',
  'redis.renameTitle': 'Đổi tên khóa',
  'redis.renameText': 'Tên mới (không ghi đè khóa đã tồn tại):',
  'redis.ttlTitle': 'Đặt TTL',
  'redis.ttlText': 'Số giây đến khi hết hạn. Để trống hoặc 0 để bỏ thời hạn.',
  'redis.badTtl': 'TTL phải là số giây',
  'redis.delKeyTitle': 'Xóa khóa này?',
  'redis.cannotUndo': 'Thao tác này không thể hoàn tác.',
  'redis.delKeysTitle': 'Xóa {n} khóa?',
  'redis.delFolderText': '{n} khóa đã tải nằm dưới "{p}" sẽ bị xóa. Khóa chưa tải sẽ không bị ảnh hưởng. Không thể hoàn tác.',
  'redis.deleted': 'Đã xóa {n} khóa',
  'redis.saveValTitle': 'Lưu giá trị mới?',
  'redis.saveValText': 'Giá trị của "{k}" sẽ bị thay thế ({n} byte). TTL hiện tại được giữ nguyên.',
  'redis.done': 'Đã lưu',
  'redis.newKeyTitle': 'Tạo khóa mới',
  'redis.keyName': 'Tên khóa',
  'redis.type': 'Kiểu',
  'redis.create': 'Tạo',
  'redis.exists': 'Khóa đã tồn tại',
  'redis.createText': 'Tạo khóa {t} "{k}"?',
  'redis.refresh': 'Làm mới',
  'redis.version': 'Phiên bản',
  'redis.mode': 'Chế độ / vai trò',
  'redis.uptime': 'Thời gian chạy',
  'redis.usedMem': 'Bộ nhớ đang dùng',
  'redis.peakMem': 'Bộ nhớ đỉnh',
  'redis.maxMem': 'Giới hạn bộ nhớ',
  'redis.noLimit': 'không giới hạn',
  'redis.clients': 'Client kết nối',
  'redis.ops': 'Lệnh / giây',
  'redis.hitRatio': 'Tỷ lệ trúng cache',
  'redis.totalCmds': 'Tổng số lệnh',
  'redis.frag': 'Phân mảnh bộ nhớ',
  'redis.keyspace': 'Khóa theo database',
  'redis.db': 'Database',
  'redis.expires': 'Có hạn',
  'redis.avgTtl': 'TTL trung bình',
  'redis.rawInfo': 'INFO gốc',
  'redis.noKeyspace': 'Chưa có khóa nào.',
  'redis.cmdConnect': 'Kết nối Redis',
  'redis.cmdPwTitle': 'Kết nối tới {p}',
  'redis.cmdPwText': 'Mật khẩu (để trống nếu không có). Kết nối ở chế độ chỉ đọc.',
  'redis.lost': 'Kết nối Redis đã đóng, hãy kết nối lại.',
  'redis.autoRefresh': 'tự làm mới mỗi 5 giây',
});

const enc = encodeURIComponent;
const S = {
  id: null, write: false, ver: '', where: '',
  keys: new Map(), cursor: '0', mode: store.get('rd.mode', 'tree') === 'list' ? 'list' : 'tree',
  open: new Set(), sel: new Set(), cur: null, kv: null, view: 'keys',
  fmt: 'json', editing: false, sub: null, creating: false, profiles: null, rawOpen: false,
};
const ttlText = s => s === -1 ? t('redis.noExpiry', 'no expiry') : s === -2 ? t('redis.missing', 'missing') : s < 90 ? s + 's' : s < 5400 ? (s / 60 | 0) + 'm' : s < 172800 ? (s / 3600 | 0) + 'h' : (s / 86400 | 0) + 'd';
const fmtN = n => Number(n || 0).toLocaleString();
const cmp = (a, b) => a < b ? -1 : a > b ? 1 : 0;
const svg = id => `<svg class="i"><use href="#i-${id}"/></svg>`;

async function copyText(s) {
  try { await navigator.clipboard.writeText(s); }
  catch { const ta = document.createElement('textarea'); ta.value = s; document.body.appendChild(ta); ta.select(); document.execCommand('copy'); ta.remove(); }
  toast(t('redis.copied', 'Copied'));
}

// ------------------------------------------------------------------ profiles
async function loadProfiles(force) {
  if (!S.profiles || force) {
    try { S.profiles = await api('/api/redis/profiles'); } catch (e) { S.profiles = S.profiles || []; toast(e.message, 'err'); }
  }
  return S.profiles;
}
function renderProfiles(selName) {
  const ps = S.profiles || [];
  $('#rd-prof').innerHTML = `<option value="">${esc(t('redis.newConn', '— New connection —'))}</option>` + ps.map((p, i) => `<option value="${i}">${esc(p.name)}</option>`).join('');
  const i = selName ? ps.findIndex(p => p.name === selName) : -1;
  $('#rd-prof').value = i >= 0 ? String(i) : '';
  $('#rd-delprof').hidden = i < 0;
}
const selectedProfile = () => $('#rd-prof').value === '' ? null : (S.profiles || [])[+$('#rd-prof').value] || null;
function fillFrom(p) { $('#rd-addr').value = p.addr || ''; $('#rd-db').value = p.db || 0; $('#rd-tls').checked = !!p.tls; $('#rd-user').value = p.username || ''; }
const formProfile = () => ({ addr: $('#rd-addr').value.trim(), db: +$('#rd-db').value || 0, tls: $('#rd-tls').checked, username: $('#rd-user').value.trim() });

$('#rd-prof').onchange = () => {
  const p = selectedProfile();
  if (p) { fillFrom(p); store.set('rd.prof', p.name); }
  $('#rd-delprof').hidden = !p;
};
$('#rd-save').onclick = async () => {
  const cur = selectedProfile();
  const name = await dialog({ title: t('redis.saveTitle', 'Save connection profile'), text: t('redis.saveText', 'The password is never saved. Leave the name empty to overwrite the selected profile.'), input: cur ? cur.name : t('redis.profileName', 'Profile name'), ok: t('redis.save', 'Save'), password: false });
  if (name === null) return;
  const nm = name.trim() || (cur ? cur.name : '');
  if (!nm) { toast(t('redis.needName', 'A profile needs a name'), 'err'); return; }
  const list = (S.profiles || []).filter(p => p.name.toLowerCase() !== nm.toLowerCase());
  list.push({ name: nm, ...formProfile() });
  list.sort((a, b) => a.name.localeCompare(b.name));
  try { S.profiles = await put('/api/redis/profiles', list); renderProfiles(nm); store.set('rd.prof', nm); toast(t('redis.saved', 'Profile saved')); }
  catch (e) { toast(e.message, 'err'); }
};
$('#rd-delprof').onclick = async () => {
  const cur = selectedProfile(); if (!cur) return;
  const ok = await dialog({ title: t('redis.delProfTitle', 'Delete this profile?'), text: `${cur.name}: ${t('redis.delProfText', 'Only the saved profile is removed; no Redis data is touched.')}`, ok: t('redis.delete', 'Delete'), danger: true });
  if (!ok) return;
  try { S.profiles = await put('/api/redis/profiles', S.profiles.filter(p => p !== cur)); renderProfiles(''); } catch (e) { toast(e.message, 'err'); }
};

// ------------------------------------------------------------------ connection
$('#rd-write').onchange = async () => {
  if (!$('#rd-write').checked) return;
  const ok = await dialog({ title: t('redis.writeWarnTitle', 'Enable write mode?'), text: t('redis.writeWarnText', 'Write mode lets you edit, rename and delete data on this server. Every change asks for confirmation, but none can be undone. Be careful with production servers.'), ok: t('redis.enable', 'Enable write mode'), danger: true });
  if (!ok) $('#rd-write').checked = false;
};
function renderStatus() {
  if (!S.id) { $('#rd-status').innerHTML = ''; return; }
  $('#rd-status').innerHTML = `<span class="pill ok">Redis ${esc(S.ver || '?')}</span> <span class="mono">${esc(S.where)}</span> ` +
    (S.write ? `<span class="pill rd-wpill">${esc(t('redis.writeOn', 'WRITE MODE'))}</span>` : `<span class="pill rd-ropill">${esc(t('redis.readonly', 'read-only'))}</span>`);
}
function showBody(on) {
  $('#rd-body').hidden = !on; $('#rd-empty').hidden = on; $('#rd-disconnect').hidden = !on;
  $('#rd-wbar').hidden = !(on && S.write);
}
async function connect() {
  const write = $('#rd-write').checked, f = formProfile();
  $('#rd-status').textContent = t('redis.connecting', 'Connecting…');
  try {
    if (S.id) { post('/api/redis/close', { id: S.id }).catch(() => {}); S.id = null; }
    const r = await post('/api/redis/open', { ...f, password: $('#rd-pass').value, write });
    $('#rd-pass').value = '';
    Object.assign(S, { id: r.id, write: !!r.write, ver: r.version, where: `${f.addr || '127.0.0.1:6379'}/${f.db}`, cur: null, kv: null, editing: false, creating: false });
    renderStatus(); showBody(true); renderDetail();
    await scan(true);
    if (S.view === 'info') loadInfo();
  } catch (e) { renderStatus(); toast(e.message, 'err'); }
}
function disconnected(msg) {
  if (S.id) post('/api/redis/close', { id: S.id }).catch(() => {});
  Object.assign(S, { id: null, write: false, cur: null, kv: null, editing: false, creating: false });
  S.keys.clear(); S.sel.clear();
  renderStatus(); showBody(false);
  if (msg) toast(msg, 'err');
}
function handleErr(e) {
  if (/unknown redis connection|expired/i.test(e.message)) disconnected(t('redis.lost', 'The Redis connection was closed; connect again.'));
  else toast(e.message, 'err');
}
$('#rd-connect').onclick = connect;
$('#rd-pass').onkeydown = e => { if (e.key === 'Enter') connect(); };
$('#rd-disconnect').onclick = () => disconnected();
beforeQuit.push(() => { S.id = null; });

// ------------------------------------------------------------------ views
$('#rd-views').onclick = e => {
  const b = e.target.closest('[data-v]'); if (!b) return;
  S.view = b.dataset.v;
  $('#rd-views').querySelectorAll('button').forEach(x => x.classList.toggle('on', x === b));
  $('#rd-keysview').hidden = S.view !== 'keys'; $('#rd-infoview').hidden = S.view !== 'info';
  if (S.view === 'info') loadInfo();
};
$('#rd-mode').onclick = e => {
  const b = e.target.closest('[data-m]'); if (!b) return;
  S.mode = b.dataset.m; store.set('rd.mode', S.mode); renderModeBtns(); renderKeys();
};
function renderModeBtns() { $('#rd-mode').querySelectorAll('button').forEach(x => x.classList.toggle('on', x.dataset.m === S.mode)); }
renderModeBtns();

// ------------------------------------------------------------------ keys: scan, tree, list
async function scan(reset) {
  if (!S.id) return;
  if (reset) { S.cursor = '0'; S.keys.clear(); S.sel.clear(); }
  const match = $('#rd-match').value.trim() || '*';
  try {
    let got = 0, loops = 0;
    // SCAN can return few keys per call on big databases; keep going a bounded number of times.
    do {
      const r = await api(`/api/redis/scan?id=${S.id}&cursor=${S.cursor}&match=${enc(match)}&count=500`);
      S.cursor = String(r.cursor);
      for (const k of r.keys || []) if (!S.keys.has(k.key)) { S.keys.set(k.key, k); got++; }
    } while (S.cursor !== '0' && got < 200 && ++loops < 10);
    renderKeys();
  } catch (e) { handleErr(e); }
}
$('#rd-scan').onclick = () => scan(true);
$('#rd-match').onkeydown = e => { if (e.key === 'Enter') scan(true); };
$('#rd-more').onclick = () => scan(false);

function buildTree() {
  const root = { c: new Map(), n: 0 };
  for (const [k, e] of S.keys) {
    const parts = k.split(':');
    let node = root;
    for (let i = 0; i < parts.length - 1; i++) {
      let ch = node.c.get(parts[i]);
      if (!ch) { ch = { c: new Map(), n: 0 }; node.c.set(parts[i], ch); }
      ch.n++; node = ch;
    }
    const last = parts[parts.length - 1];
    let leaf = node.c.get(last);
    if (!leaf) { leaf = { c: new Map(), n: 0 }; node.c.set(last, leaf); }
    leaf.e = e;
  }
  return root;
}
const ROW_CAP = 500;
function leafHTML(e, label, depth) {
  return `<div class="rd-row rd-leaf${S.cur === e.key ? ' on' : ''}" data-k="${esc(e.key)}" style="--d:${depth}" title="${esc(e.key)}">` +
    (S.write ? `<input type="checkbox" class="rd-ck"${S.sel.has(e.key) ? ' checked' : ''}>` : '') +
    `<span class="tp ${esc(e.type)}">${esc(e.type)}</span><span class="rd-kn mono">${esc(label)}</span>` +
    (e.ttl >= 0 ? `<span class="rd-ttl">${esc(ttlText(e.ttl))}</span>` : '') + '</div>';
}
function treeHTML(node, prefix, depth) {
  const names = [...node.c.keys()].sort(cmp);
  let out = '';
  for (let i = 0; i < names.length; i++) {
    if (i >= ROW_CAP) { out += `<div class="rd-note" style="--d:${depth}">+${fmtN(names.length - i)} ${esc(t('redis.moreRows', 'more (narrow the match pattern)'))}</div>`; break; }
    const name = names[i], ch = node.c.get(name), full = prefix + name;
    if (ch.c.size) {
      const open = S.open.has(full);
      out += `<div class="rd-row rd-folder${open ? ' open' : ''}" data-f="${esc(full)}" style="--d:${depth}"><span class="rd-caret"></span><span class="rd-fname mono">${esc(name === '' ? '(empty)' : name)}</span><span class="rd-n">${fmtN(ch.n)}</span>` +
        (S.write ? `<button class="rd-fdel" title="${esc(t('redis.delete', 'Delete'))}">${svg('trash')}</button>` : '') + '</div>';
      if (open) out += treeHTML(ch, full + ':', depth + 1);
    }
    if (ch.e) out += leafHTML(ch.e, name === '' ? '(empty)' : name, depth);
  }
  return out;
}
function renderKeys() {
  const el = $('#rd-keys');
  if (!S.keys.size) el.innerHTML = `<div class="empty">${esc(t('redis.noKeys', 'No keys match.'))}</div>`;
  else if (S.mode === 'tree') el.innerHTML = treeHTML(buildTree(), '', 0);
  else {
    const ks = [...S.keys.keys()].sort(cmp);
    el.innerHTML = ks.slice(0, 5000).map(k => leafHTML(S.keys.get(k), k, 0)).join('') +
      (ks.length > 5000 ? `<div class="rd-note">+${fmtN(ks.length - 5000)} ${esc(t('redis.moreRows', 'more (narrow the match pattern)'))}</div>` : '');
  }
  $('#rd-count').textContent = `${fmtN(S.keys.size)} ${t('redis.loaded', 'keys loaded')}${S.cursor !== '0' ? ' · ' + t('redis.moreAvail', 'more available') : ''}`;
  $('#rd-more').hidden = S.cursor === '0';
  renderSel();
}
function renderSel() {
  for (const k of S.sel) if (!S.keys.has(k)) S.sel.delete(k);
  $('#rd-delsel').disabled = !S.sel.size;
  $('#rd-delsel-t').textContent = `${t('redis.delete', 'Delete')}${S.sel.size ? ` (${fmtN(S.sel.size)})` : ''}`;
  $('#rd-selall').checked = S.keys.size > 0 && S.sel.size === S.keys.size;
}
function markActive() { $('#rd-keys').querySelectorAll('.rd-leaf').forEach(r => r.classList.toggle('on', r.dataset.k === S.cur)); }

$('#rd-keys').onclick = e => {
  const ck = e.target.closest('.rd-ck');
  if (ck) { const k = ck.closest('.rd-leaf').dataset.k; ck.checked ? S.sel.add(k) : S.sel.delete(k); renderSel(); return; }
  const fdel = e.target.closest('.rd-fdel');
  if (fdel) { const p = fdel.closest('.rd-folder').dataset.f; deleteKeys([...S.keys.keys()].filter(k => k.startsWith(p + ':')), p); return; }
  const f = e.target.closest('.rd-folder');
  if (f) { const p = f.dataset.f; S.open.has(p) ? S.open.delete(p) : S.open.add(p); renderKeys(); return; }
  const l = e.target.closest('.rd-leaf');
  if (l) openKey(l.dataset.k);
};
$('#rd-selall').onchange = () => { if ($('#rd-selall').checked) S.keys.forEach((_, k) => S.sel.add(k)); else S.sel.clear(); renderKeys(); };
$('#rd-delsel').onclick = () => deleteKeys([...S.sel]);
$('#rd-new').onclick = () => { S.creating = true; S.cur = null; markActive(); renderDetail(); };

// ------------------------------------------------------------------ writes
async function write(body) { return post('/api/redis/write', { id: S.id, ...body }); }
async function deleteKeys(keys, prefix) {
  if (!keys.length) return;
  const n = keys.length;
  const ok = await dialog({
    title: t('redis.delKeysTitle', `Delete ${n} key(s)?`).replace('{n}', fmtN(n)),
    text: prefix ? t('redis.delFolderText', `${n} loaded key(s) under "${prefix}" will be deleted. Keys not loaded yet are not touched. This cannot be undone.`).replace('{n}', fmtN(n)).replace('{p}', prefix)
      : keys.slice(0, 5).join(', ') + (n > 5 ? ', …' : '') + '. ' + t('redis.cannotUndo', 'This cannot be undone.'),
    ok: t('redis.delete', 'Delete'), danger: true,
  });
  if (!ok) return;
  let deleted = 0;
  try {
    for (let i = 0; i < keys.length; i += 1000) deleted += (await write({ op: 'del', keys: keys.slice(i, i + 1000) })).n;
  } catch (e) { handleErr(e); }
  keys.forEach(k => { S.keys.delete(k); S.sel.delete(k); });
  if (keys.includes(S.cur)) { S.cur = null; S.kv = null; renderDetail(); }
  renderKeys();
  toast(t('redis.deleted', `Deleted ${deleted} key(s)`).replace('{n}', fmtN(deleted)));
}
async function confirmWrite(title, text, body, okLabel, danger) {
  const ok = await dialog({ title, text, ok: okLabel || t('redis.confirm', 'Confirm'), danger: !!danger });
  if (!ok) return false;
  try { await write(body); toast(t('redis.done', 'Saved')); return true; } catch (e) { handleErr(e); return false; }
}

// ------------------------------------------------------------------ key detail
async function openKey(key) {
  S.cur = key; S.editing = false; S.creating = false; S.sub = null; markActive();
  try { S.kv = await api(`/api/redis/key?id=${S.id}&key=${enc(key)}`); } catch (e) { handleErr(e); return; }
  if (S.kv.type === 'none') S.keys.delete(key);
  else S.keys.set(key, { key, type: S.kv.type, ttl: S.kv.ttl });
  renderDetail();
}
const parseJSON = s => { try { const v = JSON.parse(s); return v !== null && typeof v === 'object' ? { v } : null; } catch { return null; } };

function jsonHTML(v, key, comma, depth) {
  const k = key === undefined ? '' : `<span class="rd-jk">${esc(JSON.stringify(String(key)))}</span><span class="rd-jp">: </span>`;
  const cm = comma ? '<span class="rd-jp">,</span>' : '';
  if (v !== null && typeof v === 'object') {
    const arr = Array.isArray(v), ents = arr ? v.map((x, i) => [i, x]) : Object.entries(v);
    const o = arr ? '[' : '{', c = arr ? ']' : '}';
    if (!ents.length) return `<div class="rd-jl">${k}<span class="rd-jp">${o}${c}</span>${cm}</div>`;
    const kids = ents.map(([kk, x], i) => jsonHTML(x, arr ? undefined : kk, i < ents.length - 1, depth + 1)).join('');
    return `<div class="rd-jn${depth >= 4 ? ' closed' : ''}"><div class="rd-jl"><span class="rd-jt"></span>${k}<span class="rd-jp">${o}</span><span class="rd-jsum">${ents.length} ${arr ? 'items' : 'keys'}<span class="rd-jp">${c}</span>${cm}</span></div><div class="rd-jc">${kids}</div><div class="rd-jl rd-jend"><span class="rd-jp">${c}</span>${cm}</div></div>`;
  }
  const cls = typeof v === 'string' ? 'rd-js' : typeof v === 'number' ? 'rd-jnum' : typeof v === 'boolean' ? 'rd-jbool' : 'rd-jnull';
  return `<div class="rd-jl">${k}<span class="${cls}">${esc(JSON.stringify(v))}</span>${cm}</div>`;
}
const valueView = s => { const j = parseJSON(s); return j ? `<div class="rd-json mono">${jsonHTML(j.v, undefined, false, 0)}</div>` : `<pre class="rd-raw">${esc(s)}</pre>`; };

function rowsTable(head, rows, rmOp) {
  return `<table class="rd-tbl"><thead><tr>${head.map(h => `<th>${esc(h)}</th>`).join('')}${S.write ? '<th></th>' : ''}</tr></thead><tbody>` +
    rows.map((cells, i) => `<tr>${cells.map((c, j) => `<td class="${j === cells.length - 1 ? 'rd-vcell' : 'rd-ccell'} mono" data-v="${i}" title="${esc(String(c).slice(0, 300))}">${esc(String(c).slice(0, 400))}</td>`).join('')}` +
      (S.write ? `<td class="act"><button class="btn sm danger" data-a="${rmOp}" data-i="${i}">${esc(t('redis.remove', 'Remove'))}</button></td>` : '') + '</tr>').join('') +
    '</tbody></table>';
}
function newKeyForm() {
  return `<div class="rd-dhead"><b>${esc(t('redis.newKeyTitle', 'Create a new key'))}</b></div><div class="rd-dbody"><div class="rd-form">
    <label>${esc(t('redis.keyName', 'Key name'))}<input id="rd-nk-name" class="mono" spellcheck="false"></label>
    <label>${esc(t('redis.type', 'Type'))}<select id="rd-nk-type"><option>string</option><option>hash</option><option>list</option><option>set</option><option>zset</option></select></label>
    <label id="rd-nk-fl" hidden>${esc(t('redis.field', 'Field'))}<input id="rd-nk-field" class="mono"></label>
    <label id="rd-nk-sl" hidden>${esc(t('redis.score', 'Score'))}<input id="rd-nk-score" type="number" value="0" step="any"></label>
    <label class="rd-span">${esc(t('redis.value', 'Value'))}<textarea id="rd-nk-val" class="mono" rows="6" spellcheck="false"></textarea></label>
    <div class="rd-span rd-factions"><button class="btn primary" data-a="create">${esc(t('redis.create', 'Create'))}</button><button class="btn" data-a="cancelnew">${esc(t('redis.cancel', 'Cancel'))}</button></div></div></div>`;
}
function renderDetail() {
  const d = $('#rd-detail'), kv = S.kv;
  if (S.creating && S.write) {
    d.innerHTML = newKeyForm();
    $('#rd-nk-type').onchange = () => { const ty = $('#rd-nk-type').value; $('#rd-nk-fl').hidden = ty !== 'hash'; $('#rd-nk-sl').hidden = ty !== 'zset'; };
    $('#rd-nk-name').focus();
    return;
  }
  if (!kv || !S.cur) { d.innerHTML = `<div class="empty">${esc(t('redis.pick', 'Select a key to inspect it.'))}</div>`; return; }
  if (kv.type === 'none') { d.innerHTML = `<div class="empty">${esc(t('redis.gone', 'This key no longer exists.'))}</div>`; renderKeys(); return; }
  const w = S.write;
  let html = `<div class="rd-dhead"><div class="rd-dkey mono">${esc(kv.key)}</div><div class="rd-dmeta">
      <span class="tp ${esc(kv.type)}">${esc(kv.type)}</span><span>TTL ${esc(ttlText(kv.ttl))}</span><span>${esc(t('redis.size', 'size'))} ${fmtN(kv.size)}</span>
      ${kv.truncated ? `<span class="pill warn">${esc(t('redis.truncated', 'truncated'))}</span>` : ''}${kv.binary ? `<span class="pill warn">${esc(t('redis.binary', 'binary'))}</span>` : ''}
      <span class="sp"></span>
      <button class="btn sm" data-a="reload">${svg('refresh')}${esc(t('redis.reload', 'Reload'))}</button>
      <button class="btn sm" data-a="copyname">${esc(t('redis.copyName', 'Copy name'))}</button>
      ${w ? `<button class="btn sm" data-a="rename">${svg('edit')}${esc(t('redis.rename', 'Rename'))}</button><button class="btn sm" data-a="ttl">TTL</button><button class="btn sm danger" data-a="del">${svg('trash')}${esc(t('redis.delete', 'Delete'))}</button>` : ''}
    </div></div><div class="rd-dbody">`;
  const v = kv.value;
  switch (kv.type) {
    case 'string': {
      const s = v ?? '', j = !kv.binary && parseJSON(s), canEdit = w && !kv.truncated && !kv.binary;
      if (S.editing) {
        html += `<textarea id="rd-edit" class="rd-edit mono" spellcheck="false">${esc(s)}</textarea>
          <div class="rd-factions"><button class="btn primary" data-a="saveval">${esc(t('redis.save', 'Save'))}</button><button class="btn" data-a="canceledit">${esc(t('redis.cancel', 'Cancel'))}</button></div>`;
        break;
      }
      html += '<div class="rd-vtools">';
      if (j) html += `<div class="rd-seg sm"><button data-a="fmt" data-f="json" class="${S.fmt === 'json' ? 'on' : ''}">${esc(t('redis.formatted', 'Formatted'))}</button><button data-a="fmt" data-f="raw" class="${S.fmt === 'raw' ? 'on' : ''}">${esc(t('redis.raw', 'Raw'))}</button></div>` +
        (S.fmt === 'json' ? `<button class="btn sm" data-a="expand">${esc(t('redis.expandAll', 'Expand all'))}</button><button class="btn sm" data-a="collapse">${esc(t('redis.collapseAll', 'Collapse all'))}</button>` : '');
      html += '<span class="sp"></span>';
      if (w) html += `<button class="btn sm" data-a="edit" ${canEdit ? '' : `disabled title="${esc(t('redis.noEdit', 'Truncated or binary values cannot be edited'))}"`}>${svg('edit')}${esc(t('redis.edit', 'Edit'))}</button>`;
      html += '</div>' + (j && S.fmt === 'json' ? `<div class="rd-json mono">${jsonHTML(j.v, undefined, false, 0)}</div>` : `<pre class="rd-raw">${esc(s)}</pre>`);
      break;
    }
    case 'hash':
      html += rowsTable([t('redis.field', 'Field'), t('redis.value', 'Value')], v || [], 'hdel');
      if (w) html += `<div class="rd-addrow"><input id="rd-f1" class="mono" placeholder="${esc(t('redis.field', 'Field'))}"><input id="rd-f2" class="mono rd-grow" placeholder="${esc(t('redis.value', 'Value'))}"><button class="btn sm primary" data-a="hset">${esc(t('redis.setField', 'Set field'))}</button></div>`;
      break;
    case 'list':
      html += rowsTable([t('redis.index', 'Index'), t('redis.value', 'Value')], (v || []).map((x, i) => [i, x]), 'lrem');
      if (w) html += `<div class="rd-addrow"><input id="rd-f2" class="mono rd-grow" placeholder="${esc(t('redis.value', 'Value'))}"><button class="btn sm" data-a="lpushl">${esc(t('redis.pushLeft', 'Push left'))}</button><button class="btn sm primary" data-a="lpushr">${esc(t('redis.pushRight', 'Push right'))}</button></div>`;
      break;
    case 'set':
      html += rowsTable([t('redis.member', 'Member')], (v || []).map(x => [x]), 'srem');
      if (w) html += `<div class="rd-addrow"><input id="rd-f2" class="mono rd-grow" placeholder="${esc(t('redis.member', 'Member'))}"><button class="btn sm primary" data-a="sadd">${esc(t('redis.add', 'Add'))}</button></div>`;
      break;
    case 'zset':
      html += rowsTable([t('redis.score', 'Score'), t('redis.member', 'Member')], (v || []).map(([m, s]) => [s, m]), 'zrem');
      if (w) html += `<div class="rd-addrow"><input id="rd-f2" class="mono rd-grow" placeholder="${esc(t('redis.member', 'Member'))}"><input id="rd-f1" type="number" step="any" value="0" class="rd-score" title="${esc(t('redis.score', 'Score'))}"><button class="btn sm primary" data-a="zadd">${esc(t('redis.add', 'Add'))}</button></div>`;
      break;
    default:
      html += `<div class="rd-json mono">${jsonHTML(v, undefined, false, 0)}</div>`;
  }
  if (['hash', 'list', 'set', 'zset'].includes(kv.type)) {
    html += `<div class="rd-sub" id="rd-sub">${S.sub != null ? valueView(S.sub) : `<div class="muted">${esc(t('redis.clickValue', 'Click a value to see all of it.'))}</div>`}</div>`;
  }
  d.innerHTML = html + '</div>';
}

// the displayed value behind a row (what the server needs to remove it)
function rowValue(i) {
  const kv = S.kv, v = kv.value || [];
  switch (kv.type) { case 'hash': return v[i][0]; case 'zset': return v[i][0]; default: return v[i]; }
}
$('#rd-detail').onclick = async e => {
  const jt = e.target.closest('.rd-jt, .rd-jsum');
  if (jt) { jt.closest('.rd-jn').classList.toggle('closed'); return; }
  const cell = e.target.closest('td[data-v]');
  if (cell) {
    const kv = S.kv, i = +cell.dataset.v, row = kv.value[i];
    S.sub = kv.type === 'hash' ? row[1] : kv.type === 'zset' ? row[0] : row;
    $('#rd-sub').innerHTML = valueView(String(S.sub));
    return;
  }
  const b = e.target.closest('[data-a]'); if (!b) return;
  const kv = S.kv, key = kv?.key, a = b.dataset.a, val = id => ($(id)?.value ?? '');
  switch (a) {
    case 'reload': return openKey(S.cur);
    case 'copyname': return copyText(key);
    case 'fmt': S.fmt = b.dataset.f; return renderDetail();
    case 'expand': return $('#rd-detail').querySelectorAll('.rd-jn').forEach(n => n.classList.remove('closed'));
    case 'collapse': return $('#rd-detail').querySelectorAll('.rd-jn').forEach((n, i) => n.classList.toggle('closed', i > 0));
    case 'edit': S.editing = true; renderDetail(); $('#rd-edit').focus(); return;
    case 'canceledit': S.editing = false; return renderDetail();
    case 'cancelnew': S.creating = false; return renderDetail();
    case 'saveval': {
      const nv = $('#rd-edit').value;
      if (await confirmWrite(t('redis.saveValTitle', 'Save the new value?'), t('redis.saveValText', `The value of "${key}" will be replaced (${nv.length} bytes). The current TTL is kept.`).replace('{k}', key).replace('{n}', fmtN(new Blob([nv]).size)), { op: 'set', key, value: nv }, t('redis.save', 'Save'))) openKey(key);
      return;
    }
    case 'rename': {
      const nk = await dialog({ title: t('redis.renameTitle', 'Rename key'), text: `${key} → ${t('redis.renameText', 'new name (an existing key is never overwritten):')}`, input: key, ok: t('redis.rename', 'Rename'), password: false });
      if (!nk || nk === key) return;
      try { await write({ op: 'rename', key, newKey: nk }); } catch (err) { return handleErr(err); }
      const ent = S.keys.get(key); S.keys.delete(key); if (ent) S.keys.set(nk, { ...ent, key: nk });
      toast(t('redis.done', 'Saved')); renderKeys(); return openKey(nk);
    }
    case 'ttl': {
      const s = await dialog({ title: t('redis.ttlTitle', 'Set TTL'), text: `${key}: ${t('redis.ttlText', 'Seconds until it expires. Empty or 0 removes the expiry.')}`, input: kv.ttl >= 0 ? String(kv.ttl) : t('redis.noExpiry', 'no expiry'), ok: t('redis.apply', 'Apply'), password: false });
      if (s === null) return;
      const ttl = s.trim() === '' ? 0 : Number(s);
      if (!Number.isInteger(ttl) || ttl < 0) { toast(t('redis.badTtl', 'TTL must be a whole number of seconds'), 'err'); return; }
      try { await write({ op: 'expire', key, ttl }); toast(t('redis.done', 'Saved')); } catch (err) { return handleErr(err); }
      return openKey(key);
    }
    case 'del': {
      const ok = await dialog({ title: t('redis.delKeyTitle', 'Delete this key?'), text: `${key} (${kv.type}). ${t('redis.cannotUndo', 'This cannot be undone.')}`, ok: t('redis.delete', 'Delete'), danger: true });
      if (!ok) return;
      try { await write({ op: 'del', keys: [key] }); } catch (err) { return handleErr(err); }
      S.keys.delete(key); S.sel.delete(key); S.cur = null; S.kv = null; renderKeys(); renderDetail();
      toast(t('redis.deleted', 'Deleted 1 key(s)').replace('{n}', '1'));
      return;
    }
    case 'hdel': case 'lrem': case 'srem': case 'zrem': {
      const m = String(rowValue(+b.dataset.i));
      const body = { op: a, key, field: m, value: m, count: 1 };
      if (await confirmWrite(`${t('redis.remove', 'Remove')}: ${String(m).slice(0, 80)}`, `${key}. ${t('redis.cannotUndo', 'This cannot be undone.')}`, body, t('redis.remove', 'Remove'), true)) openKey(key);
      return;
    }
    case 'hset': {
      const f = val('#rd-f1'), v2 = val('#rd-f2'); if (!f) return;
      if (await confirmWrite(`${t('redis.setField', 'Set field')}: ${f}`, `${key}[${f}] = ${v2.slice(0, 200)}`, { op: 'hset', key, field: f, value: v2 })) openKey(key);
      return;
    }
    case 'lpushl': case 'lpushr': {
      const v2 = val('#rd-f2'), left = a === 'lpushl';
      if (await confirmWrite(left ? t('redis.pushLeft', 'Push left') : t('redis.pushRight', 'Push right'), `${key} ← ${v2.slice(0, 200)}`, { op: 'lpush', key, value: v2, side: left ? 'left' : 'right' })) openKey(key);
      return;
    }
    case 'sadd': {
      const v2 = val('#rd-f2');
      if (await confirmWrite(t('redis.add', 'Add'), `${key} ← ${v2.slice(0, 200)}`, { op: 'sadd', key, value: v2 })) openKey(key);
      return;
    }
    case 'zadd': {
      const v2 = val('#rd-f2'), sc = Number(val('#rd-f1'));
      if (!Number.isFinite(sc)) return;
      if (await confirmWrite(t('redis.add', 'Add'), `${key} ← ${v2.slice(0, 200)} (${sc})`, { op: 'zadd', key, value: v2, score: sc })) openKey(key);
      return;
    }
    case 'create': return createKey();
  }
};
$('#rd-detail').addEventListener('keydown', e => { if (e.key === 'Enter' && e.target.matches('.rd-addrow input')) e.target.closest('.rd-addrow').querySelector('.btn.primary')?.click(); });

async function createKey() {
  const key = $('#rd-nk-name').value, ty = $('#rd-nk-type').value, v = $('#rd-nk-val').value;
  if (!key) { toast(t('redis.keyName', 'Key name') + '?', 'err'); return; }
  try {
    const cur = await api(`/api/redis/key?id=${S.id}&key=${enc(key)}`);
    if (cur.type !== 'none') { toast(`${t('redis.exists', 'Key already exists')}: ${key}`, 'err'); return; }
  } catch (e) { handleErr(e); return; }
  const body = {
    string: { op: 'set', key, value: v, create: true },
    hash: { op: 'hset', key, field: $('#rd-nk-field').value, value: v },
    list: { op: 'lpush', key, value: v, side: 'right' },
    set: { op: 'sadd', key, value: v },
    zset: { op: 'zadd', key, value: v, score: Number($('#rd-nk-score').value) || 0 },
  }[ty];
  if (await confirmWrite(t('redis.newKeyTitle', 'Create a new key'), t('redis.createText', `Create ${ty} key "${key}"?`).replace('{t}', ty).replace('{k}', key), body, t('redis.create', 'Create'))) {
    S.creating = false; S.keys.set(key, { key, type: ty, ttl: -1 }); renderKeys(); openKey(key);
  }
}

// ------------------------------------------------------------------ server info
async function loadInfo() {
  if (!S.id) return;
  try { renderInfo(await api(`/api/redis/stats?id=${S.id}`)); } catch (e) { handleErr(e); }
}
const fmtUp = s => s < 3600 ? (s / 60 | 0) + 'm' : s < 86400 ? (s / 3600 | 0) + 'h ' + ((s % 3600) / 60 | 0) + 'm' : (s / 86400 | 0) + 'd ' + ((s % 86400) / 3600 | 0) + 'h';
function renderInfo(st) {
  const card = (label, value, extra = '') => `<div class="card rd-stat"><div class="muted">${esc(label)}</div><div class="rd-sv">${esc(value)}</div>${extra}</div>`;
  const hr = st.hitRatio < 0 ? '—' : (st.hitRatio * 100).toFixed(1) + '%';
  const bar = st.hitRatio < 0 ? '' : `<div class="rd-bar"><i style="width:${(st.hitRatio * 100).toFixed(1)}%"></i></div>`;
  const ks = st.keyspace || [];
  $('#rd-infoview').innerHTML = `<div class="toolbar"><button class="btn sm" id="rd-inforef">${svg('refresh')}${esc(t('redis.refresh', 'Refresh'))}</button><span class="muted">${esc(t('redis.autoRefresh', 'refreshes every 5 s while open'))}</span></div>
    <div class="rd-stats">
      ${card(t('redis.version', 'Version'), st.version || '?', `<div class="muted rd-small">${esc(st.os || '')}</div>`)}
      ${card(t('redis.mode', 'Mode / role'), `${st.mode || '?'} / ${st.role || '?'}`)}
      ${card(t('redis.uptime', 'Uptime'), fmtUp(st.uptimeSec || 0))}
      ${card(t('redis.usedMem', 'Used memory'), st.usedMemoryHuman || mb(st.usedMemory))}
      ${card(t('redis.peakMem', 'Peak memory'), st.peakMemoryHuman || mb(st.peakMemory))}
      ${card(t('redis.maxMem', 'Max memory'), st.maxMemory ? mb(st.maxMemory) : t('redis.noLimit', 'no limit'), st.fragmentation ? `<div class="muted rd-small">${esc(t('redis.frag', 'Fragmentation'))} ${st.fragmentation}</div>` : '')}
      ${card(t('redis.clients', 'Connected clients'), fmtN(st.clients), st.blockedClients ? `<div class="muted rd-small">blocked ${fmtN(st.blockedClients)}</div>` : '')}
      ${card(t('redis.ops', 'Ops / sec'), fmtN(st.opsPerSec), `<div class="muted rd-small">${esc(t('redis.totalCmds', 'Total commands'))} ${fmtN(st.totalCommands)}</div>`)}
      ${card(t('redis.hitRatio', 'Hit ratio'), hr, bar + `<div class="muted rd-small">${fmtN(st.hits)} / ${fmtN(st.misses)}</div>`)}
    </div>
    <div class="card"><div class="card-head"><b>${esc(t('redis.keyspace', 'Keys per database'))}</b></div>
      ${ks.length ? `<table><thead><tr><th>${esc(t('redis.db', 'Database'))}</th><th>${esc(t('redis.keys', 'Keys'))}</th><th>${esc(t('redis.expires', 'With expiry'))}</th><th>${esc(t('redis.avgTtl', 'Avg TTL'))}</th></tr></thead><tbody>` +
        ks.map(k => `<tr><td>db${k.db}</td><td>${fmtN(k.keys)}</td><td>${fmtN(k.expires)}</td><td>${k.avgTtl ? esc(ttlText(Math.round(k.avgTtl / 1000))) : '—'}</td></tr>`).join('') + '</tbody></table>'
        : `<div class="empty">${esc(t('redis.noKeyspace', 'No keys yet.'))}</div>`}
    </div>
    <details class="card rd-rawinfo"${S.rawOpen ? ' open' : ''}><summary>${esc(t('redis.rawInfo', 'Raw INFO'))}</summary><pre>${esc(st.raw || '')}</pre></details>`;
  $('#rd-inforef').onclick = loadInfo;
  $('#rd-infoview .rd-rawinfo').ontoggle = e => { S.rawOpen = e.target.open; };
}
every(5000, 'redis', () => { if (S.id && S.view === 'info') loadInfo(); });

// ------------------------------------------------------------------ tab + Ctrl+K
async function initProfiles() {
  await loadProfiles();
  if (S.id) return;
  const last = store.get('rd.prof', '');
  renderProfiles(last);
  const p = selectedProfile(); if (p) fillFrom(p);
}
renderProfiles('');
onTab('redis', () => { if (!S.profiles) initProfiles(); });

async function connectProfile(p) {
  show('redis');
  await loadProfiles();
  renderProfiles(p.name); fillFrom(p); store.set('rd.prof', p.name);
  $('#rd-write').checked = false;
  const pw = await dialog({ title: t('redis.cmdPwTitle', `Connect to ${p.name}`).replace('{p}', p.name), text: t('redis.cmdPwText', 'Password (leave empty if none). Connects read-only.'), input: t('redis.pass', 'Password'), ok: t('redis.connect', 'Connect') });
  if (pw === null) return;
  $('#rd-pass').value = pw;
  return connect();
}
registerCommandProvider(async q => {
  if (!q) return [];
  const ps = await loadProfiles();
  const words = q.toLowerCase().split(/\s+/).filter(Boolean);
  return ps.filter(p => { const s = `redis connect ${p.name} ${p.addr}`.toLowerCase(); return words.every(w => s.includes(w)); })
    .map(p => ({ title: `${t('redis.cmdConnect', 'Connect to Redis')}: ${p.name}`, hint: `${p.addr} / db${p.db}`, run: () => connectProfile(p) }));
});
})();
