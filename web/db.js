'use strict';
// Database tab: read-only viewer for PostgreSQL, MySQL / MariaDB and SQLite.
// Saved profiles (no passwords), schema browser, paged data grid, SQL editor with CSV export.
(() => {
registerStrings('vi', {
  'db.profiles': 'Hồ sơ kết nối đã lưu',
  'db.driver': 'Loại cơ sở dữ liệu',
  'db.host': 'Máy chủ',
  'db.port': 'Cổng',
  'db.user': 'Người dùng',
  'db.database': 'Cơ sở dữ liệu',
  'db.ssl': 'Chế độ TLS / SSL',
  'db.path': 'Đường dẫn tệp .db / .sqlite',
  'db.pass': 'Mật khẩu (chỉ giữ trong bộ nhớ)',
  'db.connect': 'Kết nối',
  'db.connecting': 'Đang kết nối…',
  'db.saveProfile': 'Lưu hồ sơ',
  'db.deleteProfile': 'Xóa hồ sơ',
  'db.disconnect': 'Ngắt kết nối',
  'db.schema': 'Schema',
  'db.filterTables': 'Lọc bảng',
  'db.data': 'Dữ liệu',
  'db.structure': 'Cấu trúc',
  'db.sql': 'SQL',
  'db.sqlph': 'SELECT * FROM … (Ctrl+Enter để chạy; nếu có vùng chọn thì chỉ chạy vùng chọn)',
  'db.run': 'Chạy',
  'db.roNote': 'Giao dịch chỉ đọc, luôn rollback · tối đa 1000 dòng · giới hạn 15 giây',
  'db.copyCsv': 'Sao chép CSV',
  'db.downloadCsv': 'Tải CSV',
  'db.noResult': 'Chạy một truy vấn để xem kết quả.',
  'db.copy': 'Sao chép',
  'db.empty': 'Kết nối tới PostgreSQL, MySQL / MariaDB hoặc một tệp SQLite. Mọi thao tác đều chỉ đọc.',
  'db.newConn': '— Kết nối mới —',
  'db.saveTitle': 'Lưu hồ sơ kết nối',
  'db.saveText': 'Mật khẩu không bao giờ được lưu. Để trống tên để ghi đè hồ sơ đang chọn.',
  'db.profileName': 'Tên hồ sơ',
  'db.save': 'Lưu',
  'db.saved': 'Đã lưu hồ sơ',
  'db.needName': 'Hồ sơ cần có tên',
  'db.delProfTitle': 'Xóa hồ sơ này?',
  'db.delProfText': 'Chỉ xóa hồ sơ đã lưu, cơ sở dữ liệu không bị ảnh hưởng.',
  'db.delete': 'Xóa',
  'db.readonly': 'chỉ đọc',
  'db.noTables': 'Không có bảng nào.',
  'db.pickTable': 'Chọn một bảng ở bên trái.',
  'db.loading': 'Đang tải…',
  'db.filter': 'Lọc',
  'db.apply': 'Áp dụng',
  'db.clear': 'Bỏ lọc',
  'db.value': 'Giá trị',
  'db.prev': 'Trang trước',
  'db.next': 'Trang sau',
  'db.perPage': 'dòng mỗi trang',
  'db.count': 'Đếm',
  'db.refresh': 'Làm mới',
  'db.rows': 'dòng',
  'db.of': 'trên',
  'db.noRows': 'Không có dòng nào.',
  'db.capped': 'đã chạm giới hạn dòng',
  'db.cellsCut': 'giá trị dài đã được rút gọn',
  'db.columns': 'Cột',
  'db.indexes': 'Chỉ mục',
  'db.name': 'Tên',
  'db.type': 'Kiểu',
  'db.nullable': 'Cho phép NULL',
  'db.default': 'Mặc định',
  'db.pk': 'Khóa chính',
  'db.unique': 'Duy nhất',
  'db.primary': 'Chính',
  'db.noIndexes': 'Không có chỉ mục.',
  'db.yes': 'có',
  'db.row': 'dòng',
  'db.copied': 'Đã sao chép',
  'db.running': 'Đang chạy…',
  'db.cmdOpen': 'Mở cơ sở dữ liệu',
  'db.cmdPwTitle': 'Kết nối tới {p}',
  'db.cmdPwText': 'Mật khẩu (để trống nếu không có).',
  'db.lost': 'Kết nối cơ sở dữ liệu đã đóng, hãy kết nối lại.',
  'db.sortHint': 'Bấm để sắp xếp',
  'db.ops.eq': '=', 'db.ops.ne': '≠', 'db.ops.lt': '<', 'db.ops.gt': '>', 'db.ops.le': '≤', 'db.ops.ge': '≥',
  'db.ops.like': 'chứa', 'db.ops.null': 'là NULL', 'db.ops.notnull': 'khác NULL',
});

const D = {
  id: null, driver: '', ver: '', where: '', schema: '', tables: [], table: null, info: null, view: 'data', profiles: null,
  page: { limit: [50, 100, 200, 500].includes(+store.get('db.limit', 100)) ? +store.get('db.limit', 100) : 100, offset: 0, sort: '', desc: false, filter: null },
  data: null, total: null, qres: null, cell: null,
};
const SSL = { postgres: ['prefer', 'disable', 'require', 'verify-ca', 'verify-full'], mysql: ['preferred', 'false', 'true', 'skip-verify'] };
const OPS = [['eq', '='], ['ne', '≠'], ['lt', '<'], ['gt', '>'], ['le', '≤'], ['ge', '≥'], ['like', 'contains'], ['null', 'is NULL'], ['notnull', 'is not NULL']];
const fmtN = n => Number(n || 0).toLocaleString();
const svg = id => `<svg class="i"><use href="#i-${id}"/></svg>`;
const qi = (s) => D.driver === 'mysql' ? '`' + s.replace(/`/g, '``') + '`' : '"' + s.replace(/"/g, '""') + '"';

async function copyText(s) {
  try { await navigator.clipboard.writeText(s); }
  catch { const ta = document.createElement('textarea'); ta.value = s; document.body.appendChild(ta); ta.select(); document.execCommand('copy'); ta.remove(); }
  toast(t('db.copied', 'Copied'));
}

// ------------------------------------------------------------------ connection form + profiles
function setDriver(d) {
  $('#db-driver').value = d;
  const file = d === 'sqlite';
  document.querySelectorAll('#db .db-net').forEach(el => { el.hidden = file; });
  $('#db-path').hidden = !file;
  if (!file) {
    const cur = $('#db-ssl').value;
    $('#db-ssl').innerHTML = SSL[d].map(m => `<option value="${m}">${d === 'mysql' ? 'tls=' : 'sslmode='}${m}</option>`).join('');
    if (SSL[d].includes(cur)) $('#db-ssl').value = cur;
    $('#db-port').placeholder = d === 'mysql' ? '3306' : '5432';
  }
}
$('#db-driver').onchange = () => setDriver($('#db-driver').value);
const formProfile = () => ({
  driver: $('#db-driver').value, host: $('#db-host').value.trim(), port: +$('#db-port').value || 0, user: $('#db-user').value.trim(),
  database: $('#db-name').value.trim(), sslmode: $('#db-driver').value === 'sqlite' ? '' : $('#db-ssl').value, path: $('#db-path').value.trim(),
});
function fillFrom(p) {
  setDriver(p.driver || 'postgres');
  $('#db-host').value = p.host || ''; $('#db-port').value = p.port || ''; $('#db-user').value = p.user || '';
  $('#db-name').value = p.database || ''; $('#db-path').value = p.path || '';
  if (p.sslmode && p.driver !== 'sqlite') $('#db-ssl').value = p.sslmode;
}
async function loadProfiles(force) {
  if (!D.profiles || force) {
    try { D.profiles = await api('/api/db/profiles'); } catch (e) { D.profiles = D.profiles || []; toast(e.message, 'err'); }
  }
  return D.profiles;
}
function renderProfiles(selName) {
  const ps = D.profiles || [];
  $('#db-prof').innerHTML = `<option value="">${esc(t('db.newConn', '— New connection —'))}</option>` + ps.map((p, i) => `<option value="${i}">${esc(p.name)}</option>`).join('');
  const i = selName ? ps.findIndex(p => p.name === selName) : -1;
  $('#db-prof').value = i >= 0 ? String(i) : '';
  $('#db-delprof').hidden = i < 0;
}
const selectedProfile = () => $('#db-prof').value === '' ? null : (D.profiles || [])[+$('#db-prof').value] || null;
$('#db-prof').onchange = () => { const p = selectedProfile(); if (p) { fillFrom(p); store.set('db.prof', p.name); } $('#db-delprof').hidden = !p; };
$('#db-save').onclick = async () => {
  const cur = selectedProfile();
  const name = await dialog({ title: t('db.saveTitle', 'Save connection profile'), text: t('db.saveText', 'The password is never saved. Leave the name empty to overwrite the selected profile.'), input: cur ? cur.name : t('db.profileName', 'Profile name'), ok: t('db.save', 'Save'), password: false });
  if (name === null) return;
  const nm = name.trim() || (cur ? cur.name : '');
  if (!nm) { toast(t('db.needName', 'A profile needs a name'), 'err'); return; }
  const list = (D.profiles || []).filter(p => p.name.toLowerCase() !== nm.toLowerCase());
  list.push({ name: nm, ...formProfile() });
  list.sort((a, b) => a.name.localeCompare(b.name));
  try { D.profiles = await put('/api/db/profiles', list); renderProfiles(nm); store.set('db.prof', nm); toast(t('db.saved', 'Profile saved')); }
  catch (e) { toast(e.message, 'err'); }
};
$('#db-delprof').onclick = async () => {
  const cur = selectedProfile(); if (!cur) return;
  const ok = await dialog({ title: t('db.delProfTitle', 'Delete this profile?'), text: `${cur.name}: ${t('db.delProfText', 'Only the saved profile is removed; the database is not touched.')}`, ok: t('db.delete', 'Delete'), danger: true });
  if (!ok) return;
  try { D.profiles = await put('/api/db/profiles', D.profiles.filter(p => p !== cur)); renderProfiles(''); } catch (e) { toast(e.message, 'err'); }
};

function renderStatus() {
  $('#db-status').innerHTML = D.id ? `<span class="pill ok" title="${esc(D.ver)}">${esc(D.ver.split(' on ')[0].split(',')[0].slice(0, 40))}</span> <span class="mono">${esc(D.where)}</span> <span class="pill db-ropill">${esc(t('db.readonly', 'read-only'))}</span>` : '';
}
function showBody(on) { $('#db-body').hidden = !on; $('#db-empty').hidden = on; $('#db-disconnect').hidden = !on; }
async function connect() {
  const f = formProfile(), cur = selectedProfile();
  $('#db-status').textContent = t('db.connecting', 'Connecting…');
  try {
    if (D.id) { post('/api/db/close', { id: D.id }).catch(() => {}); D.id = null; }
    const r = await post('/api/db/open', { profile: { name: cur ? cur.name : '', ...f }, password: $('#db-pass').value });
    $('#db-pass').value = '';
    Object.assign(D, { id: r.id, driver: r.driver, ver: r.version || r.driver, schema: r.schema, table: null, info: null, data: null, total: null, qres: null });
    D.where = f.driver === 'sqlite' ? f.path.split(/[\\/]/).pop() : `${f.user ? f.user + '@' : ''}${f.host || '127.0.0.1'}${f.database ? '/' + f.database : ''}`;
    renderStatus(); showBody(true); closeCell(); renderData(); renderStruct(); renderQRes();
    await loadSchemas();
  } catch (e) { renderStatus(); toast(e.message, 'err'); }
}
function disconnected(msg) {
  if (D.id) post('/api/db/close', { id: D.id }).catch(() => {});
  Object.assign(D, { id: null, table: null, info: null, data: null, qres: null });
  renderStatus(); showBody(false);
  if (msg) toast(msg, 'err');
}
function handleErr(e, el) {
  if (/unknown database connection|expired/i.test(e.message)) { disconnected(t('db.lost', 'The database connection was closed; connect again.')); return; }
  if (el) el.innerHTML = `<div class="db-err">${esc(e.message)}</div>`;
  else toast(e.message, 'err');
}
$('#db-connect').onclick = connect;
$('#db-pass').onkeydown = e => { if (e.key === 'Enter') connect(); };
$('#db-path').onkeydown = e => { if (e.key === 'Enter') connect(); };
$('#db-disconnect').onclick = () => disconnected();

// ------------------------------------------------------------------ schema browser
async function loadSchemas() {
  let schemas = [];
  try { schemas = await api(`/api/db/schemas?id=${D.id}`); } catch (e) { return handleErr(e); }
  if (D.schema && !schemas.includes(D.schema)) D.schema = '';
  if (!D.schema) D.schema = schemas[0] || '';
  $('#db-schema').innerHTML = schemas.map(s => `<option>${esc(s)}</option>`).join('');
  $('#db-schema').value = D.schema;
  $('#db-schema').hidden = schemas.length <= 1 && D.driver === 'sqlite';
  return loadTables();
}
async function loadTables() {
  $('#db-tables').innerHTML = `<div class="empty">${esc(t('db.loading', 'Loading…'))}</div>`;
  try { D.tables = await api(`/api/db/tables?id=${D.id}&schema=${encodeURIComponent(D.schema)}`); } catch (e) { D.tables = []; return handleErr(e, $('#db-tables')); }
  renderTables();
}
function renderTables() {
  const q = $('#db-tq').value.trim().toLowerCase();
  const ts = D.tables.filter(x => !q || x.name.toLowerCase().includes(q));
  $('#db-tables').innerHTML = ts.length ? ts.slice(0, 3000).map(x => `<div class="db-t${x.name === D.table ? ' on' : ''}" data-t="${esc(x.name)}" title="${esc(x.name)}">
      <span class="db-ti ${x.type === 'view' ? 'v' : ''}">${x.type === 'view' ? 'V' : 'T'}</span><span class="db-tn">${esc(x.name)}</span>${x.rows >= 0 ? `<span class="db-tr">~${fmtN(x.rows)}</span>` : ''}</div>`).join('')
    : `<div class="empty">${esc(t('db.noTables', 'No tables.'))}</div>`;
}
$('#db-schema').onchange = () => { D.schema = $('#db-schema').value; D.table = null; loadTables(); renderData(); renderStruct(); };
$('#db-tq').oninput = renderTables;
$('#db-tables').onclick = e => { const r = e.target.closest('.db-t'); if (r) openTable(r.dataset.t); };

async function openTable(name) {
  Object.assign(D, { table: name, info: null, data: null, total: null });
  Object.assign(D.page, { offset: 0, sort: '', desc: false, filter: null });
  renderTables(); closeCell();
  $('#db-cur').textContent = `${D.schema}.${name}`;
  if (!$('#db-sqltext').value.trim()) $('#db-sqltext').value = `SELECT * FROM ${D.driver === 'sqlite' && D.schema === 'main' ? '' : qi(D.schema) + '.'}${qi(name)} LIMIT 100`;
  if (D.view === 'sql') setView('data');
  renderData(); renderStruct();
  try { D.info = await api(`/api/db/table?id=${D.id}&schema=${encodeURIComponent(D.schema)}&table=${encodeURIComponent(name)}`); }
  catch (e) { handleErr(e, $('#db-struct')); }
  if (D.table !== name) return;
  renderStruct(); renderData(); loadRows();
}

// ------------------------------------------------------------------ views
function setView(v) {
  D.view = v;
  $('#db-views').querySelectorAll('button').forEach(x => x.classList.toggle('on', x.dataset.v === v));
  $('#db-data').hidden = v !== 'data'; $('#db-struct').hidden = v !== 'struct'; $('#db-sql').hidden = v !== 'sql';
  if (v === 'sql') $('#db-sqltext').focus();
}
$('#db-views').onclick = e => { const b = e.target.closest('button[data-v]'); if (b) setView(b.dataset.v); };

// ------------------------------------------------------------------ grid (shared by table data and SQL results)
function cellHTML(v) {
  if (v === null || v === undefined) return '<span class="db-null">NULL</span>';
  const s = typeof v === 'string' ? v : String(v);
  return esc(s.length > 200 ? s.slice(0, 200) + '…' : s);
}
function gridHTML(res, opts = {}) {
  if (!res.columns.length) return `<div class="empty">${esc(t('db.noRows', 'No rows.'))}</div>`;
  const off = opts.offset || 0;
  const head = res.columns.map((c, i) => {
    const arrow = opts.sortable && D.page.sort === c.name ? (D.page.desc ? ' ▼' : ' ▲') : '';
    return `<th${opts.sortable ? ` class="db-sortable" data-sort="${i}" title="${esc(c.type)} · ${esc(t('db.sortHint', 'Click to sort'))}"` : ` title="${esc(c.type)}"`}>${esc(c.name)}${arrow}</th>`;
  }).join('');
  const body = res.rows.map((r, ri) => `<tr><td class="db-rn">${off + ri + 1}</td>${r.map((v, ci) => `<td data-r="${ri}" data-c="${ci}" class="${typeof v === 'number' ? 'db-num' : ''}">${cellHTML(v)}</td>`).join('')}</tr>`).join('');
  return `<table class="db-grid"><thead><tr><th class="db-rn">#</th>${head}</tr></thead><tbody>${body || `<tr><td colspan="${res.columns.length + 1}" class="empty">${esc(t('db.noRows', 'No rows.'))}</td></tr>`}</tbody></table>`;
}
function csvOf(res) {
  const f = v => { if (v === null || v === undefined) return ''; const s = String(v); return /[",\r\n]|^\s|\s$/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s; };
  return [res.columns.map(c => f(c.name)).join(','), ...res.rows.map(r => r.map(f).join(','))].join('\r\n') + '\r\n';
}
function downloadCSV(res, name) {
  const a = document.createElement('a');
  a.href = URL.createObjectURL(new Blob(['﻿' + csvOf(res)], { type: 'text/csv;charset=utf-8' }));
  a.download = (name || 'query').replace(/[^\w.-]+/g, '_') + '.csv';
  document.body.appendChild(a); a.click(); a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 2000);
}

// cell detail pane
function showCell(res, r, c, rowNo) {
  const col = res.columns[c], v = res.rows[r][c];
  D.cell = v;
  $('#db-cell-t').textContent = `${col.name} · ${col.type || '?'} · ${t('db.row', 'row')} ${rowNo}`;
  let html;
  if (v === null || v === undefined) html = '<span class="db-null">NULL</span>';
  else {
    let pretty = null;
    if (typeof v === 'string' && /^\s*[[{]/.test(v)) { try { pretty = JSON.stringify(JSON.parse(v), null, 2); } catch {} }
    html = `<pre>${esc(pretty ?? String(v))}</pre>`;
  }
  $('#db-cell-v').innerHTML = html;
  $('#db-cell').hidden = false;
}
function closeCell() { $('#db-cell').hidden = true; D.cell = null; document.querySelectorAll('#db .db-grid td.sel').forEach(x => x.classList.remove('sel')); }
$('#db-cell-x').onclick = closeCell;
$('#db-cell-copy').onclick = () => copyText(D.cell === null || D.cell === undefined ? 'NULL' : String(D.cell));
function gridClick(e, res, offset) {
  const td = e.target.closest('td[data-c]');
  if (!td || !res) return;
  document.querySelectorAll('#db .db-grid td.sel').forEach(x => x.classList.remove('sel'));
  td.classList.add('sel');
  showCell(res, +td.dataset.r, +td.dataset.c, offset + +td.dataset.r + 1);
}
document.addEventListener('keydown', e => { if (e.key === 'Escape' && current === 'db' && !$('#db-cell').hidden && !document.querySelector('dialog[open]')) closeCell(); });

// ------------------------------------------------------------------ data tab
function renderData() {
  const el = $('#db-data');
  if (!D.id || !D.table) { el.innerHTML = `<div class="empty">${esc(t('db.pickTable', 'Pick a table on the left.'))}</div>`; return; }
  const cols = D.info ? D.info.columns : [];
  const f = D.page.filter || {};
  el.innerHTML = `<div class="db-dtools">
      <span class="muted">${esc(t('db.filter', 'Filter'))}</span>
      <select id="db-fcol">${cols.map(c => `<option${c.name === f.column ? ' selected' : ''}>${esc(c.name)}</option>`).join('')}</select>
      <select id="db-fop">${OPS.map(([k, l]) => `<option value="${k}"${k === f.op ? ' selected' : ''}>${esc(t('db.ops.' + k, l))}</option>`).join('')}</select>
      <input id="db-fval" class="db-fval" placeholder="${esc(t('db.value', 'Value'))}" value="${esc(f.value || '')}">
      <button class="btn sm" id="db-fapply">${esc(t('db.apply', 'Apply'))}</button>
      <button class="btn sm" id="db-fclear"${D.page.filter ? '' : ' hidden'}>${esc(t('db.clear', 'Clear filter'))}</button>
      <span class="sp"></span>
      <span id="db-range" class="muted"></span>
      <button class="btn sm" id="db-prev" title="${esc(t('db.prev', 'Previous page'))}">‹</button>
      <button class="btn sm" id="db-next" title="${esc(t('db.next', 'Next page'))}">›</button>
      <select id="db-limit" title="${esc(t('db.perPage', 'rows per page'))}">${[50, 100, 200, 500].map(n => `<option${n === D.page.limit ? ' selected' : ''}>${n}</option>`).join('')}</select>
      <button class="btn sm" id="db-count">${esc(t('db.count', 'Count'))}</button>
      <button class="btn sm" id="db-reload" title="${esc(t('db.refresh', 'Refresh'))}">${svg('refresh')}</button>
      <button class="btn sm" id="db-dcopy">${esc(t('db.copyCsv', 'Copy CSV'))}</button>
      <button class="btn sm" id="db-ddl">${esc(t('db.downloadCsv', 'Download CSV'))}</button>
    </div><div id="db-grid" class="db-gridwrap"><div class="empty">${esc(t('db.loading', 'Loading…'))}</div></div>`;
  const fop = () => { const o = $('#db-fop').value; $('#db-fval').hidden = o === 'null' || o === 'notnull'; };
  $('#db-fop').onchange = fop; fop();
  const apply = () => { D.page.filter = { column: $('#db-fcol').value, op: $('#db-fop').value, value: $('#db-fval').value }; D.page.offset = 0; D.total = null; $('#db-fclear').hidden = false; loadRows(); };
  $('#db-fapply').onclick = apply;
  $('#db-fval').onkeydown = e => { if (e.key === 'Enter') apply(); };
  $('#db-fclear').onclick = () => { D.page.filter = null; D.page.offset = 0; D.total = null; renderData(); loadRows(); };
  $('#db-prev').onclick = () => { D.page.offset = Math.max(0, D.page.offset - D.page.limit); loadRows(); };
  $('#db-next').onclick = () => { D.page.offset += D.page.limit; loadRows(); };
  $('#db-limit').onchange = () => { D.page.limit = +$('#db-limit').value; store.set('db.limit', D.page.limit); D.page.offset = 0; loadRows(); };
  $('#db-reload').onclick = loadRows;
  $('#db-count').onclick = countRows;
  $('#db-dcopy').onclick = () => D.data && copyText(csvOf(D.data));
  $('#db-ddl').onclick = () => D.data && downloadCSV(D.data, D.table);
  $('#db-grid').onclick = e => {
    const th = e.target.closest('th[data-sort]');
    if (th && D.data) {
      const name = D.data.columns[+th.dataset.sort].name;
      if (D.page.sort !== name) Object.assign(D.page, { sort: name, desc: false });
      else if (!D.page.desc) D.page.desc = true;
      else Object.assign(D.page, { sort: '', desc: false });
      D.page.offset = 0; loadRows(); return;
    }
    gridClick(e, D.data, D.page.offset);
  };
  if (D.data) paintRows();
}
const rowsBody = () => ({ id: D.id, schema: D.schema, table: D.table, limit: D.page.limit, offset: D.page.offset, sort: D.page.sort, desc: D.page.desc, filter: D.page.filter });
async function loadRows() {
  if (!D.id || !D.table || !$('#db-grid')) return;
  const want = D.table;
  try {
    const r = await post('/api/db/rows', rowsBody());
    if (D.table !== want) return;
    D.data = r;
  } catch (e) { D.data = null; return handleErr(e, $('#db-grid')); }
  paintRows();
}
function paintRows() {
  const r = D.data; if (!r || !$('#db-grid')) return;
  closeCell();
  $('#db-grid').innerHTML = gridHTML(r, { sortable: true, offset: D.page.offset });
  const a = r.rows.length ? D.page.offset + 1 : 0, b = D.page.offset + r.rows.length;
  $('#db-range').textContent = `${fmtN(a)}–${fmtN(b)}${D.total != null ? ` ${t('db.of', 'of')} ${fmtN(D.total)}` : ''} · ${r.elapsedMs} ms`;
  $('#db-prev').disabled = D.page.offset === 0;
  $('#db-next').disabled = !r.truncated;
}
async function countRows() {
  try { const r = await post('/api/db/count', rowsBody()); D.total = r.count; paintRows(); } catch (e) { handleErr(e); }
}

// ------------------------------------------------------------------ structure tab
function renderStruct() {
  const el = $('#db-struct');
  if (!D.id || !D.table) { el.innerHTML = `<div class="empty">${esc(t('db.pickTable', 'Pick a table on the left.'))}</div>`; return; }
  if (!D.info) { el.innerHTML = `<div class="empty">${esc(t('db.loading', 'Loading…'))}</div>`; return; }
  const yes = esc(t('db.yes', 'yes'));
  el.innerHTML = `<div class="card-head"><b>${esc(t('db.columns', 'Columns'))} (${D.info.columns.length})</b></div>
    <table class="db-stbl"><thead><tr><th>#</th><th>${esc(t('db.name', 'Name'))}</th><th>${esc(t('db.type', 'Type'))}</th><th>${esc(t('db.nullable', 'Nullable'))}</th><th>${esc(t('db.default', 'Default'))}</th><th>${esc(t('db.pk', 'Primary key'))}</th></tr></thead><tbody>
    ${D.info.columns.map((c, i) => `<tr><td class="muted">${i + 1}</td><td class="mono"><b>${esc(c.name)}</b></td><td class="mono">${esc(c.type)}</td><td>${c.nullable ? yes : '<span class="muted">NOT NULL</span>'}</td><td class="mono db-def">${c.default == null ? '<span class="db-null">—</span>' : esc(c.default)}</td><td>${c.pk ? `<span class="pill ok">PK</span>` : ''}</td></tr>`).join('')}
    </tbody></table>
    <div class="card-head db-ihead"><b>${esc(t('db.indexes', 'Indexes'))} (${D.info.indexes.length})</b></div>
    ${D.info.indexes.length ? `<table class="db-stbl"><thead><tr><th>${esc(t('db.name', 'Name'))}</th><th>${esc(t('db.columns', 'Columns'))}</th><th>${esc(t('db.unique', 'Unique'))}</th><th>${esc(t('db.primary', 'Primary'))}</th></tr></thead><tbody>
      ${D.info.indexes.map(x => `<tr><td class="mono">${esc(x.name)}</td><td class="mono">${esc(x.columns.join(', '))}</td><td>${x.unique ? yes : ''}</td><td>${x.primary ? `<span class="pill ok">PK</span>` : ''}</td></tr>`).join('')}
    </tbody></table>` : `<div class="empty">${esc(t('db.noIndexes', 'No indexes.'))}</div>`}`;
}

// ------------------------------------------------------------------ SQL tab
$('#db-sqltext').value = store.get('db.sql', '');
$('#db-sqltext').oninput = () => store.set('db.sql', $('#db-sqltext').value.slice(0, 20000));
$('#db-sqltext').onkeydown = e => {
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') { e.preventDefault(); runSQL(); }
  else if (e.key === 'Tab' && !e.shiftKey) { e.preventDefault(); document.execCommand('insertText', false, '  '); }
};
$('#db-run').onclick = runSQL;
async function runSQL() {
  if (!D.id) return;
  const ta = $('#db-sqltext');
  const sel = ta.value.slice(ta.selectionStart, ta.selectionEnd);
  const sql = (sel.trim() ? sel : ta.value).trim();
  if (!sql) return;
  $('#db-qstat').textContent = t('db.running', 'Running…');
  $('#db-run').disabled = true;
  try {
    D.qres = await post('/api/db/query', { id: D.id, sql });
    const r = D.qres;
    $('#db-qstat').textContent = `${fmtN(r.rows.length)} ${t('db.rows', 'rows')} · ${r.elapsedMs} ms${r.truncated ? ' · ' + t('db.capped', 'row cap reached') : ''}${r.cellsCut ? ' · ' + t('db.cellsCut', 'long values shortened') : ''}`;
    renderQRes();
  } catch (e) {
    D.qres = null; $('#db-qstat').textContent = '';
    if (/unknown database connection|expired/i.test(e.message)) handleErr(e);
    else { renderQRes(); $('#db-qres').innerHTML = `<div class="db-err">${esc(e.message)}</div>`; }
  } finally { $('#db-run').disabled = false; }
}
function renderQRes() {
  const r = D.qres;
  $('#db-qcopy').disabled = $('#db-qdl').disabled = !r;
  $('#db-qres').innerHTML = r ? gridHTML(r) : `<div class="empty">${esc(t('db.noResult', 'Run a query to see results.'))}</div>`;
}
$('#db-qres').onclick = e => gridClick(e, D.qres, 0);
$('#db-qcopy').onclick = () => D.qres && copyText(csvOf(D.qres));
$('#db-qdl').onclick = () => D.qres && downloadCSV(D.qres, 'query');

// ------------------------------------------------------------------ tab + Ctrl+K
setDriver('postgres');
renderProfiles('');
async function initProfiles() {
  await loadProfiles();
  if (D.id) return;
  const last = store.get('db.prof', '');
  renderProfiles(last);
  const p = selectedProfile(); if (p) fillFrom(p);
}
onTab('db', () => { if (!D.profiles) initProfiles(); });

async function openProfile(p) {
  show('db');
  await loadProfiles();
  renderProfiles(p.name); fillFrom(p); store.set('db.prof', p.name);
  if (p.driver !== 'sqlite') {
    const pw = await dialog({ title: t('db.cmdPwTitle', `Connect to ${p.name}`).replace('{p}', p.name), text: t('db.cmdPwText', 'Password (leave empty if none).'), input: t('db.pass', 'Password'), ok: t('db.connect', 'Connect') });
    if (pw === null) return;
    $('#db-pass').value = pw;
  }
  return connect();
}
registerCommandProvider(async q => {
  if (!q) return [];
  const ps = await loadProfiles();
  const words = q.toLowerCase().split(/\s+/).filter(Boolean);
  return ps.filter(p => { const s = `database db sql open ${p.name} ${p.driver} ${p.host || ''} ${p.database || ''} ${p.path || ''}`.toLowerCase(); return words.every(w => s.includes(w)); })
    .map(p => ({ title: `${t('db.cmdOpen', 'Open database')}: ${p.name}`, hint: p.driver === 'sqlite' ? p.path : `${p.driver} · ${p.host}${p.database ? '/' + p.database : ''}`, run: () => openProfile(p) }));
});
})();
