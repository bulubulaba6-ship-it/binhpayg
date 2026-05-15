# Railway Setup

Paste these environment variables into the Railway Dashboard:

```env
PGSTORE_DSN=postgres://avnadmin:<INSERT_REAL_PASSWORD>@pg-34ab11a6-zok210305-4ca5.g.aivencloud.com:10436/defaultdb?sslmode=require
MANAGEMENT_PASSWORD=trumcuoi213*
PORT=8317
```

Notes:
- Keep the PostgreSQL password private.
- Railway should build from the root `Dockerfile`.
- `config.yaml` is already set to bind on all interfaces.
- Do NOT set `PROXY_URL` on Railway — VPS IPs are clean and should connect directly to AI providers.

---

## Proxy Configuration (Per-Environment)

The proxy is configured via the `PROXY_URL` environment variable.
**Priority: `PROXY_URL` env > `routing.upstream-proxy` in config.yaml > none (direct).**

| Deployment | PROXY_URL | Reason |
|---|---|---|
| Railway / VPS | ❌ Not set | VPS IP is clean, direct is fastest |
| Jetson (behind firewall) | `socks5h://127.0.0.1:1080` | Route through MikroTik WireGuard tunnel |
| Local dev with wireproxy | `socks5h://127.0.0.1:1080` | Same as Jetson |

### To enable WireGuard proxy on Jetson / local:
```bash
# Start wireproxy first
wireproxy -c wireproxy.conf

# Then run the server with proxy enabled
PROXY_URL=socks5h://127.0.0.1:1080 ./CLIProxyAPI --config config.yaml
```

### To verify proxy is NOT leaking DNS:
```bash
# Should show your MikroTik/home IP, NOT the VPS/Jetson IP
curl --socks5-hostname 127.0.0.1:1080 https://ifconfig.me
```
