'use strict';
// Redis tab (read-only browser).

let rid = null, rcursor = '0';
const ttlText = t => t === -1 ? 'no expiry' : t === -2 ? 'missing' : t < 90 ? t + 's' : t < 5400 ? (t / 60 | 0) + 'm' : t < 172800 ? (t / 3600 | 0) + 'h' : (t / 86400 | 0) + 'd';
async function rScan(reset) {
  if (reset) { rcursor = '0'; $('#r-keys').innerHTML = ''; }
  try {
    const r = await api(`/api/redis/scan?id=${rid}&cursor=${rcursor}&match=${encodeURIComponent($('#r-match').value || '*')}&count=500`);
    rcursor = r.cursor;
    $('#r-keys').insertAdjacentHTML('beforeend', (r.keys || []).map(k => `<div class="rk" data-k="${esc(k.key)}"><span class="tp ${esc(k.type)}">${esc(k.type)}</span><span class="k mono">${esc(k.key)}</span>${k.ttl >= 0 ? `<span class="ttl">${ttlText(k.ttl)}</span>` : ''}</div>`).join(''));
    if (reset && !(r.keys || []).length && rcursor === '0') $('#r-keys').innerHTML = '<div class="empty">No keys match.</div>';
    $('#r-more').hidden = rcursor === '0';
  } catch (e) { $('#r-keys').textContent = e.message; }
}
$('#r-connect').onclick = async () => {
  $('#r-info').textContent = 'Connecting…';
  try {
    if (rid) post('/api/redis/close', { id: rid });
    const r = await post('/api/redis/connect', { addr: $('#r-addr').value, db: +$('#r-db').value || 0, password: $('#r-pass').value, tls: $('#r-tls').checked });
    rid = r.id; $('#r-info').textContent = `Connected · Redis ${r.version} · read-only`;
    $('#r-body').hidden = false; $('#r-empty').hidden = true; $('#r-pass').value = ''; rScan(true);
  } catch (e) { $('#r-info').textContent = ''; toast(e.message, 'err'); }
};
$('#r-scan').onclick = () => rScan(true);
$('#r-match').onkeydown = e => { if (e.key === 'Enter') rScan(true); };
$('#r-more').onclick = () => rScan(false);
$('#r-keys').onclick = async e => {
  const row = e.target.closest('.rk'); if (!row) return;
  document.querySelectorAll('.rk.on').forEach(x => x.classList.remove('on')); row.classList.add('on');
  try {
    const kv = await api(`/api/redis/key?id=${rid}&key=${encodeURIComponent(row.dataset.k)}`);
    const head = `${kv.key}\ntype=${kv.type}   ttl=${ttlText(kv.ttl)}   size=${kv.size}${kv.truncated ? '   (truncated)' : ''}\n\n`;
    let body = kv.value;
    if (kv.type === 'string') { try { body = JSON.stringify(JSON.parse(kv.value), null, 2); } catch {} }
    else if (kv.type === 'hash') body = (kv.value || []).map(([f, v]) => `${f}\t${v}`).join('\n');
    else if (kv.type === 'zset') body = (kv.value || []).map(([m, s]) => `${s}\t${m}`).join('\n');
    else if (Array.isArray(kv.value)) body = typeof kv.value[0] === 'string' ? kv.value.join('\n') : JSON.stringify(kv.value, null, 2);
    $('#r-val').textContent = head + body;
  } catch (err) { $('#r-val').textContent = err.message; }
};
