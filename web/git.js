'use strict';
// Git tab: a lane graph with context menus, commit/compare details, diffs, code review, find,
// remotes/settings, and keyboard shortcuts. Backend: /api/git/* (see gitroutes.go).
(() => {
const root = document.getElementById('gg');
if (!root) return;
if (typeof api !== 'function' || typeof post !== 'function' || typeof esc !== 'function') {
  root.textContent = 'git.js needs the helpers from app.js (api, post, esc, store, toast).';
  return;
}
const q = (s, r = document) => r.querySelector(s);
const qa = (s, r = document) => [...r.querySelectorAll(s)];
const enc = encodeURIComponent;
const RH = 26, LW = 16;
const COLORS = ['#5b8def', '#2fb26c', '#e0a030', '#e0524d', '#a574e8', '#e8703a', '#22a5b8', '#d4559f', '#8a9a2a', '#6b7cff'];
const colorOf = l => COLORS[l % COLORS.length];
const LS = 'gg.';
const jget = (k, d) => { try { const v = JSON.parse(store.get(LS + k, '')); return v ?? d; } catch { return d; } };
const jset = (k, v) => store.set(LS + k, JSON.stringify(v));
const p2 = n => String(n).padStart(2, '0');
const fmtDate = ts => { const d = new Date(ts * 1000); return `${d.getFullYear()}-${p2(d.getMonth() + 1)}-${p2(d.getDate())} ${p2(d.getHours())}:${p2(d.getMinutes())}`; };
const short = x => (x || '').slice(0, 7);
const base = p => p.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || p;

const DEF = {
  remote: true, tags: true, stashes: true, uncommitted: true, muteNonHead: true, muteMerge: true, firstParent: false,
  order: 'date', dateType: 'author', style: 'curved', combine: true,
  cols: { date: true, author: true, hash: true }, widths: { date: 150, author: 140, hash: 76 },
  dock: 'bottom', fileView: 'tree', compact: true, diffMode: 'unified', initial: 300, more: 300, auto: true, detH: 320, detW: 520,
};
const P = Object.assign({}, DEF, jget('prefs', {}));
P.cols = Object.assign({}, DEF.cols, P.cols);
P.widths = Object.assign({}, DEF.widths, P.widths);
const savePrefs = () => jset('prefs', P);

const S = {
  repo: store.get(LS + 'repo', '') || store.get('repo', ''), repos: [], cfg: null, refs: null, log: null, rows: [],
  idx: new Map(), kids: new Map(), limit: P.initial, loading: false, busy: false, sel: { a: null, b: null },
  model: null, detSeq: 0, filter: [], sig: '', tip: new Map(), find: { open: false, q: '', m: [], i: -1 },
  collapsed: new Set(), inited: false,
};

// ------------------------------------------------------------------ emoji / rich text
const EMOJI = Object.fromEntries(('sparkles:✨ bug:🐛 memo:📝 art:🎨 zap:⚡ fire:🔥 ambulance:🚑 rocket:🚀 lipstick:💄 tada:🎉 white_check_mark:✅ ' +
  'lock:🔒 bookmark:🔖 rotating_light:🚨 construction:🚧 green_heart:💚 arrow_down:⬇️ arrow_up:⬆️ pushpin:📌 construction_worker:👷 ' +
  'chart_with_upwards_trend:📈 recycle:♻️ heavy_plus_sign:➕ heavy_minus_sign:➖ wrench:🔧 hammer:🔨 globe_with_meridians:🌐 pencil2:✏️ ' +
  'poop:💩 rewind:⏪ twisted_rightwards_arrows:🔀 package:📦 alien:👽 truck:🚚 page_facing_up:📄 boom:💥 bento:🍱 wheelchair:♿ bulb:💡 ' +
  'beers:🍻 speech_balloon:💬 card_file_box:🗃️ loud_sound:🔊 mute:🔇 busts_in_silhouette:👥 children_crossing:🚸 building_construction:🏗️ ' +
  'iphone:📱 clown_face:🤡 egg:🥚 see_no_evil:🙈 camera_flash:📸 alembic:⚗️ mag:🔍 label:🏷️ seedling:🌱 triangular_flag_on_post:🚩 ' +
  'goal_net:🥅 dizzy:💫 wastebasket:🗑️ passport_control:🛂 adhesive_bandage:🩹 monocle_face:🧐 coffin:⚰️ test_tube:🧪 necktie:👔 ' +
  'stethoscope:🩺 bricks:🧱 technologist:🧑‍💻 money_with_wings:💸 thread:🧵 safety_vest:🦺 +1:👍 -1:👎 heart:❤️ warning:⚠️ x:❌ ' +
  'heavy_check_mark:✔️ star:⭐ smile:😄 rage:😡 eyes:👀 pray:🙏 hankey:💩 shipit:🐿️ lock_with_ink_pen:🔏 closed_lock_with_key:🔐 ' +
  'wip:🚧 mega:📣 books:📚 gear:⚙️ link:🔗 hourglass:⌛ trophy:🏆 sunglasses:😎 thinking:🤔 white_flag:🏳️ tada2:🎊 snowflake:❄️').split(' ').map(s => {
  const i = s.indexOf(':', 1); return [s.slice(0, i), s.slice(i + 1)];
}));

function splitTok(toks, re, fn) {
  const out = [];
  for (const k of toks) {
    if (k.h !== undefined) { out.push(k); continue; }
    let last = 0;
    for (const m of k.t.matchAll(re)) {
      if (m[0] === '') continue;
      if (m.index > last) out.push({ t: k.t.slice(last, m.index) });
      out.push({ h: fn(m) });
      last = m.index + m[0].length;
    }
    if (last < k.t.length) out.push({ t: k.t.slice(last) });
  }
  return out;
}
function issueCfg() {
  if (S.issueFor !== S.repo) {
    S.issueFor = S.repo; S.issue = null;
    const c = jget('issue:' + S.repo, null);
    if (c && c.regex && /^https?:\/\//i.test(c.url || '')) { try { S.issue = { re: new RegExp(c.regex, 'g'), url: c.url }; } catch { /* bad regex: ignore */ } }
  }
  return S.issue;
}
function rich(text) {
  let t = splitTok([{ t: String(text ?? '') }], /`([^`\n]+)`/g, m => `<code>${esc(m[1])}</code>`);
  t = splitTok(t, /https?:\/\/[^\s<>"'`)\]]+/g, m => `<a href="${esc(m[0])}" target="_blank" rel="noopener noreferrer">${esc(m[0])}</a>`);
  const iss = issueCfg();
  if (iss) t = splitTok(t, iss.re, m => `<a href="${esc(iss.url.replace(/\$(\d)/g, (x, n) => enc(m[+n] || '')))}" target="_blank" rel="noopener noreferrer">${esc(m[0])}</a>`);
  t = splitTok(t, /\*\*([^*\n]+)\*\*/g, m => `<b>${esc(m[1])}</b>`);
  t = splitTok(t, /(?<![\w*])\*([^*\n]+)\*(?![\w*])/g, m => `<i>${esc(m[1])}</i>`);
  return t.map(k => k.h !== undefined ? k.h : esc(k.t).replace(/:([a-z0-9_+-]+):/g, (m, n) => EMOJI[n] || m)).join('');
}
function plainMsg(text) { return String(text ?? '').replace(/:([a-z0-9_+-]+):/g, (m, n) => EMOJI[n] || m); }

// ------------------------------------------------------------------ small UI kit
const modals = [];
function modal(inner, o = {}) {
  const el = document.createElement('div');
  el.className = 'gg-modal';
  el.innerHTML = `<div class="gg-mbox ${o.cls || ''}">${inner}</div>`;
  document.body.appendChild(el);
  const m = {
    el, box: el.firstElementChild,
    close() { if (!el.isConnected) return; el.remove(); const i = modals.indexOf(m); if (i >= 0) modals.splice(i, 1); o.onClose && o.onClose(); },
  };
  el.addEventListener('mousedown', e => { if (e.target === el && !o.sticky) m.close(); });
  modals.push(m);
  return m;
}

function fieldHTML(f) {
  const id = 'f_' + f.key, w = f.showIf ? ' data-showif="1"' : '';
  const hint = f.hint ? `<small class="hint">${f.hint}</small>` : '';
  switch (f.type) {
    case 'checkbox': return `<label class="f chk" data-k="${f.key}"${w}><input type="checkbox" name="${f.key}" ${f.value ? 'checked' : ''}> <span>${f.label}</span>${hint}</label>`;
    case 'textarea': return `<label class="f" data-k="${f.key}"${w}><span>${f.label}</span><textarea name="${f.key}" rows="${f.rows || 4}" placeholder="${esc(f.placeholder || '')}">${esc(f.value || '')}</textarea>${hint}</label>`;
    case 'select': return `<label class="f" data-k="${f.key}"${w}><span>${f.label}</span><select name="${f.key}">${f.options.map(o => `<option value="${esc(o.v)}" ${o.v === f.value ? 'selected' : ''}>${esc(o.label)}</option>`).join('')}</select>${hint}</label>`;
    case 'radio': return `<div class="f" data-k="${f.key}"${w}><span>${f.label}</span>${f.options.map(o => `<label class="chk"><input type="radio" name="${f.key}" value="${esc(o.v)}" ${o.v === f.value ? 'checked' : ''}> <b>${esc(o.label)}</b>${o.desc ? ` <small class="hint">${esc(o.desc)}</small>` : ''}</label>`).join('')}${hint}</div>`;
    case 'multi': return `<div class="f" data-k="${f.key}"${w}><span>${f.label}</span>${f.options.map(o => `<label class="chk"><input type="checkbox" name="${f.key}" value="${esc(o.v)}" ${(f.value || []).includes(o.v) ? 'checked' : ''}> ${esc(o.label)}</label>`).join('')}${hint}</div>`;
    default: return `<label class="f" data-k="${f.key}"${w}><span>${f.label}</span><input id="${id}" name="${f.key}" value="${esc(f.value || '')}" placeholder="${esc(f.placeholder || '')}" autocomplete="off" spellcheck="false">${hint}</label>`;
  }
}
// form(): resolves with the values, or null when cancelled. Labels/intro/hints are trusted HTML (escape dynamic parts).
function form({ title, intro = '', fields = [], ok = 'OK', danger = false, wide = false }) {
  return new Promise(res => {
    let done = false;
    const m = modal(`<form class="gg-form" novalidate><h3>${title}</h3>${intro ? `<div class="gg-intro">${intro}</div>` : ''}
      <div class="gg-fields">${fields.map(fieldHTML).join('')}</div><div class="gg-err" hidden></div>
      <div class="gg-factions"><button type="submit" class="btn ${danger ? 'danger-solid' : 'primary'}">${esc(ok)}</button><button type="button" class="btn" data-cancel>Cancel</button></div></form>`,
      { cls: wide ? 'wide' : '', onClose: () => { if (!done) { done = true; res(null); } } });
    const f = q('form', m.box);
    const collect = () => {
      const v = {};
      for (const fd of fields) {
        const els = qa(`[name="${fd.key}"]`, f);
        if (fd.type === 'checkbox') v[fd.key] = els[0].checked;
        else if (fd.type === 'radio') v[fd.key] = (els.find(e => e.checked) || {}).value;
        else if (fd.type === 'multi') v[fd.key] = els.filter(e => e.checked).map(e => e.value);
        else v[fd.key] = els[0].value;
      }
      return v;
    };
    const refresh = () => { const v = collect(); for (const fd of fields) if (fd.showIf) q(`[data-k="${fd.key}"]`, f).hidden = !fd.showIf(v); };
    f.addEventListener('input', refresh); f.addEventListener('change', refresh); refresh();
    q('[data-cancel]', f).onclick = () => m.close();
    f.onsubmit = e => {
      e.preventDefault();
      const v = collect();
      for (const fd of fields) {
        if (fd.showIf && !fd.showIf(v)) continue;
        const val = v[fd.key];
        let err = null;
        if (fd.required && (!val || (typeof val === 'string' && !val.trim()) || (Array.isArray(val) && !val.length))) err = `${fd.label.replace(/<[^>]+>/g, '')} is required`;
        else if (fd.validate) err = fd.validate(val, v);
        if (err) { const el = q('.gg-err', f); el.textContent = err; el.hidden = false; return; }
      }
      done = true; m.close(); res(v);
    };
    const first = q('input:not([type=checkbox]):not([type=radio]),textarea,select', f);
    (first || q('button[type=submit]', f)).focus();
    if (first && first.select) first.select();
  });
}
const confirmBox = (title, intro, ok = 'OK', danger = false) => form({ title, intro, ok, danger });

let menuEl = null;
function closeMenu() { if (menuEl) { menuEl.remove(); menuEl = null; } }
function showMenu(x, y, items) {
  closeMenu(); closeDd();
  const el = document.createElement('div');
  el.className = 'gg-menu';
  el.innerHTML = items.filter(Boolean).map((it, i) => it === '-' ? '<hr>' :
    `<div class="mi ${it.disabled ? 'dis' : ''} ${it.danger ? 'dg' : ''}" data-i="${i}">${esc(it.label)}</div>`).join('');
  document.body.appendChild(el);
  el.style.left = Math.max(4, Math.min(x, innerWidth - el.offsetWidth - 6)) + 'px';
  el.style.top = Math.max(4, Math.min(y, innerHeight - el.offsetHeight - 6)) + 'px';
  const list = items.filter(Boolean);
  el.onclick = e => {
    const mi = e.target.closest('.mi'); if (!mi || mi.classList.contains('dis')) return;
    closeMenu(); list[+mi.dataset.i].run();
  };
  el.oncontextmenu = e => e.preventDefault();
  menuEl = el;
}

let ddEl = null, ddAnchor = null;
function closeDd() { if (ddEl) { ddEl.remove(); ddEl = null; ddAnchor = null; } }
function openDd(anchor, html, mount) {
  if (ddEl && ddAnchor === anchor) { closeDd(); return; }
  closeDd(); closeMenu();
  const el = document.createElement('div');
  el.className = 'gg-dd'; el.innerHTML = html; document.body.appendChild(el);
  const r = anchor.getBoundingClientRect();
  el.style.left = Math.max(4, Math.min(r.left, innerWidth - el.offsetWidth - 8)) + 'px';
  el.style.top = (r.bottom + 4) + 'px';
  ddEl = el; ddAnchor = anchor;
  mount && mount(el);
}
document.addEventListener('mousedown', e => {
  if (ddEl && !ddEl.contains(e.target) && !ddAnchor.contains(e.target)) closeDd();
  if (menuEl && !menuEl.contains(e.target)) closeMenu();
}, true);
window.addEventListener('resize', () => { closeMenu(); closeDd(); });

function copyText(t, what) {
  const ok = () => toast(`${what || 'Text'} copied to clipboard`);
  const fb = () => { const ta = document.createElement('textarea'); ta.value = t; ta.style.position = 'fixed'; ta.style.opacity = '0'; document.body.appendChild(ta); ta.select(); try { document.execCommand('copy'); ok(); } catch { toast('Copy failed', 'err'); } ta.remove(); };
  if (navigator.clipboard) navigator.clipboard.writeText(t).then(ok, fb); else fb();
}

// ------------------------------------------------------------------ scaffold
root.innerHTML = `
<div class="gg-bar toolbar">
  <select id="gg-repo" title="Repository"></select>
  <button class="btn sm" id="gg-addrepo" title="Add repository">+</button>
  <button class="btn sm" id="gg-rmrepo" title="Remove this repository from the list">&minus;</button>
  <button class="btn" id="gg-br"></button>
  <button class="btn" id="gg-view">View &#9662;</button>
  <span class="sp"></span>
  <span id="gg-op"></span>
  <span id="gg-info" class="info"></span>
  <button class="btn" id="gg-find" title="Find (Ctrl+F)">Find</button>
  <button class="btn" id="gg-fetch" title="Fetch from remotes">Fetch</button>
  <button class="btn" id="gg-refresh" title="Refresh (Ctrl+R)">Refresh</button>
  <button class="btn" id="gg-settings" title="Repository settings">Settings</button>
</div>
<div class="gg-split dock-${P.dock}" id="gg-split">
  <div class="card gg-main" id="gg-main">
    <div class="gg-findbar" id="gg-findbar" hidden>
      <input id="gg-fq" placeholder="Find in commit message, author, hash, date, branch or tag…" autocomplete="off">
      <span id="gg-fc" class="muted"></span>
      <button class="btn sm" id="gg-fprev" title="Previous (Shift+Enter)">&uarr;</button>
      <button class="btn sm" id="gg-fnext" title="Next (Enter)">&darr;</button>
      <button class="btn sm" id="gg-fclose" title="Close (Esc)">&times;</button>
    </div>
    <div class="gg-head" id="gg-head"></div>
    <div class="gg-scroll" id="gg-scroll"><div class="gg-rows" id="gg-rows"></div><div class="gg-foot" id="gg-foot"></div></div>
    <div class="gg-busy" id="gg-busy" hidden><span class="gg-spin"></span><span id="gg-busytx"></span></div>
  </div>
  <div class="gg-gutter" id="gg-gutter" hidden></div>
  <div class="card gg-det" id="gg-det" hidden></div>
</div>`;
const el = {
  repo: q('#gg-repo'), br: q('#gg-br'), info: q('#gg-info'), op: q('#gg-op'), main: q('#gg-main'), head: q('#gg-head'),
  scroll: q('#gg-scroll'), rows: q('#gg-rows'), foot: q('#gg-foot'), det: q('#gg-det'), gutter: q('#gg-gutter'),
  split: q('#gg-split'), find: q('#gg-findbar'), fq: q('#gg-fq'),
};
let tipEl = null;

function setBusy(msg) {
  S.busy = !!msg;
  q('#gg-busy').hidden = !msg;
  q('#gg-busytx').textContent = msg || '';
}

// ------------------------------------------------------------------ repositories
async function loadConfig() {
  try { S.cfg = await api('/api/config'); } catch { S.cfg = S.cfg || {}; }
  S.repos = (S.cfg.repos || []).filter(r => typeof r === 'string');
}
async function saveRepos(list) {
  await loadConfig();
  const cfg = Object.assign({ hosts: [], redis: [], repos: [] }, S.cfg, { repos: list });
  await api('/api/config', { method: 'PUT', body: JSON.stringify(cfg) });
  S.cfg = cfg; S.repos = list;
}
function renderRepoSelect() {
  const list = S.repos.slice();
  if (S.repo && !list.includes(S.repo)) list.unshift(S.repo);
  el.repo.innerHTML = list.length ? list.map(r => `<option value="${esc(r)}" ${r === S.repo ? 'selected' : ''} title="${esc(r)}">${esc(base(r))} — ${esc(r)}</option>`).join('')
    : '<option value="">(no repositories — click +)</option>';
  q('#gg-rmrepo').disabled = !S.repo;
}
function setRepo(p) {
  S.repo = p; store.set(LS + 'repo', p);
  S.filter = jget('filter:' + p, []); S.issueFor = null; S.tip.clear();
  S.sel = { a: null, b: null }; closeDetailsUI(); S.limit = P.initial; S.sig = '';
  renderRepoSelect(); loadAll(false);
}
async function addRepoFlow() {
  const v = await form({
    title: 'Add repository', fields: [
      { key: 'path', label: 'Folder path', required: true, placeholder: 'D:\\projects\\my-app' },
      { key: 'mode', label: 'This folder is', type: 'radio', value: 'one', options: [{ v: 'one', label: 'a Git repository' }, { v: 'scan', label: 'a parent folder', desc: 'search up to 3 levels deep for repositories' }] },
    ], ok: 'Add',
  });
  if (!v) return;
  try {
    let found = [];
    if (v.mode === 'one') found = [(await api(`/api/git/repo?path=${enc(v.path.trim())}`)).root];
    else {
      found = (await api(`/api/git/discover?path=${enc(v.path.trim())}&depth=3`)).repos;
      if (!found.length) { toast('No repositories found in that folder', 'err'); return; }
      const pick = await form({
        title: `Found ${found.length} repositories`, wide: true, ok: 'Add selected',
        fields: [{ key: 'sel', label: 'Repositories', type: 'multi', value: found, options: found.map(r => ({ v: r, label: r })) }],
      });
      if (!pick || !pick.sel.length) return;
      found = pick.sel;
    }
    const list = S.repos.slice();
    for (const r of found) if (!list.includes(r)) list.push(r);
    await saveRepos(list);
    setRepo(found[0]);
  } catch (e) { toast(e.message, 'err'); }
}

// ------------------------------------------------------------------ data loading
function logURL() {
  const u = new URLSearchParams({
    repo: S.repo, limit: S.limit, remote: +P.remote, tags: +P.tags, stashes: +P.stashes, uncommitted: +P.uncommitted,
    firstparent: +P.firstParent, order: P.order, inhead: +(P.muteNonHead),
  });
  for (const b of S.filter) u.append('b', b);
  return '/api/git/log?' + u;
}
function indexRows() {
  S.idx = new Map(); S.kids = new Map();
  S.rows.forEach((r, i) => {
    S.idx.set(r.h, i);
    for (const p of r.p || []) { if (!S.kids.has(p)) S.kids.set(p, []); S.kids.get(p).push(r.h); }
  });
}
let loadSeq = 0;
async function loadAll(keep) {
  if (!S.repo) { renderEmpty(); return; }
  const seq = ++loadSeq;
  S.loading = true; el.info.textContent = 'loading…';
  try {
    const [refs, log] = await Promise.all([api(`/api/git/refs?repo=${enc(S.repo)}`), api(logURL())]);
    if (seq !== loadSeq) return;
    S.refs = refs; S.log = log; S.rows = log.rows || []; indexRows();
    S.filter = S.filter.filter(f => f.startsWith('glob:') || [...refs.branches, ...refs.remoteBranches].some(b => b.ref === f));
    renderAll(keep);
    if (S.sel.a && !S.idx.has(S.sel.a)) { S.sel = { a: null, b: null }; closeDetailsUI(); }
    else if (S.sel.a === '*' || S.sel.b === '*') openDetails();
    api(`/api/git/signature?repo=${enc(S.repo)}`).then(r => { if (seq === loadSeq) S.sig = r.sig; }).catch(() => {});
  } catch (e) {
    if (seq !== loadSeq) return;
    el.rows.innerHTML = `<div class="empty">${esc(e.message)}</div>`; el.head.innerHTML = ''; el.info.textContent = '';
  } finally { if (seq === loadSeq) S.loading = false; }
}
let reloadTimer = 0;
const reloadSoon = () => { clearTimeout(reloadTimer); reloadTimer = setTimeout(() => loadAll(true), 250); };

function renderEmpty() {
  el.head.innerHTML = ''; el.info.textContent = ''; el.op.innerHTML = '';
  el.rows.innerHTML = `<div class="empty">Add a repository to see its graph.<br><br><button class="btn primary" id="gg-empty-add">Add repository</button></div>`;
  const b = q('#gg-empty-add'); if (b) b.onclick = addRepoFlow;
}

// ------------------------------------------------------------------ rendering: graph
const colTemplate = gw => [gw + 'px', 'minmax(0,1fr)', P.cols.date && P.widths.date + 'px', P.cols.author && P.widths.author + 'px', P.cols.hash && P.widths.hash + 'px'].filter(Boolean).join(' ');

function renderBar() {
  q('#gg-br').innerHTML = `Branches: ${S.filter.length ? S.filter.length + ' selected' : 'Show All'} &#9662;`;
  const n = S.rows.filter(r => r.h !== '*').length;
  const b = S.refs ? (S.refs.head || 'detached HEAD') : '';
  el.info.innerHTML = S.refs ? `<span class="pill ok">${esc(b)}</span> ${n} commit${n === 1 ? '' : 's'}${S.log && S.log.truncated ? '+' : ''}` : '';
}
function renderOp() {
  const st = S.refs && S.refs.state;
  el.op.innerHTML = st ? `<span class="pill warn">${esc(st)} in progress</span> <button class="btn sm" data-op="continue">Continue</button> <button class="btn sm danger" data-op="abort">Abort</button>` : '';
}
function renderHead() {
  const c = P.cols;
  el.head.style.setProperty('--gg-cols', colTemplate(gwidth()));
  el.head.innerHTML = `<div class="c-graph">Graph</div><div class="c-desc">Description</div>` +
    (c.date ? `<div class="c-date">${P.dateType === 'author' ? 'Date' : 'Commit date'}<i class="rz" data-c="date"></i></div>` : '') +
    (c.author ? `<div class="c-author">Author<i class="rz" data-c="author"></i></div>` : '') +
    (c.hash ? `<div class="c-hash">Commit<i class="rz" data-c="hash"></i></div>` : '');
}
const gwidth = () => Math.min(Math.max(((S.log && S.log.lanes) || 1) * LW + LW, 48), 420);

function edge(xa, ya, xb, yb, color, dash) {
  const d = xa === xb ? `M${xa} ${ya}L${xb} ${yb}` : P.style === 'angular' ? `M${xa} ${ya}L${xb} ${yb}` : `M${xa} ${ya}C${xa} ${(ya + yb) / 2} ${xb} ${(ya + yb) / 2} ${xb} ${yb}`;
  return `<path d="${d}" stroke="${color}" ${dash ? 'stroke-dasharray="3 3"' : ''}/>`;
}
function svgHTML(gw) {
  const x = l => 12 + l * LW, H = S.rows.length * RH;
  let lines = '', dots = '';
  S.rows.forEach((r, i) => {
    const y0 = i * RH, ym = y0 + RH / 2, y1 = y0 + RH;
    for (const l of r.pass || []) lines += `<path d="M${x(l)} ${y0}L${x(l)} ${y1}" stroke="${colorOf(l)}"/>`;
    for (const l of r.conv || []) lines += edge(x(l), y0, x(r.col), ym, colorOf(l));
    for (const l of r.out || []) lines += edge(x(r.col), ym, x(l), y1, colorOf(l), r.h === '*');
    const c = colorOf(r.col), isHead = S.refs && r.h === S.refs.headHash;
    if (r.h === '*') dots += `<circle class="gg-v unc" data-i="${i}" cx="${x(r.col)}" cy="${ym}" r="4.5" stroke="${c}"/>`;
    else if (r.st) dots += `<circle class="gg-v stash" data-i="${i}" cx="${x(r.col)}" cy="${ym}" r="4.5" stroke="${c}"/>`;
    else dots += `<circle class="gg-v ${isHead ? 'head' : ''}" data-i="${i}" cx="${x(r.col)}" cy="${ym}" r="${(r.p || []).length > 1 ? 3.6 : 4.6}" fill="${c}"/>`;
  });
  return `<svg class="gg-svg" width="${gw}" height="${H}" viewBox="0 0 ${gw} ${H}"><g class="ln">${lines}</g><g>${dots}</g></svg>`;
}
function refsHTML(r) {
  const refs = r.r || [];
  if (!refs.length) return '';
  const remoteNames = new Set(((S.refs && S.refs.remotes) || []).map(x => x.name));
  const remotes = refs.filter(x => x.t === 'remote'), usedRemote = new Set();
  let out = '';
  for (const x of refs) if (x.t === 'stash') out += `<span class="gg-ref stash" data-t="stash" data-n="${esc(x.n)}">${esc(x.n)}</span>`;
  for (const x of refs) {
    if (x.t === 'detached') out += `<span class="gg-ref cur" data-t="detached" data-n="HEAD" style="--c:${colorOf(r.col)}">HEAD</span>`;
    else if (x.t === 'head') {
      const rms = P.combine ? remotes.filter(y => { const pre = y.n.slice(0, y.n.length - x.n.length - 1); return y.n.endsWith('/' + x.n) && remoteNames.has(pre); }) : [];
      rms.forEach(y => usedRemote.add(y.n));
      out += `<span class="gg-ref head ${x.h ? 'cur' : ''}" style="--c:${colorOf(r.col)}"><b data-t="head" data-n="${esc(x.n)}">${esc(x.n)}</b>` +
        rms.map(y => `<em data-t="remote" data-n="${esc(y.n)}" title="${esc(y.n)}">${esc(y.n.slice(0, y.n.length - x.n.length - 1))}</em>`).join('') + '</span>';
    }
  }
  for (const x of remotes) if (!usedRemote.has(x.n)) out += `<span class="gg-ref remote" data-t="remote" data-n="${esc(x.n)}">${esc(x.n)}</span>`;
  for (const x of refs) if (x.t === 'tag') out += `<span class="gg-ref tag" data-t="tag" data-n="${esc(x.n)}">${esc(x.n)}</span>`;
  return out;
}
function rowHTML(r, i) {
  const unc = r.h === '*', c = P.cols;
  const muted = !unc && ((P.muteNonHead && S.log && !r.ih && S.refs && S.refs.headHash) || (P.muteMerge && (r.p || []).length > 1));
  const cls = ['gg-row'];
  if (unc) cls.push('unc'); if (muted) cls.push('mu');
  if (S.sel.a === r.h || S.sel.b === r.h) cls.push('sel');
  const desc = unc ? `<span class="gg-msg unc-msg">Uncommitted Changes (${r.un})</span>` : `${refsHTML(r)}<span class="gg-msg">${rich(r.s)}</span>`;
  return `<div class="${cls.join(' ')}" data-i="${i}" data-h="${esc(r.h)}"><div class="c-graph"></div><div class="c-desc">${desc}</div>` +
    (c.date ? `<div class="c-date">${unc ? '' : fmtDate(P.dateType === 'author' ? r.at : r.ct)}</div>` : '') +
    (c.author ? `<div class="c-author" title="${esc(r.e || '')}">${unc ? '' : esc(r.a)}</div>` : '') +
    (c.hash ? `<div class="c-hash">${unc ? '' : short(r.h)}</div>` : '') + '</div>';
}
function renderRows() {
  const gw = gwidth();
  el.main.style.setProperty('--gg-cols', colTemplate(gw));
  if (!S.rows.length) { el.rows.innerHTML = '<div class="empty">No commits to show.</div>'; el.foot.innerHTML = ''; return; }
  el.rows.innerHTML = svgHTML(gw) + S.rows.map(rowHTML).join('');
  el.rows.style.height = S.rows.length * RH + 'px';
  el.foot.innerHTML = S.log && S.log.truncated ? `<button class="btn" id="gg-more">Load more commits</button>` : '';
}
// the header sits outside the scroller, so give it the scrollbar's width to keep columns aligned
function syncHeadPad() { el.head.style.paddingRight = (el.scroll.offsetWidth - el.scroll.clientWidth + 6) + 'px'; }
window.addEventListener('resize', syncHeadPad);
function renderAll(keep) {
  const top = el.scroll.scrollTop;
  renderBar(); renderHead(); renderRows(); renderOp(); renderRepoSelect();
  if (keep) el.scroll.scrollTop = top;
  syncHeadPad(); paintFind();
}
function paintSelection() {
  qa('.gg-row', el.rows).forEach(r => r.classList.toggle('sel', r.dataset.h === S.sel.a || r.dataset.h === S.sel.b));
}
function scrollToRow(i, center = true) {
  const top = i * RH, h = el.scroll.clientHeight;
  if (center) el.scroll.scrollTop = Math.max(0, top - h / 2 + RH / 2);
  else if (top < el.scroll.scrollTop) el.scroll.scrollTop = top;
  else if (top + RH > el.scroll.scrollTop + h) el.scroll.scrollTop = top + RH - h;
}
function flash(i) {
  const r = qa('.gg-row', el.rows)[i]; if (!r) return;
  r.classList.add('flash'); setTimeout(() => r.classList.remove('flash'), 1400);
}

// column resize + visibility
el.head.addEventListener('mousedown', e => {
  const rz = e.target.closest('.rz'); if (!rz) return;
  e.preventDefault();
  const c = rz.dataset.c, x0 = e.clientX, w0 = P.widths[c];
  const mv = ev => { P.widths[c] = Math.max(48, Math.min(480, w0 + (x0 - ev.clientX))); el.main.style.setProperty('--gg-cols', colTemplate(gwidth())); el.head.style.setProperty('--gg-cols', colTemplate(gwidth())); };
  const up = () => { removeEventListener('mousemove', mv); removeEventListener('mouseup', up); savePrefs(); };
  addEventListener('mousemove', mv); addEventListener('mouseup', up);
});
el.head.addEventListener('contextmenu', e => {
  e.preventDefault();
  const t = (k, label) => ({ label: (P.cols[k] ? '\u2713 ' : '    ') + label, run() { P.cols[k] = !P.cols[k]; savePrefs(); renderHead(); renderRows(); } });
  showMenu(e.clientX, e.clientY, [t('date', 'Date'), t('author', 'Author'), t('hash', 'Commit')]);
});

// ------------------------------------------------------------------ tooltip on graph vertices
let tipTimer = 0;
function hideTip() { clearTimeout(tipTimer); if (tipEl) { tipEl.remove(); tipEl = null; } }
el.rows.addEventListener('mouseover', e => {
  const v = e.target.closest('.gg-v'); if (!v) return;
  const r = S.rows[+v.dataset.i]; if (!r) return;
  const x = e.clientX, y = e.clientY;
  clearTimeout(tipTimer);
  tipTimer = setTimeout(async () => {
    const show = html => { hideTip(); tipEl = document.createElement('div'); tipEl.className = 'gg-tip'; tipEl.innerHTML = html; document.body.appendChild(tipEl);
      tipEl.style.left = Math.min(x + 14, innerWidth - tipEl.offsetWidth - 8) + 'px'; tipEl.style.top = Math.min(y + 14, innerHeight - tipEl.offsetHeight - 8) + 'px'; };
    if (r.h === '*') { show('<b>Uncommitted changes</b><br>' + r.un + ' file(s) changed'); return; }
    const key = S.repo + '|' + r.h;
    let c = S.tip.get(key);
    if (!c) { show(`<b>${short(r.h)}</b><br><span class="muted">loading…</span>`); try { c = await api(`/api/git/contains?repo=${enc(S.repo)}&hash=${r.h}`); S.tip.set(key, c); } catch (err) { show(esc(err.message)); return; } }
    const lst = a => a.length ? esc(a.slice(0, 12).join(', ')) + (a.length > 12 ? ` +${a.length - 12} more` : '') : '<span class="muted">none</span>';
    show(`<b>${short(r.h)}</b> ${esc(plainMsg(r.s)).slice(0, 80)}<br>In HEAD: <b>${c.inHead ? 'yes' : 'no'}</b><br>Branches: ${lst(c.branches)}<br>Tags: ${lst(c.tags)}` + (c.stashes.length ? `<br>Stashes: ${lst(c.stashes)}` : ''));
  }, 350);
});
el.rows.addEventListener('mouseout', e => { if (e.target.closest('.gg-v')) hideTip(); });
el.scroll.addEventListener('scroll', () => {
  hideTip();
  if (P.auto && S.log && S.log.truncated && !S.loading && el.scroll.scrollTop + el.scroll.clientHeight > el.scroll.scrollHeight - 300) loadMore();
});
function loadMore() { S.limit += P.more; loadAll(true); }

// ------------------------------------------------------------------ selection + details
el.rows.addEventListener('click', e => {
  if (e.target.closest('a')) return;
  const row = e.target.closest('.gg-row'); if (!row) return;
  selectRow(row.dataset.h, e.ctrlKey || e.metaKey);
});
el.rows.addEventListener('dblclick', e => {
  const t = e.target.closest('[data-t]'); if (!t) return;
  if (t.dataset.t === 'head' && t.dataset.n !== S.refs.head) A.checkoutBranch(t.dataset.n);
  else if (t.dataset.t === 'remote') A.checkoutRemote(t.dataset.n);
});
q('#gg-foot').addEventListener('click', e => { if (e.target.id === 'gg-more') loadMore(); });

function selectRow(h, additive) {
  if (additive && S.sel.a && S.sel.a !== h) S.sel.b = h;
  else if (S.sel.a === h && !S.sel.b) { closeDetails(); return; }
  else S.sel = { a: h, b: null };
  paintSelection(); openDetails();
}
function closeDetailsUI() { el.det.hidden = true; el.gutter.hidden = true; S.model = null; S.detSeq++; }
function closeDetails() { S.sel = { a: null, b: null }; closeDetailsUI(); paintSelection(); }
function layoutDock() {
  el.split.className = 'gg-split dock-' + P.dock;
  if (el.det.hidden) return;
  if (P.dock === 'bottom') { el.det.style.height = P.detH + 'px'; el.det.style.width = ''; }
  else { el.det.style.width = P.detW + 'px'; el.det.style.height = ''; }
}
el.gutter.addEventListener('mousedown', e => {
  e.preventDefault();
  const bottom = P.dock === 'bottom', x0 = e.clientX, y0 = e.clientY, h0 = P.detH, w0 = P.detW;
  const mv = ev => {
    if (bottom) P.detH = Math.max(140, Math.min(el.split.clientHeight - 160, h0 + (y0 - ev.clientY)));
    else P.detW = Math.max(300, Math.min(el.split.clientWidth - 360, w0 + (x0 - ev.clientX)));
    layoutDock();
  };
  const up = () => { removeEventListener('mousemove', mv); removeEventListener('mouseup', up); savePrefs(); };
  addEventListener('mousemove', mv); addEventListener('mouseup', up);
});

async function openDetails() {
  const { a, b } = S.sel; if (!a) return;
  const seq = ++S.detSeq;
  el.det.hidden = false; el.gutter.hidden = false; layoutDock();
  el.det.innerHTML = '<div class="empty">Loading…</div>';
  try {
    let model;
    if (b) {
      const unc = a === '*' || b === '*', other = a === '*' ? b : a;
      const from = unc ? other : a, to = unc ? 'WORKTREE' : b;
      const r = await api(`/api/git/compare?repo=${enc(S.repo)}&from=${enc(from)}&to=${enc(to)}`);
      model = { type: 'cmp', from, to, files: r.files, a, b };
    } else if (a === '*') {
      const r = await api(`/api/git/uncommitted?repo=${enc(S.repo)}`);
      model = { type: 'unc', from: r.against, to: 'WORKTREE', files: r.files };
    } else {
      const d = await api(`/api/git/details?repo=${enc(S.repo)}&hash=${a}`);
      model = { type: 'commit', d, from: (d.parents || [])[0] || '', to: d.hash, files: d.files };
    }
    if (seq !== S.detSeq) return;
    S.model = model; renderDetails();
  } catch (e) { if (seq === S.detSeq) el.det.innerHTML = `<div class="empty">${esc(e.message)}</div>`; }
}

// ---- code review (persisted per repo + revision pair, expires after 90 days)
const REVIEW_TTL = 90 * 864e5;
function reviews() {
  const all = jget('review', {}), now = Date.now(); let dirty = false;
  for (const k of Object.keys(all)) if (now - all[k].t > REVIEW_TTL) { delete all[k]; dirty = true; }
  if (dirty) jset('review', all);
  return all;
}
const reviewKey = m => `${S.repo}|${m.from || ''}|${m.to}`;
function curReview() { const m = S.model; if (!m || m.type === 'unc' || m.to === 'WORKTREE') return null; return reviews()[reviewKey(m)] || null; }
function markReviewed(path) {
  const rv = curReview(); if (!rv) return;
  const all = reviews(); const e = all[reviewKey(S.model)]; e.files[path] = 1; e.t = Date.now(); jset('review', all);
  qa('.gg-file', el.det).forEach(n => { const f = S.model.files[+n.dataset.i]; if (f && f.p === path) n.classList.remove('todo'); });
  updateReviewBtn();
}
function updateReviewBtn() {
  const b = q('[data-act=review]', el.det); if (!b) return;
  const rv = curReview();
  if (!rv) { b.textContent = 'Start code review'; return; }
  const done = S.model.files.filter(f => rv.files[f.p]).length;
  b.textContent = `End review (${done}/${S.model.files.length})`;
}

function buildTree(files) {
  const rootN = { dirs: new Map(), files: [] };
  files.forEach((f, i) => {
    const parts = f.p.split('/'); let n = rootN;
    for (const d of parts.slice(0, -1)) { if (!n.dirs.has(d)) n.dirs.set(d, { dirs: new Map(), files: [] }); n = n.dirs.get(d); }
    n.files.push({ name: parts[parts.length - 1], f, i });
  });
  return rootN;
}
function fileRow(f, i, depth, label, rv) {
  const todo = rv && !rv.files[f.p] ? ' todo' : '';
  const stat = f.b ? '<span class="muted">bin</span>' : (f.a || f.d) ? `<b class="a">+${f.a}</b><b class="d">&minus;${f.d}</b>` : '';
  return `<div class="gg-file${todo}" style="--d:${depth}" data-i="${i}" title="${esc(f.o ? f.o + ' \u2192 ' + f.p : f.p)}"><span class="st st-${f.s === '!' ? 'x' : f.s}">${f.s}</span><span class="fn">${label}</span><span class="fs">${stat}</span></div>`;
}
function treeHTML(node, depth, prefix, rv) {
  let out = '';
  for (const [name0, child0] of [...node.dirs.entries()].sort((x, y) => x[0].localeCompare(y[0]))) {
    let name = name0, child = child0;
    while (P.compact && child.files.length === 0 && child.dirs.size === 1) { const [n2, c2] = [...child.dirs.entries()][0]; name += '/' + n2; child = c2; }
    const key = prefix + name, open = !S.collapsed.has(key);
    out += `<div class="gg-dir" style="--d:${depth}" data-k="${esc(key)}"><i>${open ? '&#9662;' : '&#9656;'}</i>${esc(name)}</div>`;
    if (open) out += treeHTML(child, depth + 1, key + '/', rv);
  }
  for (const x of node.files.sort((p, q2) => p.name.localeCompare(q2.name))) out += fileRow(x.f, x.i, depth, `${esc(x.name)}${x.f.o ? ` <span class="muted">\u2190 ${esc(x.f.o)}</span>` : ''}`, rv);
  return out;
}
function filesHTML() {
  const m = S.model, rv = curReview();
  if (!m.files.length) return '<div class="empty">No file changes.</div>';
  if (P.fileView === 'list') return m.files.map((f, i) => { const k = f.p.lastIndexOf('/'); return fileRow(f, i, 0, `${esc(f.p.slice(k + 1))} <span class="muted">${esc(k >= 0 ? f.p.slice(0, k) : '')}</span>`, rv); }).join('');
  return treeHTML(buildTree(m.files), 0, '', rv);
}
function rowFor(h) { const i = S.idx.get(h); return i === undefined ? null : S.rows[i]; }
function refChips(r) { return r ? (r.r || []).map(x => `<span class="gg-ref ${x.t === 'tag' ? 'tag' : x.t === 'remote' ? 'remote' : x.t === 'stash' ? 'stash' : 'head'}">${esc(x.n)}</span>`).join('') : ''; }

function renderDetails() {
  const m = S.model; if (!m) return;
  const adds = m.files.reduce((s, f) => s + (f.a || 0), 0), dels = m.files.reduce((s, f) => s + (f.d || 0), 0);
  let head = '';
  if (m.type === 'commit') {
    const d = m.d, same = d.author === d.committer && d.authorEmail === d.committerEmail && d.authorTime === d.commitTime;
    head = `<div class="gg-dtitle">${rich(d.subject)}</div>
      <div class="gg-dmeta">
        <span>Commit</span><span><code>${esc(d.hash)}</code> <a href="#" data-act="copy" data-v="${esc(d.hash)}">copy</a></span>
        <span>Parents</span><span>${(d.parents || []).map(p => `<a href="#" class="gg-plink" data-h="${p}"><code>${short(p)}</code></a>`).join(' ') || '<span class="muted">none (root commit)</span>'}</span>
        <span>Author</span><span>${esc(d.author)} &lt;${esc(d.authorEmail)}&gt; &middot; ${fmtDate(d.authorTime)}</span>
        ${same ? '' : `<span>Committer</span><span>${esc(d.committer)} &lt;${esc(d.committerEmail)}&gt; &middot; ${fmtDate(d.commitTime)}</span>`}
        ${refChips(rowFor(d.hash)) ? `<span>Refs</span><span>${refChips(rowFor(d.hash))}</span>` : ''}
      </div>${d.body ? `<div class="gg-dbody">${rich(d.body)}</div>` : ''}`;
  } else if (m.type === 'unc') {
    head = `<div class="gg-dtitle">Uncommitted Changes</div><div class="gg-dmeta"><span>Against</span><span><code>${short(m.from) || 'empty tree'}</code> (HEAD)</span></div>`;
  } else {
    const lab = h => { if (h === 'WORKTREE' || h === '*') return 'Uncommitted Changes'; const r = rowFor(h); return `<code>${short(h)}</code> ${r ? esc(plainMsg(r.s)).slice(0, 60) : ''}`; };
    head = `<div class="gg-dtitle">Comparing commits</div><div class="gg-dmeta"><span>From</span><span>${lab(m.from)}</span><span>To</span><span>${lab(m.to)}</span></div>`;
  }
  const canReview = m.type !== 'unc' && m.to !== 'WORKTREE';
  el.det.innerHTML = `<div class="gg-dh">${head}<div class="gg-dact"><button class="btn sm" data-act="dock" title="Move panel">${P.dock === 'bottom' ? 'Dock right' : 'Dock bottom'}</button><button class="btn sm" data-act="close" title="Close (Esc)">&times;</button></div></div>
    <div class="gg-ftool"><b>${m.files.length} file${m.files.length === 1 ? '' : 's'}</b> <span class="a">+${adds}</span> <span class="d">&minus;${dels}</span>
      <span class="sp"></span>
      <label class="check ${P.fileView === 'tree' ? '' : 'hid'}"><input type="checkbox" data-act="compact" ${P.compact ? 'checked' : ''}> compact folders</label>
      <div class="seg"><button class="btn sm ${P.fileView === 'tree' ? 'on' : ''}" data-act="view" data-v="tree">Tree</button><button class="btn sm ${P.fileView === 'list' ? 'on' : ''}" data-act="view" data-v="list">List</button></div>
      ${canReview ? '<button class="btn sm" data-act="review"></button>' : ''}
    </div><div class="gg-files">${filesHTML()}</div>`;
  updateReviewBtn();
}
el.det.addEventListener('click', e => {
  const t = e.target;
  const act = t.closest('[data-act]');
  if (act) {
    const a = act.dataset.act;
    if (a === 'close') closeDetails();
    else if (a === 'dock') { P.dock = P.dock === 'bottom' ? 'right' : 'bottom'; savePrefs(); layoutDock(); renderDetails(); }
    else if (a === 'view') { P.fileView = act.dataset.v; savePrefs(); renderDetails(); }
    else if (a === 'compact') { P.compact = act.checked; savePrefs(); renderDetails(); }
    else if (a === 'copy') { e.preventDefault(); copyText(act.dataset.v, 'Hash'); }
    else if (a === 'review') {
      const all = reviews(), k = reviewKey(S.model);
      if (all[k]) delete all[k]; else all[k] = { t: Date.now(), files: {} };
      jset('review', all); renderDetails();
    }
    return;
  }
  const pl = t.closest('.gg-plink');
  if (pl) { e.preventDefault(); if (S.idx.has(pl.dataset.h)) { selectRow(pl.dataset.h); scrollToRow(S.idx.get(pl.dataset.h), false); } else toast('That commit is not loaded yet (load more commits)'); return; }
  const dir = t.closest('.gg-dir');
  if (dir) { const k = dir.dataset.k; S.collapsed.has(k) ? S.collapsed.delete(k) : S.collapsed.add(k); renderDetails(); return; }
  const fr = t.closest('.gg-file');
  if (fr) openDiff(S.model.files[+fr.dataset.i]);
});
el.det.addEventListener('contextmenu', e => {
  const fr = e.target.closest('.gg-file'); if (!fr) return;
  e.preventDefault();
  const f = S.model.files[+fr.dataset.i], m = S.model, root = S.refs ? S.refs.root : '';
  const sep = root.includes('\\') ? '\\' : '/';
  showMenu(e.clientX, e.clientY, [
    { label: 'View Diff', run: () => openDiff(f) },
    { label: 'View File at this Revision', disabled: f.s === 'D' && !m.from, run: () => viewFile(f) },
    '-',
    { label: 'Copy File Path to Clipboard', run: () => copyText(f.p, 'Path') },
    { label: 'Copy Absolute File Path to Clipboard', run: () => copyText(root + sep + f.p.split('/').join(sep), 'Path') },
  ]);
});

// ------------------------------------------------------------------ diff + file viewer
function parseDiff(text) {
  const out = { meta: [], hunks: [], binary: false };
  let h = null, o = 0, n = 0;
  for (const ln of text.split('\n')) {
    if (ln.startsWith('@@')) {
      const m = /^@@ -(\d+)(?:,\d+)? \+(\d+)/.exec(ln); o = m ? +m[1] : 0; n = m ? +m[2] : 0;
      h = { head: ln, lines: [] }; out.hunks.push(h); continue;
    }
    if (!h) { if (/^Binary files? /.test(ln)) out.binary = true; if (ln) out.meta.push(ln); continue; }
    if (ln.startsWith('diff --git')) { h = null; out.meta.push(ln); continue; }
    const c = ln[0];
    if (c === '+') h.lines.push({ k: '+', t: ln.slice(1), n: n++ });
    else if (c === '-') h.lines.push({ k: '-', t: ln.slice(1), o: o++ });
    else if (c === ' ') h.lines.push({ k: ' ', t: ln.slice(1), o: o++, n: n++ });
    else if (c === '\\') h.lines.push({ k: '\\', t: ln });
  }
  return out;
}
function renderDiff(text) {
  const d = parseDiff(text);
  if (d.binary) return '<div class="empty">Binary file: no textual diff.</div>';
  if (!d.hunks.length) return `<div class="empty">No textual changes.${d.meta.length ? `<pre class="plain">${esc(d.meta.join('\n'))}</pre>` : ''}</div>`;
  const meta = d.meta.length ? `<div class="dl meta"><span class="ln"></span><span class="ln"></span><span class="tx">${esc(d.meta.join('\n'))}</span></div>` : '';
  if (P.diffMode === 'split') {
    let rows = '';
    for (const hk of d.hunks) {
      rows += `<div class="sl hunk"><span class="tx">${esc(hk.head)}</span></div>`;
      let dels = [], adds = [];
      const flush = () => {
        const n = Math.max(dels.length, adds.length);
        for (let i = 0; i < n; i++) {
          const L = dels[i], R = adds[i];
          rows += `<div class="sl"><span class="ln">${L ? L.o : ''}</span><span class="tx ${L ? 'del' : 'emp'}">${L ? esc(L.t) : ''}</span><span class="ln">${R ? R.n : ''}</span><span class="tx ${R ? 'add' : 'emp'}">${R ? esc(R.t) : ''}</span></div>`;
        }
        dels = []; adds = [];
      };
      for (const l of hk.lines) {
        if (l.k === '-') dels.push(l); else if (l.k === '+') adds.push(l);
        else if (l.k === ' ') { flush(); rows += `<div class="sl"><span class="ln">${l.o}</span><span class="tx">${esc(l.t)}</span><span class="ln">${l.n}</span><span class="tx">${esc(l.t)}</span></div>`; }
        else { flush(); rows += `<div class="sl hunk"><span class="tx">${esc(l.t)}</span></div>`; }
      }
      flush();
    }
    return `<div class="gg-diff split">${meta}${rows}</div>`;
  }
  let out = meta;
  for (const hk of d.hunks) {
    out += `<div class="dl hunk"><span class="ln"></span><span class="ln"></span><span class="tx">${esc(hk.head)}</span></div>`;
    for (const l of hk.lines) {
      const cls = l.k === '+' ? 'add' : l.k === '-' ? 'del' : l.k === '\\' ? 'meta' : '';
      out += `<div class="dl ${cls}"><span class="ln">${l.o ?? ''}</span><span class="ln">${l.n ?? ''}</span><span class="tx">${l.k === '\\' ? esc(l.t) : esc(l.k + l.t)}</span></div>`;
    }
  }
  return `<div class="gg-diff">${out}</div>`;
}
async function openDiff(f) {
  const m = S.model; if (!m || !f) return;
  markReviewed(f.p);
  const mm = modal(`<div class="gg-mh"><span class="st st-${f.s === '!' ? 'x' : f.s}">${f.s}</span><b>${esc(f.o ? f.o + ' \u2192 ' + f.p : f.p)}</b><span class="sp"></span>
    <div class="seg"><button class="btn sm ${P.diffMode === 'unified' ? 'on' : ''}" data-dm="unified">Unified</button><button class="btn sm ${P.diffMode === 'split' ? 'on' : ''}" data-dm="split">Split</button></div>
    <button class="btn sm" data-a="file">View file</button><button class="btn sm" data-a="close">&times;</button></div><div class="gg-mb"><div class="empty">Loading…</div></div>`, { cls: 'xl' });
  let text = '';
  const body = q('.gg-mb', mm.box);
  const paint = () => { body.innerHTML = renderDiff(text); qa('[data-dm]', mm.box).forEach(b => b.classList.toggle('on', b.dataset.dm === P.diffMode)); };
  mm.box.addEventListener('click', e => {
    const b = e.target.closest('button'); if (!b) return;
    if (b.dataset.dm) { P.diffMode = b.dataset.dm; savePrefs(); paint(); }
    else if (b.dataset.a === 'close') mm.close();
    else if (b.dataset.a === 'file') { mm.close(); viewFile(f); }
  });
  try {
    const r = await api(`/api/git/filediff?repo=${enc(S.repo)}&from=${enc(m.from || '')}&to=${enc(m.to)}&path=${enc(f.p)}&old=${enc(f.o || '')}&untracked=${f.s === 'U' ? 1 : 0}`);
    text = r.text; paint();
  } catch (e) { body.innerHTML = `<div class="empty">${esc(e.message)}</div>`; }
}
async function viewFile(f) {
  const m = S.model; if (!m) return;
  markReviewed(f.p);
  const del = f.s === 'D', rev = del ? m.from : m.to, path = del ? (f.o || f.p) : f.p;
  const mm = modal(`<div class="gg-mh"><b>${esc(path)}</b> <span class="muted">${rev === 'WORKTREE' ? 'working copy' : '@ ' + short(rev)}</span><span class="sp"></span><button class="btn sm" data-a="copy">Copy path</button><button class="btn sm" data-a="close">&times;</button></div><div class="gg-mb"><div class="empty">Loading…</div></div>`, { cls: 'xl' });
  mm.box.addEventListener('click', e => { const b = e.target.closest('button'); if (!b) return; if (b.dataset.a === 'close') mm.close(); else if (b.dataset.a === 'copy') copyText(path, 'Path'); });
  const body = q('.gg-mb', mm.box);
  try {
    const r = await api(`/api/git/fileat?repo=${enc(S.repo)}&rev=${enc(rev)}&path=${enc(path)}`);
    if (r.binary) body.innerHTML = `<div class="empty">Binary file (${r.size} bytes).</div>`;
    else {
      const lines = r.text.split('\n'); if (lines[lines.length - 1] === '') lines.pop();
      body.innerHTML = `<div class="gg-diff">${lines.map((l, i) => `<div class="dl"><span class="ln">${i + 1}</span><span class="tx">${esc(l)}</span></div>`).join('')}</div>` + (r.size >= 2 << 20 ? '<div class="empty">Truncated at 2 MB.</div>' : '');
    }
  } catch (e) { body.innerHTML = `<div class="empty">${esc(e.message)}</div>`; }
}

// ------------------------------------------------------------------ actions
async function act(action, args, msg) {
  setBusy('Running git…');
  try {
    const r = await post('/api/git/action', { repo: S.repo, action, args });
    toast(msg || 'Done');
    await loadAll(true);
    return r;
  } catch (e) {
    await loadAll(true).catch(() => {});
    showGitError(e.message);
    return null;
  } finally { setBusy(false); }
}
function showGitError(msg) {
  const st = S.refs && S.refs.state;
  const m = modal(`<form class="gg-form"><h3>Git command failed</h3><pre class="gg-out">${esc(msg)}</pre>
    ${st ? `<div class="gg-intro">A ${esc(st)} is still in progress. Resolve the conflicts in your editor, then continue, or abort it.</div>` : ''}
    <div class="gg-factions"><button type="button" class="btn primary" data-c>Close</button>${st ? `<button type="button" class="btn" data-op="continue">Continue ${esc(st)}</button><button type="button" class="btn danger" data-op="abort">Abort ${esc(st)}</button>` : ''}</div></form>`, { cls: 'wide' });
  m.box.addEventListener('click', e => {
    if (e.target.closest('[data-c]')) m.close();
    const o = e.target.closest('[data-op]'); if (o) { m.close(); opControl(o.dataset.op); }
  });
}
async function opControl(which) {
  const st = S.refs && S.refs.state; if (!st) return;
  if (which === 'abort' && !(await confirmBox(`Abort ${esc(st)}?`, 'All progress of this operation is discarded and the repository returns to its previous state.', 'Abort', true))) return;
  act(which === 'abort' ? 'op.abort' : 'op.continue', { op: st }, which === 'abort' ? `${st} aborted` : `${st} continued`);
}
el.op.addEventListener('click', e => { const b = e.target.closest('[data-op]'); if (b) opControl(b.dataset.op); });

const remoteOpts = (none) => [...(none ? [{ v: '', label: none }] : []), ...(S.refs.remotes || []).map(r => ({ v: r.name, label: r.name }))];
const curBranch = () => S.refs.head;
const curLabel = () => S.refs.head ? `<b>${esc(S.refs.head)}</b>` : '<b>detached HEAD</b>';
const splitRemote = full => {
  const rs = (S.refs.remotes || []).map(r => r.name).sort((a, b) => b.length - a.length);
  for (const r of rs) if (full.startsWith(r + '/')) return { remote: r, name: full.slice(r.length + 1) };
  const i = full.indexOf('/'); return { remote: full.slice(0, i), name: full.slice(i + 1) };
};
const parentOpts = hash => {
  const r = rowFor(hash); const ps = (r && r.p) || [];
  return ps.map((p, i) => { const pr = rowFor(p); return { v: String(i + 1), label: `${i + 1}: ${short(p)}${pr ? ' ' + plainMsg(pr.s).slice(0, 50) : ''}` }; });
};

const A = {
  addTag: async hash => {
    const v = await form({ title: `Add tag to <code>${short(hash)}</code>`, ok: 'Add tag', fields: [
      { key: 'name', label: 'Tag name', required: true, validate: x => /\s/.test(x) ? 'Tag names cannot contain spaces' : null },
      { key: 'type', label: 'Type', type: 'radio', value: 'ann', options: [{ v: 'ann', label: 'Annotated', desc: 'has a message, tagger and date' }, { v: 'light', label: 'Lightweight' }] },
      { key: 'message', label: 'Message', type: 'textarea', showIf: v => v.type === 'ann', validate: x => x.trim() ? null : 'An annotated tag needs a message (or choose lightweight)' },
      ...(S.refs.remotes.length ? [{ key: 'push', label: 'Push to remote', type: 'select', value: '', options: remoteOpts('Do not push') }] : []),
    ] });
    if (v) act('tag.add', { name: v.name.trim(), hash, lightweight: v.type === 'light', message: v.message || '', pushRemote: v.push || '' }, `Tag ${v.name.trim()} added`);
  },
  createBranch: async hash => {
    const v = await form({ title: `Create branch at <code>${short(hash)}</code>`, ok: 'Create', fields: [
      { key: 'name', label: 'Branch name', required: true, validate: x => /\s/.test(x) ? 'Branch names cannot contain spaces' : null },
      { key: 'checkout', label: 'Check out after creating', type: 'checkbox', value: true },
      { key: 'force', label: 'Overwrite the branch if it already exists (<code>-f</code>)', type: 'checkbox', value: false },
    ] });
    if (v) act('branch.create', { name: v.name.trim(), hash, checkout: v.checkout, force: v.force }, `Branch ${v.name.trim()} created`);
  },
  checkoutCommit: async hash => {
    if (await confirmBox('Checkout commit?', `Check out <code>${short(hash)}</code> as a <b>detached HEAD</b>. Commits you make there belong to no branch.`, 'Checkout'))
      act('commit.checkout', { hash }, 'Checked out ' + short(hash));
  },
  cherryPick: async hash => {
    const po = parentOpts(hash), merge = po.length > 1;
    const v = await form({ title: `Cherry pick <code>${short(hash)}</code>`, intro: `Apply this commit on top of ${curLabel()}.`, ok: 'Cherry pick', fields: [
      ...(merge ? [{ key: 'parent', label: 'This is a merge commit. Apply changes relative to parent', type: 'select', value: '1', options: po }] : []),
      { key: 'record', label: 'Record origin in the message (<code>-x</code>)', type: 'checkbox' },
      { key: 'noCommit', label: 'Stage the changes only, do not commit (<code>--no-commit</code>)', type: 'checkbox' },
    ] });
    if (v) act('commit.cherrypick', { hash, parent: merge ? +v.parent : 0, record: v.record, noCommit: v.noCommit }, 'Cherry picked ' + short(hash));
  },
  revert: async hash => {
    const po = parentOpts(hash), merge = po.length > 1;
    const v = await form({ title: `Revert <code>${short(hash)}</code>`, intro: `Create a new commit on ${curLabel()} that undoes this commit.`, ok: 'Revert', fields:
      merge ? [{ key: 'parent', label: 'This is a merge commit. Revert relative to parent', type: 'select', value: '1', options: po }] : [] });
    if (v) act('commit.revert', { hash, parent: merge ? +v.parent : 0 }, 'Reverted ' + short(hash));
  },
  drop: async hash => {
    if (await confirmBox('Drop commit?', `Remove <code>${short(hash)}</code> from the history of ${curLabel()} by rebasing everything after it. This <b>rewrites history</b>.`, 'Drop', true))
      act('commit.drop', { hash }, 'Dropped ' + short(hash));
  },
  merge: async (target, label, action = 'branch.merge') => {
    const v = await form({ title: `Merge <b>${esc(label)}</b>`, intro: `Merge into the current branch ${curLabel()}.`, ok: 'Merge', fields: [
      { key: 'noFF', label: 'Always create a merge commit (<code>--no-ff</code>)', type: 'checkbox', value: false },
      { key: 'squash', label: 'Squash commits (<code>--squash</code>)', type: 'checkbox', value: false },
      { key: 'noCommit', label: 'Merge but do not commit (<code>--no-commit</code>)', type: 'checkbox', value: false },
    ] });
    if (!v) return;
    if (v.noFF && v.squash) { toast('--no-ff and --squash cannot be combined', 'err'); return; }
    act(action, { target, ...v }, `Merged ${label}`);
  },
  rebase: async (onto, label, action = 'branch.rebase') => {
    const v = await form({ title: 'Rebase', intro: `Rebase ${curLabel()} onto <b>${esc(label)}</b>. This <b>rewrites the commits</b> of the current branch.`, ok: 'Rebase', danger: true, fields: [
      { key: 'ignoreDate', label: 'Use current time as the author date (<code>--ignore-date</code>)', type: 'checkbox' }] });
    if (v) act(action, { onto, ignoreDate: v.ignoreDate }, 'Rebased onto ' + label);
  },
  reset: async to => {
    const v = await form({ title: `Reset ${curLabel()} to <code>${short(to)}</code>`, ok: 'Reset', danger: true, fields: [
      { key: 'mode', label: 'Mode', type: 'radio', value: 'mixed', options: [
        { v: 'soft', label: 'Soft', desc: 'keep all changes staged' }, { v: 'mixed', label: 'Mixed', desc: 'keep changes, unstaged' }, { v: 'hard', label: 'Hard', desc: 'DISCARD all uncommitted changes' }] }] });
    if (v) act('reset', { to, mode: v.mode }, `Reset (${v.mode}) to ${short(to)}`);
  },
  checkoutBranch: name => act('branch.checkout', { name }, 'Checked out ' + name),
  checkoutRemote: async full => {
    const { remote, name } = splitRemote(full);
    if ((S.refs.branches || []).some(b => b.name === name)) { act('branch.checkout', { name }, 'Checked out ' + name); return; }
    const v = await form({ title: `Checkout <b>${esc(full)}</b>`, intro: `Create a local branch that tracks <code>${esc(remote)}</code>.`, ok: 'Checkout', fields: [
      { key: 'name', label: 'Local branch name', value: name, required: true }] });
    if (v) act('branch.checkout_remote', { remoteBranch: full, name: v.name.trim() }, 'Checked out ' + v.name.trim());
  },
  renameBranch: async name => {
    const v = await form({ title: `Rename branch <b>${esc(name)}</b>`, ok: 'Rename', fields: [{ key: 'name', label: 'New name', value: name, required: true }] });
    if (v && v.name.trim() !== name) act('branch.rename', { old: name, name: v.name.trim() }, 'Renamed to ' + v.name.trim());
  },
  deleteBranch: async name => {
    const remotes = (S.refs.remoteBranches || []).map(b => splitRemote(b.name)).filter(r => r.name === name).map(r => r.remote);
    const v = await form({ title: `Delete branch <b>${esc(name)}</b>`, ok: 'Delete', danger: true, fields: [
      { key: 'force', label: 'Force delete (<code>-D</code>), even if it is not merged', type: 'checkbox' },
      ...(remotes.length ? [{ key: 'remote', label: 'Also delete on remote', type: 'select', value: '', options: [{ v: '', label: 'No' }, ...remotes.map(r => ({ v: r, label: r }))] }] : []),
    ] });
    if (v) act('branch.delete', { name, force: v.force, remote: v.remote || '' }, 'Deleted ' + name);
  },
  deleteRemoteBranch: async full => {
    const { remote, name } = splitRemote(full);
    if (await confirmBox('Delete remote branch?', `Delete <b>${esc(name)}</b> on <code>${esc(remote)}</code>. This changes the remote repository.`, 'Delete', true))
      act('branch.delete_remote', { remote, name }, 'Deleted ' + full);
  },
  pull: async (remote, branch) => {
    const v = await form({ title: `Pull <b>${esc(remote)}/${esc(branch)}</b>`, intro: `Pull into the current branch ${curLabel()}.`, ok: 'Pull', fields: [
      { key: 'noFF', label: 'Always create a merge commit (<code>--no-ff</code>)', type: 'checkbox' },
      { key: 'squash', label: 'Squash commits (<code>--squash</code>)', type: 'checkbox' }] });
    if (!v) return;
    if (v.noFF && v.squash) { toast('--no-ff and --squash cannot be combined', 'err'); return; }
    act('branch.pull', { remote, branch, ...v }, `Pulled ${remote}/${branch}`);
  },
  push: async name => {
    if (!S.refs.remotes.length) { toast('This repository has no remotes. Add one in Settings.', 'err'); return; }
    const br = S.refs.branches.find(b => b.name === name), up = br && br.upstream ? br.upstream.split('/')[0] : '';
    const v = await form({ title: `Push branch <b>${esc(name)}</b>`, ok: 'Push', fields: [
      { key: 'remotes', label: 'Remote(s)', type: 'multi', value: [up || S.refs.remotes[0].name], options: S.refs.remotes.map(r => ({ v: r.name, label: `${r.name} (${r.push || r.fetch})` })), required: true },
      { key: 'setUpstream', label: 'Set upstream (<code>-u</code>)', type: 'checkbox', value: !up },
      { key: 'mode', label: 'Force', type: 'radio', value: 'normal', options: [{ v: 'normal', label: 'Normal' }, { v: 'force-with-lease', label: 'Force with lease', desc: 'safe force push' }, { v: 'force', label: 'Force', desc: 'overwrites the remote branch' }] },
    ] });
    if (v) act('branch.push', { branch: name, remotes: v.remotes, setUpstream: v.setUpstream, mode: v.mode }, `Pushed ${name}`);
  },
  fetchInto: async full => {
    const { remote, name } = splitRemote(full);
    const v = await form({ title: `Fetch <b>${esc(full)}</b> into a local branch`, ok: 'Fetch', fields: [
      { key: 'name', label: 'Local branch', value: name, required: true },
      { key: 'force', label: 'Force (<code>--force</code>), allow non fast-forward updates', type: 'checkbox' }] });
    if (v) act('branch.fetch_into', { remote, remoteBranch: name, name: v.name.trim(), force: v.force }, 'Fetched into ' + v.name.trim());
  },
  tagDetails: async name => {
    try {
      const t = await api(`/api/git/tag?repo=${enc(S.repo)}&name=${enc(name)}`);
      const m = modal(`<form class="gg-form"><h3>Tag <b>${esc(name)}</b></h3>${t.annotated
        ? `<div class="gg-dmeta"><span>Object</span><span><code>${esc(t.hash)}</code></span><span>Tagger</span><span>${esc(t.tagger)} &lt;${esc(t.email)}&gt;</span><span>Date</span><span>${esc(t.date)}</span></div><div class="gg-dbody">${rich(t.message)}</div>`
        : '<div class="gg-intro">This is a lightweight tag: it has no message, tagger or date of its own.</div>'}
        <div class="gg-factions"><button type="button" class="btn primary" data-c>Close</button></div></form>`);
      q('[data-c]', m.box).onclick = () => m.close();
    } catch (e) { toast(e.message, 'err'); }
  },
  deleteTag: async name => {
    const v = await form({ title: `Delete tag <b>${esc(name)}</b>`, ok: 'Delete', danger: true, fields:
      S.refs.remotes.length ? [{ key: 'remote', label: 'Also delete on remote', type: 'select', value: '', options: remoteOpts('No') }] : [] });
    if (v) act('tag.delete', { name, remote: v.remote || '' }, 'Deleted tag ' + name);
  },
  pushTag: async name => {
    if (!S.refs.remotes.length) { toast('This repository has no remotes.', 'err'); return; }
    const v = await form({ title: `Push tag <b>${esc(name)}</b>`, ok: 'Push', fields: [{ key: 'remote', label: 'Remote', type: 'select', value: S.refs.remotes[0].name, options: remoteOpts() }] });
    if (v) act('tag.push', { name, remote: v.remote }, 'Pushed tag ' + name);
  },
  stash: async (op, sel) => {
    if (op === 'drop') { if (await confirmBox('Drop stash?', `Permanently delete <b>${esc(sel)}</b>.`, 'Drop', true)) act('stash.drop', { selector: sel }, 'Dropped ' + sel); return; }
    const v = await form({ title: `${op === 'pop' ? 'Pop' : 'Apply'} <b>${esc(sel)}</b>`, ok: op === 'pop' ? 'Pop' : 'Apply', fields: [
      { key: 'reinstateIndex', label: 'Reinstate the index (<code>--index</code>)', type: 'checkbox' }] });
    if (v) act('stash.' + op, { selector: sel, reinstateIndex: v.reinstateIndex }, `${op === 'pop' ? 'Popped' : 'Applied'} ${sel}`);
  },
  stashBranch: async sel => {
    const v = await form({ title: `Create branch from <b>${esc(sel)}</b>`, ok: 'Create', fields: [{ key: 'name', label: 'Branch name', required: true }] });
    if (v) act('stash.branch', { selector: sel, name: v.name.trim() }, 'Branch created from ' + sel);
  },
  stashUncommitted: async () => {
    const v = await form({ title: 'Stash uncommitted changes', ok: 'Stash', fields: [
      { key: 'message', label: 'Message (optional)' }, { key: 'includeUntracked', label: 'Include untracked files (<code>--include-untracked</code>)', type: 'checkbox' }] });
    if (v) act('stash.push', v, 'Changes stashed');
  },
  resetUncommitted: async () => {
    const v = await form({ title: 'Reset uncommitted changes', ok: 'Reset', danger: true, fields: [
      { key: 'mode', label: 'Mode', type: 'radio', value: 'mixed', options: [{ v: 'mixed', label: 'Mixed', desc: 'unstage, keep the files as they are' }, { v: 'hard', label: 'Hard', desc: 'DISCARD all uncommitted changes to tracked files' }] }] });
    if (v) act('uncommitted.reset', { mode: v.mode }, `Reset (${v.mode})`);
  },
  clean: async () => {
    const v = await form({ title: 'Clean untracked files', intro: '<b>Permanently deletes</b> untracked files. Ignored files are kept. This cannot be undone.', ok: 'Clean', danger: true, fields: [
      { key: 'directories', label: 'Also remove untracked directories (<code>-d</code>)', type: 'checkbox', value: true }] });
    if (v) act('uncommitted.clean', v, 'Untracked files removed');
  },
  fetch: async () => {
    if (!S.refs.remotes.length) { toast('This repository has no remotes. Add one in Settings.', 'err'); return; }
    const v = await form({ title: 'Fetch', ok: 'Fetch', fields: [
      { key: 'remote', label: 'Remote', type: 'select', value: '', options: remoteOpts('All remotes') },
      { key: 'prune', label: 'Prune remote-tracking branches that no longer exist (<code>--prune</code>)', type: 'checkbox' },
      { key: 'pruneTags', label: 'Prune local tags that no longer exist on the remote (<code>--prune-tags</code>)', type: 'checkbox' }] });
    if (v) act('remote.fetch', v, 'Fetched');
  },
  copy: (t, what) => copyText(t, what),
  createPR: async (headName, remoteHint) => {
    if (!S.refs.remotes.length) { toast('This repository has no remotes.', 'err'); return; }
    const cfg = jget('pr:' + S.repo, { provider: 'auto', template: '' });
    const names = S.refs.branches.map(b => b.name);
    const v = await form({ title: `Create pull request for <b>${esc(headName)}</b>`, ok: 'Open in browser', fields: [
      { key: 'remote', label: 'Remote', type: 'select', value: remoteHint || S.refs.remotes[0].name, options: remoteOpts() },
      { key: 'base', label: 'Target (base) branch', value: names.includes('main') ? 'main' : names.includes('master') ? 'master' : names.find(n => n !== headName) || 'main', required: true }] });
    if (!v) return;
    const rm = S.refs.remotes.find(r => r.name === v.remote), url = prUrl(rm.fetch, headName, v.base.trim(), cfg);
    if (url) window.open(url, '_blank', 'noopener,noreferrer'); else toast('Unknown hosting provider. Set a custom URL template in Settings → Pull requests.', 'err');
  },
};

function webBase(u) {
  u = (u || '').trim(); let m;
  try { if (/^https?:\/\//i.test(u)) { const x = new URL(u); return { host: x.hostname, web: `${x.protocol}//${x.hostname}${x.pathname.replace(/\.git\/?$/, '').replace(/\/$/, '')}` }; } } catch { return null; }
  if ((m = /^(?:ssh:\/\/)?(?:[^@/]+@)?([^:/]+)(?::\d+)?[:/](.+?)(?:\.git)?\/?$/.exec(u))) return { host: m[1], web: `https://${m[1]}/${m[2]}` };
  return null;
}
function prUrl(remoteUrl, head, baseB, cfg) {
  const w = webBase(remoteUrl); if (!w) return null;
  let prov = cfg.provider;
  if (prov === 'auto') prov = /github/i.test(w.host) ? 'github' : /gitlab/i.test(w.host) ? 'gitlab' : /bitbucket/i.test(w.host) ? 'bitbucket' : (cfg.template ? 'custom' : '');
  switch (prov) {
    case 'github': return `${w.web}/compare/${enc(baseB)}...${enc(head)}?expand=1`;
    case 'gitlab': return `${w.web}/-/merge_requests/new?merge_request%5Bsource_branch%5D=${enc(head)}&merge_request%5Btarget_branch%5D=${enc(baseB)}`;
    case 'bitbucket': return `${w.web}/pull-requests/new?source=${enc(head)}&dest=${enc(baseB)}`;
    case 'custom': {
      if (!/^https?:\/\//i.test(cfg.template || '')) return null;
      return cfg.template.replace(/\{web\}/g, w.web).replace(/\{head\}/g, enc(head)).replace(/\{base\}/g, enc(baseB));
    }
  }
  return null;
}

// ------------------------------------------------------------------ context menus
function commitMenu(r) {
  const h = r.h, headHash = S.refs.headHash;
  return [
    { label: 'Add Tag…', run: () => A.addTag(h) },
    { label: 'Create Branch…', run: () => A.createBranch(h) },
    '-',
    { label: 'Checkout…', disabled: h === headHash && !S.refs.head, run: () => A.checkoutCommit(h) },
    { label: 'Cherry Pick…', disabled: h === headHash, run: () => A.cherryPick(h) },
    { label: 'Revert…', run: () => A.revert(h) },
    { label: 'Drop…', disabled: !r.ih || h === '*' || (r.p || []).length !== 1, run: () => A.drop(h) },
    '-',
    { label: 'Merge into current branch…', disabled: h === headHash, run: () => A.merge(h, short(h), 'commit.merge') },
    { label: 'Rebase current branch on this Commit…', disabled: h === headHash, run: () => A.rebase(h, short(h), 'commit.rebase') },
    { label: 'Reset current branch to this Commit…', run: () => A.reset(h) },
    '-',
    { label: 'Copy Commit Hash to Clipboard', run: () => A.copy(h, 'Commit hash') },
    { label: 'Copy Commit Subject to Clipboard', run: () => A.copy(plainMsg(r.s), 'Commit subject') },
  ];
}
function localBranchMenu(name) {
  const cur = name === S.refs.head;
  const br = S.refs.branches.find(b => b.name === name);
  const up = br && br.upstream ? br.upstream : '';
  return [
    { label: 'Checkout Branch', disabled: cur, run: () => A.checkoutBranch(name) },
    { label: 'Rename Branch…', run: () => A.renameBranch(name) },
    { label: 'Delete Branch…', disabled: cur, danger: true, run: () => A.deleteBranch(name) },
    '-',
    { label: 'Merge into current branch…', disabled: cur, run: () => A.merge(name, name) },
    { label: 'Rebase current branch on Branch…', disabled: cur, run: () => A.rebase(name, name) },
    ...(cur && up ? [{ label: `Pull ${up}…`, run: () => { const x = splitRemote(up); A.pull(x.remote, x.name); } }] : []),
    { label: 'Push Branch…', run: () => A.push(name) },
    '-',
    { label: 'Create Pull Request…', run: () => A.createPR(name, up ? splitRemote(up).remote : '') },
    { label: 'Copy Branch Name to Clipboard', run: () => A.copy(name, 'Branch name') },
  ];
}
function remoteBranchMenu(full) {
  const { remote, name } = splitRemote(full);
  return [
    { label: 'Checkout Branch…', run: () => A.checkoutRemote(full) },
    { label: 'Delete Remote Branch…', danger: true, run: () => A.deleteRemoteBranch(full) },
    { label: 'Fetch into local branch…', run: () => A.fetchInto(full) },
    '-',
    { label: 'Merge into current branch…', run: () => A.merge(full, full) },
    { label: 'Pull into current branch…', run: () => A.pull(remote, name) },
    { label: 'Rebase current branch on Branch…', run: () => A.rebase(full, full) },
    '-',
    { label: 'Create Pull Request…', run: () => A.createPR(name, remote) },
    { label: 'Copy Branch Name to Clipboard', run: () => A.copy(full, 'Branch name') },
  ];
}
const tagMenu = name => [
  { label: 'View Details', run: () => A.tagDetails(name) },
  { label: 'Delete Tag…', danger: true, run: () => A.deleteTag(name) },
  { label: 'Push Tag…', run: () => A.pushTag(name) },
  '-',
  { label: 'Copy Tag Name to Clipboard', run: () => A.copy(name, 'Tag name') },
];
const stashMenu = (sel, hash) => [
  { label: 'Apply Stash…', run: () => A.stash('apply', sel) },
  { label: 'Create Branch from Stash…', run: () => A.stashBranch(sel) },
  { label: 'Pop Stash…', run: () => A.stash('pop', sel) },
  { label: 'Drop Stash…', danger: true, run: () => A.stash('drop', sel) },
  '-',
  { label: 'Copy Stash Name to Clipboard', run: () => A.copy(sel, 'Stash name') },
  { label: 'Copy Stash Hash to Clipboard', run: () => A.copy(hash, 'Stash hash') },
];
const uncommittedMenu = () => [
  { label: 'Stash uncommitted changes…', run: A.stashUncommitted },
  '-',
  { label: 'Reset uncommitted changes…', danger: true, run: A.resetUncommitted },
  { label: 'Clean untracked files…', danger: true, run: A.clean },
];
el.rows.addEventListener('contextmenu', e => {
  const row = e.target.closest('.gg-row'); if (!row || !S.refs) return;
  e.preventDefault(); hideTip();
  const r = S.rows[+row.dataset.i], lab = e.target.closest('[data-t]');
  let items;
  if (r.h === '*') items = uncommittedMenu();
  else if (lab) {
    const t = lab.dataset.t, n = lab.dataset.n;
    items = t === 'head' ? localBranchMenu(n) : t === 'remote' ? remoteBranchMenu(n) : t === 'tag' ? tagMenu(n) : t === 'stash' ? stashMenu(n, r.h) : commitMenu(r);
  } else items = r.st ? stashMenu(r.st, r.h) : commitMenu(r);
  showMenu(e.clientX, e.clientY, items);
});

// ------------------------------------------------------------------ toolbar
el.repo.addEventListener('change', () => el.repo.value && setRepo(el.repo.value));
q('#gg-addrepo').onclick = addRepoFlow;
q('#gg-rmrepo').onclick = async () => {
  if (!S.repo || !(await confirmBox('Remove repository from the list?', `<code>${esc(S.repo)}</code> stays on disk; it is only removed from devdeck.`, 'Remove'))) return;
  const list = S.repos.filter(r => r !== S.repo);
  try { await saveRepos(list); } catch (e) { toast(e.message, 'err'); return; }
  S.repo = ''; store.set(LS + 'repo', '');
  if (list.length) setRepo(list[0]); else { renderRepoSelect(); renderEmpty(); }
};
q('#gg-refresh').onclick = () => loadAll(true);
q('#gg-fetch').onclick = () => S.refs && A.fetch();
q('#gg-find').onclick = () => openFind();
q('#gg-settings').onclick = () => S.refs && openSettings('remotes');
q('#gg-br').onclick = e => branchDd(e.currentTarget);
q('#gg-view').onclick = e => viewDd(e.currentTarget);

function branchDd(btn) {
  if (!S.refs) return;
  openDd(btn, '<div class="dd-h"><input class="dd-s" placeholder="Search branches…" autocomplete="off"></div><div class="dd-list"></div>', d => {
    const list = q('.dd-list', d), inp = q('.dd-s', d);
    const draw = () => {
      const f = inp.value.toLowerCase(), sel = new Set(S.filter);
      const item = (key, label) => label.toLowerCase().includes(f) || !f ? `<label class="dd-i"><input type="checkbox" data-k="${esc(key)}" ${sel.has(key) ? 'checked' : ''}><span>${esc(label)}</span></label>` : '';
      const globs = jget('globs', []);
      const sec = (title, arr) => arr.join('') ? `<div class="dd-t">${title}</div>${arr.join('')}` : '';
      list.innerHTML = `<label class="dd-i"><input type="checkbox" data-all ${S.filter.length ? '' : 'checked'}><b>Show All</b></label>` +
        sec('Local branches', S.refs.branches.map(b => item(b.ref, b.name))) +
        sec('Remote branches', S.refs.remoteBranches.map(b => item(b.ref, b.name))) +
        sec('Custom patterns', globs.map(g => item('glob:' + g.glob, `${g.name}  (${g.glob})`)));
    };
    list.onchange = e => {
      const t = e.target;
      if (t.dataset.all !== undefined) S.filter = [];
      else S.filter = t.checked ? [...new Set([...S.filter, t.dataset.k])] : S.filter.filter(x => x !== t.dataset.k);
      jset('filter:' + S.repo, S.filter); draw(); renderBar(); reloadSoon();
    };
    inp.oninput = draw; draw(); inp.focus();
  });
}
function viewDd(btn) {
  const chk = (k, label) => `<label class="dd-i"><input type="checkbox" data-p="${k}" ${P[k] ? 'checked' : ''}><span>${label}</span></label>`;
  const sel = (k, label, opts) => `<label class="dd-i sel"><span>${label}</span><select data-p="${k}">${opts.map(([v, l]) => `<option value="${v}" ${P[k] === v ? 'selected' : ''}>${l}</option>`).join('')}</select></label>`;
  const ccol = (k, label) => `<label class="dd-i"><input type="checkbox" data-c="${k}" ${P.cols[k] ? 'checked' : ''}><span>${label}</span></label>`;
  openDd(btn, `<div class="dd-list">
    <div class="dd-t">Show</div>${chk('remote', 'Remote branches')}${chk('tags', 'Tags')}${chk('stashes', 'Stashes')}${chk('uncommitted', 'Uncommitted changes')}
    <div class="dd-t">Mute</div>${chk('muteNonHead', 'Commits that are not ancestors of HEAD')}${chk('muteMerge', 'Merge commits')}
    <div class="dd-t">History</div>${chk('firstParent', 'Only follow the first parent')}${chk('combine', 'Combine local and remote branch labels')}
    ${sel('order', 'Commit order', [['date', 'Commit date'], ['author-date', 'Author date'], ['topo', 'Topological']])}
    ${sel('dateType', 'Date column shows', [['author', 'Author date'], ['commit', 'Commit date']])}
    ${sel('style', 'Graph style', [['curved', 'Curved'], ['angular', 'Angular']])}
    <div class="dd-t">Columns</div>${ccol('date', 'Date')}${ccol('author', 'Author')}${ccol('hash', 'Commit')}
    <div class="dd-t">Loading</div>${chk('auto', 'Load more commits automatically')}</div>`, d => {
    d.onchange = e => {
      const t = e.target;
      if (t.dataset.c) { P.cols[t.dataset.c] = t.checked; savePrefs(); renderHead(); renderRows(); return; }
      const k = t.dataset.p; if (!k) return;
      P[k] = t.type === 'checkbox' ? t.checked : t.value; savePrefs();
      if (['remote', 'tags', 'stashes', 'uncommitted', 'firstParent', 'order', 'muteNonHead'].includes(k)) reloadSoon(); else { renderHead(); renderRows(); paintFind(); }
    };
  });
}

// ------------------------------------------------------------------ find widget
function openFind() {
  S.find.open = true; el.find.hidden = false; el.fq.focus(); el.fq.select();
}
function closeFind() { S.find.open = false; el.find.hidden = true; S.find.m = []; S.find.i = -1; paintFind(); }
function computeFind() {
  const f = el.fq.value.trim().toLowerCase(); S.find.q = f; S.find.m = []; S.find.i = -1;
  if (f) S.rows.forEach((r, i) => {
    if (r.h === '*') return;
    const hay = [r.h, plainMsg(r.s), r.a, r.e || '', fmtDate(r.at), fmtDate(r.ct), ...(r.r || []).map(x => x.n)].join('\n').toLowerCase();
    if (hay.includes(f)) S.find.m.push(i);
  });
}
function paintFind() {
  const set = new Set(S.find.m), cur = S.find.m[S.find.i];
  qa('.gg-row', el.rows).forEach((r, i) => { r.classList.toggle('found', set.has(i)); r.classList.toggle('fcur', i === cur); });
  q('#gg-fc').textContent = S.find.q ? (S.find.m.length ? `${S.find.i >= 0 ? S.find.i + 1 : 0} of ${S.find.m.length}` : 'no matches') : '';
}
function findStep(d) {
  if (!S.find.m.length) return;
  S.find.i = (S.find.i + d + S.find.m.length) % S.find.m.length;
  scrollToRow(S.find.m[S.find.i]); paintFind();
}
el.fq.addEventListener('input', () => { computeFind(); if (S.find.m.length) { S.find.i = 0; scrollToRow(S.find.m[0]); } paintFind(); });
el.fq.addEventListener('keydown', e => { if (e.key === 'Enter') { e.preventDefault(); findStep(e.shiftKey ? -1 : 1); } });
q('#gg-fnext').onclick = () => findStep(1);
q('#gg-fprev').onclick = () => findStep(-1);
q('#gg-fclose').onclick = closeFind;

// ------------------------------------------------------------------ settings widget
let settingsM = null, settingsTab = 'remotes';
function openSettings(tab) {
  settingsTab = tab || settingsTab;
  if (!settingsM || !settingsM.el.isConnected) {
    settingsM = modal('', { cls: 'wide', onClose: () => { settingsM = null; } });
    settingsM.box.addEventListener('click', onSettingsClick);
  }
  renderSettings();
}
function renderSettings() {
  if (!settingsM) return;
  const tabs = [['remotes', 'Remotes'], ['issues', 'Issue linking'], ['pr', 'Pull requests'], ['globs', 'Branch patterns']];
  let body = '';
  if (settingsTab === 'remotes') {
    const rs = S.refs.remotes;
    body = `<div class="gg-toolrow"><button class="btn primary sm" data-s="addremote">Add remote</button><button class="btn sm" data-s="fetchall">Fetch all</button></div>` +
      (rs.length ? `<table class="gg-tbl"><thead><tr><th>Name</th><th>URL</th><th></th></tr></thead><tbody>${rs.map(r => `<tr><td><b>${esc(r.name)}</b></td><td class="mono">${esc(r.fetch)}${r.push && r.push !== r.fetch ? `<br><span class="muted">push: ${esc(r.push)}</span>` : ''}</td>
        <td class="act"><button class="btn sm" data-s="fetch" data-n="${esc(r.name)}">Fetch</button><button class="btn sm" data-s="prune" data-n="${esc(r.name)}">Prune</button><button class="btn sm" data-s="edit" data-n="${esc(r.name)}">Edit</button><button class="btn sm danger" data-s="delremote" data-n="${esc(r.name)}">Delete</button></td></tr>`).join('')}</tbody></table>` : '<div class="empty">No remotes configured.</div>');
  } else if (settingsTab === 'issues') {
    const c = jget('issue:' + S.repo, { regex: '', url: '' });
    body = `<p class="muted">Turn issue references in commit messages into links. <code>$1</code> in the URL is replaced with the first capture group.</p>
      <label class="f"><span>Regular expression</span><input id="is-re" value="${esc(c.regex)}" placeholder="#(\\d+)"></label>
      <label class="f"><span>Issue URL</span><input id="is-url" value="${esc(c.url)}" placeholder="https://github.com/owner/repo/issues/$1"></label>
      <div class="gg-toolrow"><button class="btn primary sm" data-s="saveissue">Save</button><button class="btn sm" data-s="detectissue">Detect from remote</button><button class="btn sm" data-s="clearissue">Clear</button></div>`;
  } else if (settingsTab === 'pr') {
    const c = jget('pr:' + S.repo, { provider: 'auto', template: '' });
    body = `<p class="muted">GitHub, GitLab and Bitbucket URLs are built from the remote URL. For other hosts use a template with <code>{web}</code>, <code>{base}</code> and <code>{head}</code>.</p>
      <label class="f"><span>Provider</span><select id="pr-prov">${[['auto', 'Detect from remote'], ['github', 'GitHub'], ['gitlab', 'GitLab'], ['bitbucket', 'Bitbucket'], ['custom', 'Custom template']].map(([v, l]) => `<option value="${v}" ${c.provider === v ? 'selected' : ''}>${l}</option>`).join('')}</select></label>
      <label class="f"><span>Custom URL template</span><input id="pr-tpl" value="${esc(c.template)}" placeholder="{web}/pulls/new?from={head}&amp;to={base}"></label>
      <div class="gg-toolrow"><button class="btn primary sm" data-s="savepr">Save</button></div>`;
  } else {
    const gl = jget('globs', []);
    body = `<p class="muted">Custom patterns appear in the Branches dropdown. Example: <code>heads/feature/*</code> shows all local feature branches.</p>
      ${gl.length ? `<table class="gg-tbl"><tbody>${gl.map((g, i) => `<tr><td><b>${esc(g.name)}</b></td><td class="mono">${esc(g.glob)}</td><td class="act"><button class="btn sm danger" data-s="delglob" data-n="${i}">Remove</button></td></tr>`).join('')}</tbody></table>` : ''}
      <div class="gg-toolrow"><button class="btn primary sm" data-s="addglob">Add pattern</button></div>`;
  }
  settingsM.box.innerHTML = `<div class="gg-mh"><b>Repository settings</b> <span class="muted">${esc(S.repo)}</span><span class="sp"></span><button class="btn sm" data-s="close">&times;</button></div>
    <div class="gg-tabs">${tabs.map(([k, l]) => `<button class="${k === settingsTab ? 'on' : ''}" data-tab="${k}">${l}</button>`).join('')}</div><div class="gg-mb pad">${body}</div>
    <div class="gg-mfoot muted">Issue-linking and pull-request settings are stored in this browser. Exporting the configuration to a file is not supported.</div>`;
}
async function onSettingsClick(e) {
  const tab = e.target.closest('[data-tab]'); if (tab) { settingsTab = tab.dataset.tab; renderSettings(); return; }
  const b = e.target.closest('[data-s]'); if (!b) return;
  const s = b.dataset.s, n = b.dataset.n;
  const rem = n && S.refs.remotes.find(r => r.name === n);
  const again = async r => { if (r !== null) renderSettings(); };
  if (s === 'close') settingsM.close();
  else if (s === 'addremote') {
    const v = await form({ title: 'Add remote', ok: 'Add', fields: [
      { key: 'name', label: 'Name', required: true, placeholder: 'origin', validate: x => /^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(x.trim()) ? null : 'Use letters, digits, . _ -' },
      { key: 'url', label: 'Fetch URL', required: true, placeholder: 'https://github.com/owner/repo.git' }, { key: 'pushUrl', label: 'Push URL (optional)' },
      { key: 'fetch', label: 'Fetch after adding', type: 'checkbox', value: true }] });
    if (v) again(await act('remote.add', { name: v.name.trim(), url: v.url.trim(), pushUrl: v.pushUrl.trim(), fetch: v.fetch }, 'Remote added'));
  } else if (s === 'edit' && rem) {
    const v = await form({ title: `Edit remote <b>${esc(n)}</b>`, ok: 'Save', fields: [
      { key: 'name', label: 'Name', value: n, required: true }, { key: 'url', label: 'Fetch URL', value: rem.fetch, required: true },
      { key: 'pushUrl', label: 'Push URL (optional)', value: rem.push !== rem.fetch ? rem.push : '' }] });
    if (v) again(await act('remote.edit', { old: n, name: v.name.trim(), url: v.url.trim(), pushUrl: v.pushUrl.trim() }, 'Remote updated'));
  } else if (s === 'delremote') {
    if (await confirmBox('Delete remote?', `Remove <b>${esc(n)}</b> and its remote-tracking branches from this repository.`, 'Delete', true)) again(await act('remote.delete', { name: n }, 'Remote deleted'));
  } else if (s === 'fetch') again(await act('remote.fetch', { remote: n, prune: true }, `Fetched ${n}`));
  else if (s === 'fetchall') again(await act('remote.fetch', {}, 'Fetched all remotes'));
  else if (s === 'prune') again(await act('remote.prune', { name: n }, `Pruned ${n}`));
  else if (s === 'saveissue') { jset('issue:' + S.repo, { regex: q('#is-re', settingsM.box).value, url: q('#is-url', settingsM.box).value }); S.issueFor = null; renderRows(); toast('Saved'); }
  else if (s === 'clearissue') { jset('issue:' + S.repo, null); S.issueFor = null; renderRows(); renderSettings(); }
  else if (s === 'detectissue') {
    const w = S.refs.remotes[0] && webBase(S.refs.remotes[0].fetch);
    if (!w) { toast('No remote URL to detect from', 'err'); return; }
    const url = /gitlab/i.test(w.host) ? `${w.web}/-/issues/$1` : /bitbucket/i.test(w.host) ? `${w.web}/issues/$1` : `${w.web}/issues/$1`;
    q('#is-re', settingsM.box).value = '#(\\d+)'; q('#is-url', settingsM.box).value = url;
  } else if (s === 'savepr') { jset('pr:' + S.repo, { provider: q('#pr-prov', settingsM.box).value, template: q('#pr-tpl', settingsM.box).value }); toast('Saved'); }
  else if (s === 'addglob') {
    const v = await form({ title: 'Add branch pattern', ok: 'Add', fields: [{ key: 'name', label: 'Name', required: true }, { key: 'glob', label: 'Glob pattern', required: true, placeholder: 'heads/feature/*',
      validate: x => /^[A-Za-z0-9._/*?[\]+@#-]+$/.test(x) && !x.startsWith('-') && !x.includes('..') ? null : 'Only letters, digits and . _ / * ? [ ] + @ # - are allowed' }] });
    if (v) { jset('globs', [...jget('globs', []), { name: v.name.trim(), glob: v.glob.trim() }]); renderSettings(); }
  } else if (s === 'delglob') { const g = jget('globs', []); g.splice(+n, 1); jset('globs', g); renderSettings(); }
}

// ------------------------------------------------------------------ keyboard
const gitActive = () => { const s = document.getElementById('git'); return s && s.classList.contains('on'); };
function stepRow(d) {
  if (!S.rows.length) return;
  const cur = S.sel.a ? S.idx.get(S.sel.a) : -1;
  const i = Math.max(0, Math.min(S.rows.length - 1, (cur === undefined ? -1 : cur) + d));
  S.sel = { a: S.rows[i].h, b: null }; paintSelection(); scrollToRow(i, false); openDetails();
}
function followParent(alt) {
  const r = rowFor(S.sel.a); if (!r || !r.p || !r.p.length) return;
  const p = alt && r.p.length > 1 ? r.p[1] : r.p[0];
  if (S.idx.has(p)) { S.sel = { a: p, b: null }; paintSelection(); scrollToRow(S.idx.get(p), false); openDetails(); }
}
function followChild(alt) {
  const ks = S.kids.get(S.sel.a); if (!ks || !ks.length) return;
  const first = ks.filter(k => (rowFor(k).p || [])[0] === S.sel.a);
  const pick = alt ? (ks.find(k => k !== (first[0] || ks[0])) || ks[0]) : (first[0] || ks[0]);
  S.sel = { a: pick, b: null }; paintSelection(); scrollToRow(S.idx.get(pick), false); openDetails();
}
function jumpStash(dir) {
  const idxs = S.rows.map((r, i) => r.st ? i : -1).filter(i => i >= 0); if (!idxs.length) return;
  const mid = Math.round((el.scroll.scrollTop + el.scroll.clientHeight / 2) / RH);
  const pick = dir > 0 ? (idxs.find(i => i > mid) ?? idxs[0]) : ([...idxs].reverse().find(i => i < mid) ?? idxs[idxs.length - 1]);
  scrollToRow(pick); flash(pick);
}
document.addEventListener('keydown', e => {
  if (!gitActive()) return;
  if (e.key === 'Escape') {
    if (modals.length) { modals[modals.length - 1].close(); e.preventDefault(); return; }
    if (menuEl) { closeMenu(); return; }
    if (ddEl) { closeDd(); return; }
    if (S.find.open) { closeFind(); return; }
    if (S.sel.a) closeDetails();
    return;
  }
  if (modals.length) return;
  const mod = e.ctrlKey || e.metaKey, k = e.key.toLowerCase();
  if (mod && k === 'f') { e.preventDefault(); openFind(); return; }
  if (/^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName) || e.target.isContentEditable) return;
  if (mod && k === 'r') { e.preventDefault(); loadAll(true); }
  else if (mod && k === 'h') { e.preventDefault(); const i = S.rows.findIndex(r => r.h === (S.refs && S.refs.headHash)); if (i >= 0) { scrollToRow(i); flash(i); } }
  else if (mod && k === 's') { e.preventDefault(); jumpStash(e.shiftKey ? -1 : 1); }
  else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    const d = e.key === 'ArrowDown' ? 1 : -1;
    if (!S.sel.a) return;
    e.preventDefault();
    if (mod) d > 0 ? followParent(e.shiftKey) : followChild(e.shiftKey); else stepRow(d);
  }
});

// ------------------------------------------------------------------ lifecycle
async function init() {
  S.inited = true;
  await loadConfig();
  if (S.repo && !S.repos.includes(S.repo)) {
    try { const r = await api(`/api/git/repo?path=${enc(S.repo)}`); S.repo = r.root; await saveRepos([...S.repos, S.repo]); } catch { S.repo = ''; }
  }
  if (!S.repo && S.repos.length) S.repo = S.repos[0];
  store.set(LS + 'repo', S.repo);
  S.filter = jget('filter:' + S.repo, []);
  renderRepoSelect(); renderBar();
  loadAll(false);
}
document.addEventListener('devdeck:tab', e => {
  if (e.detail !== 'git') { hideTip(); closeMenu(); closeDd(); return; }
  if (!S.inited) init(); else checkSig();
});
async function checkSig() {
  if (!S.repo || S.loading || S.busy || modals.length || !gitActive() || document.hidden) return;
  try { const r = await api(`/api/git/signature?repo=${enc(S.repo)}`); if (S.sig && r.sig !== S.sig) { S.tip.clear(); loadAll(true); } else if (!S.sig) S.sig = r.sig; } catch { /* ignore */ }
}
setInterval(checkSig, 15000);
if (gitActive()) init();
})();
