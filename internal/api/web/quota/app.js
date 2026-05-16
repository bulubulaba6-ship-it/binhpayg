// ─────────────────────────────────────────────────────────────────────────────
// AI API Giá Rẻ — Quota Dashboard  app.js  v7
// Features: key caching (5min), auto-refresh (30s), reasoning token cost,
//           null-safe DOM helpers, client-side credit computation.
// ─────────────────────────────────────────────────────────────────────────────

const app = {
  usageChartInstance: null,
  modelChartInstance: null,
  refreshTimer: null,
  CACHE_KEY: 'quota_dash_key',
  CACHE_TTL: 5 * 60 * 1000,        // 5 minutes
  REFRESH_INTERVAL: 30 * 1000,     // 30 seconds (matches usage aggregation cycle)

  // ── Null-safe DOM helpers ────────────────────────────────────────────────
  setText: (id, value) => {
    const el = document.getElementById(id);
    if (el) el.textContent = value;
  },
  setStyle: (id, prop, value) => {
    const el = document.getElementById(id);
    if (el) el.style[prop] = value;
  },

  // ── Virtual credit pricing ────────────────────────────────────────────────
  // Mirrors config.yaml model-pricing exactly (credits per 1M tokens).
  // Reasoning tokens are billed at output rate (industry standard).
  // Update here whenever you change model-pricing in config.yaml.
  pricing: {
    'claude-opus-4-7':  { input: 280.0, output: 560.0, cache: 28.0  },
    'claude-opus-4-6':  { input: 280.0, output: 560.0, cache: 28.0  },
    'claude-sonnet-4-6':{ input: 210.0, output: 420.0, cache: 21.0  },
    'claude-haiku-4-5': { input: 175.0, output: 350.0, cache: 17.5  },
  },

  // Compute virtual credits for a single session.
  // reasoningTok billed at output rate (same as Anthropic billing).
  computeCredits: (alias, inputTok, outputTok, cacheTok, reasoningTok) => {
    const p = app.pricing[alias] || { input: 0.3, output: 1.0, cache: 0.03 };
    return (
      (inputTok       * p.input  ) +
      (outputTok      * p.output ) +
      (cacheTok       * p.cache  ) +
      ((reasoningTok || 0) * p.output)   // reasoning billed at output rate
    ) / 1_000_000;
  },

  // ── Key Cache (sessionStorage + 5-min TTL) ───────────────────────────────
  saveKey: (key) => {
    try {
      sessionStorage.setItem(app.CACHE_KEY, JSON.stringify({ key, ts: Date.now() }));
    } catch (_) { /* storage unavailable */ }
  },

  loadKey: () => {
    try {
      const raw = sessionStorage.getItem(app.CACHE_KEY);
      if (!raw) return null;
      const { key, ts } = JSON.parse(raw);
      if (Date.now() - ts > app.CACHE_TTL) {
        sessionStorage.removeItem(app.CACHE_KEY);
        return null;
      }
      return key;
    } catch (_) { return null; }
  },

  clearKey: () => {
    try { sessionStorage.removeItem(app.CACHE_KEY); } catch (_) { }
  },

  // ── Helpers ──────────────────────────────────────────────────────────────
  formatNumber: (num) => (num || 0).toLocaleString('en-US'),

  formatDate: (dateStr) => {
    if (!dateStr || dateStr === '0001-01-01T00:00:00Z') return 'N/A';
    return new Date(dateStr).toLocaleString('en-US', {
      month: 'short', day: 'numeric',
      hour: '2-digit', minute: '2-digit', second: '2-digit'
    });
  },

  timeAgo: (date) => {
    const sec = Math.floor((Date.now() - date) / 1000);
    if (sec < 5) return 'just now';
    if (sec < 60) return sec + 's ago';
    const min = Math.floor(sec / 60);
    if (min < 60) return min + 'm ago';
    return Math.floor(min / 60) + 'h ago';
  },

  // ── Charts ───────────────────────────────────────────────────────────────
  initCharts: () => {
    Chart.defaults.font.family = '"Inter", sans-serif';
    Chart.defaults.color = '#6b7280';
  },

  // Raw model → Claude alias mapping (mirrors config.yaml oauth-model-alias)
  resolveAlias: (rawModel) => {
    if (!rawModel) return rawModel;
    const m = rawModel.toLowerCase();
    if (m === 'gemini-3.1-flash-lite-preview' || m === 'gemini-3-flash') return 'claude-opus-4-7';
    if (m === 'gemini-3-flash-preview')   return 'claude-opus-4-6';
    if (m === 'gemini-3.1-flash-lite')    return 'claude-sonnet-4-6';
    if (m === 'gemini-2.5-flash-lite' || m === 'gemini-2.5-flash') return 'claude-haiku-4-5';
    if (m.includes('gemini'))             return 'claude-sonnet-4-6';
    return rawModel;
  },

  renderCharts: (sessions, models) => {
    // 1. Usage Trend (30-day)
    const ctxUsage = document.getElementById('usageChart');
    if (ctxUsage) {
      if (app.usageChartInstance) { app.usageChartInstance.destroy(); app.usageChartInstance = null; }

      const days = Array.from({ length: 30 }, (_, i) => {
        const d = new Date();
        d.setDate(d.getDate() - (29 - i));
        return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
      });

      const dayCounts = {};
      if (sessions) {
        sessions.forEach(s => {
          const ts = s.UpdatedAt || s.StartedAt || s.Timestamp;
          if (ts) {
            const key = new Date(ts).toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
            dayCounts[key] = (dayCounts[key] || 0) + 1;
          }
        });
      }

      app.usageChartInstance = new Chart(ctxUsage, {
        type: 'line',
        data: {
          labels: days,
          datasets: [{
            label: 'Requests',
            data: days.map(d => dayCounts[d] || 0),
            borderColor: '#d97757',
            backgroundColor: 'rgba(217,119,87,0.08)',
            borderWidth: 2,
            fill: true,
            tension: 0.45,
            pointRadius: 2.5,
            pointHitRadius: 10,
            pointBackgroundColor: '#d97757'
          }]
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: { legend: { display: false } },
          scales: {
            x: { grid: { display: false }, ticks: { maxTicksLimit: 7, font: { size: 11 } } },
            y: { beginAtZero: true, border: { display: false }, ticks: { precision: 0, font: { size: 11 } } }
          }
        }
      });
    }

    // 2. Model Breakdown doughnut
    const ctxModel = document.getElementById('modelChart');
    if (ctxModel) {
      if (app.modelChartInstance) { app.modelChartInstance.destroy(); app.modelChartInstance = null; }

      const aliasData = {};
      Object.keys(models).forEach(k => {
        const alias = app.resolveAlias(k);
        aliasData[alias] = (aliasData[alias] || 0) + (models[k].total_requests || 0);
      });

      const chartLabels = Object.keys(aliasData);
      const chartValues = chartLabels.map(k => aliasData[k]);
      if (chartLabels.length === 0) { chartLabels.push('No Data'); chartValues.push(1); }

      app.modelChartInstance = new Chart(ctxModel, {
        type: 'doughnut',
        data: {
          labels: chartLabels,
          datasets: [{
            data: chartValues,
            backgroundColor: ['#d97757', '#3b82f6', '#10b981', '#f59e0b', '#6366f1'],
            borderWidth: 0,
            hoverOffset: 6
          }]
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          cutout: '68%',
          plugins: {
            legend: { position: 'bottom', labels: { boxWidth: 10, padding: 14, font: { size: 12 } } }
          }
        }
      });
    }
  },

  // ── Ledger ───────────────────────────────────────────────────────────────
  renderLedger: (sessions, usageModels) => {
    const tbody = document.getElementById('historyBody');
    if (!tbody) return;
    tbody.innerHTML = '';

    let rows = [];

    if (sessions && sessions.length > 0) {
      // Postgres-backed rows (full persistent history)
      rows = sessions.map(s => {
        const alias = app.resolveAlias(s.Model || '');
        const inTok  = s.InputTokens     || 0;
        const outTok = s.OutputTokens    || 0;
        const caTok  = s.CachedTokens    || 0;
        const reTok  = s.ReasoningTokens || 0;
        return {
          id: s.SessionID || '-',
          model: alias,
          inputTokens: inTok, outputTokens: outTok, cachedTokens: caTok, reasoningTokens: reTok,
          timestamp: s.UpdatedAt || s.StartedAt,
          credits: app.computeCredits(alias, inTok, outTok, caTok, reTok)
        };
      });
    } else if (usageModels && Object.keys(usageModels).length > 0) {
      // In-memory fallback: flatten model details into rows
      Object.keys(usageModels).forEach(rawModel => {
        const alias = app.resolveAlias(rawModel);
        (usageModels[rawModel].details || []).forEach(d => {
          const t = d.tokens || {};
          const inTok  = t.input_tokens     || 0;
          const outTok = t.output_tokens    || 0;
          const caTok  = t.cached_tokens    || 0;
          const reTok  = t.reasoning_tokens || 0;
          rows.push({
            id: (d.auth_index || 'session').substring(0, 8),
            model: alias,
            inputTokens: inTok, outputTokens: outTok, cachedTokens: caTok, reasoningTokens: reTok,
            timestamp: d.timestamp,
            credits: app.computeCredits(alias, inTok, outTok, caTok, reTok)
          });
        });
      });
      rows.sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));
    }

    if (rows.length === 0) {
      tbody.innerHTML = '<tr><td colspan="5" style="text-align:center;color:var(--text-muted);padding:40px;">No recent session activity</td></tr>';
      return;
    }

    rows.forEach(r => {
      const tr = document.createElement('tr');
      const shortId = r.id.length > 12 ? r.id.substring(0, 8) + '…' : r.id;
      const reTokPart = r.reasoningTokens > 0 ? ` / ${r.reasoningTokens}` : '';
      tr.innerHTML = `
        <td class="mono" title="${r.id}">${shortId}</td>
        <td>${r.model}</td>
        <td style="color:var(--text-muted);">${r.inputTokens} / ${r.outputTokens} / ${r.cachedTokens}${reTokPart}</td>
        <td>${app.formatDate(r.timestamp)}</td>
        <td class="text-right"><span class="badge-cost">${r.credits.toFixed(5)} CR</span></td>
      `;
      tbody.appendChild(tr);
    });
  },

  // ── Auto-refresh ─────────────────────────────────────────────────────────
  startAutoRefresh: (key) => {
    app.stopAutoRefresh();
    app.refreshTimer = setInterval(() => {
      app.fetchData(key, false); // silent background update
    }, app.REFRESH_INTERVAL);
  },

  stopAutoRefresh: () => {
    if (app.refreshTimer) { clearInterval(app.refreshTimer); app.refreshTimer = null; }
  },

  updateLastUpdated: () => {
    const now = new Date();
    const strip = document.getElementById('refreshStrip');
    const badge = document.getElementById('lastUpdatedBadge');
    const full  = document.getElementById('lastUpdatedFull');
    const text  = document.getElementById('lastUpdatedText');
    if (strip) strip.classList.add('visible');
    if (badge) badge.style.display = 'flex';

    const tick = () => {
      const ago = app.timeAgo(now);
      if (full)  full.textContent = ago;
      if (text)  text.textContent = ago;
    };
    tick();
    // Update "X ago" label every 15s
    clearInterval(app._agoTimer);
    app._agoTimer = setInterval(tick, 15000);
  },

  // ── Core fetch ───────────────────────────────────────────────────────────
  fetchData: async (key, showErrors = true) => {
    try {
      const res = await fetch('/v1/billing/quota?key=' + encodeURIComponent(key), {
        headers: { 'Authorization': 'Bearer ' + key }
      });
      if (!res.ok) throw new Error('Authentication failed (' + res.status + ')');
      const data = await res.json();

      // Show dashboard, hide overlay
      document.getElementById('welcomeState').style.display = 'none';
      document.getElementById('dashboard').style.display = 'block';

      // Stat cards
      app.setText('valTotalReq',   app.formatNumber(data.usage.success_requests));
      app.setText('valSuccessReq', app.formatNumber(data.usage.success_requests));
      const totalTokens = data.quota.total_tokens || data.usage.total_tokens || 0;
      app.setText('valTokens',  app.formatNumber(totalTokens));
      app.setText('valRPM',     (data.usage.rpm || 0).toFixed(1));
      
      // Store for toggle
      app.lastQuotaData = data.quota;
      app.updateCreditDisplay();

      // Reset timer
      const exp = data.quota.window_expires_at;
      if (exp && exp !== '0001-01-01T00:00:00Z') {
        const diffMs = new Date(exp) - new Date();
        if (diffMs > 0) {
          const h = Math.floor(diffMs / 3600000);
          const m = Math.floor((diffMs % 3600000) / 60000);
          app.setText('valReset', 'Resets in ' + h + 'h ' + m + 'm');
        } else {
          app.setText('valReset', 'Resetting shortly…');
        }
      } else {
        app.setText('valReset', 'Cumulative session limit');
      }

      // Charts + ledger
      app.renderCharts(data.quota.recent_sessions, data.usage.models);
      app.renderLedger(data.quota.recent_sessions, data.usage.models);

      // Last-updated indicator
      app.updateLastUpdated();

      // Cache & URL sanitise
      app.saveKey(key);
      const url = new URL(window.location);
      url.searchParams.delete('key');
      window.history.replaceState({}, '', url);

      // Sync visible input in navbar
      const navInput = document.getElementById('apiKeyInput');
      if (navInput && !navInput.value) navInput.value = key;

    } catch (err) {
      if (showErrors) alert('Error: ' + err.message);
      else console.warn('[quota-dash] silent refresh failed:', err.message);
    }
  },
  pollTimer: null,
  lastQuotaData: null,
  
  // ── Pricing definitions ───────────────────────────────────────────────────
  fetchQuota: () => {
    const input = document.getElementById('apiKeyInput');
    const key = (input && input.value.trim()) || new URLSearchParams(window.location.search).get('key');
    if (!key) return alert('Please enter a Workspace API Key');
    app.saveKey(key);
    app.fetchData(key, true);
    app.startAutoRefresh(key);
  },

  authFromOverlay: () => {
    const input = document.getElementById('welcomeApiKeyInput');
    const key = (input && input.value.trim()) || new URLSearchParams(window.location.search).get('key');
    if (!key) return alert('Please enter a Workspace API Key');
    // Copy to navbar input so user can re-auth from there later
    const navInput = document.getElementById('apiKeyInput');
    if (navInput) navInput.value = key;
    app.saveKey(key);
    app.fetchData(key, true);
    app.startAutoRefresh(key);
  },

  updateCreditDisplay: () => {
    if (!app.lastQuotaData) return;
    const quota = app.lastQuotaData;
    const mode = document.getElementById('creditToggle') ? document.getElementById('creditToggle').value : '5h';
    const val = mode === '5h' ? (quota.credits_used || 0) : (quota.total_credits_used || 0);
    
    app.setText('valCredits', val.toFixed(5));
    const limit = quota.credit_limit;
    if (!limit || limit === -1 || limit === 0) {
      app.setText('valLimit', 'Unlimited');
      app.setStyle('creditProgress', 'width', '0%');
    } else {
      app.setText('valLimit', app.formatNumber(limit));
      let pct = Math.min(100, (val / limit) * 100);
      app.setStyle('creditProgress', 'width', pct + '%');
      app.setStyle('creditProgress', 'backgroundColor', pct >= 90 ? 'var(--danger)' : 'var(--accent-color)');
    }
  }
};

// ── Bootstrap ─────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', () => {
  app.initCharts();
  const authBtn = document.getElementById('btnAuth');
  if (authBtn) {
    authBtn.addEventListener('click', app.handleAuth);
  }
  const pwd = document.getElementById('inpPwd');
  if (pwd) {
    pwd.addEventListener('keydown', e => { if (e.key === 'Enter') app.handleAuth(); });
  }
  const toggle = document.getElementById('creditToggle');
  if (toggle) {
    toggle.addEventListener('change', app.updateCreditDisplay);
  }

  // Enter key on navbar input
  const navInput = document.getElementById('apiKeyInput');
  if (navInput) {
    navInput.addEventListener('keypress', (e) => {
      if (e.key === 'Enter') app.fetchQuota();
    });
  }

  // Check URL key first
  const urlKey = new URLSearchParams(window.location.search).get('key');
  if (urlKey) {
    app.authFromOverlay();
    return;
  }

  // Check cached key (5-min TTL)
  const cachedKey = app.loadKey();
  if (cachedKey) {
    const navInput2 = document.getElementById('apiKeyInput');
    if (navInput2) navInput2.value = cachedKey;
    app.fetchData(cachedKey, false);
    app.startAutoRefresh(cachedKey);
    return;
  }

  // No key → show overlay
  document.getElementById('welcomeState').style.display = 'flex';
  document.getElementById('dashboard').style.display = 'none';
});
