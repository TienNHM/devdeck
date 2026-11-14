'use strict';
// Claude tab: live Claude Code sessions reported by hooks.

async function loadClaude() {
  try {
    const s = await api('/api/claude/sessions');
    $('#c-t').innerHTML = s.length ? s.map(x => `<tr><td><span class="badge ${esc(x.state)}">${esc(x.state)}</span></td><td class="mono">${esc(x.cwd)}</td><td>${esc(x.event)}</td>
      <td class="muted">${esc(x.message || x.tool || '')}</td><td class="muted">${ago(x.updated)} ago</td>
      <td class="act"><button class="btn sm" data-cwd="${esc(x.cwd)}">Open terminal here</button></td></tr>`).join('')
      : '<tr><td colspan=6 class="empty">No sessions reported yet. Install the hooks (devdeck hooks-config) and start claude.</td></tr>';
  } catch (e) { $('#c-t').innerHTML = `<tr><td colspan=6>${esc(e.message)}</td></tr>`; }
}
$('#c-t').onclick = e => {
  const b = e.target.closest('button[data-cwd]'); if (!b) return;
  show('term'); newTerm('shell', { kind: 'local', cwd: b.dataset.cwd });
};
onTab('claude', loadClaude);
every(3000, 'claude', loadClaude);
