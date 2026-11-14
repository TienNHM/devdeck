'use strict';
// Ports + Processes tabs (feature module). Owns <section id="ports"> and <section id="procs">.
// Exposes: openProcDetails(pid), killPid(pid, tree, name) -> Promise<bool>, devKindPill(kind, cat).

registerStrings('vi', {
  'ports.filter': 'Lọc theo cổng, tiến trình, PID, loại hoặc thư mục',
  'ports.group': 'Gom theo tiến trình',
  'ports.refresh': 'Làm mới',
  'ports.col.port': 'Cổng', 'ports.col.addr': 'Địa chỉ', 'ports.col.proc': 'Tiến trình', 'ports.col.started': 'Khởi động', 'ports.col.where': 'Thư mục / lệnh',
  'ports.listening': '{n} cổng đang lắng nghe',
  'ports.groups': '{p} tiến trình · {n} cổng',
  'ports.none': 'Không có kết quả phù hợp.',
  'ports.local': 'nội bộ', 'ports.exposed': 'mở ra ngoài', 'ports.exposedTip': 'Máy khác trong mạng có thể truy cập',
  'ports.denied': '(không có quyền truy cập)',
  'ports.open': 'Mở', 'ports.openTip': 'Mở trong trình duyệt',
  'ports.kill': 'Dừng',
  'ports.details': 'Xem chi tiết tiến trình',
  'ports.cmdKill': 'Dừng cổng {port}', 'ports.cmdOpen': 'Mở localhost:{port} trong trình duyệt', 'ports.cmdNone': 'Không có gì lắng nghe trên cổng {port}',
  'ports.cmdGroup': 'Bật/tắt gom cổng theo tiến trình',
  'kill.title': 'Dừng {name}?', 'kill.text': 'PID {pid} sẽ bị kết thúc. Dữ liệu chưa lưu trong đó sẽ mất.',
  'kill.textTree': 'PID {pid} và các tiến trình con sẽ bị kết thúc. Dữ liệu chưa lưu trong đó sẽ mất.',
  'kill.ok': 'Dừng', 'kill.done': 'Đã dừng {name} ({pid})',
  'procs.filter': 'Lọc theo tên hoặc PID', 'procs.flat': 'Danh sách', 'procs.tree': 'Cây', 'procs.refresh': 'Làm mới',
  'procs.col.name': 'Tên', 'procs.col.mem': 'Bộ nhớ', 'procs.col.started': 'Khởi động',
  'procs.shown': '{n} tiến trình', 'procs.capped': 'hiển thị {n}/{total}',
  'procs.details': 'Chi tiết', 'procs.kill': 'Dừng', 'procs.killTree': 'Dừng cả cây',
  'procs.none': 'Không có kết quả phù hợp.',
  'procs.cpuWait': 'CPU sẽ có sau lần đo thứ hai',
  'drawer.close': 'Đóng', 'drawer.cpu': 'CPU', 'drawer.ram': 'RAM', 'drawer.threads': 'Luồng', 'drawer.started': 'Khởi động',
  'drawer.cmdline': 'Dòng lệnh', 'drawer.cwd': 'Thư mục làm việc', 'drawer.parent': 'Tiến trình cha', 'drawer.children': 'Tiến trình con',
  'drawer.ports': 'Cổng đang lắng nghe', 'drawer.noPorts': 'Không lắng nghe cổng nào.', 'drawer.noKids': 'Không có tiến trình con.',
  'drawer.unknown': 'không đọc được (thiếu quyền)', 'drawer.copy': 'Sao chép', 'drawer.copied': 'Đã sao chép',
  'drawer.term': 'Mở terminal tại đây', 'drawer.exited': 'Tiến trình đã kết thúc.', 'drawer.sampling': 'Đo mỗi 2 giây khi bảng này mở',
  'drawer.more': 'và {n} tiến trình khác',
});

const killPid = async (pid, tree, name) => {
  const ok = await dialog({
    title: t('kill.title', 'Kill {name}?').replace('{name}', name),
    text: (tree ? t('kill.textTree', 'PID {pid} and its child processes will be terminated. Unsaved work in them is lost.') : t('kill.text', 'PID {pid} will be terminated. Unsaved work in it is lost.')).replace('{pid}', pid),
    ok: t('kill.ok', 'Kill'), danger: true,
  });
  if (!ok) return false;
  try { await post('/api/kill', { pid, tree }); toast(t('kill.done', 'Killed {name} ({pid})').replace('{name}', name).replace('{pid}', pid)); }
  catch (e) { toast(e.message, 'err'); return false; }
  document.dispatchEvent(new CustomEvent('devdeck:killed', { detail: pid }));
  return true;
};
const devKindPill = (kind, cat) => kind ? `<span class="pt-kind pt-kind-${esc(cat || 'other')}">${esc(kind)}</span>` : '';
let openProcDetails = () => {};

(() => {
  const fmtTime = ms => ms ? new Date(ms).toLocaleString() : '';
  const agoMs = ms => ms ? ago(ms / 1000) : '';
  const isLoopback = a => a === '127.0.0.1' || a === '::1';
  const isAny = a => a === '0.0.0.0' || a === '::' || a === '';
  const portURL = p => `http://${isLoopback(p.addr) || isAny(p.addr) ? 'localhost' : (p.addr.includes(':') ? `[${p.addr}]` : p.addr)}:${p.port}`;
  const copy = async s => { try { await navigator.clipboard.writeText(s); toast(t('drawer.copied', 'Copied')); } catch (e) { toast(e.message, 'err'); } };

  // ---------------------------------------------------------------- ports
  let portsData = [], portsAt = 0, portsBusy = null;
  const groupBox = $('#ports-group');
  groupBox.checked = store.get('ports.group', '0') === '1';

  async function fetchPorts(maxAge = 0) {
    if (portsBusy) return portsBusy;
    if (maxAge && Date.now() - portsAt < maxAge) return portsData;
    portsBusy = api('/api/ports').then(d => { portsData = d || []; portsAt = Date.now(); return portsData; }).finally(() => { portsBusy = null; });
    return portsBusy;
  }
  async function loadPorts() {
    try { await fetchPorts(); renderPorts(); }
    catch (e) { $('#ports-t').innerHTML = `<tr><td colspan=7 class="empty">${esc(e.message)}</td></tr>`; }
  }
  const whereCell = p => {
    const cwd = p.cwd ? `<div class="pt-cwd mono" title="${esc(p.cwd)}">${esc(p.cwd)}</div>` : '';
    const cmd = p.cmdline ? `<div class="pt-cmd mono muted" title="${esc(p.cmdline)}">${esc(p.cmdline)}</div>` : '';
    return `<td class="pt-where">${cwd}${cmd}</td>`;
  };
  const addrPill = a => isLoopback(a) ? `<span class="pill ok">${esc(t('ports.local', 'local'))}</span>`
    : `<span class="pill warn" title="${esc(t('ports.exposedTip', 'Reachable from other machines'))}">${esc(t('ports.exposed', 'exposed'))}</span>`;
  const procCell = p => `<td><div class="pt-proc">${p.process ? esc(p.process) : `<span class="muted">${esc(t('ports.denied', '(access denied)'))}</span>`}${devKindPill(p.kind, p.kindCat)}</div></td>`;
  const actions = (p, pids) => `<td class="act"><button class="btn sm" data-open="${esc(portURL(p))}" title="${esc(t('ports.openTip', 'Open in browser'))}"><svg class="i"><use href="#i-globe"/></svg>${esc(t('ports.open', 'Open'))}</button>`
    + `<button class="btn sm danger" data-kill="${pids}" data-name="${esc(p.process || 'process')}">${esc(t('ports.kill', 'Kill'))}</button></td>`;

  function renderPorts() {
    const q = $('#ports-q').value.trim().toLowerCase();
    const rows = portsData.filter(p => !q || `${p.port} ${p.process} ${p.pid} ${p.kind || ''} ${p.cwd || ''} ${p.cmdline || ''}`.toLowerCase().includes(q));
    const tb = $('#ports-t');
    if (!rows.length) { tb.innerHTML = `<tr><td colspan=7 class="empty">${esc(t('ports.none', 'Nothing matches.'))}</td></tr>`; $('#ports-count').textContent = ''; return; }
    if (!groupBox.checked) {
      $('#ports-count').textContent = t('ports.listening', '{n} listening').replace('{n}', rows.length);
      tb.innerHTML = rows.map(p => `<tr>
        <td><a class="port" href="${esc(portURL(p))}" target="_blank" rel="noopener" title="${esc(t('ports.openTip', 'Open in browser'))}">${p.port}</a></td>
        <td><span class="mono">${esc(p.addr)}</span> ${addrPill(p.addr)}</td>
        ${procCell(p)}
        <td class="pid"><a href="#" data-pid="${p.pid}" title="${esc(t('ports.details', 'Process details'))}">${p.pid}</a></td>
        <td class="muted" title="${esc(fmtTime(p.started))}">${esc(agoMs(p.started))}</td>
        ${whereCell(p)}${actions(p, p.pid)}</tr>`).join('');
      return;
    }
    const groups = new Map();
    for (const p of rows) { const g = groups.get(p.pid) || []; g.push(p); groups.set(p.pid, g); }
    $('#ports-count').textContent = t('ports.groups', '{p} processes · {n} ports').replace('{p}', groups.size).replace('{n}', rows.length);
    tb.innerHTML = [...groups.values()].map(g => {
      const p = g[0], uniq = [...new Map(g.map(x => [x.port, x])).values()];
      const addrs = [...new Set(g.map(x => x.addr))];
      return `<tr class="pt-grp">
        <td><div class="pt-chips">${uniq.map(x => `<a class="port pt-chip" href="${esc(portURL(x))}" target="_blank" rel="noopener">${x.port}</a>`).join('')}</div></td>
        <td>${addrs.map(a => `<span class="mono">${esc(a)}</span>`).join(' ')} ${addrs.some(a => !isLoopback(a)) ? addrPill('0.0.0.0') : addrPill('127.0.0.1')}</td>
        ${procCell(p)}
        <td class="pid"><a href="#" data-pid="${p.pid}">${p.pid}</a></td>
        <td class="muted" title="${esc(fmtTime(p.started))}">${esc(agoMs(p.started))}</td>
        ${whereCell(p)}${actions(p, p.pid)}</tr>`;
    }).join('');
  }
  $('#ports-q').oninput = renderPorts;
  groupBox.onchange = () => { store.set('ports.group', groupBox.checked ? '1' : '0'); renderPorts(); };
  $('#ports-r').onclick = loadPorts;
  $('#ports-t').onclick = async e => {
    const o = e.target.closest('[data-open]'), k = e.target.closest('[data-kill]'), d = e.target.closest('a[data-pid]');
    if (o) window.open(o.dataset.open, '_blank', 'noopener');
    else if (k) { if (await killPid(+k.dataset.kill, true, k.dataset.name)) loadPorts(); }
    else if (d) { e.preventDefault(); show('procs'); openProcDetails(+d.dataset.pid); }
  };
  onTab('ports', loadPorts);
  every(3000, 'ports', loadPorts);

  // Ctrl+K: "kill port 3000", "free 3000", "stop :3000", "open 3000", or just "3000".
  registerCommandProvider(async q => {
    const m = q.match(/^(kill|free|stop|open)?\s*(?:port\s*)?:?(\d{2,5})$/);
    if (!m || +m[2] > 65535) return [];
    const verb = m[1] || '', port = +m[2];
    const ps = (await fetchPorts(4000)).filter(p => p.port === port);
    if (!ps.length) return verb && verb !== 'open' ? [{ title: t('ports.cmdNone', 'Nothing is listening on port {port}').replace('{port}', port), run: () => {} }] : [];
    const out = [];
    if (verb !== 'open') {
      for (const p of new Map(ps.map(x => [x.pid, x])).values()) {
        out.push({
          title: `${t('ports.cmdKill', 'Kill port {port}').replace('{port}', port)} — ${p.process || 'PID ' + p.pid}${p.kind ? ` (${p.kind})` : ''}`,
          hint: `PID ${p.pid}`, run: async () => { if (await killPid(p.pid, true, p.process || 'process') && current === 'ports') loadPorts(); },
        });
      }
    }
    if (verb !== 'kill' && verb !== 'free' && verb !== 'stop') out.push({ title: t('ports.cmdOpen', 'Open localhost:{port} in browser').replace('{port}', port), hint: ps[0].process, run: () => window.open(portURL(ps[0]), '_blank', 'noopener') });
    return out;
  });
  registerCommand({ title: t('ports.cmdGroup', 'Toggle grouping ports by process'), keywords: 'ports group process', run: () => { show('ports'); groupBox.click(); } });

  // ---------------------------------------------------------------- processes
  let procsData = [], mode = store.get('procs.mode', 'flat') === 'tree' ? 'tree' : 'flat';
  let sortKey = store.get('procs.sort', 'rss'), collapsed = new Set();
  const MAX_ROWS = 400;

  async function loadProcs() {
    try { procsData = await api('/api/procs'); renderProcs(); }
    catch (e) { $('#procs-t').innerHTML = `<tr><td colspan=7 class="empty">${esc(e.message)}</td></tr>`; }
  }
  const cmp = {
    rss: (a, b) => b.rss - a.rss, cpu: (a, b) => (b.cpu - a.cpu) || (b.rss - a.rss), name: (a, b) => a.name.localeCompare(b.name) || a.pid - b.pid,
    pid: (a, b) => a.pid - b.pid, started: (a, b) => (b.started || 0) - (a.started || 0),
  };
  const cpuCell = c => c < 0 ? `<td class="pr-cpu muted" title="${esc(t('procs.cpuWait', 'CPU appears after the second sample'))}">–</td>`
    : `<td class="pr-cpu${c >= 50 ? ' hot' : c >= 10 ? ' warm' : ''}">${c.toFixed(1)}%</td>`;

  function procRow(p, max, depth, twisty) {
    const pad = mode === 'tree' ? `<span class="pr-ind" style="width:${depth * 16}px"></span>${twisty}` : '';
    return `<tr data-row="${p.pid}" class="${drawer.pid === p.pid ? 'pr-sel' : ''}"><td><div class="pr-name">${pad}<span title="${esc(p.name)}">${esc(p.name) || '<span class="muted">?</span>'}</span></div></td>
      <td class="pid">${p.pid}</td><td class="pid">${p.ppid}</td>${cpuCell(p.cpu)}
      <td><div class="memcell"><span>${mb(p.rss)}</span><i style="width:${Math.max(2, p.rss / max * 100)}px"></i></div></td>
      <td class="muted" title="${esc(fmtTime(p.started))}">${esc(agoMs(p.started))}</td>
      <td class="act"><button class="btn sm" data-det="${p.pid}">${esc(t('procs.details', 'Details'))}</button><button class="btn sm danger" data-kill="${p.pid}" data-name="${esc(p.name)}">${esc(t('procs.kill', 'Kill'))}</button><button class="btn sm" data-kill="${p.pid}" data-name="${esc(p.name)}" data-tree="1">${esc(t('procs.killTree', 'Kill tree'))}</button></td></tr>`;
  }

  function renderProcs() {
    const q = $('#procs-q').value.trim().toLowerCase();
    const match = p => !q || `${p.name} ${p.pid}`.toLowerCase().includes(q);
    const sorter = cmp[sortKey] || cmp.rss;
    document.querySelectorAll('.pr-table th[data-sort]').forEach(th => th.classList.toggle('pr-sorted', th.dataset.sort === sortKey));
    document.querySelectorAll('#procs-mode button').forEach(b => b.classList.toggle('on', b.dataset.mode === mode));
    const max = Math.max(1, ...procsData.map(p => p.rss));
    let html = '', shown = 0;
    if (mode === 'flat') {
      const rows = procsData.filter(match).sort(sorter);
      shown = Math.min(rows.length, MAX_ROWS);
      html = rows.slice(0, MAX_ROWS).map(p => procRow(p, max, 0, '')).join('');
      $('#procs-count').textContent = rows.length > MAX_ROWS ? t('procs.capped', 'showing {n} of {total}').replace('{n}', MAX_ROWS).replace('{total}', rows.length) : t('procs.shown', '{n} processes').replace('{n}', rows.length);
    } else {
      // Parent link only when the parent started before the child (Windows reuses PIDs).
      const byPid = new Map(procsData.map(p => [p.pid, p])), kids = new Map();
      const parentOf = p => { const par = byPid.get(p.ppid); return par && par.pid !== p.pid && (!par.started || !p.started || par.started <= p.started) ? par : null; };
      const roots = [];
      for (const p of procsData) { const par = parentOf(p); if (par) { if (!kids.has(par.pid)) kids.set(par.pid, []); kids.get(par.pid).push(p); } else roots.push(p); }
      let keep = null; // when filtering: matches plus their ancestors
      if (q) {
        keep = new Set();
        for (const p of procsData) if (match(p)) for (let x = p, n = 0; x && !keep.has(x.pid) && n < 64; x = parentOf(x), n++) keep.add(x.pid);
      }
      const out = [];
      const walk = (p, depth, guard) => {
        if (keep && !keep.has(p.pid)) return;
        if (out.length >= MAX_ROWS || guard.has(p.pid)) return; guard.add(p.pid);
        const ch = (kids.get(p.pid) || []).filter(c => !keep || keep.has(c.pid)).sort(sorter);
        const open = q || !collapsed.has(p.pid);
        const tw = ch.length ? `<button class="pr-tw" data-tw="${p.pid}" aria-label="toggle">${open ? '▾' : '▸'}</button>` : '<span class="pr-tw"></span>';
        out.push(procRow(p, max, depth, tw));
        if (open) ch.forEach(c => walk(c, depth + 1, guard));
      };
      const guard = new Set();
      roots.sort(sorter).forEach(r => walk(r, 0, guard));
      shown = out.length; html = out.join('');
      const total = keep ? keep.size : procsData.length;
      $('#procs-count').textContent = out.length >= MAX_ROWS && total > MAX_ROWS ? t('procs.capped', 'showing {n} of {total}').replace('{n}', out.length).replace('{total}', total) : t('procs.shown', '{n} processes').replace('{n}', total);
    }
    $('#procs-t').innerHTML = shown ? html : `<tr><td colspan=7 class="empty">${esc(t('procs.none', 'Nothing matches.'))}</td></tr>`;
  }
  $('#procs-q').oninput = renderProcs;
  $('#procs-r').onclick = loadProcs;
  $('#procs-mode').onclick = e => { const b = e.target.closest('[data-mode]'); if (!b) return; mode = b.dataset.mode; store.set('procs.mode', mode); renderProcs(); };
  document.querySelector('.pr-table thead').onclick = e => {
    const th = e.target.closest('th[data-sort]'); if (!th) return;
    sortKey = th.dataset.sort; store.set('procs.sort', sortKey); renderProcs();
  };
  $('#procs-t').onclick = async e => {
    const tw = e.target.closest('[data-tw]'), k = e.target.closest('[data-kill]'), d = e.target.closest('[data-det]');
    if (tw) { const pid = +tw.dataset.tw; collapsed.has(pid) ? collapsed.delete(pid) : collapsed.add(pid); renderProcs(); }
    else if (k) { if (await killPid(+k.dataset.kill, !!k.dataset.tree, k.dataset.name)) loadProcs(); }
    else if (d) openProcDetails(+d.dataset.det);
    else { const r = e.target.closest('tr[data-row]'); if (r && !e.target.closest('button,a')) openProcDetails(+r.dataset.row); }
  };
  onTab('procs', () => { loadProcs(); setTimeout(() => current === 'procs' && loadProcs(), 1200); });
  every(3000, 'procs', loadProcs);

  // ---------------------------------------------------------------- details drawer
  // Samples RAM/CPU every 2 s only while the drawer is open and the Processes tab is visible.
  const HIST = 120;
  const drawer = { pid: null, info: null, hist: [], dead: false, n: 0 };
  const dEl = $('#procs-drawer');

  function spark(vals, lo, hi, cls) {
    const W = 340, H = 54;
    if (vals.length < 2) return `<svg class="pr-spark ${cls}" viewBox="0 0 ${W} ${H}" preserveAspectRatio="none"></svg>`;
    const span = hi - lo || 1, step = W / (HIST - 1), x0 = W - (vals.length - 1) * step;
    const pts = vals.map((v, i) => `${(x0 + i * step).toFixed(1)},${(H - 2 - (v - lo) / span * (H - 6)).toFixed(1)}`);
    return `<svg class="pr-spark ${cls}" viewBox="0 0 ${W} ${H}" preserveAspectRatio="none"><polygon points="${x0.toFixed(1)},${H} ${pts.join(' ')} ${W},${H}"/><polyline points="${pts.join(' ')}"/></svg>`;
  }
  function renderStats() {
    const box = dEl.querySelector('.pr-stats'); if (!box) return;
    const last = drawer.hist[drawer.hist.length - 1] || {};
    const cpus = drawer.hist.map(h => Math.max(0, h.cpu)), rams = drawer.hist.map(h => h.rss);
    const rlo = Math.min(...rams), rhi = Math.max(...rams);
    box.innerHTML = `
      <div class="pr-stat"><div class="pr-stat-h"><span>${esc(t('drawer.cpu', 'CPU'))}</span><b>${last.cpu >= 0 ? last.cpu.toFixed(1) + '%' : '–'}</b></div>${spark(cpus, 0, Math.max(5, ...cpus), 'cpu')}</div>
      <div class="pr-stat"><div class="pr-stat-h"><span>${esc(t('drawer.ram', 'RAM'))}</span><b>${last.rss ? mb(last.rss) : '–'}</b></div>${spark(rams, rlo * 0.98, rhi * 1.02 || 1, 'ram')}</div>
      <div class="pr-hint muted">${esc(drawer.dead ? t('drawer.exited', 'The process has exited.') : t('drawer.sampling', 'Sampled every 2 s while this panel is open'))}</div>`;
  }
  function renderDrawer() {
    const d = drawer.info; if (!d) return;
    const field = (label, val, extra = '') => `<div class="pr-f"><div class="pr-fl">${esc(label)}${extra}</div>${val}</div>`;
    const unknown = `<span class="muted">${esc(t('drawer.unknown', 'not readable (access denied)'))}</span>`;
    const kids = d.children || [];
    dEl.innerHTML = `
      <div class="card-head"><div class="pr-dt"><b title="${esc(d.name)}">${esc(d.name)}</b>${devKindPill(d.kind, d.kindCat)}<span class="pid">PID ${d.pid}</span></div>
        <button class="icon-btn" data-x title="${esc(t('drawer.close', 'Close'))}">×</button></div>
      <div class="pr-body">
        <div class="pr-stats"></div>
        <div class="pr-meta"><span>${esc(t('drawer.threads', 'Threads'))}: <b>${d.threads || '–'}</b></span><span title="${esc(fmtTime(d.started))}">${esc(t('drawer.started', 'Started'))}: <b>${esc(agoMs(d.started) || '–')}</b></span></div>
        ${field(t('drawer.cmdline', 'Command line'), d.cmdline ? `<pre class="pr-pre mono">${esc(d.cmdline)}</pre>` : unknown, d.cmdline ? ` <button class="btn sm ghost" data-copy="cmdline">${esc(t('drawer.copy', 'Copy'))}</button>` : '')}
        ${field(t('drawer.cwd', 'Working directory'), d.cwd ? `<pre class="pr-pre mono">${esc(d.cwd)}</pre>` : unknown, d.cwd ? ` <button class="btn sm ghost" data-copy="cwd">${esc(t('drawer.copy', 'Copy'))}</button><button class="btn sm ghost" data-term>${esc(t('drawer.term', 'Open terminal here'))}</button>` : '')}
        ${field(t('drawer.parent', 'Parent'), d.parent ? `<a href="#" data-go="${d.ppid}">${esc(d.parent)}</a> <span class="pid">${d.ppid}</span>` : `<span class="pid">${d.ppid}</span>`)}
        ${field(t('drawer.ports', 'Listening ports'), (d.ports || []).length ? `<div class="pt-chips">${d.ports.map(p => `<a class="port pt-chip" href="${esc(portURL(p))}" target="_blank" rel="noopener" title="${esc(p.addr)}">${p.port}</a>`).join('')}</div>` : `<span class="muted">${esc(t('drawer.noPorts', 'No listening ports.'))}</span>`)}
        ${field(`${t('drawer.children', 'Children')} (${kids.length})`, kids.length ? `<div class="pr-kids">${kids.slice(0, 50).map(c => `<a href="#" data-go="${c.pid}"><span>${esc(c.name)}</span><span class="pid">${c.pid}</span><span class="muted">${mb(c.rss)}</span></a>`).join('')}${kids.length > 50 ? `<div class="muted">${esc(t('drawer.more', 'and {n} more').replace('{n}', kids.length - 50))}</div>` : ''}</div>` : `<span class="muted">${esc(t('drawer.noKids', 'No child processes.'))}</span>`)}
        <div class="pr-dact"><button class="btn sm danger" data-dkill>${esc(t('procs.kill', 'Kill'))}</button><button class="btn sm" data-dkill="tree">${esc(t('procs.killTree', 'Kill tree'))}</button></div>
      </div>`;
    renderStats();
  }
  function push(s) {
    drawer.hist.push({ cpu: s.cpu, rss: s.rss });
    if (drawer.hist.length > HIST) drawer.hist.splice(0, drawer.hist.length - HIST);
  }
  async function loadFull(pid) {
    try {
      const d = await api(`/api/procs/detail?pid=${pid}`);
      if (drawer.pid !== pid) return;
      drawer.info = d; drawer.dead = false; push(d); renderDrawer();
    } catch (e) {
      if (drawer.pid !== pid) return;
      if (!drawer.info) { dEl.innerHTML = `<div class="card-head"><b>PID ${pid}</b><button class="icon-btn" data-x>×</button></div><div class="empty">${esc(e.message)}</div>`; drawer.dead = true; }
      else { drawer.dead = true; renderStats(); }
    }
  }
  async function sample() {
    const pid = drawer.pid;
    if (pid == null || drawer.dead) return;
    // Children and ports change rarely: refresh them every 5th sample (10 s), RAM/CPU every time.
    if (++drawer.n % 5 === 0) return loadFull(pid);
    try { const s = await api(`/api/procs/detail?pid=${pid}&lite=1`); if (drawer.pid === pid) { push(s); renderStats(); } }
    catch { if (drawer.pid === pid) { drawer.dead = true; renderStats(); } }
  }
  openProcDetails = pid => {
    Object.assign(drawer, { pid, info: null, hist: [], dead: false, n: 0 });
    dEl.hidden = false; dEl.innerHTML = '<div class="empty">…</div>';
    document.querySelectorAll('#procs-t tr.pr-sel').forEach(r => r.classList.remove('pr-sel'));
    document.querySelector(`#procs-t tr[data-row="${pid}"]`)?.classList.add('pr-sel');
    loadFull(pid);
  };
  const closeDrawer = () => { Object.assign(drawer, { pid: null, info: null, hist: [], dead: false }); dEl.hidden = true; dEl.innerHTML = ''; document.querySelectorAll('#procs-t tr.pr-sel').forEach(r => r.classList.remove('pr-sel')); };
  every(2000, 'procs', sample);
  dEl.onclick = async e => {
    const go = e.target.closest('[data-go]'), cp = e.target.closest('[data-copy]'), dk = e.target.closest('[data-dkill]');
    if (e.target.closest('[data-x]')) closeDrawer();
    else if (go) { e.preventDefault(); openProcDetails(+go.dataset.go); }
    else if (cp && drawer.info) copy(drawer.info[cp.dataset.copy]);
    else if (e.target.closest('[data-term]') && drawer.info) {
      if (typeof newTerm !== 'function') return toast('Terminal module is not loaded', 'err');
      show('term'); newTerm(drawer.info.name, { kind: 'local', cwd: drawer.info.cwd });
    } else if (dk && drawer.info) {
      if (await killPid(drawer.pid, dk.dataset.dkill === 'tree', drawer.info.name)) { drawer.dead = true; renderStats(); loadProcs(); }
    }
  };
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape' && current === 'procs' && drawer.pid != null && !document.querySelector('dialog[open]')) closeDrawer();
  });
})();
