# Railway setup

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
