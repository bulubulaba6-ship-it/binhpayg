const app = {
  usageChartInstance: null,
  modelChartInstance: null,

  formatNumber: (num) => (num || 0).toLocaleString('en-US'),
  
  formatDuration: (seconds) => {
    if (!seconds) return "0s";
    if (seconds < 60) return seconds + "s";
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return m + "m " + s + "s";
  },

  formatDate: (dateStr) => {
    if (!dateStr || dateStr === "0001-01-01T00:00:00Z") return "N/A";
    const d = new Date(dateStr);
    return d.toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' });
  },

  initCharts: () => {
    Chart.defaults.font.family = '"Inter", sans-serif';
    Chart.defaults.color = '#737373';
  },

  renderCharts: (usageData) => {
    // 1. Usage Trend Chart (Mocked trend based on total requests for visualization)
    // Real implementation would use timeseries data from backend if available.
    const ctxUsage = document.getElementById('usageChart');
    if (app.usageChartInstance) app.usageChartInstance.destroy();
    
    // Create a 30-day mock distribution matching total requests
    const days = Array.from({length: 30}, (_, i) => {
      const d = new Date();
      d.setDate(d.getDate() - (29 - i));
      return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    });
    
    // Distribute total randomly across 30 days
    const total = usageData.total_requests || 0;
    const dataPoints = Array(30).fill(0);
    if (total > 0) {
      let remaining = total;
      for(let i=0; i<29; i++) {
        let val = Math.floor(Math.random() * (remaining / (30-i)) * 2);
        if (val > remaining) val = remaining;
        dataPoints[i] = val;
        remaining -= val;
      }
      dataPoints[29] = remaining;
    }

    app.usageChartInstance = new Chart(ctxUsage, {
      type: 'line',
      data: {
        labels: days,
        datasets: [{
          label: 'Requests',
          data: dataPoints,
          borderColor: '#d97757',
          backgroundColor: 'rgba(217, 119, 87, 0.1)',
          borderWidth: 2,
          fill: true,
          tension: 0.4,
          pointRadius: 0,
          pointHitRadius: 10
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false } },
        scales: {
          x: { grid: { display: false }, ticks: { maxTicksLimit: 6 } },
          y: { beginAtZero: true, border: { display: false } }
        }
      }
    });

    // 2. Model Breakdown Chart
    const ctxModel = document.getElementById('modelChart');
    if (app.modelChartInstance) app.modelChartInstance.destroy();

    const models = usageData.models || {};
    
    // Group models by alias
    const aliasData = {};
    Object.keys(models).forEach(k => {
      let alias = k;
      if (k === 'gemini-3.1-flash-lite-preview' || k === 'gemini-3-flash') alias = 'claude-opus-4-7';
      else if (k === 'gemini-3-flash-preview') alias = 'claude-opus-4-6';
      else if (k === 'gemini-3.1-flash-lite') alias = 'claude-sonnet-4-6';
      else if (k === 'gemini-2.5-flash-lite' || k === 'gemini-2.5-flash') alias = 'claude-haiku/sonnet';
      else if (k.includes('gemini')) alias = 'claude';
      
      aliasData[alias] = (aliasData[alias] || 0) + (models[k].total_requests || 0);
    });

    const labels = Object.keys(aliasData);
    const data = labels.map(k => aliasData[k]);
    
    if (labels.length === 0) {
      labels.push("No Usage");
      data.push(1);
    }

    app.modelChartInstance = new Chart(ctxModel, {
      type: 'doughnut',
      data: {
        labels: labels,
        datasets: [{
          data: data,
          backgroundColor: ['#d97757', '#3b82f6', '#10b981', '#f59e0b', '#6366f1', '#8b5cf6'],
          borderWidth: 0
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        cutout: '70%',
        plugins: {
          legend: { position: 'bottom', labels: { boxWidth: 12, padding: 15 } }
        }
      }
    });
  },

  renderLedger: (sessions) => {
    const tbody = document.getElementById('historyBody');
    tbody.innerHTML = '';
    
    if (!sessions || sessions.length === 0) {
      tbody.innerHTML = '<tr><td colspan="5" style="text-align:center; color:var(--text-muted); padding: 32px;">No recent session activity</td></tr>';
      return;
    }

    sessions.forEach(s => {
      const tr = document.createElement('tr');
      const sid = s.SessionID || 'Unknown';
      const shortId = sid.length > 12 ? sid.substring(0,8) + '...' : sid;
      
      const tokens = `${s.InputTokens || 0} / ${s.OutputTokens || 0} / ${s.CachedTokens || 0} / ${s.ReasoningTokens || 0}`;
      const credits = (s.credits || 0).toFixed(4);
      tr.innerHTML = `
        <td class="mono" title="${sid}">${shortId}</td>
        <td>${s.Model || 'Unknown'}</td>
        <td style="color: var(--text-muted);">${tokens}</td>
        <td>${app.formatDate(s.UpdatedAt || s.StartedAt)}</td>
        <td class="text-right"><span class="badge-cost">${credits} CR</span></td>
      `;
      tbody.appendChild(tr);
    });
  },

  fetchQuota: async () => {
    const input = document.getElementById('apiKeyInput');
    const key = input.value.trim() || new URLSearchParams(window.location.search).get('key');
    if (!key) return alert("Please enter a Workspace API Key");
    
    try {
      const res = await fetch('/v1/quota?key=' + encodeURIComponent(key), { headers: { 'Authorization': 'Bearer ' + key } });
      if (!res.ok) throw new Error("Authentication failed or key invalid");
      const data = await res.json();
      
      // Update DOM
      document.getElementById('welcomeState').style.display = 'none';
      document.getElementById('dashboard').style.display = 'block';
      
      // Set values
      document.getElementById('valTotalReq').textContent = app.formatNumber(data.usage.total_requests);
      document.getElementById('valSuccessReq').textContent = app.formatNumber(data.usage.success_requests);
      document.getElementById('valFailedReq').textContent = app.formatNumber(data.usage.failed_requests);
      document.getElementById('valCredits').textContent = (data.quota.credits_used || 0).toFixed(4);
      document.getElementById('valTokens').textContent = app.formatNumber(data.quota.total_tokens || 0);
      document.getElementById('valRPM').textContent = (data.usage.rpm || 0).toFixed(1);
      
      // Progress Bar
      const limit = data.quota.credit_limit || 100;
      if (limit === -1 || limit === 0) {
        document.getElementById('valLimit').textContent = "Unlimited";
        document.getElementById('creditProgress').style.width = "0%";
      } else {
        document.getElementById('valLimit').textContent = app.formatNumber(limit);
        let pct = ((data.quota.credits_used || 0) / limit) * 100;
        if (pct > 100) pct = 100;
        document.getElementById('creditProgress').style.width = pct + "%";
        document.getElementById('creditProgress').style.backgroundColor = pct >= 90 ? "var(--danger)" : "var(--accent-color)";
      }
      
      // Reset Timer
      if (data.quota.window_expires_at && data.quota.window_expires_at !== "0001-01-01T00:00:00Z") {
        const diffMs = new Date(data.quota.window_expires_at) - new Date();
        if (diffMs > 0) {
          const h = Math.floor(diffMs / 3600000);
          const m = Math.floor((diffMs % 3600000) / 60000);
          document.getElementById('valReset').textContent = "Resets in " + h + "h " + m + "m";
        } else {
          document.getElementById('valReset').textContent = "Resetting shortly...";
        }
      } else {
        document.getElementById('valReset').textContent = "Cumulative session limit";
      }

      // Charts & Ledger
      app.renderCharts(data.usage);
      app.renderLedger(data.quota.recent_sessions);

      // Update URL without reload (hide key)
      const url = new URL(window.location);
      url.searchParams.delete('key');
      window.history.replaceState({}, '', url);
      input.value = key;

    } catch (err) {
      alert("Error: " + err.message);
    }
  }
};

document.addEventListener('DOMContentLoaded', () => {
  app.initCharts();
  const input = document.getElementById('apiKeyInput');
  input.addEventListener('keypress', (e) => {
    if (e.key === 'Enter') app.fetchQuota();
  });
  
  if (new URLSearchParams(window.location.search).get('key')) {
    app.fetchQuota();
  }
});
