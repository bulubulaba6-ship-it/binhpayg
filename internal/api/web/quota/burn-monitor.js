// Burn Monitor UI Injector
// Fetches from /v0/management/admin/keys and displays burst billing status

(function () {
    // Inject CSS
    const style = document.createElement('style');
    style.textContent = `
        #burn-monitor-fab {
            position: fixed;
            bottom: 24px;
            right: 24px;
            background: linear-gradient(135deg, #ff4b2b, #ff416c);
            color: white;
            border: none;
            border-radius: 50px;
            padding: 12px 24px;
            font-size: 16px;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            font-weight: bold;
            cursor: pointer;
            box-shadow: 0 4px 12px rgba(255, 65, 108, 0.4);
            z-index: 999999;
            display: flex;
            align-items: center;
            gap: 8px;
            transition: transform 0.2s, box-shadow 0.2s;
        }
        #burn-monitor-fab:hover {
            transform: translateY(-2px);
            box-shadow: 0 6px 16px rgba(255, 65, 108, 0.5);
        }
        #burn-monitor-panel {
            position: fixed;
            bottom: 80px;
            right: 24px;
            width: 450px;
            max-height: calc(100vh - 120px);
            background: #ffffff;
            border-radius: 12px;
            box-shadow: 0 10px 40px rgba(0, 0, 0, 0.2);
            z-index: 999998;
            display: none;
            flex-direction: column;
            overflow: hidden;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            border: 1px solid #e2e8f0;
        }
        #burn-monitor-panel.open {
            display: flex;
        }
        .burn-header {
            padding: 16px 20px;
            background: #f8fafc;
            border-bottom: 1px solid #e2e8f0;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .burn-header h3 {
            margin: 0;
            font-size: 16px;
            color: #0f172a;
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .burn-close {
            background: none;
            border: none;
            font-size: 20px;
            color: #64748b;
            cursor: pointer;
            padding: 0;
            line-height: 1;
        }
        .burn-close:hover {
            color: #0f172a;
        }
        .burn-content {
            padding: 0;
            overflow-y: auto;
            flex: 1;
            background: #f1f5f9;
        }
        .burn-card {
            background: white;
            margin: 12px;
            padding: 16px;
            border-radius: 8px;
            box-shadow: 0 1px 3px rgba(0,0,0,0.05);
            border: 1px solid #e2e8f0;
        }
        .burn-card-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 12px;
        }
        .burn-key {
            font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
            font-size: 14px;
            font-weight: 600;
            color: #1e293b;
        }
        .burn-plan {
            font-size: 12px;
            font-weight: bold;
            background: #e2e8f0;
            color: #475569;
            padding: 2px 8px;
            border-radius: 12px;
        }
        .burn-stats {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 12px;
            margin-bottom: 16px;
        }
        .burn-stat {
            display: flex;
            flex-direction: column;
            gap: 4px;
        }
        .burn-stat-label {
            font-size: 11px;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            color: #64748b;
            font-weight: 600;
        }
        .burn-stat-value {
            font-size: 14px;
            font-weight: 600;
            color: #0f172a;
        }
        .burn-multiplier {
            font-size: 18px;
            font-weight: 700;
        }
        .burn-progress-container {
            margin-top: 8px;
        }
        .burn-progress-label {
            display: flex;
            justify-content: space-between;
            font-size: 12px;
            color: #475569;
            margin-bottom: 6px;
            font-weight: 500;
        }
        .burn-progress-bar {
            height: 8px;
            background: #e2e8f0;
            border-radius: 4px;
            overflow: hidden;
            position: relative;
        }
        .burn-progress-fill {
            height: 100%;
            border-radius: 4px;
            transition: width 0.3s ease, background-color 0.3s ease;
        }
        .burn-tiers {
            display: flex;
            justify-content: space-between;
            margin-top: 4px;
            padding: 0 2px;
        }
        .burn-tier-mark {
            width: 2px;
            height: 4px;
            background: #cbd5e1;
        }
        .burn-loading {
            padding: 40px;
            text-align: center;
            color: #64748b;
            font-size: 14px;
        }
        .burn-error {
            padding: 20px;
            margin: 12px;
            background: #fef2f2;
            border: 1px solid #fecaca;
            color: #ef4444;
            border-radius: 8px;
            font-size: 14px;
        }
        
        /* Dark mode overrides if parent has dark mode */
        @media (prefers-color-scheme: dark) {
            #burn-monitor-panel {
                background: #1e293b;
                border-color: #334155;
            }
            .burn-header {
                background: #0f172a;
                border-color: #334155;
            }
            .burn-header h3 {
                color: #f8fafc;
            }
            .burn-content {
                background: #0f172a;
            }
            .burn-card {
                background: #1e293b;
                border-color: #334155;
            }
            .burn-key {
                color: #f8fafc;
            }
            .burn-plan {
                background: #334155;
                color: #cbd5e1;
            }
            .burn-stat-value {
                color: #f8fafc;
            }
            .burn-progress-bar {
                background: #334155;
            }
            .burn-progress-label {
                color: #cbd5e1;
            }
            .burn-error {
                background: rgba(239, 68, 68, 0.1);
                border-color: rgba(239, 68, 68, 0.2);
            }
        }
    `;
    document.head.appendChild(style);

    // Create FAB
    const fab = document.createElement('button');
    fab.id = 'burn-monitor-fab';
    fab.innerHTML = '🔥 Burn Monitor';
    document.body.appendChild(fab);

    // Create Panel
    const panel = document.createElement('div');
    panel.id = 'burn-monitor-panel';
    panel.innerHTML = `
        <div class="burn-header">
            <h3>🔥 Live Burst Billing Monitor</h3>
            <button class="burn-close">&times;</button>
        </div>
        <div class="burn-content" id="burn-content">
            <div class="burn-loading">Loading key data...</div>
        </div>
    `;
    document.body.appendChild(panel);

    let refreshInterval = null;

    fab.addEventListener('click', () => {
        panel.classList.toggle('open');
        if (panel.classList.contains('open')) {
            fetchData();
            refreshInterval = setInterval(fetchData, 5000);
        } else {
            clearInterval(refreshInterval);
        }
    });

    panel.querySelector('.burn-close').addEventListener('click', () => {
        panel.classList.remove('open');
        clearInterval(refreshInterval);
    });

    function getProgressColor(ratio) {
        if (ratio < 0.2) return '#10b981'; // Green
        if (ratio < 0.4) return '#eab308'; // Yellow
        if (ratio < 0.6) return '#f59e0b'; // Orange
        if (ratio < 0.8) return '#f97316'; // Red-Orange
        if (ratio < 1.0) return '#ef4444'; // Red
        return '#8b5cf6'; // Purple (Ceiling)
    }

    function formatNumber(num) {
        return new Intl.NumberFormat('en-US', { maximumFractionDigits: 2 }).format(num);
    }

    async function fetchData() {
        try {
            // Check auth header if available (from existing app state)
            const token = localStorage.getItem('auth_token') || sessionStorage.getItem('auth_token');
            const headers = {};
            if (token) {
                headers['Authorization'] = 'Bearer ' + token;
            } else {
                // If it's loaded within the same app, it might use cookies, or we might need the management password
                // But typically admin APIs are available if we are logged in or via localhost
            }

            const res = await fetch('/v0/management/admin/keys', { headers });
            if (!res.ok) {
                throw new Error('API returned ' + res.status);
            }
            const data = await res.json();
            renderData(data.keys || []);
        } catch (err) {
            console.error('Burn Monitor Error:', err);
            document.getElementById('burn-content').innerHTML = `
                <div class="burn-error">
                    <strong>Connection Failed:</strong> ${err.message}<br>
                    <small>Retrying in 5 seconds...</small>
                </div>
            `;
        }
    }

    function renderData(keys) {
        const content = document.getElementById('burn-content');
        if (keys.length === 0) {
            content.innerHTML = '<div class="burn-loading">No active keys found.</div>';
            return;
        }

        // Sort by burn ratio descending (highest risk first)
        keys.sort((a, b) => b.burn_ratio - a.burn_ratio);

        let html = '';
        keys.forEach(k => {
            const limit = k.five_h_limit || 0;
            const credits = k.five_h_credits || 0;
            let ratio = limit > 0 ? credits / limit : 0;
            
            const pct = Math.min(Math.round(ratio * 100), 100);
            const color = getProgressColor(ratio);
            
            const multColor = k.burn_multiplier > 1.0 ? '#ef4444' : '#10b981';

            html += `
                <div class="burn-card">
                    <div class="burn-card-header">
                        <span class="burn-key">${k.key_masked}</span>
                        <span class="burn-plan">${k.plan}</span>
                    </div>
                    
                    <div class="burn-stats">
                        <div class="burn-stat">
                            <span class="burn-stat-label">Active Multiplier</span>
                            <span class="burn-stat-value burn-multiplier" style="color: ${multColor}">
                                ${k.burn_multiplier.toFixed(2)}x
                            </span>
                        </div>
                        <div class="burn-stat">
                            <span class="burn-stat-label">Daily Burn Today</span>
                            <span class="burn-stat-value">$${formatNumber(k.daily_burn_today)}</span>
                        </div>
                        <div class="burn-stat">
                            <span class="burn-stat-label">Status</span>
                            <span class="burn-stat-value" style="color: ${k.status === 'active' ? '#10b981' : '#ef4444'}">
                                ${k.status.toUpperCase()}
                            </span>
                        </div>
                        <div class="burn-stat">
                            <span class="burn-stat-label">Total Consumed</span>
                            <span class="burn-stat-value">$${formatNumber(k.credits_consumed)}</span>
                        </div>
                    </div>

                    <div class="burn-progress-container">
                        <div class="burn-progress-label">
                            <span>${k.plan === 'PAYG' ? '5H Window Usage (No Burst Penalty)' : '5H Window Limit'}</span>
                            <span>$${formatNumber(credits)} / ${limit > 0 ? '$' + formatNumber(limit) : 'Unlimited'}</span>
                        </div>
                        <div class="burn-progress-bar">
                            <div class="burn-progress-fill" style="width: ${pct}%; background-color: ${color}"></div>
                        </div>
                        <div class="burn-tiers">
                            <div class="burn-tier-mark"></div>
                            <div class="burn-tier-mark"></div>
                            <div class="burn-tier-mark"></div>
                            <div class="burn-tier-mark"></div>
                            <div class="burn-tier-mark"></div>
                            <div class="burn-tier-mark"></div>
                        </div>
                    </div>
                </div>
            `;
        });

        content.innerHTML = html;
    }
})();
