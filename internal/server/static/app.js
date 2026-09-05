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
      renderSummary(); renderInventory(); renderGraph(); renderWaves(); bind();
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
  boot();
})();
