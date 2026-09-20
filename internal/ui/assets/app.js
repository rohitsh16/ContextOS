// ContextOS Dashboard Client Logic
(function() {
  'use strict';

  // Dual-mode static & live API resolver
  const isStaticMode = window.__STATIC_MODE__ === true ||
                       window.location.hostname.endsWith('github.io') ||
                       window.location.protocol === 'file:';

  const nativeFetch = window.fetch.bind(window);
  async function fetch(endpoint, options) {
    const isGet = !options || !options.method || options.method.toUpperCase() === 'GET';
    if (isStaticMode) {
      if (isGet) {
        const cleanName = String(endpoint).replace(/^\/api\//, '').split('?')[0];
        try {
          const staticRes = await nativeFetch(`./data/${cleanName}.json`);
          if (staticRes.ok) return staticRes;
        } catch (err) {
          console.warn(`Static data fallback failed for ${endpoint}:`, err);
        }
      } else {
        alert("GitHub Pages Static Preview: Modifications and plan generation require running ContextOS locally ('ctx ui').");
        return new Response(JSON.stringify({
          error: "Static Mode: Live mutations disabled in GitHub Pages preview."
        }), { status: 403, headers: { 'Content-Type': 'application/json' } });
      }
    }

    try {
      const res = await nativeFetch(endpoint, options);
      if (res.ok) return res;
      if (isGet) {
        const cleanName = String(endpoint).replace(/^\/api\//, '').split('?')[0];
        const staticFallback = await nativeFetch(`./data/${cleanName}.json`);
        if (staticFallback.ok) return staticFallback;
      }
      return res;
    } catch (netErr) {
      if (isGet) {
        const cleanName = String(endpoint).replace(/^\/api\//, '').split('?')[0];
        return nativeFetch(`./data/${cleanName}.json`);
      }
      throw netErr;
    }
  }

  let currentMemories = [];
  let currentFilter = 'all';

  // DOM Elements
  const tabButtons = document.querySelectorAll('.nav-tab');
  const tabViews = document.querySelectorAll('.tab-view');
  const pillButtons = document.querySelectorAll('.pill');
  const subtabContents = document.querySelectorAll('.subtab-content');

  const budgetSlider = document.getElementById('range-budget');
  const budgetLabel = document.getElementById('label-budget-val');
  const taskPrompt = document.getElementById('input-task-prompt');
  const targetModel = document.getElementById('select-target-model');
  const btnRunPlan = document.getElementById('btn-run-plan');
  const btnCopyPreview = document.getElementById('btn-copy-preview');
  const btnRefresh = document.getElementById('btn-refresh');

  const candidatesTbody = document.getElementById('candidates-tbody');
  const planRenderedPreview = document.getElementById('plan-rendered-preview');
  const candCount = document.getElementById('cand-count');
  const gaugeNumbers = document.getElementById('gauge-numbers');
  const gaugeFill = document.getElementById('gauge-fill');

  const modalAddMemory = document.getElementById('modal-add-memory');
  const btnOpenAddMemory = document.getElementById('btn-open-add-memory');
  const btnCloseModal = document.getElementById('btn-close-modal');
  const btnCancelMemory = document.getElementById('btn-cancel-memory');
  const btnSaveMemory = document.getElementById('btn-save-memory');

  const memKind = document.getElementById('mem-kind');
  const memContent = document.getElementById('mem-content');
  const memAuthority = document.getElementById('mem-authority');
  const memConfidence = document.getElementById('mem-confidence');

  const memorySearchInput = document.getElementById('memory-search-input');
  const memoryCardsContainer = document.getElementById('memory-cards-container');
  const filterBtns = document.querySelectorAll('.filter-btn');

  // Navigation Tabs
  tabButtons.forEach(btn => {
    btn.addEventListener('click', () => {
      const tab = btn.getAttribute('data-tab');
      tabButtons.forEach(b => b.classList.remove('active'));
      tabViews.forEach(v => v.classList.remove('active'));
      btn.classList.add('active');
      const targetView = document.getElementById('view-' + tab);
      if (targetView) targetView.classList.add('active');

      if (tab === 'memories') loadMemories();
      if (tab === 'sessions') loadSessions();
      if (tab === 'integrations') loadIntegrations();
    });
  });

  // Results Subtabs
  pillButtons.forEach(pill => {
    pill.addEventListener('click', () => {
      const subtab = pill.getAttribute('data-subtab');
      pillButtons.forEach(p => p.classList.remove('active'));
      subtabContents.forEach(c => c.classList.remove('active'));
      pill.classList.add('active');
      const targetContent = document.getElementById('subtab-' + subtab);
      if (targetContent) targetContent.classList.add('active');
    });
  });

  // Budget slider label
  budgetSlider.addEventListener('input', () => {
    const val = parseInt(budgetSlider.value, 10);
    budgetLabel.textContent = val.toLocaleString() + ' tok';
  });

  // Presets
  document.querySelectorAll('.chip[data-preset]').forEach(chip => {
    chip.addEventListener('click', () => {
      taskPrompt.value = chip.getAttribute('data-preset');
      runAllocatorPlan();
    });
  });

  // Copy Preview
  btnCopyPreview.addEventListener('click', () => {
    const text = planRenderedPreview.textContent;
    if (!text) return;
    navigator.clipboard.writeText(text).then(() => {
      const orig = btnCopyPreview.innerHTML;
      btnCopyPreview.innerHTML = '✓ Copied!';
      setTimeout(() => btnCopyPreview.innerHTML = orig, 2000);
    });
  });

  // Modal Open / Close
  btnOpenAddMemory.addEventListener('click', () => modalAddMemory.classList.add('open'));
  btnCloseModal.addEventListener('click', () => modalAddMemory.classList.remove('open'));
  btnCancelMemory.addEventListener('click', () => modalAddMemory.classList.remove('open'));
  modalAddMemory.addEventListener('click', (e) => {
    if (e.target === modalAddMemory) modalAddMemory.classList.remove('open');
  });

  // Save Memory
  btnSaveMemory.addEventListener('click', async () => {
    const content = memContent.value.trim();
    if (!content) {
      alert('Content is required');
      return;
    }
    const payload = {
      kind: memKind.value,
      content: content,
      authority: memAuthority.value,
      confidence: parseFloat(memConfidence.value) || 1.0,
      scope: 'repo'
    };

    try {
      const res = await fetch('/api/memories', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      if (!res.ok) throw new Error(await res.text());
      memContent.value = '';
      modalAddMemory.classList.remove('open');
      await loadStatus();
      await loadMemories();
    } catch (err) {
      alert('Error saving memory: ' + err.message);
    }
  });

  // Run Allocator Plan
  async function runAllocatorPlan() {
    const task = taskPrompt.value.trim() || 'General software engineering context';
    const model = targetModel.value;
    const budget = parseInt(budgetSlider.value, 10);

    btnRunPlan.disabled = true;
    btnRunPlan.innerHTML = '<span class="status-dot pulse"></span> Computing 6-Pass ASC-1 Allocation...';

    try {
      const res = await fetch('/api/plan', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ task, model, budget })
      });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      renderPlanResults(data, budget);
    } catch (err) {
      alert('Failed to plan context: ' + err.message);
    } finally {
      btnRunPlan.disabled = false;
      btnRunPlan.innerHTML = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="5 3 19 12 5 21 5 3"/></svg> Assemble Minimum Sufficient Context';
    }
  }

  btnRunPlan.addEventListener('click', runAllocatorPlan);
  btnRefresh.addEventListener('click', () => {
    loadStatus();
    loadMemories();
    loadSessions();
    loadIntegrations();
  });

  // Report Modal
  const btnOpenReport = document.getElementById('btn-open-report');
  const modalReport = document.getElementById('modal-report');
  const btnCloseReport = document.getElementById('btn-close-report');
  const btnDoneReport = document.getElementById('btn-done-report');
  const reportPreview = document.getElementById('report-markdown-preview');
  const btnCopyReportMd = document.getElementById('btn-copy-report-md');
  const btnDownloadReportMd = document.getElementById('btn-download-report-md');
  const btnDownloadReportJson = document.getElementById('btn-download-report-json');

  let currentReportMarkdown = '';
  let currentReportData = null;

  async function openReportModal() {
    if (!modalReport) return;
    modalReport.classList.add('open');
    if (reportPreview) reportPreview.textContent = 'Generating latest benchmark report...';
    try {
      const res = await fetch('/api/report');
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      currentReportData = data.report;
      currentReportMarkdown = data.markdown;
      if (reportPreview) reportPreview.textContent = currentReportMarkdown;
    } catch (err) {
      if (reportPreview) reportPreview.textContent = 'Error generating report: ' + err.message;
    }
  }

  function closeReportModal() {
    if (modalReport) modalReport.classList.remove('open');
  }

  if (btnOpenReport) btnOpenReport.addEventListener('click', openReportModal);
  if (btnCloseReport) btnCloseReport.addEventListener('click', closeReportModal);
  if (btnDoneReport) btnDoneReport.addEventListener('click', closeReportModal);

  if (btnCopyReportMd) {
    btnCopyReportMd.addEventListener('click', async () => {
      if (!currentReportMarkdown) return;
      try {
        await navigator.clipboard.writeText(currentReportMarkdown);
        const originalText = btnCopyReportMd.innerHTML;
        btnCopyReportMd.innerHTML = '✓ Copied!';
        setTimeout(() => { btnCopyReportMd.innerHTML = originalText; }, 2000);
      } catch (e) {
        alert('Failed to copy: ' + e.message);
      }
    });
  }

  function downloadBlob(content, filename, type) {
    const blob = new Blob([content], { type });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }

  if (btnDownloadReportMd) {
    btnDownloadReportMd.addEventListener('click', () => {
      if (!currentReportMarkdown) return;
      downloadBlob(currentReportMarkdown, 'BENCHMARK_REPORT.md', 'text/markdown');
    });
  }

  if (btnDownloadReportJson) {
    btnDownloadReportJson.addEventListener('click', () => {
      if (!currentReportData) return;
      downloadBlob(JSON.stringify(currentReportData, null, 2), 'results.json', 'application/json');
    });
  }

  // Render Plan Results
  function renderPlanResults(data, budget) {
    const plan = data.plan || {};
    const selected = plan.selected || [];
    const rejected = plan.rejected || [];
    const selectedTokens = plan.selected_tokens || 0;
    const rendered = data.rendered || '/* No context rendered */';

    // Update gauge
    const pct = Math.min(100, Math.round((selectedTokens / budget) * 100));
    gaugeFill.style.width = pct + '%';
    gaugeNumbers.textContent = `${selectedTokens.toLocaleString()} / ${budget.toLocaleString()} tokens (${pct}%)`;

    // Render Preview
    planRenderedPreview.textContent = rendered;

    // Render Candidates Table
    candCount.textContent = selected.length;
    let rowsHtml = '';

    const allCandidates = [
      ...selected.map(c => ({ ...c, status: 'SELECTED' })),
      ...rejected.map(c => ({ ...c, status: 'REJECTED' }))
    ];

    if (allCandidates.length === 0) {
      candidatesTbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">No candidates matched the task objective</td></tr>';
      return;
    }

    allCandidates.forEach(cand => {
      const isSel = cand.status === 'SELECTED';
      const statusBadge = isSel
        ? '<span class="badge badge-success">SELECTED</span>'
        : '<span class="badge badge-muted">REJECTED</span>';

      const kindClass = getKindBadge(cand.kind);
      const title = escapeHtml(cand.title || cand.content || cand.id);
      const score = (cand.score || 0).toFixed(4);
      const density = (cand.density || 0).toFixed(4);
      const tokens = cand.tokens || 0;

      rowsHtml += `
        <tr>
          <td>${statusBadge}</td>
          <td><span class="badge ${kindClass}">${cand.kind || 'fact'}</span></td>
          <td class="text-truncate" style="max-width: 380px;" title="${title}">${title}</td>
          <td class="font-mono">${tokens}</td>
          <td class="font-mono">${score}</td>
          <td class="font-mono">${density}</td>
        </tr>
      `;
    });

    candidatesTbody.innerHTML = rowsHtml;
  }

  function getKindBadge(kind) {
    switch ((kind || '').toLowerCase()) {
      case 'decision': return 'badge-info';
      case 'failure': return 'badge-danger';
      case 'constraint': return 'badge-warning';
      case 'code': return 'badge-success';
      default: return 'badge-muted';
    }
  }

  function escapeHtml(str) {
    return (str || '').replace(/[&<>"']/g, m => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    })[m]);
  }

  // Load Status & Metrics
  async function loadStatus() {
    try {
      const res = await fetch('/api/status');
      if (!res.ok) return;
      const data = await res.json();

      const repo = data.repo || {};
      document.getElementById('stat-repo-branch').textContent = repo.branch || 'main';
      document.getElementById('stat-repo-commit').textContent = repo.revision ? repo.revision.substring(0, 7) : 'HEAD';

      const stg = data.storage || 'sqlite';
      if (isStaticMode) {
        document.getElementById('storage-engine-label').textContent = 'FileStore (Static Snapshot)';
      } else {
        document.getElementById('storage-engine-label').textContent = stg === 'file' ? 'FileStore (Pure-Go)' : 'SQLite (WAL)';
      }

      const stats = data.stats || {};
      const totalMemories = stats.memories || 0;
      document.getElementById('stat-total-memories').textContent = totalMemories;

      const decisions = stats.decisions || 0;
      const failures = stats.failures || 0;
      document.getElementById('stat-memories-breakdown').textContent = `${decisions} decisions · ${failures} failures`;

      const workItem = data.work_item;
      if (workItem && workItem.title) {
        document.getElementById('stat-work-item').textContent = workItem.title;
        document.getElementById('stat-work-item-time').textContent = 'Branch: ' + (workItem.branch || 'current');
      } else {
        document.getElementById('stat-work-item').textContent = 'None active';
        document.getElementById('stat-work-item-time').textContent = 'Ready for tasks';
      }

      // Simulated savings based on memory reuse and ASC-1 packing
      const baselineTokens = Math.max(12000, totalMemories * 650);
      const ascTokens = Math.min(3200, Math.round(baselineTokens * 0.28));
      const saved = Math.max(0, baselineTokens - ascTokens);
      const pctSavings = baselineTokens > 0 ? Math.round((saved / baselineTokens) * 100) : 73;

      document.getElementById('stat-token-savings').textContent = pctSavings + '%';
      document.getElementById('stat-tokens-saved').textContent = `~${(saved / 1000).toFixed(1)}k tokens saved`;
    } catch (e) {
      console.warn('Failed to load status:', e);
    }
  }

  // Load Memories
  async function loadMemories() {
    try {
      const res = await fetch('/api/memories');
      if (!res.ok) return;
      currentMemories = await res.json() || [];
      renderMemoryCards();
    } catch (e) {
      console.warn('Failed to load memories:', e);
    }
  }

  // Filter Buttons
  filterBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      filterBtns.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      currentFilter = btn.getAttribute('data-kind');
      renderMemoryCards();
    });
  });

  memorySearchInput.addEventListener('input', () => {
    renderMemoryCards();
  });

  function renderMemoryCards() {
    const q = (memorySearchInput.value || '').toLowerCase();
    const filtered = currentMemories.filter(m => {
      if (currentFilter !== 'all' && (m.kind || '').toLowerCase() !== currentFilter) return false;
      if (q && !(m.content || '').toLowerCase().includes(q) && !(m.id || '').toLowerCase().includes(q)) return false;
      return true;
    });

    if (filtered.length === 0) {
      memoryCardsContainer.innerHTML = '<div class="text-center text-muted" style="grid-column: 1/-1; padding: 40px;">No engineering memories found matching criteria</div>';
      return;
    }

    let html = '';
    filtered.forEach(m => {
      const kindClass = getKindBadge(m.kind);
      const content = escapeHtml(m.content);
      const authority = m.authority || 'user';
      const confidence = Math.round((m.confidence || 1.0) * 100);
      const reuse = m.reuse_count || 0;
      const isStale = m.stale;

      html += `
        <div class="memory-card">
          <div class="memory-card-header">
            <span class="badge ${kindClass}">${m.kind}</span>
            <div style="display: flex; gap: 6px; align-items: center;">
              <span class="badge badge-muted">${authority}</span>
              ${isStale ? '<span class="badge badge-danger">STALE</span>' : ''}
            </div>
          </div>
          <div class="memory-content">${content}</div>
          <div class="memory-card-footer">
            <span>Confidence: ${confidence}% · Reused: ${reuse}x</span>
            <button class="btn btn-secondary btn-xs btn-invalidate" data-id="${m.id}" title="Invalidate memory">Invalidate</button>
          </div>
        </div>
      `;
    });

    memoryCardsContainer.innerHTML = html;

    // Attach Invalidate handlers
    document.querySelectorAll('.btn-invalidate').forEach(btn => {
      btn.addEventListener('click', async () => {
        const id = btn.getAttribute('data-id');
        if (!confirm(`Invalidate memory ${id}?`)) return;
        try {
          const res = await fetch('/api/memories/invalidate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id })
          });
          if (!res.ok) throw new Error(await res.text());
          loadMemories();
          loadStatus();
        } catch (e) {
          alert('Failed to invalidate: ' + e.message);
        }
      });
    });
  }

  // Load Sessions & Events
  let currentSessionFilter = null;
  let showAllRepos = false;

  function resolveModelRates(modelName, modelProfiles) {
    const m = (modelName || '').toLowerCase();
    for (const p of (modelProfiles || [])) {
      if (p.name && m.includes(p.name.toLowerCase())) {
        return { input: p.input_per_m || 0, cached: p.cached_input_per_m || 0 };
      }
    }
    if (m.includes('local')) return { input: 0, cached: 0 };
    if (m.includes('claude')) return { input: 3.0, cached: 0.30 };
    if (m.includes('gemini')) return { input: 2.0, cached: 0.20 };
    if (m.includes('codex') || m.includes('gpt')) return { input: 1.75, cached: 0.175 };
    return { input: 1.5, cached: 0.15 };
  }

  async function loadSessions(filterSessionId = undefined) {
    if (filterSessionId !== undefined) {
      currentSessionFilter = filterSessionId;
    }
    try {
      let url = '/api/sessions';
      const params = [];
      if (showAllRepos) params.push('all=true');
      if (currentSessionFilter) params.push('session_id=' + encodeURIComponent(currentSessionFilter));
      if (params.length > 0) url += '?' + params.join('&');
      const res = await fetch(url);
      if (!res.ok) return;
      const data = await res.json();
      const sessions = data.sessions || [];
      const events = data.events || [];
      const modelProfiles = data.models || [];
      const telemetry = data.session_telemetry || {};

      document.getElementById('session-count').textContent = `${sessions.length} sessions`;
      document.getElementById('event-count').textContent = `${events.length} events`;

      renderSessionEfficiency(sessions, data.traces || [], currentSessionFilter, modelProfiles, telemetry);

      const sessCont = document.getElementById('sessions-container');
      if (sessions.length === 0) {
        sessCont.innerHTML = '<div class="text-center text-muted" style="padding: 24px 0;">No agent sessions recorded yet</div>';
      } else {
        sessCont.innerHTML = sessions.map(s => {
          const isSelected = s.id === currentSessionFilter;
          let sTraces = [];
          if (s.work_item) {
            sTraces = (data.traces || []).filter(t => t.work_item_id === s.work_item);
          } else {
            const targetRepoId = s.repo === "ContextOS" ? "1" : "2";
            sTraces = (data.traces || []).filter(t => String(t.repo_id) === String(targetRepoId));
          }

          let sBudget = 0, sSelected = 0, sCacheHits = 0, sActual = 0, sBaseline = 0;
          sTraces.forEach(t => {
            const b = t.budget || 0;
            const sel = t.selected_tokens || 0;
            sBudget += b;
            sSelected += sel;
            if (t.cache_hit) sCacheHits++;
            const rates = resolveModelRates(t.model, modelProfiles);
            const rate = t.cache_hit ? rates.cached : rates.input;
            sActual += (sel / 1e6) * rate;
            sBaseline += (b / 1e6) * rates.input;
          });
          const sSavedTok = Math.max(0, sBudget - sSelected);
          const sPct = sBudget > 0 ? ((sSavedTok / sBudget) * 100).toFixed(0) : "0";
          const sCacheRate = sTraces.length > 0 ? ((sCacheHits / sTraces.length) * 100).toFixed(0) : "0";
          const sCostSaved = Math.max(0, sBaseline - sActual);

          return `
          <div class="session-item ${isSelected ? "active" : ""}" data-id="${s.id}">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
              <div>
                <span class="badge badge-info">${escapeHtml(s.agent || "agent")}</span>
                ${s.repo ? `<span class="badge badge-muted" style="margin-left: 4px; font-size: 10px; color: var(--accent-emerald-light);">${escapeHtml(s.repo)}</span>` : ""}
                <span class="font-mono" style="margin-left: 8px; font-size: 12px; font-weight: 600;">${s.id}</span>
              </div>
              <span class="text-muted font-mono" style="font-size: 11px;">${s.started_at ? s.started_at.substring(0, 19).replace("T", " ") : ""}</span>
            </div>
            ${sTraces.length > 0 ? `
              <div style="margin: 6px 0; padding: 6px 10px; background: rgba(0,0,0,0.3); border: 1px solid rgba(255,255,255,0.06); border-radius: 6px; display: flex; justify-content: space-between; align-items: center; font-size: 11px;">
                <span style="color: var(--accent-emerald-light); font-weight: 600;">
                  🌱 ${sSavedTok.toLocaleString()} tok (${sPct}%)
                </span>
                <span style="color: #60a5fa; font-weight: 500;">
                  ⚡ ${sCacheRate}% KV Hit
                </span>
                <span style="color: var(--accent-amber); font-weight: 500;">
                  $${sActual.toFixed(4)} · +$${sCostSaved.toFixed(4)}
                </span>
              </div>
            ` : ""}
            <div style="font-size: 11px; color: var(--text-secondary); display: flex; justify-content: space-between; align-items: center;">
              <span>${isSelected ? '▶ <strong style="color: var(--accent-indigo-light);">Inspecting efficiency details</strong> (click to reset)' : 'Click to inspect efficiency'}</span>
              ${s.work_item ? `<span class="badge badge-muted">Item: ${s.work_item.substring(0, 8)}</span>` : ""}
            </div>
          </div>
        `;
        }).join("");

        sessCont.querySelectorAll('.session-item').forEach(el => {
          el.addEventListener('click', () => {
            const sid = el.getAttribute('data-id');
            if (currentSessionFilter === sid) {
              loadSessions(null);
            } else {
              loadSessions(sid);
            }
          });
        });
      }

      const evCont = document.getElementById('events-container');
      let filterHeader = '';
      if (currentSessionFilter) {
        filterHeader = `
          <div class="filter-bar">
            <span>Filter: Session <strong class="font-mono">${escapeHtml(currentSessionFilter)}</strong></span>
            <button class="btn btn-secondary btn-xs" id="btn-clear-session-filter">Show All Events</button>
          </div>
        `;
      }

      if (events.length === 0) {
        evCont.innerHTML = filterHeader + '<div class="text-center text-muted" style="padding: 24px 0;">No hook events recorded yet</div>';
      } else {
        const eventsHtml = events.map((e, idx) => {
          let parsed = null;
          try {
            parsed = JSON.parse(e.payload);
          } catch (_) {}

          let toolInfo = '';
          let summaryText = '';
          let badgeClass = 'badge-muted';
          if (e.event_type === 'PreInvocation') badgeClass = 'badge-info';
          if (e.event_type === 'PostToolUse') badgeClass = 'badge-success';
          if (e.event_type === 'SessionStart') badgeClass = 'badge-warning';

          if (parsed) {
            if (parsed.toolCall && parsed.toolCall.name) {
              toolInfo = `<span class="badge badge-warning" style="margin-left: 6px;">${escapeHtml(parsed.toolCall.name)}</span>`;
              if (parsed.toolCall.args) {
                const args = parsed.toolCall.args;
                if (args.toolAction) summaryText = args.toolAction;
                else if (args.toolSummary) summaryText = args.toolSummary;
                else if (args.CommandLine) summaryText = args.CommandLine;
                else if (args.TargetFile) summaryText = args.TargetFile;
                else if (args.AbsolutePath) summaryText = args.AbsolutePath;
              }
            } else if (parsed.modelName) {
              summaryText = `Model: ${parsed.modelName}`;
              if (parsed.stepIdx !== undefined) summaryText += ` • Step ${parsed.stepIdx}`;
            }
          }

          if (!summaryText) {
            summaryText = e.payload.length > 90 ? e.payload.substring(0, 90) + '...' : e.payload;
          }

          const formattedJson = parsed ? JSON.stringify(parsed, null, 2) : e.payload;

          return `
          <div class="event-item" data-idx="${idx}">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
              <div style="display: flex; align-items: center;">
                <span class="badge ${badgeClass}">${escapeHtml(e.event_type)}</span>
                ${toolInfo}
              </div>
              <div style="display: flex; align-items: center; gap: 8px;">
                <span class="text-muted font-mono" style="font-size: 11px;">${e.created_at ? e.created_at.substring(11, 19) : ''}</span>
                <span class="text-muted" style="font-size: 10px; cursor: pointer;">🔍 details</span>
              </div>
            </div>
            <div class="text-truncate text-secondary" style="font-size: 12px;">${escapeHtml(summaryText)}</div>
            <pre class="event-details-pre" id="event-pre-${idx}" style="display: none;">${escapeHtml(formattedJson)}</pre>
          </div>
        `;
        }).join('');

        evCont.innerHTML = filterHeader + eventsHtml;

        evCont.querySelectorAll('.event-item').forEach(el => {
          el.addEventListener('click', (ev) => {
            if (ev.target.tagName === 'BUTTON') return;
            const idx = el.getAttribute('data-idx');
            const pre = document.getElementById(`event-pre-${idx}`);
            if (pre) {
              pre.style.display = pre.style.display === 'none' ? 'block' : 'none';
            }
          });
        });
      }

      const clearBtn = document.getElementById('btn-clear-session-filter');
      if (clearBtn) {
        clearBtn.addEventListener('click', () => {
          loadSessions(null);
        });
      }
    } catch (e) {
      console.warn('Failed to load sessions:', e);
    }
  }

  const btnRefreshSessions = document.getElementById('btn-refresh-sessions');
  if (btnRefreshSessions) {
    btnRefreshSessions.addEventListener('click', () => {
      loadSessions();
    });
  }

  const btnScopeCurrent = document.getElementById('btn-scope-current');
  const btnScopeAll = document.getElementById('btn-scope-all');
  if (btnScopeCurrent && btnScopeAll) {
    btnScopeCurrent.addEventListener('click', () => {
      showAllRepos = false;
      currentSessionFilter = null;
      btnScopeCurrent.classList.add('active');
      btnScopeAll.classList.remove('active');
      loadSessions();
    });
    btnScopeAll.addEventListener('click', () => {
      showAllRepos = true;
      currentSessionFilter = null;
      btnScopeAll.classList.add('active');
      btnScopeCurrent.classList.remove('active');
      loadSessions();
    });
  }

  // Load Integrations Status
  async function loadIntegrations() {
    try {
      const res = await fetch('/api/integrations');
      if (!res.ok) return;
      const data = await res.json();

      for (const agent of ['antigravity', 'cursor', 'claude', 'codex', 'gemini']) {
        const badge = document.getElementById('badge-' + agent);
        if (!badge) continue;
        const info = data[agent];
        if (info && info.installed) {
          badge.className = 'badge badge-success int-status-badge';
          badge.textContent = 'Configured';
        } else {
          badge.className = 'badge badge-warning int-status-badge';
          badge.textContent = 'Needs Setup';
        }
      }
    } catch (e) {
      console.warn('Failed to load integrations:', e);
    }
  }

  // Agent Install Action
  document.querySelectorAll('.btn-install-agent').forEach(btn => {
    btn.addEventListener('click', async () => {
      const agent = btn.getAttribute('data-agent');
      btn.disabled = true;
      btn.textContent = 'Configuring...';
      try {
        const res = await fetch('/api/install', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ agent })
        });
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        alert(`Successfully configured ${agent} integration!\nGenerated files:\n` + (data.files || []).join('\n'));
        loadIntegrations();
      } catch (e) {
        alert('Failed to install ' + agent + ': ' + e.message);
      } finally {
        btn.disabled = false;
        btn.textContent = `Configure ${agent.charAt(0).toUpperCase() + agent.slice(1)}`;
      }
    });
  });


  // Render Session Efficiency & Token Savings with Model-Specific Actual Cost vs Cost Saved
  function renderSessionEfficiency(sessions, traces, selectedSessionId, modelProfiles, telemetry) {
    const titleEl = document.getElementById("analytics-title");
    const subtitleEl = document.getElementById("analytics-subtitle");
    const sessIdEl = document.getElementById("analytics-session-id");
    const actualCostEl = document.getElementById("metric-actual-cost");
    const actualDetailEl = document.getElementById("metric-actual-detail");
    const baselineCostEl = document.getElementById("metric-baseline-cost");
    const baselineDetailEl = document.getElementById("metric-baseline-detail");
    const costSavedEl = document.getElementById("metric-cost-saved");
    const costDetailEl = document.getElementById("metric-cost-detail");
    const savedEl = document.getElementById("metric-tokens-saved");
    const savedPctEl = document.getElementById("metric-tokens-pct");
    const rawReducEl = document.getElementById("metric-raw-reduction");
    const rawDetailEl = document.getElementById("metric-raw-detail");
    const cacheHitsEl = document.getElementById("metric-cache-hits");
    const cacheDetailEl = document.getElementById("metric-cache-detail");
    const tracesSecEl = document.getElementById("analytics-traces-section");
    const tracesListEl = document.getElementById("analytics-traces-list");
    const traceCountEl = document.getElementById("analytics-trace-count");

    if (!titleEl) return;

    let relevantTraces = [];
    if (selectedSessionId) {
      const sess = sessions.find(s => s.id === selectedSessionId);
      if (sess) {
        sessIdEl.textContent = selectedSessionId.substring(0, 12) + "...";
        if (sess.work_item) {
          relevantTraces = traces.filter(t => t.work_item_id === sess.work_item);
          titleEl.textContent = "Efficiency: WorkItem " + sess.work_item.substring(0, 8) + "...";
        } else {
          const targetRepoId = sess.repo === "ContextOS" ? "1" : "2";
          relevantTraces = traces.filter(t => String(t.repo_id) === String(targetRepoId));
          if (relevantTraces.length === 0) relevantTraces = traces;
          titleEl.textContent = "Session Efficiency (" + (sess.repo || "repo") + ")";
        }
        subtitleEl.textContent = "Agent: " + (sess.agent || "antigravity") + " • Started: " + (sess.started_at ? sess.started_at.substring(0, 19).replace("T", " ") : "");
      }
    } else {
      sessIdEl.textContent = "All Sessions";
      titleEl.textContent = "Aggregated Efficiency Gains";
      subtitleEl.textContent = "Context reduction, KV-cache reuse, and model-specific cost savings";
      relevantTraces = traces;
    }

    let totalBudget = 0;
    let totalSelected = 0;
    let cacheHits = 0;
    let totalActualCost = 0;
    let totalBaselineCost = 0;

    relevantTraces.forEach(t => {
      const b = t.budget || 0;
      const s = t.selected_tokens || 0;
      totalBudget += b;
      totalSelected += s;
      if (t.cache_hit) cacheHits++;

      const rates = resolveModelRates(t.model, modelProfiles);
      const activeRate = t.cache_hit ? rates.cached : rates.input;
      const traceActual = (s / 1e6) * activeRate;
      const traceBaseline = (b / 1e6) * rates.input;
      totalActualCost += traceActual;
      totalBaselineCost += traceBaseline;
    });

    const netSaved = Math.max(0, totalBudget - totalSelected);
    const savingsPct = totalBudget > 0 ? ((netSaved / totalBudget) * 100).toFixed(1) : "0.0";
    const rawDump = relevantTraces.length * 32000;
    const rawReduc = rawDump > 0 ? ((1.0 - (totalSelected / rawDump)) * 100).toFixed(1) : "0.0";
    const cacheRate = relevantTraces.length > 0 ? ((cacheHits / relevantTraces.length) * 100).toFixed(1) : "0.0";
    const netCostSaved = Math.max(0, totalBaselineCost - totalActualCost);
    const costSavingsPct = totalBaselineCost > 0 ? ((netCostSaved / totalBaselineCost) * 100).toFixed(1) : "0.0";

    if (actualCostEl) actualCostEl.textContent = "$" + totalActualCost.toFixed(4);
    if (actualDetailEl) actualDetailEl.textContent = totalSelected.toLocaleString() + " tok consumed";
    if (baselineCostEl) baselineCostEl.textContent = "$" + totalBaselineCost.toFixed(4);
    if (baselineDetailEl) baselineDetailEl.textContent = totalBudget.toLocaleString() + " tok baseline";
    if (costSavedEl) costSavedEl.textContent = "$" + netCostSaved.toFixed(4);
    if (costDetailEl) costDetailEl.textContent = costSavingsPct + "% budget cut";

    if (savedEl) savedEl.textContent = netSaved.toLocaleString();
    if (savedPctEl) savedPctEl.textContent = savingsPct + "% pruned";
    if (rawReducEl) rawReducEl.textContent = rawReduc + "%";
    if (rawDetailEl) rawDetailEl.textContent = "vs " + rawDump.toLocaleString() + " tok dump";
    if (cacheHitsEl) cacheHitsEl.textContent = cacheRate + "%";
    if (cacheDetailEl) cacheDetailEl.textContent = cacheHits + " of " + relevantTraces.length + " hits (discounted)";

    // Update Host Agent Telemetry banner
    if (telemetry) {
      const invocationsEl = document.getElementById("telemetry-invocations");
      const eventsEl = document.getElementById("telemetry-events-count");
      const badgesEl = document.getElementById("telemetry-models-badges");
      if (invocationsEl) invocationsEl.textContent = (telemetry.llm_invocations || 0).toLocaleString() + " LLM Invocations";
      if (eventsEl) eventsEl.textContent = (telemetry.total_events || 0).toLocaleString() + " events";
      if (badgesEl && telemetry.models) {
        badgesEl.innerHTML = Object.entries(telemetry.models).map(([m, cnt]) => `
          <span class="badge badge-info" style="font-size: 9px; padding: 1px 6px;">${escapeHtml(m)} (${cnt})</span>
        `).join("");
      }
    }

    if (relevantTraces.length > 0 && tracesSecEl && tracesListEl) {
      tracesSecEl.style.display = "block";
      if (traceCountEl) traceCountEl.textContent = relevantTraces.length + " traces";
      tracesListEl.innerHTML = relevantTraces.slice(0, 8).map(t => {
        const tSaved = Math.max(0, t.budget - t.selected_tokens);
        const tPct = t.budget > 0 ? ((tSaved / t.budget) * 100).toFixed(0) : "0";
        const rates = resolveModelRates(t.model, modelProfiles);
        const rate = t.cache_hit ? rates.cached : rates.input;
        const traceCost = (t.selected_tokens / 1e6) * rate;
        const traceSaved = (tSaved / 1e6) * rates.input;

        return `
          <div style="display: flex; justify-content: space-between; align-items: center; padding: 5px 0; border-bottom: 1px solid rgba(255,255,255,0.04);">
            <div style="max-width: 45%;">
              <div class="text-truncate font-mono text-secondary" style="font-size: 11px;">${escapeHtml(t.task)}</div>
              <span class="text-muted" style="font-size: 9px;">model: ${escapeHtml(t.model || "local")}</span>
            </div>
            <div style="display: flex; align-items: center; gap: 6px;">
              <span class="badge ${t.cache_hit ? "badge-success" : "badge-muted"}" style="font-size: 9px; padding: 1px 5px;">${t.cache_hit ? "KV Hit" : "Miss"}</span>
              <span class="font-mono text-muted" style="font-size: 10px;">${t.selected_tokens}/${t.budget} tok</span>
              <span class="font-mono" style="font-size: 10px; color: #60a5fa;">$${traceCost.toFixed(4)}</span>
              <span class="font-mono" style="font-size: 10px; color: var(--accent-emerald-light);">(+$${traceSaved.toFixed(4)})</span>
            </div>
          </div>
        `;
      }).join("");
    } else if (tracesSecEl) {
      tracesSecEl.style.display = "none";
    }
  }

  // Initial load
  loadStatus();
  loadMemories();
  loadSessions();
  loadIntegrations();
})();
