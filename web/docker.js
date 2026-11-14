'use strict';
// Docker tab (feature module): containers, images and compose projects through the docker CLI.
// Container logs reuse window.logsViewer from logs.js; "Shell" opens a devdeck terminal via newTerm().
(() => {
registerStrings('vi', {
  'docker.containers': 'Container',
  'docker.images': 'Image',
  'docker.compose': 'Compose',
  'docker.filter': 'Lọc',
  'docker.showAll': 'Hiện cả container đã dừng',
  'docker.stats': 'Thống kê',
  'docker.statsTitle': 'Chụp nhanh CPU / bộ nhớ một lần (docker stats --no-stream)',
  'docker.refresh': 'Làm mới',
  'docker.close': 'Đóng',
  'docker.loading': 'Đang kiểm tra Docker…',
  'docker.missingT': 'Chưa cài Docker',
  'docker.missingP': 'Không tìm thấy lệnh docker. Hãy cài Docker Desktop, hoặc đặt biến môi trường DEVDECK_DOCKER_BIN trỏ tới tệp docker, rồi thử lại.',
  'docker.downT': 'Docker chưa chạy',
  'docker.downP': 'Hãy mở Docker Desktop (hoặc khởi động dịch vụ docker), rồi bấm Thử lại.',
  'docker.retry': 'Thử lại',
  'docker.name': 'Tên',
  'docker.state': 'Trạng thái',
  'docker.image': 'Image',
  'docker.ports': 'Cổng',
  'docker.status': 'Thời gian chạy',
  'docker.cpu': 'CPU',
  'docker.mem': 'Bộ nhớ',
  'docker.logs': 'Log',
  'docker.shell': 'Shell',
  'docker.inspect': 'Chi tiết',
  'docker.start': 'Chạy',
  'docker.stop': 'Dừng',
  'docker.restart': 'Khởi động lại',
  'docker.remove': 'Xoá',
  'docker.noContainers': 'Không có container nào đang chạy.',
  'docker.noContainersAll': 'Không có container nào.',
  'docker.noImages': 'Không có image nào.',
  'docker.noCompose': 'Không có dự án compose nào.',
  'docker.noMatch': 'Không có mục nào khớp.',
  'docker.count': '{n} mục',
  'docker.statsAt': 'Số liệu lúc {time}',
  'docker.statsLoading': 'Đang lấy số liệu…',
  'docker.removeQ': 'Xoá container "{name}"?',
  'docker.removeP': 'Container và lớp ghi của nó sẽ bị xoá vĩnh viễn. Volume có tên vẫn được giữ.',
  'docker.removeRunning': 'Container đang chạy sẽ bị dừng và xoá vĩnh viễn. Volume có tên vẫn được giữ.',
  'docker.stopQ': 'Dừng container "{name}"?',
  'docker.stopP': 'Các tiến trình trong container sẽ nhận tín hiệu dừng.',
  'docker.rmiQ': 'Xoá image "{name}"?',
  'docker.rmiP': 'Image sẽ bị xoá khỏi máy này; có thể kéo (pull) lại sau.',
  'docker.rmiForceQ': 'Buộc xoá image "{name}"?',
  'docker.rmiForceP': 'Image đang được container sử dụng hoặc có nhiều tag. Buộc xoá vẫn giữ các container nhưng gỡ tag của image.',
  'docker.forceRemove': 'Buộc xoá',
  'docker.done': '{action}: {name}',
  'docker.running': 'Đang chạy {action} cho {name}…',
  'docker.upQ': 'Chạy "compose up -d" cho {name}?',
  'docker.upP': 'Tạo và khởi động các service của dự án; có thể phải kéo image.',
  'docker.downQ': 'Chạy "compose down" cho {name}?',
  'docker.downP2': 'Các container và network của dự án sẽ bị dừng và xoá. Volume vẫn được giữ.',
  'docker.restartQ': 'Khởi động lại dự án {name}?',
  'docker.restartP': 'Mọi service của dự án sẽ được khởi động lại.',
  'docker.files': 'Tệp cấu hình',
  'docker.repo': 'Repository:Tag',
  'docker.size': 'Dung lượng',
  'docker.created': 'Tạo lúc',
  'docker.copy': 'Sao chép JSON',
  'docker.copied': 'Đã sao chép',
  'docker.expand': 'Mở hết',
  'docker.collapse': 'Thu gọn',
  'docker.badId': 'Mã container không hợp lệ',
  'docker.noTerm': 'Chưa có module terminal',
  'docker.palLogs': 'Xem log container',
  'docker.palRestart': 'Khởi động lại container',
  'docker.palContainers': 'Docker: danh sách container',
  'docker.palImages': 'Docker: danh sách image',
  'docker.palCompose': 'Docker: dự án compose',
  'docker.output': 'Kết quả',
});
const tf = (key, en, vars = {}) => Object.entries(vars).reduce((s, [k, v]) => s.split(`{${k}}`).join(v), t(key, en));
const idOk = s => /^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$/.test(s || '');
const shortId = id => String(id || '').replace(/^sha256:/, '').slice(0, 12);

const S = {
  view: ['containers', 'images', 'compose'].includes(store.get('docker.view')) ? store.get('docker.view') : 'containers',
  all: store.get('docker.all', '0') === '1',
  status: null, containers: [], images: [], compose: [], stats: null, statsAt: null,
  busy: new Set(), viewer: null, loading: false,
};

// ------------------------------------------------------------------ status / empty state
async function checkStatus() {
  try { S.status = await api('/api/docker/status'); } catch (e) { S.status = { state: 'error', error: e.message }; }
  renderEmpty();
  return S.status.state === 'ok';
}
function renderEmpty() {
  const st = S.status ? S.status.state : 'ok';
  const empty = $('#docker-empty');
  const blocked = st === 'missing' || st === 'down';
  empty.hidden = !blocked;
  $('#docker-card').hidden = blocked;
  if (!blocked) { if (S.status && S.status.server) $('#docker-info').textContent = `Docker ${S.status.server}`; return; }
  $('#docker-info').textContent = '';
  const missing = st === 'missing';
  empty.innerHTML = `<div class="docker-empty-in">
    <svg class="i docker-empty-i"><use href="#i-docker"/></svg>
    <h3>${esc(missing ? t('docker.missingT', 'Docker is not installed') : t('docker.downT', 'Docker is not running'))}</h3>
    <p class="muted">${esc(missing ? t('docker.missingP', 'The docker CLI was not found. Install Docker Desktop, or set DEVDECK_DOCKER_BIN to the docker binary, then retry.') : t('docker.downP', 'Start Docker Desktop (or the docker service), then click Retry.'))}</p>
    ${S.status.error ? `<p class="mono docker-empty-err">${esc(S.status.error)}</p>` : ''}
    <button class="btn primary" id="docker-retry"><svg class="i"><use href="#i-refresh"/></svg>${esc(t('docker.retry', 'Retry'))}</button></div>`;
  $('#docker-retry').onclick = () => load(true);
}

// ------------------------------------------------------------------ loading
async function load(recheck) {
  if (S.loading) return;
  S.loading = true;
  try {
    if (recheck || !S.status || S.status.state !== 'ok') { if (!(await checkStatus())) return; }
    if (S.view === 'containers') S.containers = await api(`/api/docker/containers?all=${S.all ? 1 : 0}`);
    else if (S.view === 'images') S.images = await api('/api/docker/images');
    else S.compose = await api('/api/docker/compose');
    render();
  } catch (e) {
    if (!(await checkStatus())) return; // daemon went away: the empty state explains it
    $('#docker-card').innerHTML = `<div class="empty">${esc(e.message)}</div>`;
  } finally { S.loading = false; }
}

function setView(v) {
  S.view = v; store.set('docker.view', v);
  document.querySelectorAll('#docker-seg [data-view]').forEach(b => b.classList.toggle('on', b.dataset.view === v));
  $('#docker-allw').hidden = $('#docker-stats').hidden = v !== 'containers';
  $('#docker-card').innerHTML = '';
  load();
}

// ------------------------------------------------------------------ rendering
const statePill = s => {
  const cls = s === 'running' ? 'ok' : s === 'paused' || s === 'restarting' || s === 'created' ? 'warn' : s === 'dead' ? 'bad' : 'idle';
  return `<span class="pill docker-pill-${cls}">${esc(s)}</span>`;
};
function portsHTML(c) {
  if (!c.ports || !c.ports.length) return '<span class="muted">—</span>';
  return c.ports.map(p => p.host
    ? (p.proto === 'tcp'
      ? `<a class="docker-port" href="http://localhost:${p.host}" target="_blank" rel="noopener" title="${esc(`${p.ip || ''}:${p.host} → ${p.container}/${p.proto}`)}">${p.host}<span class="muted">→${p.container}</span></a>`
      : `<span class="docker-port" title="${esc(p.ip || '')}">${p.host}<span class="muted">→${p.container}/${esc(p.proto)}</span></span>`)
    : `<span class="docker-port muted">${p.container}/${esc(p.proto)}</span>`).join(' ');
}
function matches(text) { const q = $('#docker-q').value.trim().toLowerCase(); return !q || text.toLowerCase().includes(q); }
function btn(label, act, key, cls = '') { return `<button class="btn sm ${cls}" data-act="${act}" data-key="${esc(key)}"${S.busy.has(key + act) ? ' disabled' : ''}>${esc(label)}</button>`; }

function render() {
  if (S.view === 'containers') renderContainers();
  else if (S.view === 'images') renderImages();
  else renderCompose();
}
function renderContainers() {
  const rows = S.containers.filter(c => matches(`${c.name} ${c.image} ${c.id} ${c.state} ${c.compose || ''} ${c.portsRaw}`));
  const st = S.stats;
  const statOf = c => st && (st[c.id] || st[c.name]);
  $('#docker-info').textContent = tf('docker.count', '{n} items', { n: rows.length }) + (S.statsAt ? ' · ' + tf('docker.statsAt', 'stats at {time}', { time: S.statsAt }) : '');
  const head = `<tr><th>${esc(t('docker.name', 'Name'))}</th><th>${esc(t('docker.state', 'State'))}</th><th>${esc(t('docker.image', 'Image'))}</th><th>${esc(t('docker.ports', 'Ports'))}</th><th>${esc(t('docker.status', 'Uptime'))}</th>${st ? `<th>${esc(t('docker.cpu', 'CPU'))}</th><th>${esc(t('docker.mem', 'Memory'))}</th>` : ''}<th></th></tr>`;
  const cols = st ? 8 : 6;
  const body = rows.length ? rows.map(c => {
    const run = c.state === 'running', s = statOf(c);
    return `<tr data-id="${esc(c.id)}">
      <td><b>${esc(c.name)}</b>${c.compose ? ` <span class="pill docker-pill-idle" title="compose project">${esc(c.compose)}</span>` : ''}<div class="pid">${esc(c.id)}</div></td>
      <td>${statePill(c.state)}</td>
      <td class="docker-img" title="${esc(c.image)}">${esc(c.image)}</td>
      <td class="docker-ports">${portsHTML(c)}</td>
      <td class="muted">${esc(c.status)}</td>
      ${st ? `<td class="mono">${esc(s ? s.cpu : '')}</td><td class="mono" title="${esc(s ? s.memPerc : '')}">${esc(s ? s.mem : '')}</td>` : ''}
      <td class="act">
        ${btn(t('docker.logs', 'Logs'), 'logs', c.id)}
        ${run ? btn(t('docker.shell', 'Shell'), 'shell', c.id) : ''}
        ${btn(t('docker.inspect', 'Inspect'), 'inspect', c.id)}
        ${run ? btn(t('docker.stop', 'Stop'), 'stop', c.id) : btn(t('docker.start', 'Start'), 'start', c.id)}
        ${btn(t('docker.restart', 'Restart'), 'restart', c.id)}
        ${btn(t('docker.remove', 'Remove'), 'remove', c.id, 'danger')}
      </td></tr>`;
  }).join('') : `<tr><td colspan="${cols}" class="empty">${esc(S.containers.length ? t('docker.noMatch', 'Nothing matches.') : S.all ? t('docker.noContainersAll', 'No containers.') : t('docker.noContainers', 'No running containers.'))}</td></tr>`;
  $('#docker-card').innerHTML = `<table class="docker-t"><thead>${head}</thead><tbody>${body}</tbody></table>`;
}
function renderImages() {
  const rows = S.images.filter(i => matches(`${i.repository}:${i.tag} ${i.id}`));
  $('#docker-info').textContent = tf('docker.count', '{n} items', { n: rows.length });
  const body = rows.length ? rows.map(i => {
    const name = i.repository && i.repository !== '<none>' ? `${i.repository}:${i.tag}` : shortId(i.id);
    return `<tr><td><b>${esc(i.repository)}</b><span class="muted">:${esc(i.tag)}</span></td><td class="pid">${esc(shortId(i.id))}</td>
      <td>${esc(i.size)}</td><td class="muted">${esc(i.created)}</td>
      <td class="act"><button class="btn sm danger" data-act="rmi" data-key="${esc(i.id)}" data-name="${esc(name)}"${S.busy.has(i.id + 'rmi') ? ' disabled' : ''}>${esc(t('docker.remove', 'Remove'))}</button></td></tr>`;
  }).join('') : `<tr><td colspan="5" class="empty">${esc(S.images.length ? t('docker.noMatch', 'Nothing matches.') : t('docker.noImages', 'No images.'))}</td></tr>`;
  $('#docker-card').innerHTML = `<table class="docker-t"><thead><tr><th>${esc(t('docker.repo', 'Repository:Tag'))}</th><th>ID</th><th>${esc(t('docker.size', 'Size'))}</th><th>${esc(t('docker.created', 'Created'))}</th><th></th></tr></thead><tbody>${body}</tbody></table>`;
}
function renderCompose() {
  const rows = S.compose.filter(p => matches(`${p.name} ${p.status} ${(p.configFiles || []).join(' ')}`));
  $('#docker-info').textContent = tf('docker.count', '{n} items', { n: rows.length });
  const body = rows.length ? rows.map(p => `<tr>
      <td><b>${esc(p.name)}</b></td><td>${statePill(String(p.status).replace(/\(.*$/, '') || 'unknown')} <span class="muted">${esc(p.status)}</span></td>
      <td class="mono muted docker-files" title="${esc((p.configFiles || []).join('\n'))}">${esc((p.configFiles || []).join(', '))}</td>
      <td class="act">${btn('up -d', 'up', p.name)}${btn(t('docker.restart', 'Restart'), 'crestart', p.name)}${btn('down', 'down', p.name, 'danger')}</td></tr>`).join('')
    : `<tr><td colspan="4" class="empty">${esc(S.compose.length ? t('docker.noMatch', 'Nothing matches.') : t('docker.noCompose', 'No compose projects.'))}</td></tr>`;
  $('#docker-card').innerHTML = `<table class="docker-t"><thead><tr><th>${esc(t('docker.name', 'Name'))}</th><th>${esc(t('docker.state', 'State'))}</th><th>${esc(t('docker.files', 'Config files'))}</th><th></th></tr></thead><tbody>${body}</tbody></table>`;
}

// ------------------------------------------------------------------ actions
const findC = id => S.containers.find(c => c.id === id);
async function busy(key, fn) {
  S.busy.add(key); render();
  try { return await fn(); } finally { S.busy.delete(key); render(); }
}
async function containerAction(c, action) {
  if (!idOk(c.id)) return toast(t('docker.badId', 'Invalid container id'), 'err');
  if (action === 'remove') {
    const run = c.state === 'running';
    const ok = await dialog({ title: tf('docker.removeQ', 'Remove container "{name}"?', { name: c.name }), text: run ? t('docker.removeRunning', 'The running container is stopped and permanently removed. Named volumes are kept.') : t('docker.removeP', 'The container and its writable layer are deleted permanently. Named volumes are kept.'), ok: t('docker.remove', 'Remove'), danger: true });
    if (!ok) return;
  } else if (action === 'stop') {
    const ok = await dialog({ title: tf('docker.stopQ', 'Stop container "{name}"?', { name: c.name }), text: t('docker.stopP', 'Processes in the container receive a stop signal.'), ok: t('docker.stop', 'Stop'), danger: true });
    if (!ok) return;
  }
  const label = { start: t('docker.start', 'Start'), stop: t('docker.stop', 'Stop'), restart: t('docker.restart', 'Restart'), remove: t('docker.remove', 'Remove') }[action];
  try {
    await busy(c.id + action, () => post('/api/docker/container', { id: c.id, action, force: action === 'remove' && c.state === 'running' }));
    toast(tf('docker.done', '{action}: {name}', { action: label, name: c.name }));
    palCache = null;
  } catch (e) { toast(e.message, 'err'); }
  if (current === 'docker' && S.view === 'containers') load();
}
async function removeImage(id, name) {
  if (!(await dialog({ title: tf('docker.rmiQ', 'Remove image "{name}"?', { name }), text: t('docker.rmiP', 'The image is deleted from this machine; it can be pulled again later.'), ok: t('docker.remove', 'Remove'), danger: true }))) return;
  try {
    await busy(id + 'rmi', () => post('/api/docker/image/remove', { id, force: false }));
    toast(tf('docker.done', '{action}: {name}', { action: t('docker.remove', 'Remove'), name }));
  } catch (e) {
    if (!/conflict|being used|must be forced|referenced in multiple/i.test(e.message)) { toast(e.message, 'err'); return; }
    const ok = await dialog({ title: tf('docker.rmiForceQ', 'Force remove image "{name}"?', { name }), text: t('docker.rmiForceP', 'The image is used by a container or has several tags. Forcing keeps the containers but untags the image.') + '\n\n' + e.message, ok: t('docker.forceRemove', 'Force remove'), danger: true });
    if (!ok) return;
    try { await busy(id + 'rmi', () => post('/api/docker/image/remove', { id, force: true })); toast(tf('docker.done', '{action}: {name}', { action: t('docker.remove', 'Remove'), name })); }
    catch (e2) { toast(e2.message, 'err'); }
  }
  load();
}
async function composeRun(name, action) {
  const q = {
    up: [t('docker.upQ', 'Run "compose up -d" for {name}?'), t('docker.upP', 'Creates and starts the project services; images may be pulled.'), 'up -d', false],
    down: [t('docker.downQ', 'Run "compose down" for {name}?'), t('docker.downP2', 'The project containers and networks are stopped and removed. Volumes are kept.'), 'down', true],
    restart: [t('docker.restartQ', 'Restart project {name}?'), t('docker.restartP', 'Every service of the project is restarted.'), t('docker.restart', 'Restart'), false],
  }[action];
  if (!(await dialog({ title: q[0].replace('{name}', name), text: q[1], ok: q[2], danger: q[3] }))) return;
  toast(tf('docker.running', 'Running {action} for {name}…', { action: 'compose ' + action, name }));
  let out = '', err = null;
  try { out = (await busy(name + (action === 'restart' ? 'crestart' : action), () => post('/api/docker/compose', { name, action }))).output; }
  catch (e) { err = e; out = e.output || ''; }
  if (err) toast(err.message.split('\n')[0], 'err'); else toast(tf('docker.done', '{action}: {name}', { action: 'compose ' + action, name }));
  if (out) { openPanel(`compose ${action} · ${name}`); $('#docker-panel-body').innerHTML = `<pre class="docker-out">${esc(out)}</pre>`; }
  palCache = null;
  if (current === 'docker') load();
}
async function loadStats() {
  const b = $('#docker-stats'); b.disabled = true; $('#docker-info').textContent = t('docker.statsLoading', 'Fetching stats…');
  try {
    const list = await api('/api/docker/stats');
    S.stats = {};
    for (const s of list) { S.stats[s.id] = s; S.stats[shortId(s.id)] = s; if (s.name) S.stats[s.name] = s; }
    S.statsAt = new Date().toLocaleTimeString();
    render();
  } catch (e) { toast(e.message, 'err'); render(); } finally { b.disabled = false; }
}
function openShell(c) {
  if (!idOk(c.id)) return toast(t('docker.badId', 'Invalid container id'), 'err');
  if (typeof newTerm !== 'function') return toast(t('docker.noTerm', 'Terminal module is not loaded'), 'err');
  show('term');
  newTerm(`docker · ${c.name}`, { kind: 'local', cmd: `docker exec -it ${c.id} sh` });
}

// ------------------------------------------------------------------ side panel (logs, inspect, output)
function openPanel(title) {
  closePanel();
  $('#docker-panel-t').textContent = title;
  $('#docker-panel-x').innerHTML = '';
  $('#docker-panel-body').innerHTML = '';
  $('#docker-panel').hidden = false;
}
function closePanel() {
  if (S.viewer) { S.viewer.destroy(); S.viewer = null; }
  $('#docker-panel').hidden = true;
  $('#docker-panel-body').innerHTML = '';
}
function openLogs(c) {
  if (!idOk(c.id)) return toast(t('docker.badId', 'Invalid container id'), 'err');
  openPanel(`${t('docker.logs', 'Logs')} · ${c.name}`);
  if (typeof window.logsViewer !== 'function') { $('#docker-panel-body').textContent = 'log viewer not loaded'; return; }
  S.viewer = window.logsViewer($('#docker-panel-body'), { url: n => `/api/docker/logs?follow=1&tail=${n}&id=${encodeURIComponent(c.id)}` });
  S.viewer.start();
}

function jsonNode(k, v, depth, budget) {
  if (budget.n-- <= 0) return '';
  const key = k === null ? '' : `<span class="docker-jk">${esc(k)}</span>: `;
  if (v === null || typeof v !== 'object') {
    const cls = v === null ? 'null' : typeof v;
    return `<div class="docker-jl">${key}<span class="docker-j-${cls}">${esc(JSON.stringify(v))}</span></div>`;
  }
  const arr = Array.isArray(v), entries = arr ? v.map((x, i) => [i, x]) : Object.entries(v);
  if (!entries.length) return `<div class="docker-jl">${key}<span class="muted">${arr ? '[]' : '{}'}</span></div>`;
  const sum = arr ? `[${entries.length}]` : `{${entries.length}}`;
  return `<details${depth < 1 ? ' open' : ''}><summary>${key}<span class="muted">${sum}</span></summary><div class="docker-jc">${entries.map(([ck, cv]) => jsonNode(arr ? ck : ck, cv, depth + 1, budget)).join('')}</div></details>`;
}
async function openInspect(c) {
  openPanel(`${t('docker.inspect', 'Inspect')} · ${c.name}`);
  let data;
  try { data = await api(`/api/docker/inspect?id=${encodeURIComponent(c.id)}`); }
  catch (e) { $('#docker-panel-body').innerHTML = `<div class="empty">${esc(e.message)}</div>`; return; }
  $('#docker-panel-x').innerHTML = `<button class="btn sm" data-j="expand">${esc(t('docker.expand', 'Expand all'))}</button><button class="btn sm" data-j="collapse">${esc(t('docker.collapse', 'Collapse'))}</button><button class="btn sm" data-j="copy">${esc(t('docker.copy', 'Copy JSON'))}</button>`;
  $('#docker-panel-x').onclick = async e => {
    const b = e.target.closest('[data-j]'); if (!b) return;
    if (b.dataset.j === 'copy') { try { await navigator.clipboard.writeText(JSON.stringify(data, null, 2)); toast(t('docker.copied', 'Copied')); } catch (err) { toast(err.message, 'err'); } }
    else $('#docker-panel-body').querySelectorAll('details').forEach(d => { d.open = b.dataset.j === 'expand'; });
  };
  $('#docker-panel-body').innerHTML = `<div class="docker-json mono">${jsonNode(null, data, 0, { n: 20000 })}</div>`;
}

// ------------------------------------------------------------------ events
$('#docker-seg').onclick = e => { const b = e.target.closest('[data-view]'); if (b && b.dataset.view !== S.view) setView(b.dataset.view); };
$('#docker-q').oninput = render;
$('#docker-all').checked = S.all;
$('#docker-all').onchange = () => { S.all = $('#docker-all').checked; store.set('docker.all', S.all ? '1' : '0'); load(); };
$('#docker-r').onclick = () => load(true);
$('#docker-stats').onclick = loadStats;
$('#docker-panel-close').onclick = closePanel;
$('#docker-card').onclick = e => {
  const b = e.target.closest('button[data-act]'); if (!b) return;
  const key = b.dataset.key, act = b.dataset.act;
  if (act === 'rmi') return removeImage(key, b.dataset.name);
  if (act === 'up' || act === 'down') return composeRun(key, act);
  if (act === 'crestart') return composeRun(key, 'restart');
  const c = findC(key); if (!c) return;
  if (act === 'logs') openLogs(c);
  else if (act === 'shell') openShell(c);
  else if (act === 'inspect') openInspect(c);
  else containerAction(c, act);
};
document.addEventListener('keydown', e => {
  if (e.key === 'Escape' && current === 'docker' && !$('#docker-panel').hidden && !document.querySelector('dialog[open]')) closePanel();
});
document.querySelectorAll('#docker-seg [data-view]').forEach(b => b.classList.toggle('on', b.dataset.view === S.view));
$('#docker-allw').hidden = $('#docker-stats').hidden = S.view !== 'containers';
onTab('docker', () => { load(); if (S.viewer) S.viewer.resume(); });
document.addEventListener('devdeck:tab', e => { if (e.detail !== 'docker' && S.viewer) S.viewer.suspend(); });
// Poll the container list only while the tab is visible, docker is up and no panel covers the list.
every(5000, 'docker', () => { if (S.view === 'containers' && S.status && S.status.state === 'ok' && $('#docker-panel').hidden && !document.querySelector('dialog[open]')) load(); });
beforeQuit.push(() => { if (S.viewer) S.viewer.stop(); });

// ------------------------------------------------------------------ Ctrl+K
let palCache = null, palAt = 0;
function palContainers() {
  if (palCache && Date.now() - palAt < 15000) return palCache;
  palAt = Date.now();
  palCache = (async () => {
    const st = await api('/api/docker/status').catch(() => null);
    if (!st || st.state !== 'ok') return [];
    return api('/api/docker/containers?all=1').catch(() => []);
  })();
  return palCache;
}
registerCommand({ title: t('docker.palContainers', 'Docker: containers'), keywords: 'docker containers ps', run: () => { show('docker'); if (S.view !== 'containers') setView('containers'); } });
registerCommand({ title: t('docker.palImages', 'Docker: images'), keywords: 'docker images', run: () => { show('docker'); if (S.view !== 'images') setView('images'); } });
registerCommand({ title: t('docker.palCompose', 'Docker: compose projects'), keywords: 'docker compose', run: () => { show('docker'); if (S.view !== 'compose') setView('compose'); } });
registerCommandProvider(async query => {
  const m = /^(docker|logs?|restart|container)\b\s*(.*)$/.exec(query); if (!m) return [];
  const f = m[2], cs = (await palContainers()).filter(c => !f || `${c.name} ${c.image}`.toLowerCase().includes(f));
  const out = [];
  for (const c of cs.slice(0, 12)) {
    if (m[1] !== 'restart') out.push({ title: `${t('docker.palLogs', 'Container logs')}: ${c.name}`, hint: `${c.state} · ${c.image}`, run: () => { show('docker'); openLogs(c); } });
    if (!/^logs?$/.test(m[1])) out.push({ title: `${t('docker.palRestart', 'Restart container')}: ${c.name}`, hint: `${c.state} · ${c.image}`, run: () => containerAction(c, 'restart') });
  }
  return out;
});
})();
