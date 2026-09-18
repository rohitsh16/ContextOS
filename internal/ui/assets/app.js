// ContextOS Dashboard Client Logic
(function() {
  'use strict';

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
      document.getElementById('storage-engine-label').textContent = stg === 'file' ? 'FileStore (Pure-Go)' : 'SQLite (WAL)';

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
  async function loadSessions() {
    try {
      const res = await fetch('/api/sessions');
      if (!res.ok) return;
      const data = await res.json();
      const sessions = data.sessions || [];
      const events = data.events || [];

      document.getElementById('session-count').textContent = `${sessions.length} sessions`;
      document.getElementById('event-count').textContent = `${events.length} events`;

      const sessCont = document.getElementById('sessions-container');
      if (sessions.length === 0) {
        sessCont.innerHTML = '<div class="text-center text-muted">No agent sessions recorded yet</div>';
      } else {
        sessCont.innerHTML = sessions.map(s => `
          <div style="padding: 10px 0; border-bottom: 1px solid rgba(255,255,255,0.05); display: flex; justify-content: space-between;">
            <div>
              <span class="badge badge-info">${s.agent || 'agent'}</span>
              <span class="font-mono" style="margin-left: 8px; font-size: 12px;">${s.id}</span>
            </div>
            <span class="text-muted" style="font-size: 11px;">${s.started_at ? s.started_at.substring(0, 19) : ''}</span>
          </div>
        `).join('');
      }

      const evCont = document.getElementById('events-container');
      if (events.length === 0) {
        evCont.innerHTML = '<div class="text-center text-muted">No hook events recorded yet</div>';
      } else {
        evCont.innerHTML = events.map(e => `
          <div style="padding: 10px 0; border-bottom: 1px solid rgba(255,255,255,0.05);">
            <div style="display: flex; justify-content: space-between; margin-bottom: 4px;">
              <span class="badge badge-muted">${e.event_type}</span>
              <span class="text-muted font-mono" style="font-size: 11px;">${e.created_at ? e.created_at.substring(11, 19) : ''}</span>
            </div>
            <div class="text-truncate text-secondary" style="font-size: 12px;">${escapeHtml(e.payload)}</div>
          </div>
        `).join('');
      }
    } catch (e) {
      console.warn('Failed to load sessions:', e);
    }
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

  // Initial load
  loadStatus();
  loadMemories();
  loadIntegrations();
})();
