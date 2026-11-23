// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

(() => {
  const state = { inventory: null, assessments: [], graph: null, summary: null, filter: 'all', query: '' };
  const $ = (id) => document.getElementById(id);
  const esc = (s) => String(s ?? '').replace(/[&<>'"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c]));

  async function get(path) {
    const r = await fetch(path, { headers: { Accept: 'application/json' } });
    if (!r.ok) throw new Error(`${path}: ${r.status}`);
    return r.json();
  }

  async function boot() {
    try {
      [state.inventory, state.assessments, state.graph, state.summary] = await Promise.all([
        get('/api/v1/inventory'), get('/api/v1/assessments'), get('/api/v1/graph'), get('/api/v1/summary')
      ]);
      renderSummary(); renderInventory(); renderGraph(); renderWaves(); bind(); extras();
    } catch (e) {
      $('inventoryRows').innerHTML = `<div class="empty">Could not load assessment: ${esc(e.message)}</div>`;
    }
  }

  function renderSummary() {
    const s = state.summary;
    $('totalVMs').textContent = s.totalVMs;
    $('readyVMs').textContent = s.ready;
    $('reviewVMs').textContent = s.review;
    $('blockedVMs').textContent = s.blocked;
    $('wavesCount').textContent = s.waves;
    $('heroScore').textContent = `${s.averageScore}%`;
  }

  function vmById(id) { return state.inventory.vms.find(v => v.id === id); }

  function renderInventory() {
    const q = state.query.toLowerCase();
    const rows = state.assessments.filter(a => (state.filter === 'all' || a.status === state.filter) && (!q || `${a.vmName} ${a.vmId} ${vmById(a.vmId)?.os || ''}`.toLowerCase().includes(q)));
    $('inventoryRows').innerHTML = rows.length ? rows.map(a => {
      const vm = vmById(a.vmId) || {};
      const findings = a.findings.length ? a.findings.map(f => f.title).join(' · ') : 'No compatibility findings';
      return `<div class="vm-row" role="button" tabindex="0" data-id="${esc(a.vmId)}">
        <div><div class="vm-title">${esc(a.vmName)}</div><div class="vm-sub">${esc(vm.os || 'Unknown OS')} · ${vm.cpus || 0} vCPU · ${Math.round((vm.memoryMiB || 0)/1024)} GiB</div></div>
        <div class="score">${a.score}%</div><div><span class="pill ${a.status}">${a.status}</span></div>
        <div>${a.wave ? `Wave ${a.wave}` : '—'}</div><div class="finding-preview">${esc(findings)}</div>
      </div>`;
    }).join('') : '<div class="empty">No workloads match this filter.</div>';
    document.querySelectorAll('.vm-row').forEach(el => {
      el.addEventListener('click', () => openDrawer(el.dataset.id));
      el.addEventListener('keydown', e => { if (e.key === 'Enter' || e.key === ' ') openDrawer(el.dataset.id); });
    });
  }

  function openDrawer(id) {
    const a = state.assessments.find(x => x.vmId === id); const vm = vmById(id); if (!a || !vm) return;
    $('drawerContent').innerHTML = `<div class="eyebrow">WORKLOAD DETAIL</div><h3>${esc(vm.name)}</h3><div class="drawer-meta">${esc(vm.os)} · ${vm.cpus} vCPU · ${Math.round(vm.memoryMiB/1024)} GiB RAM · ${esc(vm.firmware || 'unknown')}</div>
      <div class="big-score">${a.score}%</div><span class="pill ${a.status}">${a.status}</span><div class="drawer-meta" style="margin-top:10px">${a.wave ? `Migration wave ${a.wave}` : 'Excluded from migration waves until blockers are resolved'}</div>
      <h4 style="margin-top:30px">Compatibility findings</h4>
      ${a.findings.length ? a.findings.map(f => `<div class="finding-card ${f.severity}"><div class="finding-title">${esc(f.title)} <span style="float:right">−${f.penalty}</span></div><div class="finding-detail">${esc(f.detail)}</div><div class="finding-fix"><strong>Recommendation:</strong> ${esc(f.recommendation)}</div></div>`).join('') : '<div class="finding-card"><div class="finding-title">No blockers detected</div><div class="finding-detail">The built-in rules found no compatibility issues in the available inventory data.</div></div>'}`;
    $('drawer').classList.add('open'); $('drawer').setAttribute('aria-hidden','false'); $('scrim').classList.add('on');
  }
  function closeDrawer(){ $('drawer').classList.remove('open'); $('drawer').setAttribute('aria-hidden','true'); $('scrim').classList.remove('on'); }

  function renderWaves() {
    const byWave = new Map();
    state.assessments.filter(a => a.wave > 0).forEach(a => { if (!byWave.has(a.wave)) byWave.set(a.wave, []); byWave.get(a.wave).push(a); });
    const cards = [...byWave.entries()].sort((a,b)=>a[0]-b[0]).map(([wave, items]) => {
      const cpu = items.reduce((n,a)=>n+(vmById(a.vmId)?.cpus||0),0); const mem = items.reduce((n,a)=>n+(vmById(a.vmId)?.memoryMiB||0),0)/1024;
      return `<article class="wave-card"><div class="wave-number">Wave ${wave}</div><h3>${items.length} workload${items.length===1?'':'s'}</h3><div class="wave-list">${items.map(a=>`<span class="wave-chip">${esc(a.vmName)}</span>`).join('')}</div><div class="wave-meta">${cpu} vCPU · ${Math.round(mem)} GiB RAM</div></article>`;
    });
    const blocked = state.assessments.filter(a=>a.status==='blocked');
    if (blocked.length) cards.push(`<article class="wave-card blocked-wave"><div class="wave-number">Hold</div><h3>${blocked.length} blocked workload${blocked.length===1?'':'s'}</h3><div class="wave-list">${blocked.map(a=>`<span class="wave-chip">${esc(a.vmName)}</span>`).join('')}</div><div class="wave-meta">Resolve blocker findings before scheduling.</div></article>`);
    $('waveGrid').innerHTML = cards.join('');
  }

  function renderGraph() {
    const svg = $('dependencyGraph'), g = state.graph; if (!g || !g.nodes.length) return;
    const W=1000,H=520,cx=W/2,cy=H/2,rx=380,ry=190;
    const pos = new Map(); g.nodes.forEach((n,i)=>{ const a=(Math.PI*2*i/g.nodes.length)-Math.PI/2; pos.set(n.id,{x:cx+Math.cos(a)*rx,y:cy+Math.sin(a)*ry}); });
    const colors={ready:'#32d74b',review:'#ffd60a',blocked:'#ff453a'};
    const edges=g.edges.map(e=>{ const a=pos.get(e.from),b=pos.get(e.to); if(!a||!b)return''; const mx=(a.x+b.x)/2,my=(a.y+b.y)/2; return `<line class="edge" x1="${a.x}" y1="${a.y}" x2="${b.x}" y2="${b.y}"/><text class="edge-label" x="${mx+6}" y="${my-5}">${esc(e.protocol)}/${e.port}</text>`; }).join('');
    const nodes=g.nodes.map(n=>{ const p=pos.get(n.id); return `<g><circle cx="${p.x}" cy="${p.y}" r="37" fill="#1c1714" stroke="${colors[n.status]||'#ff8f66'}" class="node-circle"/><text class="node-label" text-anchor="middle" x="${p.x}" y="${p.y-2}">${esc(short(n.label,16))}</text><text class="node-sub" text-anchor="middle" x="${p.x}" y="${p.y+15}">${n.wave?`wave ${n.wave}`:n.status}</text></g>`; }).join('');
    svg.innerHTML = edges + nodes;
  }
  function short(s,n){ return s.length>n ? s.slice(0,n-1)+'…' : s; }

  function bind() {
    $('searchInput').addEventListener('input', e => { state.query=e.target.value.trim(); renderInventory(); });
    document.querySelectorAll('.segment').forEach(b=>b.addEventListener('click',()=>{ document.querySelectorAll('.segment').forEach(x=>x.classList.remove('active')); b.classList.add('active'); state.filter=b.dataset.filter; renderInventory(); }));
    $('drawerClose').addEventListener('click',closeDrawer); $('scrim').addEventListener('click',closeDrawer); document.addEventListener('keydown',e=>{if(e.key==='Escape')closeDrawer();});
  }

  // ---- estate findings, plan check, readiness diff -----------------------
  async function post(path, body) {
    const r = await fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body });
    const j = await r.json().catch(() => ({}));
    if (!r.ok) throw new Error(j.error || `${path}: ${r.status}`);
    return j;
  }
  const sevClass = (s) => (s === 'error' || s === 'blocker') ? 'blocker' : (s === 'warning' ? 'warning' : '');
  const chips = (names) => `<div class="chip-row">${(names || []).map(n => `<span class="wave-chip">${esc(n)}</span>`).join('')}</div>`;

  async function renderEstate() {
    const box = $('estateFindings');
    try {
      const groups = await get('/api/v1/estate/findings');
      box.innerHTML = groups.length ? groups.map(g => `<article class="estate-group">
        <h3>${esc(g.title)} <span class="pill ${g.severity === 'blocker' ? 'blocked' : 'review'}">${esc(g.severity)}</span></h3>
        <div class="finding-detail">${esc(g.ruleId)} · ${g.count} VM${g.count === 1 ? '' : 's'}</div>
        ${g.vms.slice(0, 50).map(v => `<div class="finding-card ${sevClass(g.severity)}"><div class="finding-title">${esc(v.vmName)}</div><div class="finding-detail">${esc(v.detail)}</div></div>`).join('')}
        ${g.vms.length > 50 ? `<div class="finding-detail">and ${g.vms.length - 50} more</div>` : ''}
        <div class="finding-fix"><strong>Recommendation:</strong> ${esc(g.recommendation)}</div></article>`).join('')
        : '<div class="empty">No estate-level findings: no duplicate identities, missing datastores or networks, or split shared-disk groups.</div>';
    } catch (e) {
      box.innerHTML = `<div class="empty">Could not load estate findings: ${esc(e.message)}</div>`;
    }
  }

  function renderViolations(v) {
    const head = `<div class="tool-summary ${v.ok ? 'ok' : 'bad'}">${v.ok ? 'No errors' : `${v.errors} error${v.errors === 1 ? '' : 's'}`}, ${v.warnings} warning${v.warnings === 1 ? '' : 's'} · ${v.plan.totals.vms} VMs in ${v.plan.waves.length} waves · ${v.plan.totals.downtimeMinutes} min downtime · ${v.plan.totals.windows} window${v.plan.totals.windows === 1 ? '' : 's'}</div>`;
    return head + v.violations.map(x => `<div class="finding-card ${sevClass(x.severity)}"><div class="finding-title">${esc(x.code)}${x.wave ? ` · wave ${esc(x.wave)}` : ''}</div><div class="finding-detail">${esc(x.message)}</div>${chips(x.vms)}</div>`).join('');
  }

  function parseMoves(text) {
    const moves = [];
    for (const part of text.split(',').map(s => s.trim()).filter(Boolean)) {
      const i = part.lastIndexOf('=');
      const wave = Number(part.slice(i + 1));
      if (i <= 0 || !Number.isInteger(wave) || wave <= 0) throw new Error(`move "${part}" must look like vm=wave with a positive wave`);
      moves.push({ vm: part.slice(0, i).trim(), toWave: wave });
    }
    return moves;
  }

  async function runPlanCheck() {
    const out = $('planResult');
    out.innerHTML = '<div class="finding-detail">Checking…</div>';
    try {
      const plan = JSON.parse($('planInput').value);
      const moves = parseMoves($('moveInput').value);
      const res = await post('/api/v1/plan/validate', JSON.stringify({ plan, moves }));
      let html = '';
      if (res.whatIf) {
        const w = res.whatIf;
        html += `<div class="tool-summary">What-if: ${w.moves.length} move${w.moves.length === 1 ? '' : 's'} · downtime <span class="${w.downtimeMinutesDelta > 0 ? 'delta-down' : 'delta-up'}">${w.downtimeMinutesDelta > 0 ? '+' : ''}${esc(w.downtimeMinutesDelta)} min</span> · windows ${w.windowsDelta > 0 ? '+' : ''}${esc(w.windowsDelta)}</div>`;
        html += `<div class="finding-detail">Resolved: ${w.resolved.length} · Introduced: ${w.introduced.length}</div>`;
        html += w.resolved.map(x => `<div class="finding-card"><div class="finding-title">Resolved: ${esc(x.code)}</div><div class="finding-detail">${esc(x.message)}</div></div>`).join('');
        html += w.introduced.map(x => `<div class="finding-card ${sevClass(x.severity)}"><div class="finding-title">Introduced: ${esc(x.code)}</div><div class="finding-detail">${esc(x.message)}</div></div>`).join('');
        html += (w.notes || []).map(n => `<div class="finding-detail">Note: ${esc(n)}</div>`).join('');
        html += '<h4 style="margin-top:18px">Plan after the moves</h4>';
      }
      out.innerHTML = html + renderViolations(res.validation);
    } catch (e) {
      out.innerHTML = `<div class="finding-card blocker"><div class="finding-title">Could not check the plan</div><div class="finding-detail">${esc(e.message)}</div></div>`;
    }
  }

  function renderDiff(res) {
    const r = res.result, g = res.gate;
    const d = r.averageScoreDelta;
    let html = `<div class="tool-summary ${g.failed ? 'bad' : 'ok'}">${g.failed ? 'Gate failed: ' + esc(g.reasons.join('; ')) : 'Gate passed'}</div>
      <div class="finding-detail">VMs ${esc(r.old.vms)} → ${esc(r.new.vms)} · ready ${esc(r.old.ready)} → ${esc(r.new.ready)} · review ${esc(r.old.review)} → ${esc(r.new.review)} · blocked ${esc(r.old.blocked)} → ${esc(r.new.blocked)} · average score ${esc(r.old.averageScore)}% → ${esc(r.new.averageScore)}% (<span class="${d < 0 ? 'delta-down' : 'delta-up'}">${d > 0 ? '+' : ''}${esc(d)}</span>)</div>
      <div class="finding-detail">${esc(r.improved)} improved · ${esc(r.regressed)} regressed · ${esc(r.addedCount)} added · ${esc(r.removedCount)} removed</div>`;
    html += r.changed.map(c => `<div class="finding-card ${c.verdict === 'regressed' ? 'blocker' : ''}"><div class="finding-title">${esc(c.vmName)} <span class="pill ${esc(c.newStatus)}">${esc(c.newStatus)}</span> <span style="float:right">${esc(c.oldScore)}% → ${esc(c.newScore)}% (${c.scoreDelta > 0 ? '+' : ''}${esc(c.scoreDelta)})</span></div>
      <div class="finding-detail">${esc(c.verdict)}: ${esc(c.oldStatus)} → ${esc(c.newStatus)}${c.matchedBy ? ' (matched by name)' : ''}${(c.rulesStarted || []).length ? ' · started: ' + esc(c.rulesStarted.join(', ')) : ''}${(c.rulesStopped || []).length ? ' · stopped: ' + esc(c.rulesStopped.join(', ')) : ''}</div></div>`).join('');
    if (r.rules.length) html += '<h4 style="margin-top:18px">Rules</h4>' + r.rules.map(c => `<div class="finding-detail">${esc(c.ruleId)}: ${esc(c.oldCount)} → ${esc(c.newCount)} VMs</div>`).join('');
    if (r.added.length) html += '<h4 style="margin-top:18px">Added</h4>' + chips(r.added);
    if (r.removed.length) html += '<h4 style="margin-top:18px">Removed</h4>' + chips(r.removed);
    return html;
  }

  function runDiff() {
    const out = $('diffResult'), f = $('diffFile').files[0];
    if (!f) { out.innerHTML = '<div class="finding-detail">Choose a file first.</div>'; return; }
    if (f.size > 8 * 1024 * 1024) { out.innerHTML = '<div class="finding-card blocker"><div class="finding-detail">That file is over the 8 MiB limit.</div></div>'; return; }
    const reader = new FileReader();
    reader.onload = async () => {
      try {
        const res = await post('/api/v1/diff?failOn=' + encodeURIComponent($('diffGate').value), String(reader.result));
        out.innerHTML = renderDiff(res);
      } catch (e) {
        out.innerHTML = `<div class="finding-card blocker"><div class="finding-title">Could not compare</div><div class="finding-detail">${esc(e.message)}</div></div>`;
      }
    };
    reader.onerror = () => { out.innerHTML = '<div class="finding-detail">Could not read the file.</div>'; };
    reader.readAsText(f);
  }

  function extras() {
    renderEstate();
    $('planValidate').addEventListener('click', runPlanCheck);
    $('planDraft').addEventListener('click', async () => {
      try { $('planInput').value = JSON.stringify(await get('/api/v1/plan/draft'), null, 2); }
      catch (e) { $('planResult').innerHTML = `<div class="finding-detail">${esc(e.message)}</div>`; }
    });
    $('diffRun').addEventListener('click', runDiff);
  }
  boot();
})();
