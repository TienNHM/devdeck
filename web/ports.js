'use strict';
// Ports + Processes tabs.

async function kill(pid, tree, name) {
  const ok = await dialog({ title: `Kill ${name}?`, text: `PID ${pid}${tree ? ' and its child processes' : ''} will be terminated. Unsaved work in it is lost.`, ok: 'Kill', danger: true });
  if (!ok) return false;
  try { await post('/api/kill', { pid, tree }); toast(`Killed ${name} (${pid})`); } catch (e) { toast(e.message, 'err'); return false; }
  if (current === 'ports') loadPorts(); else if (current === 'procs') loadProcs();
  return true;
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
onTab('ports', loadPorts);
every(3000, 'ports', loadPorts);

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
onTab('procs', loadProcs);
every(3000, 'procs', loadProcs);
