// ─────────────────────────────────────────────────────────────────────────────
// FinkRouter — Quota Dashboard  app.js  v7
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

  // ── Ledger pagination state ──────────────────────────────────────────────
  _ledgerRows: [],
  _ledgerPage: 0,
  _ledgerPageSize: 20,
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
  // Mirrors config.yaml markup-rates exactly (credits per 1M tokens).
  // 1000 credits = $1.00 USD. Update here whenever config.yaml changes.
  // Reasoning tokens are billed at output rate (industry standard).
  // Cache = 10% of input rate (industry standard, Anthropic ratio).
  pricing: {
    // DeepSeek-V4 Flash backend: upstream $0.14/$0.28/1M → 10x
    'claude-opus-4-8':    { input: 1400, output: 2800, cache: 140 },
    'claude-opus-4-7':    { input: 1400, output: 2800, cache: 140 },
    'claude-opus-4-6':    { input: 1400, output: 2800, cache: 140 },
    'claude-sonnet-5':    { input: 1600, output: 3000, cache: 140 },
    // Mimo-v2.5 backend
    'claude-sonnet-4-6':  { input: 1000, output: 2000, cache: 100 },
    'claude-haiku-4-5':   { input:  800, output: 1600, cache:  80 },
    'claude-fable-5':     { input: 3000, output: 6000, cache: 300 },
    // GPT aliases — same backends as Claude counterparts
    'gpt-5.6-sol':        { input: 3000, output: 6000, cache: 300 },
    'gpt-5.6-luna':       { input:  800, output: 1600, cache:  80 },
    'gpt-5.5':            { input: 1400, output: 2800, cache: 140 },
    'gpt-5.4':            { input: 1400, output: 2800, cache: 140 },
    'gpt-5.4-mini':       { input:  800, output: 2000, cache:  80 },
    'gpt-5.3-codex-spark':{ input:  700, output: 2100, cache:  70 },
    'gpt-5.3-codex':      { input:  700, output: 2100, cache:  70 },
  },

  // Compute virtual credits for a single session.
  // Mirrors client_quota.go exactly:
  //   billableInput = inputTok - cacheTok  (cached tokens are re-billed at cache rate, not input rate)
  //   reasoningTok billed at output rate (Anthropic standard)
  computeCredits: (alias, inputTok, outputTok, cacheTok, reasoningTok) => {
    const p = app.pricing[alias] || { input: 0.3, output: 1.0, cache: 0.03 };
    const billableInput = Math.max(0, inputTok - cacheTok); // subtract cached from raw input
    return (
      (billableInput              * p.input  ) +
      (outputTok                  * p.output ) +
      (cacheTok                   * p.cache  ) +
      ((reasoningTok || 0)        * p.output )  // reasoning billed at output rate
    ) / 1_000_000;
  },

  // ── Plan Badge ───────────────────────────────────────────────────────────
  // Determine plan tier from key prefix + 5h rate limit.
  determinePlan: (key, rateLimit5h) => {
    const k = (key || '').toLowerCase();
    if (k.includes('fink_max_')) {
      return rateLimit5h >= 20000 ? 'MAX x20' : 'MAX x5';
    }
    if (k.includes('fink_pro_')) return 'PRO';
    if (k.startsWith('fink_')) return 'PAYG';
    return 'PRO';
  },

  updatePlanBadge: (key, rateLimit5h) => {
    const badge = document.getElementById('planBadge');
    if (!badge) return;
    const plan = app.determinePlan(key, rateLimit5h);
    badge.textContent = plan;
    if (plan.startsWith('MAX')) {
      badge.style.background = 'linear-gradient(135deg, #f59e0b, #d97706)';
      badge.style.color = '#000';
    } else if (plan === 'PRO') {
      badge.style.background = 'var(--primary)';
      badge.style.color = '#000';
    } else {
      // PAYG
      badge.style.background = '#6366f1';
      badge.style.color = '#fff';
    }
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
    const isLight = document.documentElement.getAttribute('data-theme') === 'light';
    Chart.defaults.font.family = '"Be Vietnam Pro", system-ui, sans-serif';
    Chart.defaults.font.weight = '600';
    Chart.defaults.color = isLight ? '#62677e' : '#8e8b82';
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

  renderCharts: (usage, sessions, models) => {
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
      if (usage && usage.daily_requests) {
        Object.keys(usage.daily_requests).forEach(d => {
          const parsedDate = new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
          dayCounts[parsedDate] = (dayCounts[parsedDate] || 0) + usage.daily_requests[d];
        });
      } else if (sessions) {
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
            borderColor: '#ff7900', // Vibrant Orange
            backgroundColor: 'rgba(255,121,0,0.12)',
            borderWidth: 2.5,
            fill: true,
            tension: 0.45,
            pointRadius: 3,
            pointHitRadius: 10,
            pointBackgroundColor: '#ff7900'
          }]
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: { legend: { display: false } },
          scales: {
            x: { grid: { display: false, color: 'rgba(255,255,255,0.05)' }, ticks: { maxTicksLimit: 7, font: { size: 11 } } },
            y: { beginAtZero: true, border: { display: false }, grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { precision: 0, font: { size: 11 } } }
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
            backgroundColor: ['#ff7900', '#3b82f6', '#10b981', '#f59e0b', '#6366f1'],
            borderWidth: 0,
            hoverOffset: 6
          }]
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          cutout: '68%',
          plugins: {
            legend: { position: 'bottom', labels: { boxWidth: 10, padding: 14, font: { size: 12 } } },
            tooltip: {
              callbacks: {
                label: (ctx) => {
                  const label = ctx.label || '';
                  const count = ctx.raw || 0;
                  return ` ${label}: ${app.formatNumber(count)}`;
                }
              }
            }
          }
        }
      });
    }
  },

  // ── Ledger ───────────────────────────────────────────────────────────────
  renderLedger: (sessions, usageModels) => {
    let rows = [];
    if (sessions && sessions.length > 0) {
      rows = sessions.map(s => {
        const alias = app.resolveAlias(s.Model || '');
        const inTok  = s.InputTokens     || 0;
        const outTok = s.OutputTokens    || 0;
        const caTok  = s.CachedTokens    || 0;
        const reTok  = s.ReasoningTokens || 0;
        // Prefer server-billed amount (CreditsConsumed stored at billing time).
        // Fall back to JS estimate for legacy sessions recorded before this field existed.
        const serverBilled = s.CreditsConsumed || 0;
        const estimated    = serverBilled <= 0;
        const credits      = estimated
          ? app.computeCredits(alias, inTok, outTok, caTok, reTok)
          : serverBilled;
        return {
          id: s.SessionID || '-',
          model: alias,
          inputTokens: inTok, outputTokens: outTok, cachedTokens: caTok, reasoningTokens: reTok,
          timestamp: s.UpdatedAt || s.StartedAt,
          credits,
          estimated,  // true = JS-computed (old session), false = server-billed (accurate)
        };
      });
    } else if (usageModels && Object.keys(usageModels).length > 0) {
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
            credits: app.computeCredits(alias, inTok, outTok, caTok, reTok),
            estimated: true,
          });
        });
      });
      rows.sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));
    }
    // Store all rows, reset to page 0
    app._ledgerRows = rows;
    app._ledgerPage = 0;
    app._renderLedgerPage();
  },

  _renderLedgerPage: () => {
    const tbody = document.getElementById('historyBody');
    if (!tbody) return;
    tbody.innerHTML = '';
    const rows = app._ledgerRows;
    const pageSize = app._ledgerPageSize;
    const page = app._ledgerPage;
    const totalPages = Math.max(1, Math.ceil(rows.length / pageSize));

    // Clamp page
    if (page >= totalPages) app._ledgerPage = totalPages - 1;
    if (app._ledgerPage < 0) app._ledgerPage = 0;

    const start = app._ledgerPage * pageSize;
    const pageRows = rows.slice(start, start + pageSize);

    if (pageRows.length === 0) {
      tbody.innerHTML = '<tr><td colspan="5" style="text-align:center;color:var(--text-muted);padding:40px;">No recent session activity</td></tr>';
    } else {
      pageRows.forEach(r => {
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
    }



    // Update pagination controls
    const info = document.getElementById('ledgerPageInfo');
    const prev = document.getElementById('ledgerPrev');
    const next = document.getElementById('ledgerNext');
    if (info) info.textContent = `Page ${app._ledgerPage + 1} of ${totalPages} · showing ${rows.length} most recent sessions`;
    if (prev) prev.disabled = app._ledgerPage === 0;
    if (next) next.disabled = app._ledgerPage >= totalPages - 1;
    const pg = document.getElementById('ledgerPagination');
    if (pg) pg.style.display = rows.length > pageSize ? 'flex' : 'none';
  },

  ledgerPrev: () => { app._ledgerPage--; app._renderLedgerPage(); },
  ledgerNext: () => { app._ledgerPage++; app._renderLedgerPage(); },

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
      document.getElementById('overviewTab').style.display = 'block';
      const navTabs = document.getElementById('navTabs');
      if (navTabs) navTabs.style.display = 'flex';

      // Stat cards
      app.setText('valTotalReq',   app.formatNumber(data.usage.success_requests));
      app.setText('valSuccessReq', app.formatNumber(data.usage.success_requests));
      const totalTokens = data.quota.total_tokens || data.usage.total_tokens || 0;
      app.setText('valTokens',  app.formatNumber(totalTokens));
      app.setText('valRPM',     (data.usage.rpm || 0).toFixed(1));
      
      // Store for toggle and plan detection
      app.currentKey = key;
      app.lastQuotaData = data.quota;
      app.lastBurnMultiplier  = data.burn_multiplier  || 1.0;
      app.lastBurnRatio       = data.burn_ratio        || 0.0;
      app.lastFiveHCredits    = data.five_h_credits    || 0;
      app.lastDailyBurnToday  = data.daily_burn_today  || 0;
      app.updateCreditDisplay();
      app.updatePlanBadge(key, (data.quota && data.quota.rate_limit_5h) || 0);
      app.renderBurnIndicator(app.lastBurnMultiplier, app.lastBurnRatio, app.lastFiveHCredits, data.quota.rate_limit_5h || 0);

      // Reset timer
      const exp = data.quota.window_expires_at;
      const currentMode = document.getElementById('creditToggle') ? document.getElementById('creditToggle').value : '5h';
      
      if (currentMode === '5h') {
        if (exp && exp !== '0001-01-01T00:00:00Z') {
          const diffMs = new Date(exp) - new Date();
          if (diffMs > 0) {
            const h = Math.floor(diffMs / 3600000);
            const m = Math.floor((diffMs % 3600000) / 60000);
            app.setText('valResetText', 'Resets in ' + h + 'h ' + m + 'm');
          } else {
            app.setText('valResetText', 'Resetting shortly…');
          }
        } else {
          app.setText('valResetText', 'Cumulative session limit');
        }
      }

      // Charts + ledger
      app.renderCharts(data.usage, data.quota.recent_sessions, data.usage.models);
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
    
    const limitSpan = document.getElementById('limitSpan');
    const tierBadge = document.getElementById('tierBadge');
    const valResetText = document.getElementById('valResetText');

    if (mode === '5h') {
      // ── 5H Window mode ──────────────────────────────────────────────────
      const val   = quota.credits_used || 0;
      const limit = quota.rate_limit_5h || 0;
      app.setText('valCredits', val.toFixed(5));

      if (!limit || limit === -1) {
        // Unlimited
        if (limitSpan) limitSpan.style.display = 'none';
        if (tierBadge) tierBadge.style.display = 'none';
        app.setStyle('creditProgress', 'width', '0%');
      } else {
        if (limitSpan) limitSpan.style.display = 'inline';
        app.setText('valLimit', app.formatNumber(limit));
        const remaining5h = Math.max(0, limit - val);
        const pct = Math.min(100, (val / limit) * 100);
        if (tierBadge) {
          tierBadge.style.display = 'inline-block';
          tierBadge.innerHTML = `${app.formatNumber(Math.floor(remaining5h))} cr left`;
          tierBadge.title    = `Credits remaining in this 5h window (limit: ${app.formatNumber(limit)} cr)`;
          tierBadge.style.background = pct >= 90 ? 'var(--error)' : 'rgba(255,255,255,0.10)';
          tierBadge.style.color      = '#fff';
          tierBadge.style.border     = '1px solid rgba(255,255,255,0.15)';
        }
        app.setStyle('creditProgress', 'width', pct + '%');
        app.setStyle('creditProgress', 'backgroundColor', pct >= 90 ? 'var(--error)' : 'var(--primary)');
      }
      if (valResetText) valResetText.textContent = 'Resets in 5h window';

    } else {
      // ── All-Time mode ────────────────────────────────────────────────────
      const val       = quota.total_credits_used || 0;
      const purchased = quota.credits_purchased   || 0;
      const ceiling   = (quota.credit_limit       || 0) + purchased; // hard cap
      const remaining = Math.max(0, ceiling - val);
      app.setText('valCredits', val.toFixed(5));

      if (limitSpan) limitSpan.style.display = 'none';
      if (valResetText) valResetText.textContent = 'Cumulative all-time usage';

      if (tierBadge) {
        tierBadge.style.display    = 'inline-block';
        tierBadge.innerHTML        = `${app.formatNumber(Math.floor(remaining))} cr left`;
        tierBadge.title            = `Remaining credits — ceiling: ${app.formatNumber(Math.floor(ceiling))} cr`;
        tierBadge.style.color      = '#fff';
        tierBadge.style.border     = '1px solid rgba(255,255,255,0.15)';
        if (remaining < 10000) {
          tierBadge.style.background = 'var(--error)';     // critical: < 10K
        } else if (remaining < 50000) {
          tierBadge.style.background = 'rgba(245,158,11,0.75)'; // warning: < 50K
        } else {
          tierBadge.style.background = 'rgba(255,255,255,0.10)'; // healthy
        }
      }

      if (ceiling > 0) {
        const pct = Math.min(100, (val / ceiling) * 100);
        app.setStyle('creditProgress', 'width', pct + '%');
        app.setStyle('creditProgress', 'backgroundColor', pct >= 90 ? 'var(--error)' : 'var(--primary)');
      } else {
        app.setStyle('creditProgress', 'width', '0%');
      }
    }
  },

  // ── Burn-rate indicator ──────────────────────────────────────────────────
  // Renders a contextual warning card inside the Credits stat card showing
  // the current 5H rolling window burst multiplier and usage ratio.
  // Tiers: 0-20%→x1.0 | 20-40%→x1.2 | 40-60%→x1.4 | 60-80%→x1.7 | ≥80%→x1.8-2.0
  // Hidden entirely when multiplier = 1.0 and ratio < 20% (fresh/idle window).
  renderBurnIndicator: (multiplier, burnRatio, fiveHCredits, rateLimit) => {
    const card = document.querySelector('.stat-card');
    if (!card) return;

    // Remove any existing indicator
    const old = document.getElementById('burnIndicator');
    if (old) old.remove();

    // No indicator needed when at base rate with minimal window consumption
    if ((!multiplier || multiplier <= 1.0) && (!burnRatio || burnRatio < 0.20)) return;

    // Determine tier label, color, and description
    const ratio = burnRatio || 0;
    const pct   = Math.min(100, Math.round(ratio * 100));
    const mult  = multiplier || 1.0;

    let color, tierLabel, tierDesc;
    if (mult >= 1.8) {
      color     = '#f87171'; // red
      tierLabel = `×${mult.toFixed(1)} — Ceiling`;
      tierDesc  = 'Randomised ceiling active (1.8–2.0×). At ≥80% of 5H limit.';
    } else if (mult >= 1.7) {
      color     = '#fb923c'; // orange
      tierLabel = `×1.7 — Severe burst`;
      tierDesc  = 'Severe burst: 60–80% of 5H window consumed.';
    } else if (mult >= 1.4) {
      color     = '#f59e0b'; // amber
      tierLabel = `×1.4 — Heavy burst`;
      tierDesc  = 'Heavy burst: 40–60% of 5H window consumed.';
    } else if (mult >= 1.2) {
      color     = '#fbbf24'; // yellow
      tierLabel = `×1.2 — Moderate burst`;
      tierDesc  = 'Moderate burst: 20–40% of 5H window consumed.';
    } else {
      color     = '#a3e635'; // lime — informational, approaching first tier
      tierLabel = `×1.0 — Normal`;
      tierDesc  = 'Window active. Cost multiplier will rise as window fills.';
    }

    const fiveHK    = fiveHCredits >= 1000 ? (fiveHCredits / 1000).toFixed(1) + 'K' : (fiveHCredits || 0).toFixed(0);
    const limitK    = rateLimit   >= 1000 ? (rateLimit   / 1000).toFixed(0) + 'K' : (rateLimit   || 0).toString();
    const barColor  = mult >= 1.7 ? '#f87171' : mult >= 1.4 ? '#fb923c' : mult >= 1.2 ? '#f59e0b' : '#a3e635';

    const wrap = document.createElement('div');
    wrap.id = 'burnIndicator';
    wrap.title = tierDesc + ` (${fiveHK} / ${limitK} cr in current 5H window)`;
    wrap.style.cssText = [
      'margin-top:8px', 'padding:8px 10px',
      'background:rgba(0,0,0,0.18)', 'border-radius:10px',
      `border:1px solid ${color}33`,
      'font-size:0.73rem', 'cursor:help'
    ].join(';');

    wrap.innerHTML = `
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:5px">
        <span style="font-weight:700;color:${color};letter-spacing:0.02em">⚡ ${tierLabel}</span>
        <span style="color:var(--text-muted);font-size:0.70rem">${fiveHK} / ${limitK} cr &nbsp;(${pct}%)</span>
      </div>
      <div style="height:4px;background:rgba(255,255,255,0.08);border-radius:4px;overflow:hidden">
        <div style="height:100%;width:${pct}%;background:${barColor};border-radius:4px;transition:width 0.4s ease"></div>
      </div>
      <div style="display:flex;justify-content:space-between;margin-top:4px;color:var(--text-muted);font-size:0.68rem">
        <span>0%</span>
        <span style="color:${color}40">20%&nbsp;×1.2</span>
        <span style="color:${color}60">40%&nbsp;×1.4</span>
        <span style="color:${color}80">60%&nbsp;×1.7</span>
        <span style="color:${color}">80%&nbsp;×2.0</span>
      </div>`;

    // Insert after progress bar inside first stat card
    const progress = card.querySelector('.progress-track');
    if (progress) progress.after(wrap);
    else card.appendChild(wrap);
  },

  // ── Storefront / Tabs ───────────────────────────────────────────────────
  showTab: (tabId) => {
    // Hide all tabs
    document.querySelectorAll('.tab-content').forEach(el => el.style.display = 'none');
    // Remove active from all buttons
    document.querySelectorAll('.nav-item').forEach(el => el.classList.remove('active'));
    
    // Show selected
    const tabEl = document.getElementById(tabId + 'Tab');
    if (tabEl) tabEl.style.display = 'block';
    
    const btnEl = document.getElementById('tab' + tabId.charAt(0).toUpperCase() + tabId.slice(1) + 'Btn');
    if (btnEl) btnEl.classList.add('active');

    if (tabId === 'keys') {
       const key = app.loadKey();
       if (key) {
         const shortKey = key.length > 16 ? key.substring(0, 8) + '••••••••••••' + key.substring(key.length-4) : key;
         app.setText('displayApiKey', shortKey);
       }
    }
  },

  revokeKey: () => {
    if (confirm("Are you sure you want to revoke this key and sign out? This will clear your current session.")) {
       app.clearKey();
       window.location.href = '/dashboard';
    }
  },

  checkoutPlan: '',
  checkoutDefaultAmount: 0,

  openCheckout: (plan, amount) => {
    app.checkoutPlan = plan;
    app.checkoutDefaultAmount = amount;

    const modal = document.getElementById('checkoutModal');
    const desc = document.getElementById('checkoutPlanDesc');
    const customWrap = document.getElementById('customAmountWrap');
    const customInput = document.getElementById('checkoutAmount');

    if (plan === 'pro') desc.textContent = 'Pro Tháng (350,000 VND) - 50,000 cr';
    else if (plan === 'max') desc.textContent = 'Max 5x Tháng (650,000 VND) - 250,000 cr';
    else if (plan === 'max_20x') desc.textContent = 'Max 20x Tháng (1,800,000 VND) - 1,000,000 cr';
    else if (plan === 'day1') desc.textContent = '1 Ngày (50,000 VND) - 4,000 cr';
    else if (plan === 'day7') desc.textContent = '7 Ngày (150,000 VND) - 30,000 cr';
    else if (plan === 'payg') desc.textContent = 'Pay As You Go (Custom Amount)';

    if (plan === 'payg') {
      customWrap.style.display = 'block';
      customInput.value = amount || 50000;
    } else {
      customWrap.style.display = 'none';
    }

    // Reset form state
    const emailEl = document.getElementById('checkoutEmail');
    const otpWrap = document.getElementById('checkoutOtpWrap');
    const otpInput = document.getElementById('checkoutOTP');
    const otpHint = document.getElementById('checkoutOtpHint');
    const sendBtn = document.getElementById('checkoutSendOtpBtn');
    const errEl = document.getElementById('checkoutError');
    const btn = document.getElementById('checkoutBtn');
    
    if (emailEl) { emailEl.value = ''; emailEl.disabled = false; }
    if (otpWrap) otpWrap.style.display = 'none';
    if (otpInput) otpInput.value = '';
    if (otpHint) otpHint.textContent = '';
    if (sendBtn) { 
      sendBtn.disabled = true; 
      sendBtn.style.opacity = '0.45'; 
      sendBtn.style.cursor = 'not-allowed'; 
      sendBtn.textContent = 'Send OTP'; 
      if (app.otpTimerInterval) {
        clearInterval(app.otpTimerInterval);
        app.otpTimerInterval = null;
      }
    }
    if (errEl) errEl.style.display = 'none';
    if (btn) { btn.disabled = true; btn.style.opacity = '0.45'; btn.style.cursor = 'not-allowed'; btn.textContent = 'Pay with VietQR'; }

    modal.style.display = 'flex';
    setTimeout(() => { if (emailEl) emailEl.focus(); }, 80);
  },

  closeCheckout: () => {
    document.getElementById('checkoutModal').style.display = 'none';
    const phone = document.getElementById('checkoutPhone');
    if (phone) phone.value = '';
  },

  // Validates OTP inputs and enables the relevant buttons
  validateOTPCheckout: () => {
    const email = (document.getElementById('checkoutEmail')?.value || '').trim();
    const otp = (document.getElementById('checkoutOTP')?.value || '').trim();
    const sendBtn = document.getElementById('checkoutSendOtpBtn');
    const payBtn = document.getElementById('checkoutBtn');
    const re = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    
    const emailOk = re.test(email);
    const otpOk = /^\d{6}$/.test(otp);

    // Enable Send OTP button if email is valid and hasn't been locked yet
    if (sendBtn) {
      const emailLocked = document.getElementById('checkoutEmail')?.disabled;
      const readyToSend = emailOk && !emailLocked;
      sendBtn.disabled = !readyToSend;
      sendBtn.style.opacity = readyToSend ? '1' : '0.45';
      sendBtn.style.cursor = readyToSend ? 'pointer' : 'not-allowed';
    }

    // Enable Pay button if email and OTP are valid
    if (payBtn) {
      const readyToPay = emailOk && otpOk;
      payBtn.disabled = !readyToPay;
      payBtn.style.opacity = readyToPay ? '1' : '0.45';
      payBtn.style.cursor = readyToPay ? 'pointer' : 'not-allowed';
    }
  },

  sendPurchaseOTP: async () => {
    const emailInput = document.getElementById('checkoutEmail');
    const errDiv = document.getElementById('checkoutError');
    const sendBtn = document.getElementById('checkoutSendOtpBtn');
    const otpWrap = document.getElementById('checkoutOtpWrap');
    
    const email = emailInput.value.trim();
    const re = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

    if (!email || !re.test(email)) {
      errDiv.textContent = 'Please enter a valid email address.';
      errDiv.style.display = 'block';
      return;
    }

    sendBtn.disabled = true;
    sendBtn.textContent = 'Sending...';
    errDiv.style.display = 'none';

    try {
      const phone = (document.getElementById('checkoutPhone')?.value || '').trim();
      const res = await fetch('/api/payment/send-otp', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, ...(phone ? { phone } : {}) })
      });

      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Failed to send OTP');
      
      // Success: lock email, show OTP field
      emailInput.disabled = true;
      otpWrap.style.display = 'block';
      
      const otpInput = document.getElementById('checkoutOTP');
      if (otpInput) {
        setTimeout(() => otpInput.focus(), 100);
      }

      // Start 3-minute countdown timer
      let timeLeft = 180;
      sendBtn.textContent = `Sent! (${timeLeft}s)`;
      if (app.otpTimerInterval) clearInterval(app.otpTimerInterval);
      
      app.otpTimerInterval = setInterval(() => {
        timeLeft--;
        if (timeLeft <= 0) {
          clearInterval(app.otpTimerInterval);
          app.otpTimerInterval = null;
          // Only re-enable if email is actually still filled (the form wasn't reset)
          if (emailInput.value.trim() !== '') {
            sendBtn.disabled = false;
            sendBtn.style.opacity = '1';
            sendBtn.style.cursor = 'pointer';
            sendBtn.textContent = 'Resend OTP';
          } else {
            sendBtn.textContent = 'Send OTP';
          }
        } else {
          sendBtn.textContent = `Sent! (${timeLeft}s)`;
        }
      }, 1000);

    } catch (e) {
      errDiv.textContent = e.message;
      errDiv.style.display = 'block';
      sendBtn.disabled = false;
      sendBtn.textContent = 'Send OTP';
    }
  },

  submitOTPCheckout: async () => {
    const emailInput  = document.getElementById('checkoutEmail');
    const otpInput    = document.getElementById('checkoutOTP');
    const amountInput = document.getElementById('checkoutAmount');
    const errDiv      = document.getElementById('checkoutError');
    const btn         = document.getElementById('checkoutBtn');

    const email = emailInput.value.trim();
    const otp   = otpInput.value.trim();
    const re    = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

    if (!email || !re.test(email)) {
      errDiv.textContent = 'Please enter a valid email address.';
      errDiv.style.display = 'block';
      return;
    }
    if (!otp || !/^\d{6}$/.test(otp)) {
      errDiv.textContent = 'Please enter a valid 6-digit OTP.';
      errDiv.style.display = 'block';
      return;
    }

    const amount = app.checkoutPlan === 'payg' ? parseInt(amountInput.value || 0) : 0;

    btn.disabled = true;
    btn.textContent = 'Processing...';
    errDiv.style.display = 'none';

    try {
      const phone = (document.getElementById('checkoutPhone')?.value || '').trim();
      const res = await fetch('/api/payment/create-link', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ plan: app.checkoutPlan, email, otp, custom_amount: amount, ...(phone ? { phone } : {}) })
      });

      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Failed to connect to gateway');
      if (data.checkoutUrl) {
        window.location.href = data.checkoutUrl;
      } else {
        throw new Error('Invalid gateway response');
      }
    } catch (e) {
      errDiv.textContent = e.message;
      errDiv.style.display = 'block';
      btn.disabled = false;
      btn.style.opacity = '1';
      btn.textContent = 'Pay with VietQR';
    }
  },


  requestRotation: async () => {
    const key = app.loadKey();
    if (!key) {
      alert('Please authenticate first before requesting a key revocation.');
      return;
    }

    const btn = document.querySelector('[onclick="app.requestRotation()"]');
    if (btn) { btn.disabled = true; btn.textContent = 'Sending OTP…'; }

    try {
      const res = await fetch('/api/payment/request-rotation', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ workspaceKey: key })
      });
      const data = await res.json();
      if (!res.ok) {
        alert(data.error || 'Failed to request key revocation');
        return;
      }
      // Show email hint in modal
      const hint = document.getElementById('rotationEmailHint');
      if (hint && data.emailHint) hint.textContent = data.emailHint;
      // Clear previous OTP input and error
      document.getElementById('rotationOtpInput').value = '';
      document.getElementById('rotationError').style.display = 'none';
      // Show rotation modal
      document.getElementById('rotationModal').style.display = 'flex';
    } catch (e) {
      alert('Error connecting to server. Please try again.');
    } finally {
      if (btn) { btn.disabled = false; btn.textContent = 'Revoke & Re-issue (Email OTP)'; }
    }
  },

  verifyRotation: async () => {
    const key = app.loadKey();
    const otp = document.getElementById('rotationOtpInput').value.trim();
    if (!otp) return;

    const btn = document.getElementById('rotationBtn');
    btn.textContent = 'Verifying...';
    btn.disabled = true;

    try {
      const res = await fetch('/api/payment/verify-rotation', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ workspaceKey: key, otp: otp })
      });
      const data = await res.json();
      if (!res.ok) {
        document.getElementById('rotationError').style.display = 'block';
        app.setText('rotationError', data.error || 'Invalid OTP');
        btn.textContent = 'Verify & Rotate Key';
        btn.disabled = false;
        return;
      }

      // Success — old key is now permanently revoked
      alert(
        `✅ Key revoked & new key issued!\n\n` +
        `Your OLD key has been permanently disabled.\n` +
        `Your NEW key:\n\n${data.newKey}\n\n` +
        `📧 It has also been sent to your registered email.\n` +
        `All credits and subscription conditions have been preserved.\n\n` +
        `Please update any apps/tools using the old key immediately.`
      );
      
      // Update session storage
      sessionStorage.setItem(app.CACHE_KEY, JSON.stringify({
        key: data.newKey,
        ts: Date.now()
      }));
      
      document.getElementById('rotationModal').style.display = 'none';
      document.getElementById('apiKeyInput').value = data.newKey;
      app.stopAutoRefresh();
      app.fetchQuota(); // reload dashboard with new key
      
    } catch (e) {
      document.getElementById('rotationError').style.display = 'block';
      app.setText('rotationError', 'Error connecting to server');
    }
    
    btn.textContent = 'Verify & Rotate Key';
    btn.disabled = false;
  },

  // ── Theme Management ──────────────────────────────────────────────────────
  // Sun icon = currently dark → click switches to light.
  // Moon icon = currently light → click switches to dark.
  _sunSVG: `<circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/>`,
  _moonSVG: `<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>`,

  _updateThemeIcon: (theme) => {
    const icon = document.getElementById('themeIcon');
    if (!icon) return;
    icon.innerHTML = theme === 'light' ? app._moonSVG : app._sunSVG;
  },

  initTheme: () => {
    // Priority: 1) user's explicit localStorage choice
    //           2) OS/browser prefers-color-scheme
    //           3) default: light
    const saved = localStorage.getItem('fink_theme');
    let theme;
    if (saved === 'dark' || saved === 'light') {
      theme = saved;
    } else if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
      theme = 'dark';
    } else {
      theme = 'light'; // default
    }
    document.documentElement.setAttribute('data-theme', theme);
    app._updateThemeIcon(theme);
  },

  toggleTheme: () => {
    const current = document.documentElement.getAttribute('data-theme') || 'light';
    const next = current === 'light' ? 'dark' : 'light';
    document.documentElement.setAttribute('data-theme', next);
    localStorage.setItem('fink_theme', next);
    app._updateThemeIcon(next);
    // Re-draw charts so grid/text colours match new theme
    if (app.usageChartInstance) { app.usageChartInstance.destroy(); app.usageChartInstance = null; }
    if (app.modelChartInstance) { app.modelChartInstance.destroy(); app.modelChartInstance = null; }
    app.initCharts();
    // Re-render data if available
    const key = app.loadKey();
    if (key) app.fetchData(key, false);
  },
};


// ── Bootstrap ─────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', () => {
  app.initTheme();   // apply saved theme before any render
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

  // Check if returning from payOS BEFORE key logic
  const urlParams = new URLSearchParams(window.location.search);
  const status = urlParams.get('status');
  const isCancel = urlParams.get('cancel') === 'true';

  if (status === 'PAID' || status === 'success') {
     const m = document.getElementById('paySuccessModal');
     if (m) m.style.display = 'flex';
     window.history.replaceState({}, document.title, window.location.pathname);
  } else if (isCancel || status === 'CANCELLED') {
     const m = document.getElementById('payCancelModal');
     if (m) m.style.display = 'flex';
     window.history.replaceState({}, document.title, window.location.pathname);
  }

  // Check URL key first
  const urlKey = urlParams.get('key');
  if (urlKey) {
    app.authFromOverlay();
    return;
  }

  // Check cached key (5-min TTL)
  const cachedKey = app.loadKey();
  if (cachedKey) {
    const navInput2 = document.getElementById('apiKeyInput');
    if (navInput2) navInput2.value = cachedKey;
    
    // Un-hide the tabs
    const tabs = document.getElementById('navTabs');
    if (tabs) tabs.style.display = 'flex';
    
    app.fetchData(cachedKey, false);
    app.startAutoRefresh(cachedKey);
    return;
  }

  // No key → show overlay
  document.getElementById('welcomeState').style.display = 'flex';
  document.getElementById('overviewTab').style.display = 'none';
  document.getElementById('storeTab').style.display = 'none';
});
app.otpTimerInterval = null;

