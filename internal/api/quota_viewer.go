package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const apiKeyQuotaViewerHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta http-equiv="Cache-Control" content="no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0" />
  <meta http-equiv="Pragma" content="no-cache" />
  <meta http-equiv="Expires" content="0" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>AiApiGiaRe API Key Quota Viewer</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f5f4ed;
      --bg-2: #faf9f5;
      --panel: #faf9f5;
      --panel-2: #ffffff;
      --sand: #e8e6dc;
      --border: #f0eee6;
      --border-strong: #e8e6dc;
      --accent: #c96442;
      --accent-2: #d97757;
      --focus: #3898ec;
      --success: #5e6856;
      --text: #141413;
      --muted: #5e5d59;
      --subtle: #87867f;
      --danger: #b53333;
      --shadow: 0 0 0 1px rgba(240, 238, 230, 0.96), 0 10px 28px rgba(20, 20, 19, 0.05);
    }

    * {
      box-sizing: border-box;
    }

    body {
      margin: 0;
      min-height: 100vh;
      font-family: "Aptos", "Segoe UI Variable", "Segoe UI", Arial, sans-serif;
      color: var(--text);
      background:
        radial-gradient(circle at top left, rgba(201, 100, 66, 0.12), transparent 24%),
        radial-gradient(circle at 88% 12%, rgba(216, 184, 147, 0.22), transparent 22%),
        linear-gradient(180deg, #faf8f0 0%, var(--bg) 34%, #f3efe6 100%);
    }

    .shell {
      max-width: 1200px;
      margin: 0 auto;
      padding: 48px 24px 64px;
    }

    .hero {
      display: grid;
      gap: 20px;
      grid-template-columns: minmax(0, 1.6fr) minmax(280px, 0.9fr);
      margin-bottom: 28px;
      align-items: stretch;
    }

    .eyebrow {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      margin: 0 0 12px;
      color: var(--accent);
      font-size: 12px;
      letter-spacing: 0.16em;
      text-transform: uppercase;
    }

    .title {
      margin: 0;
      font-family: Georgia, "Times New Roman", serif;
      font-size: clamp(2.75rem, 5vw, 4.9rem);
      font-weight: 500;
      line-height: 1.1;
      letter-spacing: -0.04em;
    }

    .subtitle {
      margin: 16px 0 0;
      max-width: 56ch;
      color: var(--muted);
      font-size: 1.06rem;
      line-height: 1.65;
    }

    .card {
      border: 1px solid var(--border);
      border-radius: 28px;
      background: linear-gradient(180deg, var(--panel-2), var(--panel));
      box-shadow: var(--shadow);
      backdrop-filter: blur(10px);
    }

    .hero-card {
      padding: 24px;
      display: flex;
      flex-direction: column;
      justify-content: space-between;
      gap: 14px;
    }

    .hero-stat {
      display: grid;
      gap: 8px;
      padding: 18px;
      border-radius: 18px;
      background: linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(250, 249, 245, 0.98));
      border: 1px solid var(--border-strong);
      box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.9) inset;
    }

    .hero-stat-label {
      color: var(--subtle);
      font-size: 0.78rem;
      letter-spacing: 0.14em;
      text-transform: uppercase;
    }

    .hero-stat-value {
      font-family: Georgia, "Times New Roman", serif;
      font-size: 1.06rem;
      font-weight: 400;
      line-height: 1.65;
    }

    .panel {
      padding: 28px;
    }

    .controls {
      display: grid;
      grid-template-columns: 1fr;
      gap: 16px;
      align-items: end;
    }

    .field label {
      display: block;
      margin-bottom: 8px;
      color: var(--muted);
      font-size: 0.8rem;
      letter-spacing: 0.15em;
      text-transform: uppercase;
    }

    .field input {
      width: 100%;
      padding: 15px 16px;
      border-radius: 14px;
      border: 1px solid var(--border-strong);
      background: var(--panel);
      color: var(--text);
      font: inherit;
      outline: none;
      transition: border-color 160ms ease, box-shadow 160ms ease, transform 160ms ease;
    }

    .field input:focus {
      border-color: var(--focus);
      box-shadow: 0 0 0 4px rgba(56, 152, 236, 0.14);
      transform: translateY(-1px);
    }

    .actions {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
    }

    button {
      appearance: none;
      border: 0;
      border-radius: 12px;
      padding: 14px 16px;
      font: inherit;
      font-weight: 600;
      cursor: pointer;
      transition: transform 160ms ease, box-shadow 160ms ease, opacity 160ms ease;
    }

    button:hover {
      transform: translateY(-1px);
    }

    button:disabled {
      opacity: 0.55;
      cursor: not-allowed;
      transform: none;
    }

    .primary {
      color: #faf9f5;
      background: linear-gradient(135deg, var(--accent), var(--accent-2));
      box-shadow: 0 0 0 1px rgba(201, 100, 66, 0.85), 0 10px 24px rgba(201, 100, 66, 0.16);
    }

    .secondary {
      color: var(--text);
      background: var(--sand);
      border: 1px solid var(--border-strong);
      box-shadow: 0 0 0 1px rgba(232, 230, 220, 0.7);
    }

    .status {
      margin: 18px 0 0;
      color: var(--muted);
      line-height: 1.5;
      min-height: 1.5em;
    }

    .status.error {
      color: var(--danger);
    }

    .chart-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
      gap: 16px;
      margin-bottom: 4px;
    }

    .chart-card {
      position: relative;
      overflow: hidden;
      padding: 22px;
      border-radius: 24px;
      background: linear-gradient(180deg, var(--panel-2), var(--panel));
      border: 1px solid var(--border);
      box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.9) inset;
    }

    .chart-label {
      color: var(--subtle);
      font-size: 0.78rem;
      letter-spacing: 0.14em;
      text-transform: uppercase;
    }

    .chart-note {
      margin-top: 10px;
      color: var(--muted);
      font-size: 0.92rem;
      line-height: 1.6;
    }

    .ring-chart {
      --ring-value: 0;
      --ring-color: var(--accent);
      position: relative;
      width: 160px;
      height: 160px;
      margin: 14px auto 16px;
      border-radius: 50%;
      background: conic-gradient(var(--ring-color) calc(var(--ring-value) * 1%), rgba(232, 230, 220, 0.95) 0);
    }

    .ring-chart::after {
      content: "";
      position: absolute;
      inset: 18px;
      border-radius: 50%;
      background: linear-gradient(180deg, var(--panel-2), var(--panel));
      border: 1px solid var(--border);
    }

    .ring-center {
      position: absolute;
      inset: 0;
      z-index: 1;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      text-align: center;
      padding: 24px;
    }

    .ring-value {
      font-family: Georgia, "Times New Roman", serif;
      font-size: 2.15rem;
      font-weight: 500;
      line-height: 1;
    }

    .ring-subtitle {
      margin-top: 8px;
      color: var(--muted);
      font-size: 0.82rem;
      line-height: 1.5;
    }

    .progress-track {
      display: flex;
      height: 14px;
      overflow: hidden;
      border-radius: 999px;
      background: var(--sand);
      border: 1px solid var(--border-strong);
    }

    .progress-segment {
      height: 100%;
      transition: width 180ms ease;
    }

    .progress-segment.cached {
      background: linear-gradient(90deg, #c96442, #d97757);
    }

    .progress-segment.reasoning {
      background: linear-gradient(90deg, #8b5a4a, #c96442);
    }

    .progress-segment.other {
      background: linear-gradient(90deg, #b0aea5, #87867f);
    }

    .legend-list {
      display: flex;
      flex-wrap: wrap;
      gap: 12px;
      margin-top: 14px;
    }

    .legend-item {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      color: var(--muted);
      font-size: 0.84rem;
    }

    .legend-color {
      width: 10px;
      height: 10px;
      border-radius: 999px;
      flex: 0 0 auto;
    }

    .legend-color.cached {
      background: var(--accent);
    }

    .legend-color.reasoning {
      background: var(--accent-2);
    }

    .legend-color.other {
      background: var(--subtle);
    }

    .snapshot-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
      gap: 12px;
      margin-top: 14px;
    }

    .snapshot-stat {
      padding: 14px;
      border-radius: 16px;
      background: var(--panel-2);
      border: 1px solid var(--border);
      box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.85) inset;
    }

    .snapshot-label {
      display: block;
      margin-bottom: 6px;
      color: var(--muted);
      font-size: 0.74rem;
      letter-spacing: 0.14em;
      text-transform: uppercase;
    }

    .snapshot-value {
      font-family: Georgia, "Times New Roman", serif;
      font-size: 1.55rem;
      font-weight: 500;
      line-height: 1.1;
    }

    .snapshot-detail {
      margin-top: 4px;
      color: var(--muted);
      font-size: 0.82rem;
      line-height: 1.55;
    }

    .section-title {
      margin: 28px 0 16px;
      color: var(--text);
      font-family: Georgia, "Times New Roman", serif;
      font-size: 1.5rem;
      font-weight: 500;
      line-height: 1.25;
      letter-spacing: 0;
      text-transform: none;
    }

    .section-subtitle {
      margin: -6px 0 16px;
      color: var(--muted);
      font-size: 0.94rem;
      line-height: 1.6;
    }

    .metrics {
      display: grid;
      grid-template-columns: repeat(3, minmax(0, 1fr));
      gap: 16px;
      align-items: stretch;
    }

    .metric {
      min-height: 100%;
    }

    .metric:first-child {
      border-color: rgba(201, 100, 66, 0.22);
      background: linear-gradient(180deg, rgba(255, 255, 255, 0.99), rgba(250, 247, 242, 0.98));
    }

    .metric:first-child .metric-value {
      color: var(--text);
      font-size: 2.1rem;
    }

    @media (max-width: 1100px) {
      .metrics {
        grid-template-columns: repeat(2, minmax(0, 1fr));
      }
    }

    @media (max-width: 640px) {
      .metrics {
        grid-template-columns: 1fr;
      }
    }

    .dashboard-toolbar {
      margin-top: 28px;
      padding: 22px;
    }

    .dashboard-toolbar-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      flex-wrap: wrap;
      margin-bottom: 18px;
    }

    .dashboard-toolbar-title {
      display: grid;
      gap: 6px;
    }

    .dashboard-toolbar-title h2 {
      margin: 0;
      font-family: Georgia, "Times New Roman", serif;
      font-size: 1.75rem;
      font-weight: 500;
      line-height: 1.15;
    }

    .dashboard-toolbar-title p {
      margin: 0;
      color: var(--muted);
      font-size: 0.94rem;
      line-height: 1.6;
    }

    .dashboard-toolbar-grid {
      display: grid;
      grid-template-columns: minmax(220px, 1.1fr) minmax(220px, 0.9fr) minmax(180px, 0.7fr);
      gap: 14px;
      align-items: end;
    }

    .toolbar-field {
      display: grid;
      gap: 8px;
    }

    .toolbar-field label {
      color: var(--muted);
      font-size: 0.78rem;
      letter-spacing: 0.14em;
      text-transform: uppercase;
    }

    .toolbar-field select {
      width: 100%;
      padding: 14px 16px;
      border-radius: 14px;
      border: 1px solid var(--border-strong);
      background: rgba(255, 255, 255, 0.9);
      color: var(--text);
      font: inherit;
      outline: none;
      transition: border-color 160ms ease, box-shadow 160ms ease, transform 160ms ease;
    }

    .toolbar-field select:focus {
      border-color: var(--accent);
      box-shadow: 0 0 0 4px rgba(217, 119, 6, 0.14);
      transform: translateY(-1px);
    }

    .toolbar-actions {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      justify-content: flex-start;
    }

    .toolbar-updated {
      padding: 14px 16px;
      border-radius: 14px;
      border: 1px solid var(--border-strong);
      background: rgba(255, 255, 255, 0.72);
      color: var(--muted);
      font-size: 0.9rem;
      line-height: 1.4;
    }

    .analysis-grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 16px;
      margin-top: 18px;
    }

    .analysis-card {
      display: grid;
      gap: 16px;
      padding: 22px;
      border-radius: 22px;
      border: 1px solid var(--border);
      background: linear-gradient(180deg, rgba(255, 255, 255, 0.94), rgba(248, 241, 231, 0.94));
      box-shadow: var(--shadow);
    }

    .analysis-card[data-accent='teal'] {
      border-color: rgba(47, 111, 94, 0.16);
    }

    .analysis-card[data-accent='amber'] {
      border-color: rgba(217, 119, 6, 0.16);
    }

    .analysis-card[data-accent='violet'] {
      border-color: rgba(124, 92, 70, 0.16);
    }

    .analysis-card[data-accent='slate'] {
      border-color: rgba(120, 98, 72, 0.16);
    }

    .analysis-header {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 12px;
      flex-wrap: wrap;
    }

    .analysis-header h3 {
      margin: 0;
      font-family: Georgia, "Times New Roman", serif;
      font-size: 1.35rem;
      font-weight: 500;
      line-height: 1.2;
    }

    .analysis-header p {
      margin: 4px 0 0;
      color: var(--muted);
      font-size: 0.9rem;
      line-height: 1.55;
    }

    .period-tabs {
      display: inline-flex;
      gap: 6px;
      padding: 4px;
      border-radius: 999px;
      border: 1px solid var(--border-strong);
      background: rgba(255, 255, 255, 0.8);
      flex-wrap: wrap;
    }

    .period-tab {
      appearance: none;
      border: 0;
      border-radius: 999px;
      padding: 9px 12px;
      font: inherit;
      font-size: 0.84rem;
      color: var(--muted);
      background: transparent;
      cursor: pointer;
      transition: background-color 160ms ease, color 160ms ease, box-shadow 160ms ease;
    }

    .period-tab.active {
      color: var(--text);
      background: rgba(255, 255, 255, 0.98);
      box-shadow: 0 0 0 1px rgba(120, 98, 72, 0.12) inset;
    }

    .analysis-body {
      display: grid;
      gap: 16px;
    }

    .analysis-body.is-empty {
      min-height: 250px;
    }

    .empty-state {
      display: grid;
      gap: 10px;
      align-content: center;
      justify-items: start;
      min-height: 220px;
      padding: 18px;
      border-radius: 18px;
      border: 1px dashed var(--border-strong);
      background: rgba(255, 255, 255, 0.66);
    }

    .empty-state-title {
      font-family: Georgia, "Times New Roman", serif;
      font-size: 1.1rem;
      font-weight: 500;
      color: var(--text);
    }

    .empty-state-subtitle {
      color: var(--muted);
      font-size: 0.92rem;
      line-height: 1.6;
    }

    .breakdown-layout {
      display: grid;
      grid-template-columns: minmax(180px, 220px) minmax(0, 1fr);
      gap: 18px;
      align-items: center;
    }

    .breakdown-ring {
      position: relative;
      width: 180px;
      height: 180px;
      margin: 0 auto;
      border-radius: 50%;
      background: conic-gradient(var(--ring-slices, #d97706 0deg 0deg), rgba(120, 98, 72, 0.08) 0);
    }

    .breakdown-ring::after {
      content: "";
      position: absolute;
      inset: 24px;
      border-radius: 50%;
      background: linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(250, 244, 236, 0.98));
      border: 1px solid var(--border);
    }

    .breakdown-center {
      position: absolute;
      inset: 0;
      z-index: 1;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      text-align: center;
      padding: 22px;
    }

    .breakdown-total {
      font-family: Georgia, "Times New Roman", serif;
      font-size: 2rem;
      font-weight: 500;
      line-height: 1;
    }

    .breakdown-caption {
      margin-top: 8px;
      color: var(--muted);
      font-size: 0.82rem;
      line-height: 1.45;
    }

    .breakdown-list {
      display: grid;
      gap: 10px;
    }

    .breakdown-row {
      display: grid;
      gap: 6px;
      padding: 12px 14px;
      border-radius: 14px;
      border: 1px solid var(--border);
      background: rgba(255, 255, 255, 0.76);
    }

    .breakdown-row-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      color: var(--text);
      font-size: 0.9rem;
    }

    .breakdown-row-name {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      font-weight: 600;
    }

    .breakdown-swatch {
      width: 10px;
      height: 10px;
      border-radius: 999px;
      flex: 0 0 auto;
    }

    .breakdown-track {
      display: flex;
      height: 12px;
      overflow: hidden;
      border-radius: 999px;
      background: rgba(120, 98, 72, 0.1);
      border: 1px solid rgba(120, 98, 72, 0.12);
    }

    .breakdown-segment {
      height: 100%;
      transition: width 180ms ease;
    }

    .trend-chart {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(44px, 1fr));
      gap: 10px;
      align-items: end;
      min-height: 220px;
    }

    .trend-bar {
      display: grid;
      gap: 8px;
      justify-items: center;
      align-content: end;
      min-height: 220px;
    }

    .trend-bar-value {
      color: var(--text);
      font-size: 0.8rem;
      font-weight: 600;
      text-align: center;
    }

    .trend-bar-track {
      position: relative;
      width: 100%;
      height: 150px;
      display: flex;
      align-items: flex-end;
      justify-content: center;
      padding: 0 4px;
      border-radius: 16px 16px 10px 10px;
      background: linear-gradient(180deg, rgba(255, 255, 255, 0.48), rgba(248, 241, 231, 0.84));
      border: 1px solid rgba(120, 98, 72, 0.1);
    }

    .trend-bar-fill {
      width: 100%;
      border-radius: 12px 12px 6px 6px;
      background: linear-gradient(180deg, #d97706, #f59e0b);
      min-height: 6px;
      transition: height 180ms ease;
    }

    .trend-bar-label {
      color: var(--muted);
      font-size: 0.74rem;
      line-height: 1.2;
      text-align: center;
    }

    .cost-grid {
      display: grid;
      gap: 12px;
    }

    .cost-summary {
      display: grid;
      gap: 10px;
      padding: 16px;
      border-radius: 16px;
      background: rgba(255, 255, 255, 0.78);
      border: 1px solid var(--border);
    }

    .cost-summary-value {
      font-family: Georgia, "Times New Roman", serif;
      font-size: 2rem;
      font-weight: 500;
      line-height: 1;
    }

    .cost-summary-note {
      color: var(--muted);
      font-size: 0.9rem;
      line-height: 1.55;
    }

    .cost-models {
      display: grid;
      gap: 10px;
    }

    .cost-model-row {
      display: grid;
      gap: 6px;
      padding: 12px 14px;
      border-radius: 14px;
      border: 1px solid var(--border);
      background: rgba(255, 255, 255, 0.76);
    }

    .cost-model-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
    }

    .cost-model-name {
      color: var(--text);
      font-size: 0.9rem;
      font-weight: 600;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .cost-model-value {
      color: var(--accent);
      font-size: 0.88rem;
      font-weight: 700;
      white-space: nowrap;
    }

    .cost-model-track {
      display: flex;
      height: 10px;
      overflow: hidden;
      border-radius: 999px;
      background: rgba(120, 98, 72, 0.1);
      border: 1px solid rgba(120, 98, 72, 0.12);
    }

    .cost-model-fill {
      height: 100%;
      background: linear-gradient(90deg, #c96442, #d97757);
      transition: width 180ms ease;
    }

    .metric-badge {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      min-width: 36px;
      height: 36px;
      padding: 0 10px;
      border-radius: 12px;
      font-size: 0.72rem;
      font-weight: 700;
      letter-spacing: 0.12em;
      text-transform: uppercase;
      color: #fff;
      background: linear-gradient(135deg, #7c5c46, #c96442);
      box-shadow: 0 10px 18px rgba(201, 100, 66, 0.14);
    }

    .metric-head {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 12px;
    }

    .metric {
      padding: 18px;
      border-radius: 18px;
      background: linear-gradient(180deg, var(--panel-2), var(--panel));
      border: 1px solid var(--border);
      box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.9) inset;
    }

    .metric[data-tone='slate'] {
      border-color: var(--border-strong);
    }

    .metric[data-tone='green'] {
      border-color: rgba(94, 104, 86, 0.22);
    }

    .metric[data-tone='red'] {
      border-color: rgba(181, 51, 51, 0.2);
    }

    .metric[data-tone='violet'] {
      border-color: rgba(201, 100, 66, 0.18);
    }

    .metric[data-tone='amber'] {
      border-color: rgba(201, 100, 66, 0.18);
    }

    .metric[data-tone='green'] .metric-value {
      color: var(--success);
    }

    .metric[data-tone='red'] .metric-value {
      color: var(--danger);
    }

    .metric[data-tone='violet'] .metric-value {
      color: var(--text);
    }

    .metric[data-tone='amber'] .metric-value {
      color: var(--accent);
    }

    .metric-label {
      color: var(--subtle);
      font-size: 0.78rem;
      letter-spacing: 0.14em;
      text-transform: uppercase;
    }

    .metric-value {
      margin-top: 8px;
      font-family: Georgia, "Times New Roman", serif;
      font-size: 1.95rem;
      font-weight: 500;
      line-height: 1.1;
    }

    .metric-detail {
      margin-top: 8px;
      color: var(--muted);
      font-size: 0.9rem;
      line-height: 1.6;
      min-height: 1.45em;
    }

    pre {
      margin: 0;
      padding: 18px;
      border-radius: 18px;
      border: 1px solid var(--border);
      background: var(--panel-2);
      color: var(--text);
      overflow: auto;
      white-space: pre-wrap;
      word-break: break-word;
      line-height: 1.5;
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
      font-size: 0.92rem;
    }

    .raw-details {
      margin-top: 18px;
      border: 1px solid var(--border);
      border-radius: 18px;
      background: var(--panel-2);
      overflow: hidden;
    }

    .raw-details summary {
      cursor: pointer;
      list-style: none;
      padding: 16px 18px;
      color: var(--text);
      font-family: Georgia, "Times New Roman", serif;
      font-weight: 500;
      letter-spacing: 0;
      text-transform: none;
    }

    .raw-details summary::-webkit-details-marker {
      display: none;
    }

    .raw-details[open] summary {
      border-bottom: 1px solid var(--border);
    }

    .raw-details pre {
      border: 0;
      border-radius: 0;
      background: transparent;
      padding: 0 18px 18px;
    }

    .hint {
      margin-top: 10px;
      color: var(--muted);
      font-size: 0.92rem;
      line-height: 1.6;
    }

    .mono {
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
    }

    @media (max-width: 860px) {
      .hero,
      .controls {
        grid-template-columns: 1fr;
      }
    }
  </style>
</head>
<body>
  <main class="shell">
    <section class="hero">
      <div>
        <p class="eyebrow">AiApiGiaRe</p>
        <h1 class="title">AiApiGiaRe API Key Quota Viewer</h1>
        <p class="subtitle">Load <span class="mono">/v1/quota</span> with a real API key and inspect the usage summary returned for that key. The base URL is fixed to the server this page is served from.</p>
      </div>
      <div class="card hero-card">
        <div class="hero-stat">
          <div class="hero-stat-label">What this page does</div>
          <div class="hero-stat-value">Calls the live quota endpoint and renders a key-scoped snapshot in a warm, editorial card layout.</div>
        </div>
        <div class="hero-stat">
          <div class="hero-stat-label">Auth format</div>
          <div class="hero-stat-value mono">Authorization: Bearer &lt;api-key&gt;</div>
        </div>
      </div>
    </section>

    <section class="card panel">
      <div class="controls">
        <div class="field">
          <label for="apiKey">API key</label>
          <input id="apiKey" type="password" autocomplete="off" spellcheck="false" placeholder="Paste an API key" />
        </div>
      </div>

      <div class="actions" style="margin-top:16px;">
        <button id="loadBtn" class="primary" type="button">Load quota</button>
        <button id="copyBtn" class="secondary" type="button">Export</button>
      </div>

      <p id="status" class="status">Enter an API key, then load the usage snapshot.</p>

      <div class="card dashboard-toolbar">
        <div class="dashboard-toolbar-header">
          <div class="dashboard-toolbar-title">
            <p class="eyebrow" style="margin-bottom:0;">Usage Statistics</p>
            <h2>Usage Statistics</h2>
            <p>Choose a time range, refresh the live snapshot, or import and export a snapshot for offline review.</p>
          </div>
          <div class="toolbar-updated">Updated: <span id="updatedAt">Not loaded</span></div>
        </div>

        <div class="dashboard-toolbar-grid">
          <div class="toolbar-field">
            <label for="timeRange">Time Range</label>
            <select id="timeRange">
              <option value="24h" selected>Last 24 Hours</option>
              <option value="7d">Last 7 Days</option>
              <option value="30d">Last 30 Days</option>
              <option value="all">All Time</option>
            </select>
          </div>
          <div class="toolbar-actions">
            <button id="importBtn" class="secondary" type="button">Import</button>
            <button id="refreshBtn" class="secondary" type="button">Refresh</button>
          </div>
          <div class="toolbar-updated">Last selected key: <span id="heroKeyValue">No key loaded</span></div>
        </div>
      </div>

      <div class="section-title">Usage charts</div>
      <div class="chart-grid">
        <div class="chart-card">
          <div class="chart-label">Total requests</div>
          <div id="requestRing" class="ring-chart" style="--ring-value: 0; --ring-color: var(--accent);">
            <div class="ring-center">
              <div id="requestTotalValue" class="ring-value">0</div>
              <div id="requestTotalCount" class="ring-subtitle">Total requests</div>
            </div>
          </div>
          <div id="requestChartNote" class="chart-note">No requests recorded yet.</div>
        </div>
        <div class="chart-card">
          <div class="chart-label">Token mix</div>
          <div class="progress-track" id="tokenTrack" aria-label="Token composition">
            <span id="tokenCachedSegment" class="progress-segment cached" style="width: 0%;"></span>
            <span id="tokenReasoningSegment" class="progress-segment reasoning" style="width: 0%;"></span>
            <span id="tokenOtherSegment" class="progress-segment other" style="width: 0%;"></span>
          </div>
          <div id="tokenChartNote" class="chart-note">0 cached, 0 reasoning, 0 other tokens</div>
          <div class="legend-list">
            <div class="legend-item"><span class="legend-color cached"></span><span>Cached</span></div>
            <div class="legend-item"><span class="legend-color reasoning"></span><span>Reasoning</span></div>
            <div class="legend-item"><span class="legend-color other"></span><span>Other</span></div>
          </div>
        </div>
        <div class="chart-card">
          <div class="chart-label">Total Cost</div>
          <div class="cost-summary" style="margin-top:14px;">
            <div id="chartTotalCostValue" class="cost-summary-value">0</div>
            <div id="chartTotalCostNote" class="cost-summary-note">From the current credits snapshot.</div>
          </div>
        </div>
      </div>

      <div class="section-title">Token Type Breakdown</div>
      <div class="analysis-grid">
        <article class="analysis-card" data-accent="amber">
          <div class="analysis-header">
            <div>
              <h3>Token Type Breakdown</h3>
              <p>Input, output, cached, and reasoning tokens across the selected time range.</p>
            </div>
            <div class="period-tabs" role="tablist" aria-label="Token type breakdown period">
              <button type="button" class="period-tab active" data-period-tab="hour">By Hour</button>
              <button type="button" class="period-tab" data-period-tab="day">By Day</button>
            </div>
          </div>
          <div id="tokenBreakdownBody" class="analysis-body">
            <div class="empty-state">
              <div class="empty-state-title">No data loaded yet</div>
              <div class="empty-state-subtitle">Load a quota snapshot to see token composition for the selected range.</div>
            </div>
          </div>
        </article>

        <article class="analysis-card" data-accent="teal">
          <div class="analysis-header">
            <div>
              <h3>Cost Overview</h3>
              <p>Track credits used for the selected range.</p>
            </div>
            <div class="period-tabs" role="tablist" aria-label="Cost overview period">
              <button type="button" class="period-tab active" data-period-tab="hour">By Hour</button>
              <button type="button" class="period-tab" data-period-tab="day">By Day</button>
            </div>
          </div>
          <div id="costOverviewBody" class="analysis-body">
            <div class="empty-state">
              <div class="empty-state-title">No cost data loaded yet</div>
              <div class="empty-state-subtitle">Set a model price and load a quota snapshot to see cost stats.</div>
            </div>
          </div>
        </article>

        <article class="analysis-card" data-accent="slate">
          <div class="analysis-header">
            <div>
              <h3>Request Trends</h3>
              <p>Requests by time bucket for the selected range.</p>
            </div>
            <div class="period-tabs" role="tablist" aria-label="Request trends period">
              <button type="button" class="period-tab active" data-period-tab="hour">By Hour</button>
              <button type="button" class="period-tab" data-period-tab="day">By Day</button>
            </div>
          </div>
          <div id="requestTrendsBody" class="analysis-body is-empty">
            <div class="empty-state">
              <div class="empty-state-title">No Data Available</div>
              <div class="empty-state-subtitle">Request trends will appear here once the quota snapshot includes request details for the selected range.</div>
            </div>
          </div>
        </article>

        <article class="analysis-card" data-accent="violet">
          <div class="analysis-header">
            <div>
              <h3>Token Usage Trends</h3>
              <p>Total tokens by time bucket for the selected range.</p>
            </div>
            <div class="period-tabs" role="tablist" aria-label="Token usage trends period">
              <button type="button" class="period-tab active" data-period-tab="hour">By Hour</button>
              <button type="button" class="period-tab" data-period-tab="day">By Day</button>
            </div>
          </div>
          <div id="tokenUsageTrendsBody" class="analysis-body is-empty">
            <div class="empty-state">
              <div class="empty-state-title">No Data Available</div>
              <div class="empty-state-subtitle">Token usage trends need request details in the loaded quota snapshot.</div>
            </div>
          </div>
        </article>
      </div>

      <input id="importInput" type="file" accept="application/json,.json" hidden />

      <div class="section-title">Usage summary</div>
      <p class="section-subtitle">At a glance: request volume, token mix, and throughput for the selected key.</p>
      <div id="metrics" class="metrics">
        <div class="metric" data-tone="slate">
          <div class="metric-label">Total Requests</div>
          <div id="metric-total-requests-value" class="metric-value">0</div>
          <div id="metric-total-requests-detail" class="metric-detail"></div>
        </div>
        <div class="metric" data-tone="violet">
          <div class="metric-label">Total Tokens</div>
          <div id="metric-total-tokens-value" class="metric-value">0.00</div>
          <div id="metric-total-tokens-detail" class="metric-detail">Cached Tokens: 0.00 | Reasoning Tokens: 0.00</div>
        </div>
        <div class="metric" data-tone="amber">
          <div class="metric-label">Cached Tokens</div>
          <div id="metric-cached-tokens-value" class="metric-value">0.00</div>
          <div id="metric-cached-tokens-detail" class="metric-detail">Included in the token accounting for this key.</div>
        </div>
        <div class="metric" data-tone="violet">
          <div class="metric-label">Reasoning Tokens</div>
          <div id="metric-reasoning-tokens-value" class="metric-value">0.00</div>
          <div id="metric-reasoning-tokens-detail" class="metric-detail">Included in the token accounting for this key.</div>
        </div>
        <div class="metric" data-tone="green">
          <div class="metric-label">RPM</div>
          <div id="metric-rpm-value" class="metric-value">0.00</div>
          <div id="metric-rpm-detail" class="metric-detail">Requests per minute over the last 30 minutes.</div>
        </div>
        <div class="metric" data-tone="amber">
          <div class="metric-label">TPM</div>
          <div id="metric-tpm-value" class="metric-value">0.00</div>
          <div id="metric-tpm-detail" class="metric-detail">Tokens per minute over the last 30 minutes.</div>
        </div>
      </div>

      <div class="hint">Tip: pass <span class="mono">?key=...</span> in the URL to preload a specific key.</div>
    </section>
  </main>

  <script>
    const apiKeyInput = document.getElementById('apiKey');
    const loadBtn = document.getElementById('loadBtn');
    const copyBtn = document.getElementById('copyBtn');
    const statusNode = document.getElementById('status');
    const timeRangeSelect = document.getElementById('timeRange');
    const importBtn = document.getElementById('importBtn');
    const refreshBtn = document.getElementById('refreshBtn');
    const importInput = document.getElementById('importInput');
    const updatedAtNode = document.getElementById('updatedAt');
    const heroKeyNode = document.getElementById('heroKeyValue');
    const tokenBreakdownBody = document.getElementById('tokenBreakdownBody');
    const costOverviewBody = document.getElementById('costOverviewBody');
    const requestTrendsBody = document.getElementById('requestTrendsBody');
    const tokenUsageTrendsBody = document.getElementById('tokenUsageTrendsBody');
    const periodTabButtons = Array.from(document.querySelectorAll('[data-period-tab]'));

    const fixedBaseUrl = (window.location.origin && window.location.origin !== 'null')
      ? window.location.origin
      : 'http://127.0.0.1:8317';

    const modelPricesStorageKey = 'cli-proxy-model-prices-v2';
    const rateWindowMinutes = 30;

    let lastPayload = null;
    let activePeriod = 'hour';
    let quotaLoadInFlight = false;
    let quotaAutoRefreshTimer = null;

    function getQueryValue(name) {
      return new URLSearchParams(window.location.search).get(name) || '';
    }

    function stripCacheBusterFromUrl() {
      const currentUrl = new URL(window.location.href);
      if (!currentUrl.searchParams.has('nocache')) {
        return;
      }
      currentUrl.searchParams.delete('nocache');
      window.history.replaceState({}, '', currentUrl.pathname + currentUrl.search + currentUrl.hash);
    }

    function maskKey(value) {
      if (!value) {
        return '(empty)';
      }
      if (value.length <= 10) {
        return value;
      }
      return value.slice(0, 4) + '...' + value.slice(-4);
    }

    function formatCount(value) {
      const numeric = Number(value) || 0;
      return numeric.toLocaleString();
    }

    function formatDecimal(value) {
      const numeric = Number(value) || 0;
      return numeric.toLocaleString(undefined, {
        minimumFractionDigits: 2,
        maximumFractionDigits: 2
      });
    }

    function formatTimestamp(value) {
      if (!value) {
        return 'Not available';
      }
      if (typeof value === 'string' && value.startsWith('0001-01-01')) {
        return 'Not available';
      }
      const parsed = new Date(value);
      if (Number.isNaN(parsed.getTime())) {
        return String(value);
      }
      return parsed.toLocaleString();
    }

    function extractUsage(payload) {
      if (payload && typeof payload === 'object' && payload.usage && typeof payload.usage === 'object') {
        return payload.usage;
      }
      if (payload && typeof payload === 'object' && payload.models && typeof payload.models === 'object') {
        return payload;
      }
      return {};
    }

    function setStatus(message, isError) {
      statusNode.textContent = message;
      statusNode.classList.toggle('error', !!isError);
    }

    function setMetric(valueId, detailId, value, detail) {
      const valueNode = document.getElementById(valueId);
      const detailNode = document.getElementById(detailId);
      if (valueNode) {
        valueNode.textContent = value;
      }
      if (detailNode) {
        const hasDetail = typeof detail === 'string' && detail.length > 0;
        detailNode.textContent = hasDetail ? detail : '';
        detailNode.style.display = hasDetail ? '' : 'none';
      }
    }

    function padNumber(value) {
      return String(value).padStart(2, '0');
    }

    function parseTimestamp(value) {
      const parsed = new Date(value);
      if (Number.isNaN(parsed.getTime())) {
        return null;
      }
      return parsed;
    }

    function getSelectedRangeMs() {
      if (!timeRangeSelect) {
        return 24 * 60 * 60 * 1000;
      }
      switch (timeRangeSelect.value) {
        case '7d':
          return 7 * 24 * 60 * 60 * 1000;
        case '30d':
          return 30 * 24 * 60 * 60 * 1000;
        case 'all':
          return null;
        default:
          return 24 * 60 * 60 * 1000;
      }
    }

    function getSelectedRangeLabel() {
      if (!timeRangeSelect) {
        return 'Last 24 Hours';
      }
      const option = timeRangeSelect.options[timeRangeSelect.selectedIndex];
      return option ? option.textContent : 'Last 24 Hours';
    }

    function collectUsageRecords(summary) {
      const models = summary && summary.models && typeof summary.models === 'object' ? summary.models : {};
      const records = [];

      Object.keys(models).forEach(function(modelName) {
        const model = models[modelName];
        const details = model && Array.isArray(model.details) ? model.details : [];
        details.forEach(function(detail) {
          records.push({ modelName: modelName, detail: detail });
        });
      });

      records.sort(function(left, right) {
        const leftDate = parseTimestamp(left.detail && left.detail.timestamp);
        const rightDate = parseTimestamp(right.detail && right.detail.timestamp);
        const leftValue = leftDate ? leftDate.getTime() : 0;
        const rightValue = rightDate ? rightDate.getTime() : 0;
        return leftValue - rightValue;
      });

      return records;
    }

    function filterRecordsByRange(records, rangeMs) {
      if (rangeMs === null) {
        return records.slice();
      }
      const cutoff = Date.now() - rangeMs;
      return records.filter(function(record) {
        const date = parseTimestamp(record.detail && record.detail.timestamp);
        return date && date.getTime() >= cutoff;
      });
    }

    function buildBucketKey(date, period) {
      const year = String(date.getFullYear());
      const month = padNumber(date.getMonth() + 1);
      const day = padNumber(date.getDate());
      if (period === 'day') {
        return year + '-' + month + '-' + day;
      }
      return year + '-' + month + '-' + day + ' ' + padNumber(date.getHours());
    }

    function formatBucketLabel(bucketKey, period) {
      if (!bucketKey) {
        return '--';
      }
      const parts = bucketKey.split(' ');
      const dateParts = parts[0].split('-');
      const year = Number(dateParts[0]);
      const month = Number(dateParts[1]) - 1;
      const day = Number(dateParts[2]);

      if (period === 'day') {
        const date = new Date(year, month, day);
        return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
      }

      const hour = Number(parts[1] || 0);
      const date = new Date(year, month, day, hour);
      return date.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric' });
    }

    function createEmptyTotals() {
      return {
        totalRequests: 0,
        successRequests: 0,
        failedRequests: 0,
        totalTokens: 0,
        cachedTokens: 0,
        reasoningTokens: 0,
        inputTokens: 0,
        outputTokens: 0,
        totalCost: 0,
        rpm: 0,
        tpm: 0,
      };
    }

    function aggregateRecords(records, prices, rangeMs) {
      const totals = createEmptyTotals();
      const modelCosts = {};
      const modelTokens = {};
      let earliest = null;
      let latest = null;

      records.forEach(function(record) {
        const detail = record.detail || {};
        const timestamp = parseTimestamp(detail.timestamp);
        if (timestamp) {
          if (!earliest || timestamp < earliest) {
            earliest = timestamp;
          }
          if (!latest || timestamp > latest) {
            latest = timestamp;
          }
        }

        const tokens = detail.tokens && typeof detail.tokens === 'object' ? detail.tokens : {};
        const inputTokens = Math.max(Number(tokens.input_tokens) || 0, 0);
        const outputTokens = Math.max(Number(tokens.output_tokens) || 0, 0);
        const cachedTokens = Math.max(Number(tokens.cached_tokens) || Number(tokens.cache_tokens) || 0, 0);
        const reasoningTokens = Math.max(Number(tokens.reasoning_tokens) || 0, 0);
        const totalTokens = Math.max(Number(tokens.total_tokens) || 0, inputTokens + outputTokens + reasoningTokens + cachedTokens);
        const cost = calculateDetailCost(record.modelName, detail, prices);

        totals.totalRequests++;
        if (detail.failed) {
          totals.failedRequests++;
        } else {
          totals.successRequests++;
        }
        totals.totalTokens += totalTokens;
        totals.cachedTokens += cachedTokens;
        totals.reasoningTokens += reasoningTokens;
        totals.inputTokens += inputTokens;
        totals.outputTokens += outputTokens;
        totals.totalCost += cost;

        if (!modelCosts[record.modelName]) {
          modelCosts[record.modelName] = 0;
        }
        modelCosts[record.modelName] += cost;

        if (!modelTokens[record.modelName]) {
          modelTokens[record.modelName] = 0;
        }
        modelTokens[record.modelName] += totalTokens;
      });

      const windowMinutes = rangeMs === null
        ? Math.max((((latest ? latest.getTime() : 0) - (earliest ? earliest.getTime() : 0)) || 0) / 60000, 1)
        : Math.max(rangeMs / 60000, 1);
      totals.rpm = totals.totalRequests / windowMinutes;
      totals.tpm = totals.totalTokens / windowMinutes;

      return {
        totals: totals,
        modelCosts: modelCosts,
        modelTokens: modelTokens,
      };
    }

    function aggregateBuckets(records, prices, period) {
      const bucketMap = {};

      records.forEach(function(record) {
        const detail = record.detail || {};
        const timestamp = parseTimestamp(detail.timestamp);
        if (!timestamp) {
          return;
        }

        const tokens = detail.tokens && typeof detail.tokens === 'object' ? detail.tokens : {};
        const inputTokens = Math.max(Number(tokens.input_tokens) || 0, 0);
        const outputTokens = Math.max(Number(tokens.output_tokens) || 0, 0);
        const cachedTokens = Math.max(Number(tokens.cached_tokens) || Number(tokens.cache_tokens) || 0, 0);
        const reasoningTokens = Math.max(Number(tokens.reasoning_tokens) || 0, 0);
        const totalTokens = Math.max(Number(tokens.total_tokens) || 0, inputTokens + outputTokens + reasoningTokens + cachedTokens);
        const cost = calculateDetailCost(record.modelName, detail, prices);
        const bucketKey = buildBucketKey(timestamp, period);

        if (!bucketMap[bucketKey]) {
          bucketMap[bucketKey] = {
            key: bucketKey,
            label: formatBucketLabel(bucketKey, period),
            requests: 0,
            successRequests: 0,
            failedRequests: 0,
            totalTokens: 0,
            cachedTokens: 0,
            reasoningTokens: 0,
            inputTokens: 0,
            outputTokens: 0,
            totalCost: 0,
          };
        }

        const bucket = bucketMap[bucketKey];
        bucket.requests++;
        if (detail.failed) {
          bucket.failedRequests++;
        } else {
          bucket.successRequests++;
        }
        bucket.totalTokens += totalTokens;
        bucket.cachedTokens += cachedTokens;
        bucket.reasoningTokens += reasoningTokens;
        bucket.inputTokens += inputTokens;
        bucket.outputTokens += outputTokens;
        bucket.totalCost += cost;
      });

      return Object.keys(bucketMap).sort().map(function(key) {
        return bucketMap[key];
      });
    }

    function buildDashboardState(summary, quota, prices) {
      const records = collectUsageRecords(summary);
      const rangeMs = getSelectedRangeMs();
      const scopedRecords = records.length > 0 ? filterRecordsByRange(records, rangeMs) : [];
      const aggregation = records.length > 0
        ? aggregateRecords(scopedRecords, prices, rangeMs)
        : {
            totals: {
              totalRequests: Number(summary.total_requests ?? 0),
              successRequests: Number(summary.success_requests ?? 0),
              failedRequests: Number(summary.failed_requests ?? 0),
              totalTokens: Number(summary.total_tokens ?? 0),
              cachedTokens: Number(summary.cached_tokens ?? 0),
              reasoningTokens: Number(summary.reasoning_tokens ?? 0),
              inputTokens: 0,
              outputTokens: 0,
              totalCost: 0,
              rpm: Number(summary.rpm ?? 0),
              tpm: Number(summary.tpm ?? 0)
            },
            modelCosts: {},
            modelTokens: {}
          };

      const buckets = records.length > 0 ? aggregateBuckets(scopedRecords, prices, activePeriod) : [];
      const modelRows = Object.keys(aggregation.modelCosts).map(function(modelName) {
        return {
          modelName: modelName,
          cost: aggregation.modelCosts[modelName],
          tokens: aggregation.modelTokens[modelName] || 0,
        };
      }).sort(function(left, right) {
        return right.cost - left.cost;
      });

      return {
        summary: summary,
        quota: quota,
        prices: prices,
        records: scopedRecords,
        hasRecords: records.length > 0,
        totals: aggregation.totals,
        modelRows: modelRows,
        buckets: buckets,
        rangeLabel: getSelectedRangeLabel(),
        periodLabel: activePeriod === 'day' ? 'By Day' : 'By Hour'
      };
    }

    function updatePeriodTabs() {
      periodTabButtons.forEach(function(button) {
        const isActive = button.getAttribute('data-period-tab') === activePeriod;
        button.classList.toggle('active', isActive);
      });
    }

    function setUpdatedTimestamp(value) {
      if (!updatedAtNode) {
        return;
      }
      if (!value) {
        updatedAtNode.textContent = 'Not loaded';
        return;
      }
      const date = value instanceof Date ? value : new Date(value);
      if (Number.isNaN(date.getTime())) {
        updatedAtNode.textContent = 'Not loaded';
        return;
      }
      updatedAtNode.textContent = date.toLocaleTimeString();
    }

    function setHeroKey(value) {
      if (!heroKeyNode) {
        return;
      }
      heroKeyNode.textContent = value ? maskKey(value) : 'No key loaded';
    }

    function renderBarChart(buckets, valueKey, accentLabel) {
      if (!buckets || buckets.length === 0) {
        return '<div class="empty-state"><div class="empty-state-title">No Data Available</div><div class="empty-state-subtitle">There is no request detail data for this period yet.</div></div>';
      }

      const maxValue = buckets.reduce(function(maximum, bucket) {
        const numeric = Number(bucket[valueKey]) || 0;
        return numeric > maximum ? numeric : maximum;
      }, 0) || 1;

      let html = '<div class="trend-chart" aria-label="' + accentLabel + '">';
      buckets.forEach(function(bucket) {
        const numeric = Number(bucket[valueKey]) || 0;
        const height = numeric > 0 ? Math.max((numeric / maxValue) * 100, 6) : 6;
        const formattedValue = valueKey === 'requests' ? formatCount(numeric) : formatDecimal(numeric);
        html += '<div class="trend-bar">';
        html += '<div class="trend-bar-value">' + formattedValue + '</div>';
        html += '<div class="trend-bar-track"><div class="trend-bar-fill" style="height:' + height.toFixed(2) + '%"></div></div>';
        html += '<div class="trend-bar-label">' + bucket.label + '</div>';
        html += '</div>';
      });
      html += '</div>';
      return html;
    }

    function renderTokenBreakdown(state) {
      if (!tokenBreakdownBody) {
        return;
      }

      const totals = state.totals || createEmptyTotals();
      const totalTokens = Math.max(totals.inputTokens + totals.outputTokens + totals.cachedTokens + totals.reasoningTokens, totals.totalTokens, 0);
      if (!state.records.length && totalTokens <= 0) {
        tokenBreakdownBody.innerHTML = '<div class="empty-state"><div class="empty-state-title">No Data Available</div><div class="empty-state-subtitle">Load a quota snapshot with request details to see the token mix.</div></div>';
        return;
      }

      const segments = [
        { name: 'Input Tokens', value: totals.inputTokens, color: '#c96442' },
        { name: 'Output Tokens', value: totals.outputTokens, color: '#d97757' },
        { name: 'Cached Tokens', value: totals.cachedTokens, color: '#7c5c46' },
        { name: 'Reasoning Tokens', value: totals.reasoningTokens, color: '#f59e0b' }
      ];

      let ringGradient = '';
      let currentAngle = 0;
      segments.forEach(function(segment) {
        if (!segment.value || totalTokens <= 0) {
          return;
        }
        const nextAngle = currentAngle + (segment.value / totalTokens) * 360;
        if (ringGradient !== '') {
          ringGradient += ', ';
        }
        ringGradient += segment.color + ' ' + currentAngle.toFixed(2) + 'deg ' + nextAngle.toFixed(2) + 'deg';
        currentAngle = nextAngle;
      });

      let html = '<div class="breakdown-layout">';
      html += '<div class="breakdown-ring" style="--ring-slices:' + (ringGradient || '#d97706 0deg 360deg') + ';">';
      html += '<div class="breakdown-center"><div class="breakdown-total">' + formatDecimal(totalTokens) + '</div><div class="breakdown-caption">' + state.periodLabel + ' · ' + state.rangeLabel + '</div></div>';
      html += '</div>';
      html += '<div class="breakdown-list">';

      segments.forEach(function(segment) {
        const percent = totalTokens > 0 ? (segment.value / totalTokens) * 100 : 0;
        html += '<div class="breakdown-row">';
        html += '<div class="breakdown-row-head">';
        html += '<div class="breakdown-row-name"><span class="breakdown-swatch" style="background:' + segment.color + ';"></span>' + segment.name + '</div>';
        html += '<div>' + formatDecimal(segment.value) + ' (' + formatDecimal(percent) + '%)</div>';
        html += '</div>';
        html += '<div class="breakdown-track"><span class="breakdown-segment" style="width:' + percent.toFixed(2) + '%; background:' + segment.color + ';"></span></div>';
        html += '</div>';
      });

      html += '</div></div>';
      tokenBreakdownBody.innerHTML = html;
    }

    function renderCostOverview(state) {
      if (!costOverviewBody) {
        return;
      }

      const quotaCredits = Math.max(Number(state.quota && state.quota.credits_used ? state.quota.credits_used : 0), 0);
      const quotaDuration = Math.max(Number(state.quota && state.quota.duration_seconds ? state.quota.duration_seconds : 0), 0);
      const quotaSessions = Math.max(Number(state.quota && state.quota.sessions ? state.quota.sessions : 0), 0);
      const totalRequests = Math.max(Number(state.totals && state.totals.totalRequests ? state.totals.totalRequests : 0), 0);

      let html = '<div class="cost-grid">';
      html += '<div class="cost-summary"><div class="cost-summary-value">' + formatCount(quotaCredits) + '</div><div class="cost-summary-note">Credits used in the current snapshot.</div></div>';
      html += '<div class="empty-state"><div class="empty-state-title">Credits snapshot</div><div class="empty-state-subtitle">Requests: ' + formatCount(totalRequests) + ' · Sessions: ' + formatCount(quotaSessions) + ' · Duration: ' + formatCount(quotaDuration) + 's</div></div>';
      html += '</div>';
      costOverviewBody.innerHTML = html;
    }

    function renderTrendSection(targetNode, buckets, valueKey, emptyTitle, emptySubtitle, ariaLabel) {
      if (!targetNode) {
        return;
      }
      if (!buckets || buckets.length === 0) {
        targetNode.innerHTML = '<div class="empty-state"><div class="empty-state-title">' + emptyTitle + '</div><div class="empty-state-subtitle">' + emptySubtitle + '</div></div>';
        return;
      }
      targetNode.innerHTML = renderBarChart(buckets, valueKey, ariaLabel);
    }

    function renderDashboard(state) {
      const totals = state.totals || createEmptyTotals();
      const totalRequests = Math.max(totals.totalRequests || 0, 0);
      const totalTokens = Math.max(totals.totalTokens || 0, 0);
      const cachedTokens = Math.max(totals.cachedTokens || 0, 0);
      const reasoningTokens = Math.max(totals.reasoningTokens || 0, 0);
      const creditsUsed = Math.max(Number(state.quota && state.quota.credits_used ? state.quota.credits_used : 0), 0);
      const cachedPct = totalTokens > 0 ? (cachedTokens / totalTokens) * 100 : 0;
      const reasoningPct = totalTokens > 0 ? (reasoningTokens / totalTokens) * 100 : 0;
      const otherPct = totalTokens > 0 ? (Math.max(totalTokens - cachedTokens - reasoningTokens, 0) / totalTokens) * 100 : 0;

      setHeroKey(state.summary && state.summary.api_key ? state.summary.api_key : '');
      setUpdatedTimestamp(new Date());

      setMetric('metric-total-requests-value', 'metric-total-requests-detail', formatCount(totalRequests), totalRequests > 0 ? 'Requests recorded over ' + state.rangeLabel + '.' : 'No requests recorded yet for this key.');
      setMetric('metric-total-tokens-value', 'metric-total-tokens-detail', formatDecimal(totalTokens), 'Cached Tokens: ' + formatDecimal(cachedTokens) + ' | Reasoning Tokens: ' + formatDecimal(reasoningTokens));
      setMetric('metric-cached-tokens-value', 'metric-cached-tokens-detail', formatDecimal(cachedTokens), 'Included in the selected time range.');
      setMetric('metric-reasoning-tokens-value', 'metric-reasoning-tokens-detail', formatDecimal(reasoningTokens), 'Included in the selected time range.');
      setMetric('metric-rpm-value', 'metric-rpm-detail', formatDecimal(totals.rpm || 0), 'Requests per minute over ' + state.rangeLabel + '.');
      setMetric('metric-tpm-value', 'metric-tpm-detail', formatDecimal(totals.tpm || 0), 'Tokens per minute over ' + state.rangeLabel + '.');

      const chartTotalCostValue = document.getElementById('chartTotalCostValue');
      if (chartTotalCostValue) {
        chartTotalCostValue.textContent = formatCount(creditsUsed);
      }
      const chartTotalCostNote = document.getElementById('chartTotalCostNote');
      if (chartTotalCostNote) {
        chartTotalCostNote.textContent = 'Credits used in the current snapshot.';
      }

      const requestRing = document.getElementById('requestRing');
      if (requestRing) {
        requestRing.style.setProperty('--ring-value', totalRequests > 0 ? '100' : '0');
      }
      const requestTotalNode = document.getElementById('requestTotalValue');
      if (requestTotalNode) {
        requestTotalNode.textContent = formatCount(totalRequests);
      }
      const requestSubtitleNode = document.getElementById('requestTotalCount');
      if (requestSubtitleNode) {
        requestSubtitleNode.textContent = totalRequests > 0 ? 'Selected range: ' + state.rangeLabel : 'No requests recorded yet';
      }
      const requestNote = document.getElementById('requestChartNote');
      if (requestNote) {
        requestNote.textContent = totalRequests > 0 ? formatCount(totalRequests) + ' total requests over ' + state.rangeLabel + '.' : 'No requests recorded yet for this key.';
      }

      setProgressSegment('tokenCachedSegment', cachedPct);
      setProgressSegment('tokenReasoningSegment', reasoningPct);
      setProgressSegment('tokenOtherSegment', otherPct);

      const tokenNote = document.getElementById('tokenChartNote');
      if (tokenNote) {
        tokenNote.textContent = formatDecimal(cachedTokens) + ' cached, ' + formatDecimal(reasoningTokens) + ' reasoning, ' + formatDecimal(Math.max(totalTokens - cachedTokens - reasoningTokens, 0)) + ' other tokens';
      }

      const quotaSessionsValue = document.getElementById('quotaSessionsValue');
      if (quotaSessionsValue) {
        quotaSessionsValue.textContent = formatCount(state.quota && state.quota.sessions ? state.quota.sessions : 0);
      }
      const quotaSessionsDetail = document.getElementById('quotaSessionsDetail');
      if (quotaSessionsDetail) {
        quotaSessionsDetail.textContent = 'Active sessions: ' + formatCount(state.quota && state.quota.active_sessions ? state.quota.active_sessions : 0);
      }

      const quotaCreditsValue = document.getElementById('quotaCreditsValue');
      if (quotaCreditsValue) {
        quotaCreditsValue.textContent = formatCount(state.quota && state.quota.credits_used ? state.quota.credits_used : 0);
      }
      const quotaCreditsDetail = document.getElementById('quotaCreditsDetail');
      if (quotaCreditsDetail) {
        quotaCreditsDetail.textContent = 'Duration: ' + formatCount(state.quota && state.quota.duration_seconds ? state.quota.duration_seconds : 0) + 's';
      }

      const quotaLastSessionNote = document.getElementById('quotaLastSessionNote');
      if (quotaLastSessionNote) {
        quotaLastSessionNote.textContent = 'Last session: ' + formatTimestamp(state.quota && state.quota.last_session_at ? state.quota.last_session_at : null);
      }

      renderTokenBreakdown(state);
      renderCostOverview(state);
      renderTrendSection(requestTrendsBody, state.buckets, 'requests', 'No Data Available', 'Request trends will appear here once the quota snapshot includes request details for the selected range.', 'Request trends by ' + state.periodLabel);
      renderTrendSection(tokenUsageTrendsBody, state.buckets, 'totalTokens', 'No Data Available', 'Token usage trends need request details in the loaded quota snapshot.', 'Token usage trends by ' + state.periodLabel);
      updatePeriodTabs();
    }

    function loadModelPrices() {
      try {
        if (typeof localStorage === 'undefined') {
          return {};
        }
        const raw = localStorage.getItem(modelPricesStorageKey);
        if (!raw) {
          return {};
        }
        const parsed = JSON.parse(raw);
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
          return {};
        }
        const prices = {};
        Object.keys(parsed).forEach(function(modelName) {
          const entry = parsed[modelName];
          if (!entry || typeof entry !== 'object') {
            return;
          }
          const prompt = Number(entry.prompt);
          const completion = Number(entry.completion);
          const cache = Number(entry.cache);
          if (!Number.isFinite(prompt) && !Number.isFinite(completion) && !Number.isFinite(cache)) {
            return;
          }
          prices[modelName] = {
            prompt: Number.isFinite(prompt) && prompt >= 0 ? prompt : 0,
            completion: Number.isFinite(completion) && completion >= 0 ? completion : 0,
            cache: Number.isFinite(cache) && cache >= 0 ? cache : (Number.isFinite(prompt) && prompt >= 0 ? prompt : 0)
          };
        });
        return prices;
      } catch (error) {
        return {};
      }
    }

    function setProgressSegment(elementId, value) {
      const node = document.getElementById(elementId);
      if (!node) {
        return;
      }
      const bounded = Math.max(0, Math.min(100, Number(value) || 0));
      node.style.width = bounded.toFixed(2) + '%';
    }

    function updateCharts(summary, quota) {
      const totalRequests = Math.max(Number(summary.total_requests ?? 0), 0);
      const totalTokens = Math.max(Number(summary.total_tokens ?? 0), 0);
      const cachedTokens = Math.max(Number(summary.cached_tokens ?? 0), 0);
      const reasoningTokens = Math.max(Number(summary.reasoning_tokens ?? 0), 0);
      const otherTokens = Math.max(totalTokens - cachedTokens - reasoningTokens, 0);
      const cachedPct = totalTokens > 0 ? (cachedTokens / totalTokens) * 100 : 0;
      const reasoningPct = totalTokens > 0 ? (reasoningTokens / totalTokens) * 100 : 0;
      const otherPct = totalTokens > 0 ? (otherTokens / totalTokens) * 100 : 0;

      const requestRing = document.getElementById('requestRing');
      if (requestRing) {
        requestRing.style.setProperty('--ring-value', totalRequests > 0 ? '100' : '0');
      }
      const requestTotalNode = document.getElementById('requestTotalValue');
      if (requestTotalNode) {
        requestTotalNode.textContent = formatCount(totalRequests);
      }
      const requestSubtitleNode = document.getElementById('requestTotalCount');
      if (requestSubtitleNode) {
        requestSubtitleNode.textContent = totalRequests > 0 ? 'Selected range: ' + getSelectedRangeLabel() : 'No requests recorded yet';
      }
      const requestNote = document.getElementById('requestChartNote');
      if (requestNote) {
        requestNote.textContent = totalRequests > 0 ? formatCount(totalRequests) + ' total requests over ' + getSelectedRangeLabel() + '.' : 'No requests recorded yet for this key.';
      }

      setProgressSegment('tokenCachedSegment', cachedPct);
      setProgressSegment('tokenReasoningSegment', reasoningPct);
      setProgressSegment('tokenOtherSegment', otherPct);

      const tokenNote = document.getElementById('tokenChartNote');
      if (tokenNote) {
        tokenNote.textContent = formatDecimal(cachedTokens) + ' cached, ' + formatDecimal(reasoningTokens) + ' reasoning, ' + formatDecimal(otherTokens) + ' other tokens';
      }

      const quotaSessionsValue = document.getElementById('quotaSessionsValue');
      if (quotaSessionsValue) {
        quotaSessionsValue.textContent = formatCount(quota.sessions ?? 0);
      }
      const quotaSessionsDetail = document.getElementById('quotaSessionsDetail');
      if (quotaSessionsDetail) {
        quotaSessionsDetail.textContent = 'Active sessions: ' + formatCount(quota.active_sessions ?? 0);
      }

      const quotaCreditsValue = document.getElementById('quotaCreditsValue');
      if (quotaCreditsValue) {
        quotaCreditsValue.textContent = formatCount(quota.credits_used ?? 0);
      }
      const quotaCreditsDetail = document.getElementById('quotaCreditsDetail');
      if (quotaCreditsDetail) {
        quotaCreditsDetail.textContent = 'Duration: ' + formatCount(quota.duration_seconds ?? 0) + 's';
      }

      const quotaLastSessionNote = document.getElementById('quotaLastSessionNote');
      if (quotaLastSessionNote) {
        quotaLastSessionNote.textContent = 'Last session: ' + formatTimestamp(quota.last_session_at);
      }
    }

    function calculateDetailCost(modelName, detail, prices) {
      const price = prices[modelName];
      if (!price) {
        return 0;
      }
      const tokens = detail && typeof detail === 'object' && detail.tokens && typeof detail.tokens === 'object' ? detail.tokens : {};
      const inputTokens = Math.max(Number(tokens.input_tokens) || 0, 0);
      const outputTokens = Math.max(Number(tokens.output_tokens) || 0, 0);
      const cachedTokens = Math.max(Number(tokens.cached_tokens) || Number(tokens.cache_tokens) || 0, 0);
      const promptTokens = Math.max(inputTokens - cachedTokens, 0);
      return (promptTokens / 1000000) * price.prompt + (cachedTokens / 1000000) * price.cache + (outputTokens / 1000000) * price.completion;
    }

    function calculateTotalCost(models, prices) {
      let total = 0;
      Object.keys(models || {}).forEach(function(modelName) {
        const model = models[modelName];
        const details = model && Array.isArray(model.details) ? model.details : [];
        details.forEach(function(detail) {
          total += calculateDetailCost(modelName, detail, prices);
        });
      });
      return total;
    }

    function updateMetrics(payload) {
      const source = payload && typeof payload === 'object' ? payload : {};
      const summary = extractUsage(source);
      const quota = source.quota && typeof source.quota === 'object' ? source.quota : {};
      const prices = loadModelPrices();
      const state = buildDashboardState(summary, quota, prices);
      renderDashboard(state);
    }

    async function loadQuota() {
      const apiKey = apiKeyInput.value.trim();

      if (!apiKey) {
        setStatus('Enter an API key first.', true);
        return;
      }
      if (quotaLoadInFlight) {
        return;
      }
      quotaLoadInFlight = true;

      loadBtn.disabled = true;
      copyBtn.disabled = true;
      if (importBtn) {
        importBtn.disabled = true;
      }
      if (refreshBtn) {
        refreshBtn.disabled = true;
      }
      setStatus('Loading usage for ' + maskKey(apiKey) + '...');

      try {
        const normalizedBaseUrl = fixedBaseUrl.replace(/\/+$/, '');
        const response = await fetch(normalizedBaseUrl + '/v1/quota', {
          method: 'GET',
          headers: {
            'Authorization': 'Bearer ' + apiKey,
            'Accept': 'application/json'
          }
        });

        const text = await response.text();
        let payload = null;
        try {
          payload = text ? JSON.parse(text) : {};
        } catch (parseError) {
          throw new Error('Response was not valid JSON: ' + text.slice(0, 200));
        }

        if (!response.ok) {
          const message = payload && payload.error ? payload.error : ('HTTP ' + response.status);
          throw new Error(message);
        }

        updateMetrics(payload);
        lastPayload = payload;
        setHeroKey(payload && payload.usage && payload.usage.api_key ? payload.usage.api_key : apiKey);
        setStatus('Loaded usage for ' + maskKey(apiKey) + ' from the current server.');
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        setStatus('Failed to load usage: ' + message, true);
      } finally {
        quotaLoadInFlight = false;
        loadBtn.disabled = false;
        copyBtn.disabled = !lastPayload;
        if (importBtn) {
          importBtn.disabled = false;
        }
        if (refreshBtn) {
          refreshBtn.disabled = false;
        }
      }
    }

    async function exportSnapshot() {
      if (!lastPayload) {
        setStatus('Load a quota response first before exporting JSON.', true);
        return;
      }
      try {
        const blob = new Blob([JSON.stringify(lastPayload, null, 2)], { type: 'application/json' });
        const url = window.URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = 'quota-snapshot.json';
        document.body.appendChild(link);
        link.click();
        link.remove();
        window.URL.revokeObjectURL(url);
        setStatus('Exported the latest response JSON.');
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        setStatus('Failed to export JSON: ' + message, true);
      }
    }

    async function importSnapshotFile() {
      if (!importInput || !importInput.files || importInput.files.length === 0) {
        return;
      }
      const file = importInput.files[0];
      try {
        const text = await file.text();
        const payload = text ? JSON.parse(text) : {};
        lastPayload = payload;
        updateMetrics(payload);
        setStatus('Imported snapshot from ' + file.name + '.');
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        setStatus('Failed to import JSON: ' + message, true);
      } finally {
        if (importInput) {
          importInput.value = '';
        }
        copyBtn.disabled = !lastPayload;
      }
    }

    function handlePeriodChange(period) {
      if (period !== 'hour' && period !== 'day') {
        return;
      }
      activePeriod = period;
      updatePeriodTabs();
      if (lastPayload) {
        updateMetrics(lastPayload);
      }
    }

    function startQuotaAutoRefresh() {
      if (quotaAutoRefreshTimer !== null) {
        window.clearInterval(quotaAutoRefreshTimer);
        quotaAutoRefreshTimer = null;
      }
      quotaAutoRefreshTimer = window.setInterval(function() {
        if (document.hidden || !apiKeyInput.value.trim()) {
          return;
        }
        void loadQuota();
      }, 60000);
    }

    stripCacheBusterFromUrl();
    apiKeyInput.value = getQueryValue('key');

    loadBtn.addEventListener('click', function() {
      void loadQuota();
    });

    copyBtn.addEventListener('click', function() {
      void exportSnapshot();
    });

    if (importBtn) {
      importBtn.addEventListener('click', function() {
        if (importInput) {
          importInput.click();
        }
      });
    }

    if (refreshBtn) {
      refreshBtn.addEventListener('click', function() {
        void loadQuota();
      });
    }

    if (importInput) {
      importInput.addEventListener('change', function() {
        void importSnapshotFile();
      });
    }

    if (timeRangeSelect) {
      timeRangeSelect.addEventListener('change', function() {
        if (lastPayload) {
          updateMetrics(lastPayload);
        }
      });
    }

    document.addEventListener('visibilitychange', function() {
      if (!document.hidden && apiKeyInput.value.trim()) {
        void loadQuota();
      }
    });

    window.addEventListener('pageshow', function(event) {
      if (event.persisted && apiKeyInput.value.trim()) {
        window.location.reload();
      }
    });

    window.addEventListener('focus', function() {
      if (apiKeyInput.value.trim()) {
        void loadQuota();
      }
    });

    periodTabButtons.forEach(function(button) {
      button.addEventListener('click', function() {
        handlePeriodChange(button.getAttribute('data-period-tab') || 'hour');
      });
    });

    apiKeyInput.addEventListener('keydown', function(event) {
      if (event.key === 'Enter') {
        event.preventDefault();
        void loadQuota();
      }
    });

    copyBtn.disabled = true;

    startQuotaAutoRefresh();

    if (apiKeyInput.value.trim()) {
      void loadQuota();
    }
  </script>
</body>
</html>`

func (s *Server) serveAPIKeyQuotaViewer(c *gin.Context) {
	if c != nil && c.Request != nil && c.Query("nocache") == "" {
		targetURL := *c.Request.URL
		query := targetURL.Query()
		query.Set("nocache", fmt.Sprintf("%d", time.Now().UTC().UnixNano()))
		targetURL.RawQuery = query.Encode()
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Header("Surrogate-Control", "no-store")
		c.Header("Clear-Site-Data", `"cache"`)
		c.Redirect(http.StatusTemporaryRedirect, targetURL.String())
		return
	}
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("Surrogate-Control", "no-store")
	c.Header("Clear-Site-Data", `"cache"`)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(apiKeyQuotaViewerHTML))
}
