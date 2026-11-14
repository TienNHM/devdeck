'use strict';
/* devdeck core. Loaded before every feature module. Everything here is a shared global:
 *   $ esc store api post put mb ago toast dialog                 helpers
 *   t(key, english) registerStrings(lang, dict) LANG            i18n (en default, vi)
 *   show(tab) onTab(tab, fn) every(ms, tab, fn) current           navigation / polling
 *   registerCommand({title, hint, keywords, run})                 Ctrl+K palette
 *   registerCommandProvider(query => [{title, hint, run}])        dynamic palette entries
 *   beforeQuit (array of fns) termCount() stopped                 shutdown
 * Feature modules live in <name>.js + <name>.css and render into <section id="name">.
 */
const $ = (s, r = document) => r.querySelector(s);
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const store = { get: (k, d) => { try { return localStorage.getItem(k) ?? d; } catch { return d; } }, set: (k, v) => { try { localStorage.setItem(k, v); } catch {} } };

async function api(path, opts) {
  const r = await fetch(path, opts);
  const j = await r.json().catch(() => ({}));
  if (!r.ok) { const e = new Error(j.error || r.statusText); e.output = j.output; throw e; }
  return j;
}
const post = (p, body) => api(p, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
const put = (p, body) => api(p, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
const mb = n => (n / 1048576).toFixed(1) + ' MB';
const ago = t => { const s = Math.max(0, Date.now() / 1000 - t) | 0; return s < 60 ? s + 's' : s < 3600 ? (s / 60 | 0) + 'm' : s < 86400 ? (s / 3600 | 0) + 'h' : (s / 86400 | 0) + 'd'; };

// ------------------------------------------------------------------ i18n
const LANG = store.get('lang', 'en') === 'vi' ? 'vi' : 'en';
const I18N = { vi: {} };
function registerStrings(lang, dict) { if (I18N[lang]) Object.assign(I18N[lang], dict); }
function t(key, english) { return (LANG !== 'en' && I18N[LANG][key]) || english; }
function applyI18n(root = document) {
  root.querySelectorAll('[data-i18n]').forEach(el => { el.dataset.en ??= el.textContent; el.textContent = t(el.dataset.i18n, el.dataset.en); });
  root.querySelectorAll('[data-i18n-ph]').forEach(el => { el.dataset.enPh ??= el.placeholder; el.placeholder = t(el.dataset.i18nPh, el.dataset.enPh); });
  root.querySelectorAll('[data-i18n-title]').forEach(el => { el.dataset.enTitle ??= el.title; el.title = t(el.dataset.i18nTitle, el.dataset.enTitle); });
}
registerStrings('vi', {
  'palette.placeholder': 'Tìm hoặc chạy lệnh…', 'palette.none': 'Không có lệnh phù hợp', 'palette.goto': 'Mở tab', 'palette.tab': 'Tab',
  'palette.theme': 'Đổi giao diện sáng / tối', 'palette.lang': 'Đổi ngôn ngữ (English / Tiếng Việt)', 'palette.sidebar': 'Thu gọn / mở thanh bên',
  'palette.quit': 'Thoát devdeck', 'quit': 'Thoát', 'cancel': 'Huỷ',
  'nav.system': 'Hệ thống', 'nav.code': 'Mã nguồn', 'nav.data': 'Dữ liệu', 'nav.ops': 'Vận hành', 'nav.remote': 'Từ xa', 'nav.ai': 'AI',
  'tab.ports': 'Cổng', 'tab.procs': 'Tiến trình', 'tab.projects': 'Dự án', 'tab.git': 'Git', 'tab.redis': 'Redis', 'tab.db': 'Cơ sở dữ liệu',
  'tab.docker': 'Docker', 'tab.logs': 'Log', 'tab.term': 'Terminal', 'tab.ssh': 'SSH', 'tab.claude': 'Claude',
  'quit.title': 'Thoát devdeck?', 'quit.terms': '{n} terminal đang mở sẽ bị đóng.',
  'quit.text': 'Server sẽ dừng, và hook của Claude bị bỏ qua cho tới khi bạn chạy lại devdeck.',
  'quit.done': 'devdeck đã dừng', 'quit.again': 'Chạy lại bằng devdeck.exe --open, rồi đóng tab này.',
});
document.documentElement.lang = LANG;
$('#lang').onclick = () => { store.set('lang', LANG === 'vi' ? 'en' : 'vi'); location.reload(); };
$('#lang').title = LANG === 'vi' ? 'Switch to English' : 'Chuyển sang tiếng Việt';

// ------------------------------------------------------------------ toasts + dialogs
function toast(msg, kind) {
  const el = document.createElement('div'); el.className = 'toast' + (kind === 'err' ? ' err' : ''); el.textContent = msg;
  $('#toasts').appendChild(el); setTimeout(() => el.remove(), kind === 'err' ? 6000 : 2800);
}
// Resolves null when cancelled, true when confirmed, or the typed string when `input` is set.
function dialog({ title, text = '', input = '', ok = 'OK', danger = false, password = true }) {
  return new Promise(res => {
    const d = $('#dlg'), inp = $('#dlg-in'), okb = $('#dlg-ok');
    $('#dlg-t').textContent = title; $('#dlg-p').textContent = text;
    inp.hidden = !input; inp.value = ''; inp.placeholder = input; inp.type = password ? 'password' : 'text';
    okb.textContent = ok; okb.className = 'btn ' + (danger ? 'danger-solid' : 'primary');
    d.returnValue = '';
    d.onclose = () => res(d.returnValue === 'ok' ? (input ? inp.value : true) : null);
    d.showModal(); (input ? inp : okb).focus();
  });
}

let stopped = false;
const beforeQuit = [];
let termCount = () => 0;

// ------------------------------------------------------------------ theme + sidebar
const root = document.documentElement;
$('#theme').onclick = () => { const v = root.dataset.theme === 'dark' ? 'light' : 'dark'; root.dataset.theme = v; store.set('theme', v); };
$('#collapse').onclick = () => {
  const c = root.classList.toggle('side-collapsed'); store.set('side', c ? 'collapsed' : 'open');
  window.dispatchEvent(new Event('resize'));
};

// ------------------------------------------------------------------ navigation
const TABS = [...document.querySelectorAll('#tabs [data-tab]')].map(b => b.dataset.tab);
let current = null;
function show(tab) {
  if (!TABS.includes(tab)) tab = 'ports';
  current = tab;
  document.querySelectorAll('#tabs [data-tab]').forEach(b => b.classList.toggle('on', b.dataset.tab === tab));
  document.querySelectorAll('main > section').forEach(s => s.classList.toggle('on', s.id === tab));
  store.set('tab', tab);
  document.dispatchEvent(new CustomEvent('devdeck:tab', { detail: tab }));
}
function onTab(tab, fn) { document.addEventListener('devdeck:tab', e => { if (e.detail === tab) fn(); }); }
// Poll only while the tab is visible, the page is in front and the server is up (keeps devdeck idle otherwise).
function every(ms, tab, fn) { setInterval(() => { if (!stopped && !document.hidden && current === tab) fn(); }, ms); }
$('#tabs').onclick = e => { const b = e.target.closest('[data-tab]'); if (b) show(b.dataset.tab); };

async function refreshSelf() {
  if (stopped) return;
  try { const s = await api('/api/self'); $('#self').textContent = `${mb(s.rss)} RAM`; } catch {}
}
refreshSelf(); setInterval(refreshSelf, 15000);

// ------------------------------------------------------------------ command palette (Ctrl+K)
const commands = [], providers = [];
function registerCommand(c) { commands.push(c); }
function registerCommandProvider(fn) { providers.push(fn); }
let palItems = [], palSel = 0;
function fuzzy(text, q) {
  text = text.toLowerCase(); let i = 0, score = 0;
  for (const ch of q) { const j = text.indexOf(ch, i); if (j < 0) return -1; score += j - i; i = j + 1; }
  return score;
}
async function palRender() {
  const q = $('#pal-q').value.trim().toLowerCase();
  let items = commands.map(c => ({ c, s: q ? fuzzy(`${c.title} ${c.keywords || ''}`, q.replace(/\s+/g, '')) : 0 })).filter(x => x.s >= 0).sort((a, b) => a.s - b.s).map(x => x.c);
  const dyn = [];
  for (const p of providers) { try { dyn.push(...(await p(q) || [])); } catch {} }
  palItems = [...dyn, ...items].slice(0, 60);
  palSel = Math.min(palSel, Math.max(0, palItems.length - 1));
  $('#pal-list').innerHTML = palItems.length ? palItems.map((c, i) => `<div class="pi${i === palSel ? ' on' : ''}" data-i="${i}"><span>${esc(c.title)}</span>${c.hint ? `<span class="muted">${esc(c.hint)}</span>` : ''}</div>`).join('')
    : `<div class="empty">${esc(t('palette.none', 'No matching command'))}</div>`;
  $('#pal-list .pi.on')?.scrollIntoView({ block: 'nearest' });
}
function openPalette(prefill = '') { const d = $('#pal'); if (d.open) return; $('#pal-q').value = prefill; palSel = 0; d.showModal(); palRender(); $('#pal-q').focus(); }
async function palRun(i) { const c = palItems[i]; if (!c) return; $('#pal').close(); try { await c.run(); } catch (e) { toast(e.message, 'err'); } }
$('#palbtn').onclick = () => openPalette();
$('#pal-q').oninput = () => { palSel = 0; palRender(); };
$('#pal-q').onkeydown = e => {
  if (e.key === 'ArrowDown') { palSel = Math.min(palSel + 1, palItems.length - 1); palRender(); e.preventDefault(); }
  else if (e.key === 'ArrowUp') { palSel = Math.max(palSel - 1, 0); palRender(); e.preventDefault(); }
  else if (e.key === 'Enter') { palRun(palSel); e.preventDefault(); }
};
$('#pal-list').onclick = e => { const p = e.target.closest('.pi'); if (p) palRun(+p.dataset.i); };
$('#pal').addEventListener('click', e => { if (e.target.id === 'pal') $('#pal').close(); });
document.addEventListener('keydown', e => {
  if ((e.ctrlKey || e.metaKey) && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 'k') { e.preventDefault(); openPalette(); }
});
document.querySelectorAll('#tabs [data-tab]').forEach(b => registerCommand({
  title: `${t('palette.goto', 'Go to')} ${t('tab.' + b.dataset.tab, b.textContent.trim())}`, keywords: b.dataset.tab, hint: t('palette.tab', 'Tab'), run: () => show(b.dataset.tab),
}));
registerCommand({ title: t('palette.theme', 'Toggle light / dark theme'), keywords: 'theme dark light', run: () => $('#theme').click() });
registerCommand({ title: t('palette.lang', 'Switch language (English / Tiếng Việt)'), keywords: 'language vietnamese english ngon ngu', run: () => $('#lang').click() });
registerCommand({ title: t('palette.sidebar', 'Toggle sidebar'), keywords: 'sidebar collapse', run: () => $('#collapse').click() });
registerCommand({ title: t('palette.quit', 'Quit devdeck'), keywords: 'quit exit stop shutdown', run: () => $('#quit').click() });

// ------------------------------------------------------------------ quit
$('#quit').onclick = async () => {
  const n = termCount();
  const ok = await dialog({
    title: t('quit.title', 'Quit devdeck?'),
    text: (n ? t('quit.terms', `${n} open terminal(s) will be closed.`).replace('{n}', n) + ' ' : '') + t('quit.text', 'The server stops, and Claude hooks are ignored until you start devdeck again.'),
    ok: t('quit', 'Quit'), danger: true,
  });
  if (!ok) return;
  stopped = true;
  try { await post('/api/quit', {}); } catch {}
  beforeQuit.forEach(f => { try { f(); } catch {} });
  document.body.innerHTML = `<div class="bye"><div><h2>${esc(t('quit.done', 'devdeck stopped'))}</h2><p>${esc(t('quit.again', 'Start it again with devdeck.exe --open, then close this tab.'))}</p></div></div>`;
};

// Modules register listeners while their scripts load; start once all of them are in.
document.addEventListener('DOMContentLoaded', () => { applyI18n(); show(store.get('tab', 'ports')); });
