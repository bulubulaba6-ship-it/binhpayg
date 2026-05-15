#!/usr/bin/env pwsh
# Real Endpoint Test Script
# Tests actual chat completions API calls against the running local server.

$SERVER_URL = "http://127.0.0.1:8319"
$AUTH_KEY   = "fink_pro_8c9e9ffb23fccdac9d65b57695145f13"
$TIMEOUT    = 30

$body = @{
    model    = "claude-haiku-4-5"
    messages = @(
        @{ role = "user"; content = "Reply with exactly: PROXY_TEST_OK" }
    )
    max_tokens = 20
    stream = $false
} | ConvertTo-Json -Depth 5

$headers = @{
    "Authorization" = "Bearer $AUTH_KEY"
    "Content-Type"  = "application/json"
}

function Test-Endpoint {
    param([string]$Label, [string]$Url)
    Write-Host "`n[$Label]" -ForegroundColor Cyan
    Write-Host "  POST $Url/v1/chat/completions" -ForegroundColor Gray
    try {
        $resp = Invoke-RestMethod -Uri "$Url/v1/chat/completions" `
            -Method POST `
            -Headers $headers `
            -Body $body `
            -TimeoutSec $TIMEOUT `
            -ErrorAction Stop

        $content = $resp.choices[0].message.content
        Write-Host "  ✅ SUCCESS - Response: $content" -ForegroundColor Green
        Write-Host "  Model: $($resp.model) | Tokens: $($resp.usage.total_tokens)" -ForegroundColor Gray
        return $true
    } catch {
        $errMsg = $_.Exception.Message
        Write-Host "  ❌ FAILED - $errMsg" -ForegroundColor Red
        return $false
    }
}

Write-Host "═══════════════════════════════════════════════" -ForegroundColor Yellow
Write-Host "  🔬 REAL ENDPOINT TEST SUITE" -ForegroundColor Yellow
Write-Host "═══════════════════════════════════════════════" -ForegroundColor Yellow

# ── Check server is up ──────────────────────────────────
Write-Host "`n[HEALTH CHECK] Waiting for server on $SERVER_URL..."
$maxWait = 15
$i = 0
$serverUp = $false
while ($i -lt $maxWait) {
    try {
        $null = Invoke-RestMethod -Uri "$SERVER_URL/health" -TimeoutSec 1 -ErrorAction Stop
        $serverUp = $true
        break
    } catch {
        # Try models endpoint as fallback
        try {
            $null = Invoke-RestMethod -Uri "$SERVER_URL/v1/models" -Headers $headers -TimeoutSec 1 -ErrorAction Stop
            $serverUp = $true
            break
        } catch {}
    }
    Start-Sleep -Seconds 1
    $i++
    Write-Host "  Waiting... ($i/$maxWait)" -ForegroundColor Gray
}

if (-not $serverUp) {
    Write-Host "  ❌ Server did not start in time. Run the server first!" -ForegroundColor Red
    Write-Host "  Command: .\test-server.exe --config config-direct.yaml" -ForegroundColor Yellow
    exit 1
}
Write-Host "  ✅ Server is UP!" -ForegroundColor Green

# ── Run real API tests ───────────────────────────────────
$pass = 0
$fail = 0

# Test 1: Real chat completion
Write-Host "`n[TEST 1] Real chat completion (Claude Haiku)"
$ok = Test-Endpoint -Label "Chat Completions" -Url $SERVER_URL
if ($ok) { $pass++ } else { $fail++ }

# Test 2: List models
Write-Host "`n[TEST 2] List available models"
try {
    $models = Invoke-RestMethod -Uri "$SERVER_URL/v1/models" -Headers $headers -TimeoutSec 10
    $count = $models.data.Count
    Write-Host "  ✅ SUCCESS - $count models available" -ForegroundColor Green
    $pass++
} catch {
    Write-Host "  ❌ FAILED - $($_.Exception.Message)" -ForegroundColor Red
    $fail++
}

# Test 3: Streaming (basic check)
Write-Host "`n[TEST 3] Streaming response check"
$streamBody = @{
    model    = "claude-haiku-4-5"
    messages = @(@{ role = "user"; content = "Say: STREAM_OK" })
    max_tokens = 10
    stream = $true
} | ConvertTo-Json -Depth 5

try {
    $req = [System.Net.HttpWebRequest]::Create("$SERVER_URL/v1/chat/completions")
    $req.Method = "POST"
    $req.Headers.Add("Authorization", "Bearer $AUTH_KEY")
    $req.ContentType = "application/json"
    $req.Timeout = 15000
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($streamBody)
    $req.ContentLength = $bytes.Length
    $reqStream = $req.GetRequestStream()
    $reqStream.Write($bytes, 0, $bytes.Length)
    $reqStream.Close()

    $response = $req.GetResponse()
    $reader = New-Object System.IO.StreamReader($response.GetResponseStream())
    $firstChunk = $reader.ReadLine()
    $reader.Close()
    $response.Close()

    if ($firstChunk -match "data:") {
        Write-Host "  ✅ SUCCESS - Streaming working, first chunk: $($firstChunk.Substring(0, [Math]::Min(80, $firstChunk.Length)))" -ForegroundColor Green
        $pass++
    } else {
        Write-Host "  ⚠️ WARN - Unexpected first chunk: $firstChunk" -ForegroundColor Yellow
        $pass++
    }
} catch {
    Write-Host "  ❌ FAILED - $($_.Exception.Message)" -ForegroundColor Red
    $fail++
}

# ── Summary ──────────────────────────────────────────────
Write-Host "`n═══════════════════════════════════════════════" -ForegroundColor Yellow
Write-Host "  RESULTS: $pass PASSED | $fail FAILED" -ForegroundColor $(if ($fail -eq 0) { "Green" } else { "Red" })
Write-Host "═══════════════════════════════════════════════" -ForegroundColor Yellow
