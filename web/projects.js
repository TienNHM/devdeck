'use strict';
// Projects tab (feature module). Renders into <section id="projects">.
// Project folders live in ~/.devdeck/projects.json; tasks are detected server-side and run in a
// devdeck terminal (newTerm from term.js, looked up at click time because term.js loads later).

registerStrings('vi', {
  'projects.pathPh': 'Thư mục dự án, ví dụ D:\\code\\my-app',
  'projects.add': 'Thêm dự án', 'projects.filter': 'Lọc dự án hoặc tác vụ', 'projects.refresh': 'Làm mới',
  'projects.added': 'Đã thêm {name}', 'projects.removed': 'Đã gỡ {name}',
  'projects.remove': 'Gỡ khỏi danh sách', 'projects.removeTitle': 'Gỡ {name}?',
  'projects.removeText': 'Chỉ gỡ khỏi devdeck. Thư mục và các tệp không bị động đến.',
  'projects.emptyTitle': 'Chưa có dự án nào',
  'projects.emptyText': 'Dán đường dẫn thư mục dự án vào ô phía trên rồi bấm Thêm dự án. devdeck sẽ tự tìm các tác vụ (npm, make, dotnet, go, cargo, python, docker compose…).',
  'projects.noMatch': 'Không có dự án nào phù hợp.',
  'projects.missing': 'Thư mục không còn tồn tại',
  'projects.noTasks': 'Không tìm thấy tác vụ nào.',
  'projects.term': 'Terminal', 'projects.termTip': 'Mở terminal trong thư mục này',
  'projects.claude': 'Claude', 'projects.claudeTip': 'Chạy claude trong thư mục này',
  'projects.git': 'Git', 'projects.gitTip': 'Mở trong tab Git',
  'projects.gitHint': 'Đã sao chép đường dẫn. Nếu tab Git đang mở kho khác, hãy chọn kho này trong danh sách (hoặc thêm bằng +).',
  'projects.clean': 'sạch', 'projects.dirty': '{n} thay đổi', 'projects.noRepo': 'không phải kho git',
  'projects.ports': 'Cổng đang chạy', 'projects.open': 'Mở', 'projects.stop': 'Dừng',
  'projects.stopTitle': 'Dừng {name} trên cổng {port}?', 'projects.stopText': 'PID {pid} và các tiến trình con sẽ bị kết thúc.',
  'projects.stopped': 'Đã dừng cổng {port}',
  'projects.runTip': 'Chạy: {cmd}', 'projects.running': 'đang chạy trên :{port}',
  'projects.noTerm': 'Chưa tải được module Terminal',
  'projects.cmdRun': 'Chạy {task} trong {project}', 'projects.cmdOpen': 'Mở dự án {project}', 'projects.cmdAdd': 'Thêm thư mục dự án',
});

(() => {
  let projects = [], loadedAt = 0, portsBy = {}, loading = null;
  const gitInfo = {};
  const list = $('#pj-list');
  const enc = encodeURIComponent;

  // ---------------------------------------------------------------- data
  function fetchProjects(maxAge = 0) {
    if (loading) return loading;
    if (maxAge && Date.now() - loadedAt < maxAge) return Promise.resolve(projects);
    loading = api('/api/projects').then(d => { projects = d || []; loadedAt = Date.now(); return projects; }).finally(() => { loading = null; });
    return loading;
  }
  async function load() {
    try { await fetchProjects(); render(); loadPorts(); loadGit(); }
    catch (e) { list.innerHTML = `<div class="empty">${esc(e.message)}</div>`; }
  }
  // git status for every project, at most 4 git processes at a time; only on open / refresh.
  async function loadGit() {
    const queue = projects.filter(p => !p.missing).map(p => p.path);
    const worker = async () => {
      for (let p; (p = queue.shift());) {
        try { gitInfo[p] = await api(`/api/projects/git?path=${enc(p)}`); } catch { gitInfo[p] = null; }
        renderGit(p);
      }
    };
    await Promise.all([worker(), worker(), worker(), worker()]);
  }
  async function loadPorts() {
    if (!projects.length) return;
    try {
      const res = await api('/api/projects/ports');
      portsBy = Object.fromEntries(res.map(x => [x.path, x.ports || []]));
      projects.forEach(p => renderLive(p.path));
    } catch { /* keep the previous view */ }
  }

  // ---------------------------------------------------------------- task <-> port link
  // The executable a task starts, guessed from its script body or command (vite, next, uvicorn, Api…).
  function taskBin(task) {
    const words = (task.body || '').split(/\s+/).filter(w => w && !w.includes('=') && !['cross-env', 'npx', 'pnpx', 'bunx', 'node', 'env', 'dotenv', '--'].includes(w));
    if (task.body) return words[0] ? words[0].split(/[\\/]/).pop().replace(/\.(js|cjs|mjs|cmd|exe)$/i, '').toLowerCase() || null : null;
    const c = task.cmd.split(/\s+/);
    if (c[0] === 'python' && c[1] === 'manage.py') return 'manage.py';
    if (/^(uv|poetry|pipenv)$/.test(c[0]) && c[1] === 'run') return (c[2] === 'python' ? c[4] || c[3] : c[2] || '').toLowerCase() || null;
    if (c[0] === 'python' && c[1] === '-m') return (c[2] || '').toLowerCase() || null;
    if (c[0] === 'dotnet' && c.includes('--project')) {
      const p = (c[c.indexOf('--project') + 1] || '').replace(/'/g, '');
      return p.split(/[\\/]/).pop().replace(/\.\w+proj$/i, '').toLowerCase() || null;
    }
    return null;
  }
  const tokens = s => new Set((s || '').toLowerCase().split(/[\s/\\"'=:]+/).map(x => x.replace(/\.(exe|js|cjs|mjs|cmd|dll)$/, '')));
  function linkTasks(p) {
    const ports = portsBy[p.path] || [], live = {};
    if (!ports.length) return live;
    const toks = ports.map(x => ({ port: x.port, set: tokens(`${x.process} ${x.cmdline || ''}`) }));
    p.tasks.forEach((task, i) => {
      const bin = taskBin(task); if (!bin) return;
      const hit = toks.find(x => x.set.has(bin));
      if (hit) live[i] = hit.port;
    });
    return live;
  }

  // ---------------------------------------------------------------- render
  const matches = (p, q) => !q || `${p.name} ${p.path} ${p.tasks.map(x => `${x.name} ${x.source}`).join(' ')}`.toLowerCase().includes(q);
  function render() {
    const q = $('#pj-q').value.trim().toLowerCase();
    if (!projects.length) {
      list.innerHTML = `<div class="pj-empty"><svg class="i"><use href="#i-projects"/></svg><h3>${esc(t('projects.emptyTitle', 'No projects yet'))}</h3><p class="muted">${esc(t('projects.emptyText', 'Paste a project folder path above and click Add project. devdeck finds its tasks (npm, make, dotnet, go, cargo, python, docker compose…).'))}</p></div>`;
      return;
    }
    const shown = projects.filter(p => matches(p, q));
    list.innerHTML = shown.length ? shown.map(card).join('') : `<div class="empty">${esc(t('projects.noMatch', 'No project matches.'))}</div>`;
    shown.forEach(p => { renderGit(p.path); renderLive(p.path); });
  }
  const cardOf = path => [...list.querySelectorAll('.pj-card')].find(c => c.dataset.path === path);
  function card(p) {
    const groups = new Map();
    p.tasks.forEach((task, i) => { if (!groups.has(task.source)) groups.set(task.source, []); groups.get(task.source).push([task, i]); });
    const tasks = p.missing ? `<div class="pj-warn">${esc(t('projects.missing', 'Folder no longer exists'))}</div>`
      : groups.size ? [...groups].map(([src, ts]) => `<div class="pj-group"><span class="pj-src pj-src-${esc(src)}">${esc(src)}</span><div class="pj-tasks">${ts.map(([task, i]) =>
        `<button class="btn sm pj-task" data-task="${i}" title="${esc(t('projects.runTip', 'Run: {cmd}').replace('{cmd}', task.cmd))}"><svg class="i"><use href="#i-play"/></svg>${esc(task.name)}<span class="pj-live-port"></span></button>`).join('')}</div></div>`).join('')
      : `<div class="muted pj-none">${esc(t('projects.noTasks', 'No tasks detected.'))}</div>`;
    return `<div class="card pj-card${p.missing ? ' pj-missing' : ''}" data-path="${esc(p.path)}">
      <div class="pj-head">
        <div class="pj-title"><div class="pj-name"><b>${esc(p.name)}</b><span class="pj-git"></span></div><div class="pj-path mono muted" title="${esc(p.path)}">${esc(p.path)}</div></div>
        <div class="pj-actions">
          <button class="btn sm" data-act="term" title="${esc(t('projects.termTip', 'Open a terminal in this folder'))}" ${p.missing ? 'disabled' : ''}><svg class="i"><use href="#i-term"/></svg>${esc(t('projects.term', 'Terminal'))}</button>
          <button class="btn sm" data-act="claude" title="${esc(t('projects.claudeTip', 'Run claude in this folder'))}" ${p.missing ? 'disabled' : ''}><svg class="i"><use href="#i-claude"/></svg>${esc(t('projects.claude', 'Claude'))}</button>
          <button class="btn sm" data-act="git" title="${esc(t('projects.gitTip', 'Open in the Git tab'))}" ${p.missing ? 'disabled' : ''}><svg class="i"><use href="#i-git"/></svg>${esc(t('projects.git', 'Git'))}</button>
          <button class="btn sm ghost pj-rm" data-act="rm" title="${esc(t('projects.remove', 'Remove from the list'))}"><svg class="i"><use href="#i-trash"/></svg></button>
        </div>
      </div>
      <div class="pj-body">${tasks}</div>
      <div class="pj-ports" hidden></div>
    </div>`;
  }
  function renderGit(path) {
    const el = cardOf(path)?.querySelector('.pj-git'); if (!el) return;
    const g = gitInfo[path];
    if (g === undefined) { el.innerHTML = ''; return; }
    if (!g || !g.repo) { el.innerHTML = `<span class="pj-chip muted">${esc(t('projects.noRepo', 'not a git repo'))}</span>`; return; }
    el.innerHTML = `<span class="pj-chip"><svg class="i"><use href="#i-git"/></svg>${esc(g.branch)}</span>`
      + (g.dirty ? `<span class="pj-chip pj-dirty" title="git status --porcelain">● ${esc(t('projects.dirty', '{n} changed').replace('{n}', g.dirty))}</span>` : `<span class="pj-chip pj-clean">${esc(t('projects.clean', 'clean'))}</span>`);
  }
  function renderLive(path) {
    const c = cardOf(path), p = projects.find(x => x.path === path); if (!c || !p) return;
    const ports = portsBy[path] || [], live = linkTasks(p);
    c.querySelectorAll('.pj-task').forEach(b => {
      const port = live[b.dataset.task];
      b.classList.toggle('pj-live', port != null);
      b.querySelector('.pj-live-port').textContent = port != null ? `:${port}` : '';
      if (port != null) b.title = `${t('projects.runTip', 'Run: {cmd}').replace('{cmd}', p.tasks[b.dataset.task].cmd)} — ${t('projects.running', 'running on :{port}').replace('{port}', port)}`;
    });
    const box = c.querySelector('.pj-ports');
    box.hidden = !ports.length;
    if (!ports.length) { box.innerHTML = ''; return; }
    const uniq = [...new Map(ports.map(x => [`${x.port}:${x.pid}`, x])).values()];
    box.innerHTML = `<div class="pj-ports-h">${esc(t('projects.ports', 'Running ports'))}</div>` + uniq.map(x => `<div class="pj-port">
      <a class="port" href="http://localhost:${x.port}" target="_blank" rel="noopener">:${x.port}</a>
      <span class="pj-pproc">${esc(x.process || '?')}${typeof devKindPill === 'function' ? devKindPill(x.kind, x.kindCat) : ''}</span>
      <a href="#" class="pid" data-pid="${x.pid}" title="${esc(x.cmdline || '')}">PID ${x.pid}</a>
      <span class="sp"></span>
      <button class="btn sm" data-openport="${x.port}"><svg class="i"><use href="#i-globe"/></svg>${esc(t('projects.open', 'Open'))}</button>
      <button class="btn sm danger" data-stop="${x.pid}" data-port="${x.port}" data-name="${esc(x.process || 'process')}"><svg class="i"><use href="#i-stop"/></svg>${esc(t('projects.stop', 'Stop'))}</button>
    </div>`).join('');
  }

  // ---------------------------------------------------------------- actions
  function term(title, init) {
    if (typeof newTerm !== 'function') { toast(t('projects.noTerm', 'Terminal module is not loaded'), 'err'); return; }
    show('term');
    newTerm(title, init);
  }
  const runTask = (p, task) => term(`${task.name} · ${p.name}`, { kind: 'local', cwd: p.path, cmd: task.cmd });
  async function openInGit(path) {
    if (typeof window.gitOpenRepo === 'function') { show('git'); return window.gitOpenRepo(path); }
    document.dispatchEvent(new CustomEvent('devdeck:git-open', { detail: path }));
    show('git');
    // The Git tab lists its saved repositories in #gg-repo once it has loaded; select ours if it is there.
    const norm = s => s.replace(/\\/g, '/').replace(/\/+$/, '').toLowerCase();
    for (let i = 0; i < 20; i++) {
      const sel = document.querySelector('#gg-repo');
      const opt = sel && [...sel.options].find(o => o.value && norm(o.value) === norm(path));
      if (opt) { if (sel.value !== opt.value) { sel.value = opt.value; sel.dispatchEvent(new Event('change')); } return; }
      await new Promise(r => setTimeout(r, 150));
    }
    try { await navigator.clipboard.writeText(path); } catch { /* clipboard may be blocked */ }
    toast(t('projects.gitHint', 'Path copied. If Git shows another repository, pick this one in its repository list (or add it with +).'));
  }
  async function addProject() {
    const inp = $('#pj-path'), path = inp.value.trim();
    if (!path) { inp.focus(); return; }
    try {
      const r = await post('/api/projects', { path });
      inp.value = '';
      toast(t('projects.added', 'Added {name}').replace('{name}', r.path.split(/[\\/]/).pop() || r.path));
      await load();
    } catch (e) { toast(e.message, 'err'); inp.focus(); }
  }
  async function removeProject(p) {
    const ok = await dialog({ title: t('projects.removeTitle', 'Remove {name}?').replace('{name}', p.name), text: t('projects.removeText', 'Only removes it from devdeck. The folder and its files are not touched.'), ok: t('projects.remove', 'Remove'), danger: true });
    if (!ok) return;
    try { await post('/api/projects/remove', { path: p.path }); toast(t('projects.removed', 'Removed {name}').replace('{name}', p.name)); await load(); }
    catch (e) { toast(e.message, 'err'); }
  }
  async function stopPort(b) {
    const pid = +b.dataset.stop, port = b.dataset.port, name = b.dataset.name;
    const ok = await dialog({
      title: t('projects.stopTitle', 'Stop {name} on port {port}?').replace('{name}', name).replace('{port}', port),
      text: t('projects.stopText', 'PID {pid} and its child processes will be terminated.').replace('{pid}', pid), ok: t('projects.stop', 'Stop'), danger: true,
    });
    if (!ok) return;
    try { await post('/api/kill', { pid, tree: true }); toast(t('projects.stopped', 'Stopped port {port}').replace('{port}', port)); }
    catch (e) { toast(e.message, 'err'); }
    loadPorts();
  }

  $('#pj-add').onclick = addProject;
  $('#pj-path').onkeydown = e => { if (e.key === 'Enter') { e.preventDefault(); addProject(); } };
  $('#pj-q').oninput = render;
  $('#pj-r').onclick = load;
  list.onclick = e => {
    const c = e.target.closest('.pj-card'); if (!c) return;
    const p = projects.find(x => x.path === c.dataset.path); if (!p) return;
    const task = e.target.closest('[data-task]'), act = e.target.closest('[data-act]');
    const stop = e.target.closest('[data-stop]'), op = e.target.closest('[data-openport]'), pid = e.target.closest('a[data-pid]');
    if (task) runTask(p, p.tasks[+task.dataset.task]);
    else if (stop) stopPort(stop);
    else if (op) window.open(`http://localhost:${op.dataset.openport}`, '_blank', 'noopener');
    else if (pid) { e.preventDefault(); if (typeof openProcDetails === 'function') { show('procs'); openProcDetails(+pid.dataset.pid); } }
    else if (act) {
      const a = act.dataset.act;
      if (a === 'term') term(p.name, { kind: 'local', cwd: p.path });
      else if (a === 'claude') term(`claude · ${p.name}`, { kind: 'local', cwd: p.path, cmd: 'claude' });
      else if (a === 'git') openInGit(p.path);
      else if (a === 'rm') removeProject(p);
    }
  };
  onTab('projects', load);
  every(5000, 'projects', loadPorts);
  document.addEventListener('devdeck:killed', () => { if (current === 'projects') loadPorts(); });

  // ---------------------------------------------------------------- Ctrl+K
  function focusProject(p) {
    show('projects');
    const c = cardOf(p.path) || (render(), cardOf(p.path));
    if (!c) return;
    c.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    c.classList.remove('pj-flash'); void c.offsetWidth; c.classList.add('pj-flash');
  }
  registerCommand({ title: t('projects.cmdAdd', 'Add project folder'), keywords: 'project folder add new', run: () => { show('projects'); $('#pj-path').focus(); } });
  registerCommandProvider(async q => {
    if (!q) return [];
    const run = q.match(/^run\b\s*(.*)$/), open = q.match(/^open\b\s*(?:project\s*)?(.*)$/);
    if (!run && !open && q.length < 2) return [];
    let ps;
    try { ps = await fetchProjects(10000); } catch { return []; }
    if (run) {
      const words = run[1].split(/\s+/).filter(w => w && w !== 'in');
      const out = [];
      for (const p of ps) {
        if (p.missing) continue;
        for (const task of p.tasks) {
          const hay = `${task.name} ${task.source} ${p.name}`.toLowerCase();
          if (words.every(w => hay.includes(w))) out.push({ title: t('projects.cmdRun', 'Run {task} in {project}').replace('{task}', task.name).replace('{project}', p.name), hint: task.cmd, run: () => runTask(p, task) });
          if (out.length >= 30) return out;
        }
      }
      return out;
    }
    const f = open ? open[1].trim() : q;
    return ps.filter(p => !f || p.name.toLowerCase().includes(f)).slice(0, open ? 20 : 5)
      .map(p => ({ title: t('projects.cmdOpen', 'Open project {project}').replace('{project}', p.name), hint: p.path, run: () => focusProject(p) }));
  });
})();
