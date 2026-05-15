#!/usr/bin/env pwsh
# =============================================================
#  wireproxy-setup.ps1
#  Tải, cấu hình và khởi động wireproxy trên máy local (Windows)
#  Dùng để test Case 7: Live proxy tunnel trước khi deploy
# =============================================================

param(
    [string]$PrivateKey     = $env:WG_PRIVATE_KEY,
    [string]$Address        = $env:WG_ADDRESS,
    [string]$PeerPublicKey  = $env:WG_PEER_PUBLIC_KEY,
    [string]$Endpoint       = $env:WG_ENDPOINT,
    [int]   $SocksPort      = 1080,
    [switch]$Stop
)

$WIREPROXY_VERSION = "v1.0.7"
$WIREPROXY_URL     = "https://github.com/octeep/wireproxy/releases/download/$WIREPROXY_VERSION/wireproxy_windows_amd64.tar.gz"
$WIREPROXY_EXE     = "$PSScriptRoot\wireproxy.exe"
$CONF_RUNTIME      = "$PSScriptRoot\wireproxy-runtime.conf"
$PID_FILE          = "$PSScriptRoot\wireproxy.pid"

# ── STOP mode ───────────────────────────────────────────────
if ($Stop) {
    if (Test-Path $PID_FILE) {
        $pid = Get-Content $PID_FILE
        Stop-Process -Id $pid -Force -ErrorAction SilentlyContinue
        Remove-Item $PID_FILE -ErrorAction SilentlyContinue
        Remove-Item $CONF_RUNTIME -ErrorAction SilentlyContinue
        Write-Host "✅ wireproxy stopped." -ForegroundColor Green
    } else {
        Write-Host "ℹ️ wireproxy is not running." -ForegroundColor Yellow
    }
    exit 0
}

Write-Host "═══════════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🚀 wireproxy Setup Script" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════════" -ForegroundColor Cyan

# ── VALIDATE credentials ─────────────────────────────────────
if (-not $PrivateKey -or -not $Address -or -not $PeerPublicKey -or -not $Endpoint) {
    Write-Host @"

❌ Missing WireGuard credentials. Provide them via:

  Option 1 — Params:
    .\wireproxy-setup.ps1 ``
      -PrivateKey    "YOUR_PRIVATE_KEY" ``
      -Address       "20.20.20.2/24" ``
      -PeerPublicKey "PEER_PUBLIC_KEY" ``
      -Endpoint      "home.yourddns.cloud:51820"

  Option 2 — Env vars (recommended):
    `$env:WG_PRIVATE_KEY    = "YOUR_PRIVATE_KEY"
    `$env:WG_ADDRESS        = "20.20.20.2/24"
    `$env:WG_PEER_PUBLIC_KEY = "PEER_PUBLIC_KEY"
    `$env:WG_ENDPOINT       = "home.yourddns.cloud:51820"
    .\wireproxy-setup.ps1

"@ -ForegroundColor Red
    exit 1
}

# ── DOWNLOAD wireproxy if not present ───────────────────────
if (-not (Test-Path $WIREPROXY_EXE)) {
    Write-Host "`n[1/4] Downloading wireproxy $WIREPROXY_VERSION..." -ForegroundColor Yellow
    $tmpTar = "$env:TEMP\wireproxy.tar.gz"
    try {
        Invoke-WebRequest -Uri $WIREPROXY_URL -OutFile $tmpTar -UseBasicParsing
        # Extract using tar (built-in Windows 10+)
        tar -xzf $tmpTar -C $PSScriptRoot wireproxy.exe 2>$null
        if (-not (Test-Path $WIREPROXY_EXE)) {
            # Some releases put the binary at root without .exe
            tar -xzf $tmpTar -C $env:TEMP wireproxy 2>$null
            if (Test-Path "$env:TEMP\wireproxy") {
                Move-Item "$env:TEMP\wireproxy" $WIREPROXY_EXE
            }
        }
        Remove-Item $tmpTar -ErrorAction SilentlyContinue
        Write-Host "  ✅ Downloaded to: $WIREPROXY_EXE" -ForegroundColor Green
    } catch {
        Write-Host "  ❌ Download failed: $_" -ForegroundColor Red
        Write-Host "  Please download manually from:" -ForegroundColor Yellow
        Write-Host "  $WIREPROXY_URL" -ForegroundColor Gray
        exit 1
    }
} else {
    Write-Host "`n[1/4] wireproxy already exists, skipping download." -ForegroundColor Gray
}

# ── GENERATE runtime config ──────────────────────────────────
Write-Host "`n[2/4] Generating runtime config..." -ForegroundColor Yellow
@"
[Interface]
PrivateKey = $PrivateKey
Address = $Address
DNS = 1.1.1.1
MTU = 1280

[Peer]
PublicKey = $PeerPublicKey
Endpoint = $Endpoint
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25

[Socks5]
BindAddress = 127.0.0.1:$SocksPort
"@ | Set-Content $CONF_RUNTIME -Encoding UTF8
Write-Host "  ✅ Config written to: $CONF_RUNTIME" -ForegroundColor Green
Write-Host "  ⚠️ This file contains your private key — NEVER commit it to Git!" -ForegroundColor Yellow

# ── START wireproxy ──────────────────────────────────────────
Write-Host "`n[3/4] Starting wireproxy tunnel..." -ForegroundColor Yellow

# Kill any existing instance
if (Test-Path $PID_FILE) {
    $oldPid = Get-Content $PID_FILE
    Stop-Process -Id $oldPid -Force -ErrorAction SilentlyContinue
    Remove-Item $PID_FILE
}

$proc = Start-Process -FilePath $WIREPROXY_EXE `
    -ArgumentList "-c", $CONF_RUNTIME `
    -WindowStyle Hidden `
    -PassThru

$proc.Id | Set-Content $PID_FILE
Write-Host "  ✅ wireproxy started (PID: $($proc.Id))" -ForegroundColor Green

# ── WAIT for SOCKS5 to be ready ─────────────────────────────
Write-Host "`n[4/4] Waiting for SOCKS5 port $SocksPort to open..." -ForegroundColor Yellow
$maxWait = 15
$ready = $false
for ($i = 1; $i -le $maxWait; $i++) {
    Start-Sleep -Seconds 1
    try {
        $tcp = New-Object System.Net.Sockets.TcpClient
        $tcp.Connect("127.0.0.1", $SocksPort)
        $tcp.Close()
        $ready = $true
        break
    } catch {}
    Write-Host "  Waiting... ($i/$maxWait)" -ForegroundColor Gray
}

if (-not $ready) {
    Write-Host "  ❌ wireproxy did not open port in time. Check your WG credentials!" -ForegroundColor Red
    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    Remove-Item $PID_FILE -ErrorAction SilentlyContinue
    exit 1
}

Write-Host "  ✅ Tunnel is UP on socks5h://127.0.0.1:$SocksPort!" -ForegroundColor Green

# ── VERIFY egress IP ─────────────────────────────────────────
Write-Host "`n[VERIFY] Checking egress IP through tunnel..." -ForegroundColor Yellow
try {
    # Use .NET WebClient with SOCKS5 proxy
    Add-Type @"
using System;
using System.Net;
using System.Net.Sockets;
using System.IO;

public class Socks5Client {
    public static string GetUrl(string host, int port, string url) {
        Uri uri = new Uri(url);
        TcpClient client = new TcpClient(host, port);
        NetworkStream stream = client.GetStream();
        // SOCKS5 handshake
        stream.Write(new byte[]{5,1,0}, 0, 3);
        byte[] resp = new byte[2];
        stream.Read(resp, 0, 2);
        // Connect request
        byte[] req = new byte[10];
        req[0]=5; req[1]=1; req[2]=0; req[3]=1;
        byte[] addr = System.Net.IPAddress.Parse(uri.Host).GetAddressBytes();
        Array.Copy(addr, 0, req, 4, 4);
        req[8]=(byte)(uri.Port>>8); req[9]=(byte)(uri.Port&0xff);
        stream.Write(req, 0, 10);
        byte[] cresp = new byte[10];
        stream.Read(cresp, 0, 10);
        // HTTP request
        string http = "GET / HTTP/1.0\r\nHost: " + uri.Host + "\r\nConnection: close\r\n\r\n";
        byte[] httpBytes = System.Text.Encoding.ASCII.GetBytes(http);
        stream.Write(httpBytes, 0, httpBytes.Length);
        StreamReader reader = new StreamReader(stream);
        string body = reader.ReadToEnd();
        client.Close();
        // Return last part (actual IP)
        string[] parts = body.Split(new[]{"\r\n\r\n"},StringSplitOptions.None);
        return parts.Length > 1 ? parts[parts.Length-1].Trim() : body.Trim();
    }
}
"@
    $egressIp = [Socks5Client]::GetUrl("127.0.0.1", $SocksPort, "http://ifconfig.me/ip")
    Write-Host "  ✅ Your egress IP = $egressIp (should be MikroTik home IP)" -ForegroundColor Green
} catch {
    Write-Host "  ⚠️ Could not check egress IP ($($_.Exception.Message))" -ForegroundColor Yellow
    Write-Host "  Tunnel is still UP — verify manually:" -ForegroundColor Gray
    Write-Host "  curl --socks5-hostname 127.0.0.1:$SocksPort https://ifconfig.me" -ForegroundColor Gray
}

Write-Host @"

═══════════════════════════════════════════════
  ✅ wireproxy is READY
  SOCKS5 Proxy : socks5h://127.0.0.1:$SocksPort
  PID          : $($proc.Id)

  Now start CLIProxyAPI:
    `$env:PROXY_URL = "socks5h://127.0.0.1:$SocksPort"
    .\test-server.exe --config config.yaml

  To stop wireproxy later:
    .\scratch\wireproxy-setup.ps1 -Stop
═══════════════════════════════════════════════
"@ -ForegroundColor Cyan
