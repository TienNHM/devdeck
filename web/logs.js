'use strict';
// Logs tab (feature module): saved log files, tail + follow over server-sent events, substring / regex
// filters, level highlighting (plain text and JSON logs), pause / resume, wrap, bounded DOM.
// Exposes window.logsViewer(host, { url: n => streamUrl }) which docker.js reuses for container logs.
(() => {
registerStrings('vi', {
  'logs.add': 'Thêm tệp',
  'logs.path': 'Đường dẫn tệp log, ví dụ C:\\app\\logs\\app.log',
  'logs.files': 'Tệp log',
  'logs.none': 'Chưa có tệp log nào. Nhập đường dẫn ở trên rồi bấm Thêm tệp.',
  'logs.pick': 'Chọn một tệp log ở bên trái để xem.',
  'logs.remove': 'Gỡ',
  'logs.removeQ': 'Gỡ "{name}" khỏi danh sách?',
  'logs.removeText': 'Chỉ gỡ khỏi danh sách của devdeck, tệp trên đĩa vẫn giữ nguyên.',
  'logs.added': 'Đã thêm {name}',
  'logs.removed': 'Đã gỡ {name}',
  'logs.missing': 'không tìm thấy tệp',
  'logs.needPath': 'Hãy nhập đường dẫn tệp',
  'logs.palTail': 'Xem log tệp',
  'logs.palAdd': 'Log: thêm tệp log',
  'lv.filter': 'Lọc dòng…',
  'lv.regex': 'Regex',
  'lv.case': 'Phân biệt hoa thường',
  'lv.levelAll': 'Mọi mức',
  'lv.levelError': 'Chỉ lỗi',
  'lv.levelWarn': 'Cảnh báo trở lên',
  'lv.levelInfo': 'Info trở lên',
  'lv.lines': '{n} dòng cuối',
  'lv.follow': 'Theo dõi',
  'lv.stop': 'Dừng',
  'lv.pause': 'Tạm dừng',
  'lv.resume': 'Tiếp tục',
  'lv.clear': 'Xoá',
  'lv.wrap': 'Xuống dòng',
  'lv.bottom': 'Xuống cuối',
  'lv.live': 'Đang theo dõi',
  'lv.idle': 'Đã dừng',
  'lv.pausedN': 'Tạm dừng · {n} dòng mới đang chờ',
  'lv.count': '{shown}/{total} dòng',
  'lv.badRegex': 'Regex không hợp lệ',
  'lv.disconnected': 'Mất kết nối với luồng log.',
  'lv.empty': 'Chưa có dòng nào.',
  'lv.noMatch': 'Không có dòng nào khớp bộ lọc.',
  'lv.suspended': 'Đã ngắt theo dõi khi rời tab; quay lại tab sẽ tải lại.',
});
const tf = (key, en, vars = {}) => Object.entries(vars).reduce((s, [k, v]) => s.split(`{${k}}`).join(v), t(key, en));

// ------------------------------------------------------------------ level detection
const LEVEL_RANK = { debug: 1, info: 2, warn: 3, error: 4 };
function normLevel(v) {
  if (typeof v === 'number') return v >= 50 ? 'error' : v >= 40 ? 'warn' : v >= 30 ? 'info' : 'debug'; // pino / bunyan
  const s = String(v || '').toLowerCase();
  if (/^(fatal|panic|crit|critical|error|err|severe|emerg|emergency|alert|e)$/.test(s)) return 'error';
  if (/^(warn|warning|w)$/.test(s)) return 'warn';
  if (/^(info|information|informational|notice|i)$/.test(s)) return 'info';
  if (/^(debug|trace|verbose|fine|finer|finest|d|t)$/.test(s)) return 'debug';
  return '';
}
const kvRe = /\b(?:level|lvl|severity|loglevel)\s*[=:]\s*"?([A-Za-z]+)/i;
const brRe = /[[(<](FATAL|PANIC|CRIT(?:ICAL)?|ERROR|ERR|WARN(?:ING)?|INFO|NOTICE|DEBUG|TRACE)[\])>]/i;
const wordRe = /\b(FATAL|PANIC|CRIT(?:ICAL)?|ERROR|ERR|SEVERE|WARN(?:ING)?|INFO|NOTICE|DEBUG|TRACE)\b/;
function detectLevel(s) {
  if (s.charCodeAt(0) === 123 && s.length < 65536) { // '{': JSON log line
    try {
      const o = JSON.parse(s);
      const v = o.level ?? o.lvl ?? o.severity ?? o.levelname ?? o.log_level ?? o['log.level'] ?? o['@l'] ?? o.loglevel ?? (o.log && o.log.level);
      if (v !== undefined) { const l = normLevel(v); if (l) return l; }
    } catch {}
  }
  const m = kvRe.exec(s) || brRe.exec(s) || wordRe.exec(s);
  return m ? normLevel(m[1]) : '';
}

// ------------------------------------------------------------------ viewer component
function logsViewer(host, { url, maxLines = 5000 }) {
  const el = document.createElement('div');
  el.className = 'logs-v';
  const nDef = store.get('logs.n', '200');
  el.innerHTML = `
    <div class="toolbar logs-bar">
      <label class="search"><svg class="i"><use href="#i-search"/></svg><input class="logs-q" placeholder="${esc(t('lv.filter', 'Filter lines…'))}" spellcheck="false" autocomplete="off"></label>
      <label class="check"><input type="checkbox" class="logs-re"> ${esc(t('lv.regex', 'Regex'))}</label>
      <label class="check" title="${esc(t('lv.case', 'Match case'))}"><input type="checkbox" class="logs-case"> Aa</label>
      <select class="logs-level">
        <option value="0">${esc(t('lv.levelAll', 'All levels'))}</option>
        <option value="4">${esc(t('lv.levelError', 'Errors only'))}</option>
        <option value="3">${esc(t('lv.levelWarn', 'Warnings and up'))}</option>
        <option value="2">${esc(t('lv.levelInfo', 'Info and up'))}</option>
      </select>
      <select class="logs-n">${[100, 200, 500, 1000, 5000].map(n => `<option value="${n}"${String(n) === nDef ? ' selected' : ''}>${esc(tf('lv.lines', 'Last {n} lines', { n }))}</option>`).join('')}</select>
      <span class="sp"></span>
      <span class="logs-stat muted"></span>
      <button class="btn sm logs-follow"></button>
      <button class="btn sm logs-pause"></button>
      <button class="btn sm logs-clear">${esc(t('lv.clear', 'Clear'))}</button>
      <label class="check"><input type="checkbox" class="logs-wrapchk"> ${esc(t('lv.wrap', 'Wrap'))}</label>
    </div>
    <div class="logs-box">
      <div class="logs-body mono" tabindex="0"></div>
      <div class="logs-ph muted"></div>
      <button class="btn sm logs-bottom" hidden>↓ ${esc(t('lv.bottom', 'Jump to bottom'))}</button>
    </div>`;
  host.appendChild(el);
  const q = s => el.querySelector(s);
  const body = q('.logs-body'), ph = q('.logs-ph'), stat = q('.logs-stat'), btnFollow = q('.logs-follow'), btnPause = q('.logs-pause'), btnBottom = q('.logs-bottom');
  const wrapChk = q('.logs-wrapchk');
  wrapChk.checked = store.get('logs.wrap', '0') === '1';
  body.classList.toggle('logs-wrap', wrapChk.checked);

  let all = [], queue = [], es = null, paused = false, suspended = false, testRe = null, hlRe = null, minLevel = 0, shown = 0;

  const atBottom = () => body.scrollHeight - body.scrollTop - body.clientHeight < 40;
  const visible = o => o.note || ((!minLevel || (LEVEL_RANK[o.l] || 0) >= minLevel) && (!testRe || testRe.test(o.t)));
  function hl(text) {
    if (!hlRe) return esc(text);
    let out = '', i = 0, m, guard = 0;
    hlRe.lastIndex = 0;
    while ((m = hlRe.exec(text)) && guard++ < 500) {
      if (!m[0].length) { hlRe.lastIndex++; continue; }
      out += esc(text.slice(i, m.index)) + '<mark>' + esc(m[0]) + '</mark>';
      i = m.index + m[0].length;
    }
    return out + esc(text.slice(i));
  }
  function lineEl(o) {
    const d = document.createElement('div');
    d.className = o.note ? 'logs-note logs-note-' + o.note : 'logs-ln' + (o.l ? ' logs-' + o.l : '');
    if (o.note) d.textContent = o.t; else d.innerHTML = hl(o.t) || ' ';
    o.el = d;
    return d;
  }
  function updateStat() {
    if (paused) stat.textContent = tf('lv.pausedN', 'Paused · {n} new lines waiting', { n: queue.length });
    else stat.textContent = (es ? '● ' + t('lv.live', 'Live') : t('lv.idle', 'Stopped')) + ' · ' + tf('lv.count', '{shown}/{total} lines', { shown, total: all.filter(o => !o.note).length });
    stat.classList.toggle('logs-live', !!es && !paused);
    ph.hidden = shown > 0 || all.some(o => o.note);
    ph.textContent = all.length ? t('lv.noMatch', 'No lines match the filter.') : t('lv.empty', 'No lines yet.');
  }
  function buttons() {
    btnFollow.textContent = es ? t('lv.stop', 'Stop') : t('lv.follow', 'Follow');
    btnFollow.classList.toggle('primary', !es);
    btnPause.textContent = paused ? t('lv.resume', 'Resume') : t('lv.pause', 'Pause');
    btnPause.disabled = !es && !paused;
    updateStat();
  }
  function trim() {
    if (all.length <= maxLines) return;
    const drop = all.splice(0, all.length - maxLines);
    for (const o of drop) if (o.el) { if (o.el.isConnected && !o.note) shown--; o.el.remove(); o.el = null; }
  }
  function push(objs) {
    if (!objs.length) return;
    const stick = atBottom();
    const frag = document.createDocumentFragment();
    for (const o of objs) {
      all.push(o);
      if (visible(o)) { frag.appendChild(lineEl(o)); if (!o.note) shown++; }
    }
    body.appendChild(frag);
    trim();
    if (stick) body.scrollTop = body.scrollHeight;
    btnBottom.hidden = atBottom();
    updateStat();
  }
  function render() {
    body.textContent = '';
    shown = 0;
    const frag = document.createDocumentFragment();
    for (const o of all) { o.el = null; if (visible(o)) { frag.appendChild(lineEl(o)); if (!o.note) shown++; } }
    body.appendChild(frag);
    body.scrollTop = body.scrollHeight;
    btnBottom.hidden = true;
    updateStat();
  }
  function addLines(lines) {
    const objs = lines.map(s => ({ t: String(s), l: detectLevel(String(s)) }));
    if (paused) {
      queue.push(...objs);
      if (queue.length > maxLines) queue.splice(0, queue.length - maxLines);
      updateStat();
      return;
    }
    push(objs);
  }
  const note = (text, kind = 'info') => push([{ t: text, note: kind }]);

  function compile() {
    const raw = q('.logs-q').value, cs = q('.logs-case').checked, isRe = q('.logs-re').checked;
    q('.logs-q').classList.remove('logs-bad');
    q('.logs-q').title = '';
    testRe = hlRe = null;
    if (raw) {
      const src = isRe ? raw : raw.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      try { testRe = new RegExp(src, cs ? '' : 'i'); hlRe = new RegExp(src, cs ? 'g' : 'gi'); }
      catch { q('.logs-q').classList.add('logs-bad'); q('.logs-q').title = t('lv.badRegex', 'Invalid regular expression'); }
    }
    minLevel = +q('.logs-level').value;
    render();
  }
  let ft = 0;
  q('.logs-q').oninput = () => { clearTimeout(ft); ft = setTimeout(compile, 160); };
  q('.logs-re').onchange = q('.logs-case').onchange = q('.logs-level').onchange = compile;

  function start() {
    stop();
    suspended = false; paused = false; queue = []; all = []; shown = 0;
    body.textContent = '';
    const src = new EventSource(url(+q('.logs-n').value));
    es = src;
    src.onmessage = e => { let ls; try { ls = JSON.parse(e.data); } catch { return; } if (Array.isArray(ls) && ls.length) addLines(ls); };
    src.addEventListener('info', e => { try { note(JSON.parse(e.data), 'info'); } catch {} });
    src.addEventListener('fail', e => { let m = ''; try { m = JSON.parse(e.data); } catch {} stop(); note(m, 'fail'); });
    src.addEventListener('end', e => { let m = ''; try { m = JSON.parse(e.data); } catch {} stop(); note(m, 'end'); });
    src.onerror = () => { if (es === src) { stop(); note(t('lv.disconnected', 'Disconnected from the log stream.'), 'fail'); } };
    buttons();
  }
  function stop() {
    if (es) { es.close(); es = null; }
    if (paused && queue.length) { paused = false; push(queue); queue = []; }
    paused = false;
    buttons();
  }
  btnFollow.onclick = () => (es ? stop() : start());
  btnPause.onclick = () => {
    if (paused) { paused = false; const qd = queue; queue = []; push(qd); } else paused = true;
    buttons();
  };
  q('.logs-clear').onclick = () => { all = []; queue = []; shown = 0; body.textContent = ''; updateStat(); };
  wrapChk.onchange = () => { body.classList.toggle('logs-wrap', wrapChk.checked); store.set('logs.wrap', wrapChk.checked ? '1' : '0'); };
  q('.logs-n').onchange = () => { store.set('logs.n', q('.logs-n').value); if (es) start(); };
  body.onscroll = () => { btnBottom.hidden = atBottom(); };
  btnBottom.onclick = () => { body.scrollTop = body.scrollHeight; btnBottom.hidden = true; };
  buttons();

  return {
    el, start, stop,
    get live() { return !!es; },
    // Close the stream while the view is hidden (keeps devdeck idle); resume() re-tails.
    suspend() { if (es) { stop(); suspended = true; note(t('lv.suspended', 'Stopped while the tab was hidden; it reloads when you come back.')); } },
    resume() { if (suspended) start(); },
    destroy() { stop(); el.remove(); },
  };
}
window.logsViewer = logsViewer;

// ------------------------------------------------------------------ Logs tab
let files = [], sel = store.get('logs.sel', ''), viewer = null, loaded = false;
const fmtSize = n => n < 1024 ? n + ' B' : n < 1048576 ? (n / 1024).toFixed(1) + ' KB' : (n / 1048576).toFixed(1) + ' MB';

async function loadFiles() {
  try { files = await api('/api/logs/files'); loaded = true; } catch (e) { toast(e.message, 'err'); }
  renderFiles();
}
function renderFiles() {
  $('#logs-count').textContent = files.length ? String(files.length) : '';
  $('#logs-list').innerHTML = files.length ? files.map(f => `
    <div class="logs-file${f.path === sel ? ' on' : ''}" data-path="${esc(f.path)}" title="${esc(f.path)}">
      <div class="logs-fmain"><b>${esc(f.name)}</b><span class="muted mono">${esc(f.path)}</span></div>
      <span class="logs-fsize ${f.error ? 'logs-ferr' : 'muted'}">${f.error ? esc(t('logs.missing', 'file not found')) : fmtSize(f.size)}</span>
      <button class="btn sm ghost logs-rm" data-rm="${esc(f.path)}" title="${esc(t('logs.remove', 'Remove'))}"><svg class="i"><use href="#i-trash"/></svg></button>
    </div>`).join('')
    : `<div class="empty">${esc(t('logs.none', 'No log files yet. Enter a path above and click Add file.'))}</div>`;
  const f = files.find(x => x.path === sel);
  $('#logs-title').textContent = f ? f.path : '';
  $('#logs-pick').hidden = !!f;
  $('#logs-view').hidden = !f;
}
function openFile(path) {
  sel = path; store.set('logs.sel', path);
  renderFiles();
  if (!viewer) viewer = logsViewer($('#logs-view'), { url: n => `/api/logs/follow?lines=${n}&path=${encodeURIComponent(sel)}` });
  viewer.start();
}
async function addFile() {
  const p = $('#logs-path').value.trim();
  if (!p) return toast(t('logs.needPath', 'Enter a file path'), 'err');
  try {
    files = await post('/api/logs/files', { path: p });
    $('#logs-path').value = '';
    const added = files.find(f => f.path.toLowerCase() === p.replace(/^"|"$/g, '').toLowerCase()) || files[files.length - 1];
    toast(tf('logs.added', 'Added {name}', { name: added ? added.name : p }));
    if (added) openFile(added.path); else renderFiles();
  } catch (e) { toast(e.message, 'err'); }
}
async function removeFile(path) {
  const f = files.find(x => x.path === path); if (!f) return;
  const ok = await dialog({ title: tf('logs.removeQ', 'Remove "{name}" from the list?', { name: f.name }), text: t('logs.removeText', 'Only the entry in devdeck is removed; the file on disk is not touched.'), ok: t('logs.remove', 'Remove'), danger: true });
  if (!ok) return;
  try {
    files = await post('/api/logs/files/remove', { path });
    if (sel === path) { viewer?.stop(); sel = ''; store.set('logs.sel', ''); }
    renderFiles();
    toast(tf('logs.removed', 'Removed {name}', { name: f.name }));
  } catch (e) { toast(e.message, 'err'); }
}
$('#logs-add').onclick = addFile;
$('#logs-path').onkeydown = e => { if (e.key === 'Enter') addFile(); };
$('#logs-list').onclick = e => {
  const rm = e.target.closest('[data-rm]');
  if (rm) { e.stopPropagation(); removeFile(rm.dataset.rm); return; }
  const row = e.target.closest('.logs-file');
  if (row && row.dataset.path !== sel) openFile(row.dataset.path);
  else if (row && viewer && !viewer.live) viewer.start();
};
onTab('logs', async () => {
  await loadFiles();
  if (!files.some(f => f.path === sel)) return;
  if (!viewer) openFile(sel); else viewer.resume();
});
document.addEventListener('devdeck:tab', e => { if (e.detail !== 'logs') viewer?.suspend(); });
beforeQuit.push(() => viewer?.stop());

// ------------------------------------------------------------------ Ctrl+K
registerCommand({ title: t('logs.palAdd', 'Logs: add a log file'), keywords: 'log tail file add', run: () => { show('logs'); $('#logs-path').focus(); } });
registerCommandProvider(async query => {
  const m = /^(tail|logs?|file)\b\s*(.*)$/.exec(query); if (!m) return [];
  if (!loaded) { try { files = await api('/api/logs/files'); loaded = true; } catch { return []; } }
  const f = m[2];
  return files.filter(x => !f || x.path.toLowerCase().includes(f)).slice(0, 15).map(x => ({
    title: `${t('logs.palTail', 'Tail log file')}: ${x.name}`, hint: x.path,
    run: () => { show('logs'); openFile(x.path); },
  }));
});
})();
