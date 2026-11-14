'use strict';
/* Claude tab: live Claude Code sessions (pushed over SSE from the hook handler), attention
 * notifications (desktop notification, beep, sidebar badge), session details read lazily from the
 * transcript, resume / terminal shortcuts, hooks setup status, and "Ask Claude" (streams `claude -p`).
 * Globals for other modules:
 *   askClaude({title, prompt, context, cwd}) -> Promise<string|null>   modal that streams the answer;
 *       context may be a string, a Promise or a function returning one; without prompt it asks first.
 *   claudeStream({prompt, context, cwd, signal, onText}) -> Promise<string>   raw streaming call.
 *   claudeMarkdown(text) -> safe HTML (code blocks, inline code, bold, lists, headings).
 */
registerStrings('vi', {
  'claude.new': 'Phiên mới…',
  'claude.ask': 'Hỏi Claude…',
  'claude.setup': 'Cài đặt',
  'claude.live': 'Đang cập nhật trực tiếp',
  'claude.offline': 'Mất kết nối, đang thử lại…',
  'claude.hooks.ok': 'Đã cài hook',
  'claude.hooks.partial': 'Hook chưa đủ',
  'claude.hooks.none': 'Chưa cài hook',
  'claude.empty': 'Chưa có phiên nào. Cài hook (nút Cài đặt) rồi chạy claude trong một thư mục.',
  'claude.loading': 'Đang tải phiên…',
  'claude.state.idle': 'rảnh',
  'claude.state.working': 'đang làm',
  'claude.state.waiting': 'cần bạn',
  'claude.updated': 'cập nhật {t} trước',
  'claude.tool': 'Công cụ',
  'claude.details': 'Chi tiết',
  'claude.hide': 'Ẩn chi tiết',
  'claude.term': 'Mở terminal tại đây',
  'claude.resume': 'Tiếp tục',
  'claude.resume.title': 'Mở lại cuộc trò chuyện này trong một terminal mới (claude --resume)',
  'claude.askhere': 'Hỏi tại đây…',
  'claude.dismiss': 'Ẩn',
  'claude.dismiss.title': 'Bỏ phiên khỏi danh sách (sẽ hiện lại khi có hook mới)',
  'claude.d.prompt': 'Yêu cầu gần nhất',
  'claude.d.reply': 'Trả lời gần nhất',
  'claude.d.model': 'Mô hình',
  'claude.d.branch': 'Nhánh',
  'claude.d.tokens': 'Token',
  'claude.d.tokens.v': 'vào {i} · ra {o} · đọc cache {r} · ghi cache {w}',
  'claude.d.started': 'Bắt đầu',
  'claude.d.duration': 'Thời lượng',
  'claude.d.partial': 'Tệp transcript lớn: chỉ đọc 2 MB cuối, số token chỉ tính phần này.',
  'claude.d.none': 'Chưa có dữ liệu transcript.',
  'claude.badid': 'Mã phiên không hợp lệ, không thể tiếp tục.',
  'claude.noterm': 'Không mở được terminal.',
  'claude.n.waiting': 'Claude cần bạn trả lời',
  'claude.n.done': 'Claude đã xong việc',
  'claude.bell': 'Thông báo Claude',
  'claude.on': 'bật',
  'claude.off': 'tắt',
  'claude.bell.notify': 'Thông báo trên màn hình',
  'claude.bell.sound': 'Âm báo ngắn',
  'claude.bell.test': 'Thử',
  'claude.bell.blocked': 'Trình duyệt đang chặn thông báo cho trang này. Hãy cho phép trong cài đặt trang.',
  'claude.bell.unsupported': 'Trình duyệt này không hỗ trợ thông báo.',
  'claude.bell.on': 'Đã bật thông báo Claude',
  'claude.bell.hint': 'Báo khi một phiên cần bạn trả lời hoặc vừa xong việc.',
  'claude.setup.title': 'Kết nối Claude Code',
  'claude.setup.ok': 'devdeck đã có hook trong {p} cho: {e}.',
  'claude.setup.missing': 'Còn thiếu: {e}.',
  'claude.setup.none': 'Chưa thấy hook devdeck trong {p}.',
  'claude.setup.how': 'Chạy lệnh dưới đây một lần, rồi gộp khối "hooks" vào settings.json. devdeck chỉ đọc tệp này, không bao giờ ghi.',
  'claude.setup.copy': 'Sao chép',
  'claude.copied': 'Đã sao chép',
  'claude.copyfail': 'Sao chép thất bại',
  'claude.ask.title': 'Hỏi Claude',
  'claude.ask.q': 'Câu hỏi',
  'claude.ask.cwd': 'Thư mục làm việc',
  'claude.ask.cwd.ph': 'Để trống để dùng thư mục người dùng',
  'claude.ask.go': 'Hỏi',
  'claude.ask.ctx': 'Đang thu thập ngữ cảnh…',
  'claude.ask.wait': 'Claude đang trả lời…',
  'claude.ask.done': 'Xong',
  'claude.ask.cancel': 'Hủy',
  'claude.ask.close': 'Đóng',
  'claude.ask.copy': 'Sao chép',
  'claude.ask.cancelled': 'Đã hủy',
  'claude.ask.empty': '(không có nội dung)',
  'claude.ask.context': 'kèm {n} KB ngữ cảnh',
  'claude.ask.note': 'Chạy claude -p trên máy của bạn và dùng hạn mức của bạn.',
  'claude.cmd.ask': 'Hỏi Claude…',
  'claude.cmd.resume': 'Tiếp tục phiên Claude gần nhất',
  'claude.cmd.notify': 'Bật thông báo Claude',
  'claude.cmd.open': 'Mở phiên Claude trong {f}',
  'claude.cmd.resumein': 'Tiếp tục phiên Claude trong {f}',
  'claude.cmd.nosession': 'Chưa có phiên Claude nào để tiếp tục.',
  'claude.new.title': 'Phiên Claude mới',
  'claude.new.cwd': 'Thư mục',
  'claude.new.go': 'Mở',
});

(() => {
const tf = (key, en, vars) => t(key, en).replace(/\{(\w+)\}/g, (m, k) => (vars && k in vars ? vars[k] : m));
const base = p => String(p || '').replace(/[\\/]+$/, '').split(/[\\/]/).pop() || p || '';
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const pref = {
  get notify() { return store.get('claude.notify', '0') === '1'; },
  set notify(v) { store.set('claude.notify', v ? '1' : '0'); },
  get sound() { return store.get('claude.sound', '0') === '1'; },
  set sound(v) { store.set('claude.sound', v ? '1' : '0'); },
};
const CL = { sessions: [], loaded: false, details: new Map(), open: new Set(), unseen: new Set(), es: null, errs: 0, backoff: 0, hooks: null, repos: null, reposAt: 0 };

function copy(text) {
  const ok = () => toast(t('claude.copied', 'Copied'));
  const fb = () => {
    const ta = document.createElement('textarea'); ta.value = text; ta.style.position = 'fixed'; ta.style.opacity = '0';
    document.body.appendChild(ta); ta.select();
    try { document.execCommand('copy'); ok(); } catch { toast(t('claude.copyfail', 'Copy failed'), 'err'); }
    ta.remove();
  };
  if (navigator.clipboard) navigator.clipboard.writeText(text).then(ok, fb); else fb();
}
const kfmt = n => n >= 1e6 ? (n / 1e6).toFixed(2) + 'M' : n >= 1e4 ? (n / 1e3).toFixed(1) + 'k' : String(n || 0);
function dur(s) {
  s = Math.max(0, s | 0);
  const h = s / 3600 | 0, m = (s % 3600) / 60 | 0, sec = s % 60;
  return h ? `${h}h ${String(m).padStart(2, '0')}m` : m ? `${m}m ${String(sec).padStart(2, '0')}s` : `${sec}s`;
}

// ------------------------------------------------------------------ light markdown (escaped first)
function inline(s) {
  return s.split(/(`[^`\n]+`)/).map(p => p.length > 2 && p[0] === '`' && p[p.length - 1] === '`'
    ? `<code>${esc(p.slice(1, -1))}</code>`
    : esc(p).replace(/\*\*([^*\n]+)\*\*/g, '<b>$1</b>')).join('');
}
function claudeMarkdown(src) {
  const out = [], lines = String(src || '').replace(/\r\n?/g, '\n').split('\n');
  let list = null, para = [];
  const flush = () => { if (para.length) { out.push(`<p>${para.map(inline).join('<br>')}</p>`); para = []; } };
  const close = () => { if (list) { out.push(`</${list}>`); list = null; } };
  for (let i = 0; i < lines.length; i++) {
    const l = lines[i];
    let m;
    if (/^\s*(```|~~~)/.test(l)) {
      flush(); close();
      const fence = l.trim().slice(0, 3), code = [];
      for (i++; i < lines.length && !lines[i].trim().startsWith(fence); i++) code.push(lines[i]);
      out.push(`<pre class="claude-code"><code>${esc(code.join('\n'))}</code></pre>`);
    } else if ((m = /^(#{1,6})\s+(.*)$/.exec(l))) {
      flush(); close(); out.push(`<div class="claude-h claude-h${Math.min(m[1].length, 3)}">${inline(m[2])}</div>`);
    } else if ((m = /^\s*[-*+]\s+(.*)$/.exec(l)) || (m = /^\s*(\d+)[.)]\s+(.*)$/.exec(l))) {
      const kind = m.length === 3 ? 'ol' : 'ul';
      flush();
      if (list !== kind) { close(); out.push(`<${kind}>`); list = kind; }
      out.push(`<li>${inline(m[m.length - 1])}</li>`);
    } else if (!l.trim()) {
      flush(); close();
    } else if (list && /^\s{2,}\S/.test(l) && out.length) {
      out[out.length - 1] = out[out.length - 1].replace(/<\/li>$/, `<br>${inline(l.trim())}</li>`);
    } else {
      close(); para.push(l);
    }
  }
  flush(); close();
  return out.join('');
}

// ------------------------------------------------------------------ streaming ask
async function claudeStream({ prompt, context = '', cwd = '', signal, onText } = {}) {
  const r = await fetch('/api/claude/ask', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ prompt, context, cwd }), signal });
  if (!r.ok) { const j = await r.json().catch(() => ({})); throw new Error(j.error || r.statusText); }
  const rd = r.body.getReader(), dec = new TextDecoder();
  let buf = '', text = '';
  const fail = msg => Object.assign(new Error(msg), { partial: text });
  for (;;) {
    const { value, done } = await rd.read();
    if (done) break;
    buf += dec.decode(value, { stream: true });
    let i;
    while ((i = buf.indexOf('\n')) >= 0) {
      const line = buf.slice(0, i); buf = buf.slice(i + 1);
      if (!line.trim()) continue;
      let m; try { m = JSON.parse(line); } catch { continue; }
      if (typeof m.t === 'string') { text += m.t; if (onText) onText(text, m.t); }
      else if (m.err) throw fail(m.err);
      else if (m.done) return text;
    }
  }
  throw fail('The connection closed before Claude finished.');
}

let askOpen = null; // the visible ask modal (Esc cancels it)
function askClaude({ title, prompt, context, cwd } = {}) {
  return new Promise(resolve => {
    const ov = document.createElement('div');
    ov.className = 'claude-modal';
    ov.innerHTML = `<div class="claude-mbox" role="dialog" aria-modal="true">
      <div class="claude-mh"><svg class="i"><use href="#i-claude"/></svg><b class="claude-mt"></b><span class="claude-status muted"></span><span class="sp"></span>
        <button class="btn sm" data-a="copy" hidden>${esc(t('claude.ask.copy', 'Copy'))}</button>
        <button class="btn sm" data-a="cancel">${esc(t('claude.ask.cancel', 'Cancel'))}</button></div>
      <form class="claude-askform" hidden>
        <label>${esc(t('claude.ask.q', 'Question'))}<textarea name="q" rows="5"></textarea></label>
        <label>${esc(t('claude.ask.cwd', 'Working directory'))}<input name="cwd" placeholder="${esc(t('claude.ask.cwd.ph', 'Empty = your home folder'))}" spellcheck="false" autocomplete="off"></label>
        <div class="claude-factions"><span class="muted">${esc(t('claude.ask.note', 'Runs claude -p on your machine and uses your quota.'))}</span><span class="sp"></span><button class="btn primary" type="submit">${esc(t('claude.ask.go', 'Ask'))}</button></div>
      </form>
      <div class="claude-q muted" hidden></div>
      <div class="claude-out" hidden></div>
      <div class="claude-mf muted"></div>
    </div>`;
    document.body.appendChild(ov);
    const $m = s => ov.querySelector(s);
    const status = $m('.claude-status'), out = $m('.claude-out'), btnCopy = $m('[data-a=copy]'), btnCancel = $m('[data-a=cancel]');
    $m('.claude-mt').textContent = title || t('claude.ask.title', 'Ask Claude');
    let ctrl = null, text = '', finished = false, raf = 0;
    const me = { cancel: () => close(null) };
    askOpen = me;
    function close(result) {
      if (finished && !ov.isConnected) return;
      if (ctrl) ctrl.abort();
      ov.remove(); if (askOpen === me) askOpen = null;
      if (!finished) { finished = true; resolve(result); }
    }
    const paint = () => { raf = 0; out.innerHTML = text ? claudeMarkdown(text) : `<span class="muted">${esc(t('claude.ask.empty', '(no output)'))}</span>`; };
    btnCopy.onclick = () => copy(text);
    btnCancel.onclick = () => close(finished ? text : null);
    ov.addEventListener('mousedown', e => { if (e.target === ov && !ctrl) close(finished ? text : null); });

    async function run(q, dir, ctxIn) {
      $m('.claude-askform').hidden = true;
      const qEl = $m('.claude-q'); qEl.hidden = false; qEl.textContent = q.length > 400 ? q.slice(0, 400) + '…' : q;
      out.hidden = false; out.innerHTML = '';
      status.innerHTML = `<span class="claude-spin"></span>${esc(t('claude.ask.ctx', 'Collecting context…'))}`;
      let ctx = '';
      try {
        ctx = typeof ctxIn === 'function' ? await ctxIn() : await ctxIn;
        ctx = ctx == null ? '' : String(ctx);
      } catch (e) { status.textContent = ''; out.innerHTML = `<div class="claude-err">${esc(e.message)}</div>`; btnCancel.textContent = t('claude.ask.close', 'Close'); return; }
      if (!ov.isConnected) return;
      $m('.claude-mf').textContent = [dir, ctx ? tf('claude.ask.context', 'with {n} KB of context', { n: Math.max(1, Math.round(ctx.length / 1024)) }) : ''].filter(Boolean).join(' · ');
      status.innerHTML = `<span class="claude-spin"></span>${esc(t('claude.ask.wait', 'Claude is answering…'))}`;
      ctrl = new AbortController();
      try {
        text = await claudeStream({ prompt: q, context: ctx, cwd: dir, signal: ctrl.signal, onText: x => { text = x; btnCopy.hidden = false; if (!raf) raf = setTimeout(paint, 60); } });
        status.textContent = t('claude.ask.done', 'Done');
      } catch (e) {
        if (!ov.isConnected) return;
        if (e.partial) text = e.partial;
        status.textContent = '';
        paint();
        out.insertAdjacentHTML('beforeend', `<div class="claude-err">${esc(e.name === 'AbortError' ? t('claude.ask.cancelled', 'Cancelled') : e.message)}</div>`);
        ctrl = null; btnCancel.textContent = t('claude.ask.close', 'Close'); btnCopy.hidden = !text;
        return;
      }
      ctrl = null;
      if (raf) { clearTimeout(raf); raf = 0; }
      paint(); btnCopy.hidden = !text; btnCancel.textContent = t('claude.ask.close', 'Close');
      finished = true; resolve(text); // the modal stays open for reading; closing later is a no-op for the promise
    }

    if (prompt) run(prompt, cwd || '', context);
    else {
      const f = $m('.claude-askform'); f.hidden = false;
      f.cwd.value = cwd || (CL.sessions[0] && CL.sessions[0].cwd) || '';
      f.q.focus();
      f.q.addEventListener('keydown', e => { if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); f.requestSubmit(); } });
      f.onsubmit = e => {
        e.preventDefault();
        const q = f.q.value.trim(); if (!q) { f.q.focus(); return; }
        run(q, f.cwd.value.trim(), context);
      };
    }
  });
}
// Esc cancels the ask modal before other modules' Escape handlers see it.
window.addEventListener('keydown', e => {
  if (e.key === 'Escape' && askOpen) { e.preventDefault(); e.stopImmediatePropagation(); askOpen.cancel(); }
}, true);

// ------------------------------------------------------------------ sessions (SSE)
function connect() {
  if (stopped || CL.es) return;
  const es = new EventSource('/api/claude/events');
  CL.es = es;
  es.onopen = () => { CL.errs = 0; CL.backoff = 0; renderLive(); };
  es.onmessage = e => { let m; try { m = JSON.parse(e.data); } catch { return; } onSnapshot(m); };
  es.onerror = () => {
    renderLive();
    // The browser retries by itself; after a few failures (server gone, auth) back off instead.
    if (es.readyState === EventSource.CLOSED || ++CL.errs >= 3) {
      es.close(); CL.es = null; renderLive();
      if (!stopped) setTimeout(connect, Math.min(60000, 3000 * 2 ** CL.backoff++));
    }
  };
}
document.addEventListener('visibilitychange', () => {
  if (!document.hidden) { if (!CL.es) connect(); if (current === 'claude') seen(); }
});
beforeQuit.push(() => { if (CL.es) { CL.es.close(); CL.es = null; } });

function onSnapshot(m) {
  const prev = new Map(CL.sessions.map(s => [s.id, s]));
  CL.sessions = Array.isArray(m.sessions) ? m.sessions : [];
  CL.loaded = true;
  const ids = new Set(CL.sessions.map(s => s.id));
  for (const id of [...CL.unseen]) if (!ids.has(id)) CL.unseen.delete(id);
  for (const id of [...CL.details.keys()]) if (!ids.has(id)) { CL.details.delete(id); CL.open.delete(id); }
  if (m.alert) onAlert(m.alert);
  for (const s of CL.sessions) {
    const p = prev.get(s.id);
    if (CL.open.has(s.id) && p && p.event !== s.event && (s.event === 'Stop' || s.event === 'Notification')) loadDetails(s.id);
  }
  updateBadge();
  render();
}

// ------------------------------------------------------------------ attention: badge, notification, beep
function seen() { if (CL.unseen.size) { CL.unseen.clear(); updateBadge(); } }
function updateBadge() {
  const n = CL.sessions.filter(s => s.state === 'waiting').length + CL.unseen.size;
  const btn = document.querySelector('#tabs [data-tab=claude]');
  if (btn) {
    let b = btn.querySelector('.badge-n');
    if (n && !b) { b = document.createElement('span'); b.className = 'badge-n'; btn.appendChild(b); }
    if (b) { b.textContent = n; b.hidden = !n; }
  }
  document.title = (n ? `(${n}) ` : '') + document.title.replace(/^\(\d+\) /, '');
}

let actx = null;
function audio() {
  try {
    actx = actx || new (window.AudioContext || window.webkitAudioContext)();
    if (actx.state === 'suspended') actx.resume();
  } catch { actx = null; }
  return actx;
}
function beep(kind) {
  const ac = audio(); if (!ac) return;
  const notes = kind === 'waiting' ? [660, 880, 880] : [880, 660];
  notes.forEach((f, i) => {
    const o = ac.createOscillator(), g = ac.createGain(), t0 = ac.currentTime + 0.02 + i * 0.15;
    o.type = 'sine'; o.frequency.value = f;
    g.gain.setValueAtTime(0.0001, t0);
    g.gain.exponentialRampToValueAtTime(0.12, t0 + 0.02);
    g.gain.exponentialRampToValueAtTime(0.0001, t0 + 0.13);
    o.connect(g); g.connect(ac.destination); o.start(t0); o.stop(t0 + 0.14);
  });
}
// Browsers only allow audio after a user gesture: unlock the context on the first click.
document.addEventListener('pointerdown', () => { if (pref.sound) audio(); }, { once: true, capture: true });

function onAlert(a) {
  const visible = !document.hidden && document.hasFocus();
  if (a.kind === 'done' && !(visible && current === 'claude')) CL.unseen.add(a.id);
  const title = a.kind === 'waiting' ? t('claude.n.waiting', 'Claude needs your input') : t('claude.n.done', 'Claude finished');
  const body = base(a.cwd) + (a.message ? ' · ' + a.message : '');
  if (pref.sound) beep(a.kind);
  if (visible) { if (current !== 'claude' || a.kind === 'waiting') toast(`${title}: ${body}`); return; }
  if (pref.notify && 'Notification' in window && Notification.permission === 'granted') {
    try {
      const n = new Notification(title, { body: `${body}\n${a.cwd || ''}`.trim(), tag: 'devdeck-claude-' + a.id, renotify: true });
      n.onclick = () => { window.focus(); show('claude'); n.close(); };
    } catch { /* some browsers only allow notifications from a service worker */ }
  }
}

async function enableNotify() {
  if (!('Notification' in window)) { toast(t('claude.bell.unsupported', 'This browser does not support notifications.'), 'err'); return false; }
  let p = Notification.permission;
  if (p === 'default') { try { p = await Notification.requestPermission(); } catch { p = 'denied'; } }
  if (p !== 'granted') { pref.notify = false; renderBell(); toast(t('claude.bell.blocked', 'Notifications are blocked for this page. Allow them in the site settings.'), 'err'); return false; }
  pref.notify = true; audio(); renderBell();
  toast(t('claude.bell.on', 'Claude notifications are on'));
  return true;
}

// bell in the header (appended, so other modules can share #header-extra)
const bell = document.createElement('span');
bell.className = 'claude-bellwrap';
bell.innerHTML = `<button class="icon-btn claude-bell" type="button"><svg class="i"><use href="#i-bell"/></svg></button>`;
($('#header-extra') || document.querySelector('header')).appendChild(bell);
const bellBtn = bell.firstElementChild;
let pop = null;
function renderBell() {
  const on = pref.notify && 'Notification' in window && Notification.permission === 'granted';
  bellBtn.classList.toggle('on', on || pref.sound);
  bellBtn.title = `${t('claude.bell', 'Claude notifications')}: ${on ? t('claude.on', 'on') : t('claude.off', 'off')}${pref.sound ? ' + ' + t('claude.bell.sound', 'Short sound').toLowerCase() : ''}`;
  if (pop) drawPop();
}
function drawPop() {
  const perm = 'Notification' in window ? Notification.permission : 'unsupported';
  pop.innerHTML = `<div class="claude-pop-h">${esc(t('claude.bell', 'Claude notifications'))}</div>
    <div class="muted claude-pop-hint">${esc(t('claude.bell.hint', 'Tells you when a session needs your input or has finished.'))}</div>
    <label class="check"><input type="checkbox" data-k="notify" ${pref.notify && perm === 'granted' ? 'checked' : ''} ${perm === 'unsupported' ? 'disabled' : ''}> ${esc(t('claude.bell.notify', 'Desktop notifications'))}</label>
    ${perm === 'denied' ? `<div class="claude-err">${esc(t('claude.bell.blocked', 'Notifications are blocked for this page. Allow them in the site settings.'))}</div>` : ''}
    <label class="check"><input type="checkbox" data-k="sound" ${pref.sound ? 'checked' : ''}> ${esc(t('claude.bell.sound', 'Short sound'))}</label>
    <div><button class="btn sm" data-k="test" type="button">${esc(t('claude.bell.test', 'Test'))}</button></div>`;
}
function closePop() { if (pop) { pop.remove(); pop = null; } }
bellBtn.onclick = () => {
  if (pop) { closePop(); return; }
  pop = document.createElement('div'); pop.className = 'claude-pop'; document.body.appendChild(pop); drawPop();
  const r = bellBtn.getBoundingClientRect();
  pop.style.top = (r.bottom + 6) + 'px';
  pop.style.left = Math.max(8, Math.min(r.right - pop.offsetWidth, innerWidth - pop.offsetWidth - 8)) + 'px';
  pop.addEventListener('change', async e => {
    const k = e.target.dataset.k;
    if (k === 'notify') { if (e.target.checked) await enableNotify(); else { pref.notify = false; renderBell(); } }
    if (k === 'sound') { pref.sound = e.target.checked; if (pref.sound) { audio(); beep('done'); } renderBell(); }
  });
  pop.addEventListener('click', e => {
    if (e.target.dataset.k !== 'test') return;
    if (pref.sound) beep('waiting');
    if (pref.notify && 'Notification' in window && Notification.permission === 'granted') {
      try { const n = new Notification(t('claude.n.waiting', 'Claude needs your input'), { body: 'devdeck', tag: 'devdeck-claude-test' }); n.onclick = () => { window.focus(); show('claude'); n.close(); }; } catch {}
    } else toast(`${t('claude.n.waiting', 'Claude needs your input')}: devdeck`);
  });
};
document.addEventListener('mousedown', e => { if (pop && !pop.contains(e.target) && !bell.contains(e.target)) closePop(); }, true);
document.addEventListener('keydown', e => { if (e.key === 'Escape') closePop(); });
renderBell();

// ------------------------------------------------------------------ rendering
const stateLabel = s => t('claude.state.' + s, s);
function renderLive() {
  const el = $('#claude-live'); if (!el) return;
  const ok = CL.es && CL.es.readyState === EventSource.OPEN;
  el.className = 'claude-live muted' + (ok ? ' ok' : '');
  el.textContent = ok ? t('claude.live', 'Live') : CL.loaded || CL.errs || !CL.es ? t('claude.offline', 'Disconnected, retrying…') : '';
}

function detailsHTML(s) {
  const d = CL.details.get(s.id);
  if (!d) return `<div class="claude-det"><span class="claude-spin"></span></div>`;
  if (d.error && !d.transcript) return `<div class="claude-det muted">${esc(d.error)}</div>`;
  const x = d.transcript || {};
  const started = x.started || s.started, last = x.last || s.updated;
  const row = (k, v) => v ? `<span>${esc(k)}</span><div>${v}</div>` : '';
  return `<div class="claude-det"><div class="claude-grid">
    ${row(t('claude.d.prompt', 'Last prompt'), x.lastPrompt ? `<div class="claude-prompt">${esc(x.lastPrompt)}</div>` : '')}
    ${row(t('claude.d.reply', 'Last reply'), x.lastReply ? `<div class="claude-md">${claudeMarkdown(x.lastReply)}</div>` : '')}
    ${row(t('claude.d.model', 'Model'), x.model ? `<code>${esc(x.model)}</code>${x.version ? ` <span class="muted">Claude Code ${esc(x.version)}</span>` : ''}` : '')}
    ${row(t('claude.d.branch', 'Branch'), x.branch ? `<code>${esc(x.branch)}</code>` : '')}
    ${row(t('claude.d.tokens', 'Tokens'), x.inputTokens || x.outputTokens || x.cacheReadTokens ? esc(tf('claude.d.tokens.v', 'in {i} · out {o} · cache read {r} · cache write {w}', { i: kfmt(x.inputTokens), o: kfmt(x.outputTokens), r: kfmt(x.cacheReadTokens), w: kfmt(x.cacheWriteTokens) })) : '')}
    ${row(t('claude.d.started', 'Started'), started ? esc(new Date(started * 1000).toLocaleString()) : '')}
    ${row(t('claude.d.duration', 'Duration'), started && last >= started ? esc(dur(last - started)) : '')}
  </div>${x.partial ? `<div class="muted claude-small">${esc(t('claude.d.partial', 'Large transcript: only the last 2 MB were read; token totals cover that part.'))}</div>` : ''}
  ${!d.transcript ? `<div class="muted claude-small">${esc(t('claude.d.none', 'No transcript data yet.'))}</div>` : ''}</div>`;
}

function cardHTML(s) {
  const open = CL.open.has(s.id);
  const msg = s.message || (s.tool ? `${t('claude.tool', 'Tool')}: ${s.tool}` : '');
  return `<div class="claude-ch">
      <span class="badge ${esc(s.state)}">${esc(stateLabel(s.state))}</span>
      <b class="claude-folder" title="${esc(s.cwd)}">${esc(base(s.cwd) || s.id.slice(0, 8))}</b>
      <span class="claude-path mono muted" title="${esc(s.cwd)}">${esc(s.cwd)}</span>
      <span class="sp"></span>
      <span class="muted claude-ago">${esc(tf('claude.updated', 'updated {t} ago', { t: ago(s.updated) }))}</span>
    </div>
    ${msg ? `<div class="claude-msg${s.state === 'waiting' ? ' warn' : ''}">${esc(msg)}</div>` : ''}
    ${open ? detailsHTML(s) : ''}
    <div class="claude-act">
      <button class="btn sm" data-a="details">${esc(open ? t('claude.hide', 'Hide details') : t('claude.details', 'Details'))}</button>
      <button class="btn sm" data-a="resume" ${UUID.test(s.id) && s.cwd ? '' : 'disabled'} title="${esc(t('claude.resume.title', 'Reopen this conversation in a new terminal (claude --resume)'))}"><svg class="i"><use href="#i-play"/></svg>${esc(t('claude.resume', 'Resume'))}</button>
      <button class="btn sm" data-a="term" ${s.cwd ? '' : 'disabled'}><svg class="i"><use href="#i-term"/></svg>${esc(t('claude.term', 'Open terminal here'))}</button>
      <button class="btn sm" data-a="ask"><svg class="i"><use href="#i-claude"/></svg>${esc(t('claude.askhere', 'Ask here…'))}</button>
      <span class="sp"></span>
      <button class="btn sm ghost" data-a="dismiss" title="${esc(t('claude.dismiss.title', 'Remove from the list (it comes back on its next hook event)'))}">${esc(t('claude.dismiss', 'Dismiss'))}</button>
    </div>`;
}

function render() {
  const list = $('#claude-list'); if (!list) return;
  renderLive();
  if (!CL.loaded) return;
  if (!CL.sessions.length) {
    list.innerHTML = `<div class="empty claude-empty">${esc(t('claude.empty', 'No sessions reported yet. Install the hooks (Setup) and start claude in a folder.'))}</div>`;
    return;
  }
  list.querySelector('.claude-empty, :scope > .empty')?.remove();
  const keep = new Map([...list.querySelectorAll('.claude-card')].map(e => [e.dataset.id, e]));
  let prevEl = null;
  for (const s of CL.sessions) {
    let el = keep.get(s.id); keep.delete(s.id);
    const html = cardHTML(s);
    if (!el) { el = document.createElement('div'); el.dataset.id = s.id; }
    el.className = `card claude-card st-${s.state}${CL.unseen.has(s.id) ? ' unseen' : ''}`;
    if (el._h !== html) { el.innerHTML = html; el._h = html; }
    const want = prevEl ? prevEl.nextSibling : list.firstChild;
    if (el !== want) list.insertBefore(el, want);
    prevEl = el;
  }
  keep.forEach(e => e.remove());
}

async function loadDetails(id) {
  try { CL.details.set(id, await api(`/api/claude/session?id=${encodeURIComponent(id)}`)); }
  catch (e) { CL.details.set(id, { error: e.message }); }
  render();
}

function openTerm(title, init) {
  if (typeof newTerm !== 'function') { toast(t('claude.noterm', 'Cannot open a terminal.'), 'err'); return; }
  show('term'); newTerm(title, init);
}
function resume(s) {
  if (!s || !UUID.test(s.id)) { toast(t('claude.badid', 'This session id is not valid, cannot resume.'), 'err'); return; }
  openTerm(`claude · ${base(s.cwd)}`, { kind: 'local', cwd: s.cwd, cmd: 'claude --resume ' + s.id });
}
const openClaudeIn = dir => openTerm(`claude · ${base(dir)}`, { kind: 'local', cwd: dir, cmd: 'claude' });

$('#claude-list').addEventListener('click', async e => {
  const b = e.target.closest('button[data-a]'), card = e.target.closest('.claude-card');
  if (!b || !card) return;
  const s = CL.sessions.find(x => x.id === card.dataset.id); if (!s) return;
  switch (b.dataset.a) {
    case 'details':
      if (CL.open.has(s.id)) CL.open.delete(s.id); else { CL.open.add(s.id); loadDetails(s.id); }
      render(); break;
    case 'resume': resume(s); break;
    case 'term': openTerm(`shell · ${base(s.cwd)}`, { kind: 'local', cwd: s.cwd }); break;
    case 'ask': askClaude({ cwd: s.cwd }); break;
    case 'dismiss':
      try { await post('/api/claude/dismiss', { id: s.id }); } catch (err) { toast(err.message, 'err'); }
      break;
  }
});

// ------------------------------------------------------------------ setup (read-only hooks status)
async function loadSetup() {
  try { CL.hooks = await api('/api/claude/setup'); } catch (e) { CL.hooks = { error: e.message, events: [], missing: [] }; }
  const h = CL.hooks, pill = $('#claude-hooks');
  pill.hidden = false;
  pill.className = 'pill ' + (h.installed && !h.missing.length ? 'ok' : 'warn');
  pill.textContent = h.installed ? (h.missing.length ? t('claude.hooks.partial', 'Hooks incomplete') : t('claude.hooks.ok', 'Hooks installed')) : t('claude.hooks.none', 'Hooks not installed');
  pill.style.cursor = 'pointer';
  pill.onclick = () => toggleSetup(true);
  const card = $('#claude-setup');
  card.innerHTML = `<div class="card-head"><b>${esc(t('claude.setup.title', 'Connect Claude Code'))}</b><button class="btn sm" data-a="copy">${esc(t('claude.setup.copy', 'Copy'))}</button></div>
    <div class="claude-setup-b">
      <p>${h.installed ? esc(tf('claude.setup.ok', 'devdeck hooks are set in {p} for: {e}.', { p: h.settingsPath, e: h.events.join(', ') })) : esc(tf('claude.setup.none', 'No devdeck hooks found in {p}.', { p: h.settingsPath || '~/.claude/settings.json' }))}
      ${h.installed && h.missing.length ? ' ' + esc(tf('claude.setup.missing', 'Missing: {e}.', { e: h.missing.join(', ') })) : ''}</p>
      ${h.error ? `<p class="claude-err">${esc(h.error)}</p>` : ''}
      <p class="muted">${esc(t('claude.setup.how', 'Run the command below once, then merge the "hooks" block into settings.json. devdeck only reads that file, it never writes it.'))}</p>
      <pre class="claude-code">${esc(h.snippet || '')}</pre>
    </div>`;
  card.querySelector('[data-a=copy]').onclick = () => copy(h.snippet || '');
  if (!h.installed && CL.loaded && !CL.sessions.length) toggleSetup(true);
}
function toggleSetup(force) {
  const c = $('#claude-setup');
  c.hidden = force === true ? false : !c.hidden;
}
$('#claude-setup-btn').onclick = () => toggleSetup();
$('#claude-ask').onclick = () => askClaude({});
$('#claude-new').onclick = () => newSessionDialog();

async function repoDirs() {
  if (!CL.repos || Date.now() - CL.reposAt > 60000) {
    try { const c = await api('/api/config'); CL.repos = (c.repos || []).filter(r => typeof r === 'string'); } catch { CL.repos = CL.repos || []; }
    CL.reposAt = Date.now();
  }
  const seen = new Set(), out = [];
  for (const d of [...CL.sessions.map(s => s.cwd), ...CL.repos]) {
    const k = String(d || '').toLowerCase();
    if (d && !seen.has(k)) { seen.add(k); out.push(d); }
  }
  return out;
}
async function newSessionDialog() {
  const dirs = await repoDirs();
  const v = await dialog({ title: t('claude.new.title', 'New Claude session'), text: t('claude.new.cwd', 'Folder') + (dirs.length ? ': ' + dirs.slice(0, 3).map(base).join(', ') + '…' : ''), input: dirs[0] || 'C:\\path\\to\\project', ok: t('claude.new.go', 'Open'), password: false });
  if (v === null) return;
  openClaudeIn(v.trim() || dirs[0] || '');
}

// ------------------------------------------------------------------ command palette
registerCommand({ title: t('claude.cmd.ask', 'Ask Claude…'), keywords: 'claude ai ask question prompt hoi', hint: 'Claude', run: () => askClaude({}) });
registerCommand({
  title: t('claude.cmd.resume', 'Resume last Claude session'), keywords: 'claude resume continue last tiep tuc', hint: 'Claude',
  run: () => { const s = CL.sessions.find(x => UUID.test(x.id) && x.cwd); if (s) resume(s); else toast(t('claude.cmd.nosession', 'No Claude session to resume yet.'), 'err'); },
});
registerCommand({ title: t('claude.cmd.notify', 'Enable Claude notifications'), keywords: 'claude notify notification bell alert sound thong bao', hint: 'Claude', run: enableNotify });
registerCommandProvider(async q => {
  if (!/claude|resume/.test(q)) return [];
  const f = q.replace(/\b(open|claude|session|in|resume|new)\b/g, '').trim();
  const items = [];
  if (/resume/.test(q)) {
    for (const s of CL.sessions) if (UUID.test(s.id) && s.cwd && (!f || s.cwd.toLowerCase().includes(f)))
      items.push({ title: tf('claude.cmd.resumein', 'Resume Claude session in {f}', { f: base(s.cwd) }), hint: s.cwd, run: () => resume(s) });
  }
  for (const d of await repoDirs()) if (!f || d.toLowerCase().includes(f))
    items.push({ title: tf('claude.cmd.open', 'Open Claude session in {f}', { f: base(d) }), hint: d, run: () => openClaudeIn(d) });
  return items.slice(0, 12);
});

// ------------------------------------------------------------------ lifecycle
onTab('claude', () => { seen(); render(); if (!CL.hooks) loadSetup(); });
every(30000, 'claude', render); // refresh the "updated … ago" labels only; data arrives over SSE
connect();

window.askClaude = askClaude;
window.claudeStream = claudeStream;
window.claudeMarkdown = claudeMarkdown;
})();
