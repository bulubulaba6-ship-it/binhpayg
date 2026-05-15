#!/bin/sh

echo "🚀 Starting Industrialization Proxy Stack..."

# Graceful shutdown handler
WIREPROXY_PID=""
SERVER_PID=""
cleanup() {
  echo "🛑 Shutdown signal received. Stopping all processes..."
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null || true
  [ -n "$WIREPROXY_PID" ] && kill "$WIREPROXY_PID" 2>/dev/null || true
  exit 0
}
trap cleanup INT TERM

# 1. Điền thông tin từ ENV vào wireproxy.conf (Runtime Templating)
# Chỉ chạy tunnel nếu có Private Key
if [ -f "/CLIProxyAPI/wireproxy.conf" ] && [ -n "$WG_PRIVATE_KEY" ]; then
    echo "🔧 Injecting environment variables into wireproxy.conf..."
    sed -i "s|\${WG_PRIVATE_KEY}|$WG_PRIVATE_KEY|g" /CLIProxyAPI/wireproxy.conf
    sed -i "s|\${WG_ADDRESS}|$WG_ADDRESS|g" /CLIProxyAPI/wireproxy.conf
    sed -i "s|\${WG_PEER_PUBLIC_KEY}|$WG_PEER_PUBLIC_KEY|g" /CLIProxyAPI/wireproxy.conf
    sed -i "s|\${WG_ENDPOINT}|$WG_ENDPOINT|g" /CLIProxyAPI/wireproxy.conf

    echo "📡 Starting wireproxy tunnel..."
    /usr/local/bin/wireproxy -c /CLIProxyAPI/wireproxy.conf &
    WIREPROXY_PID=$!
    echo "✅ wireproxy started with PID $WIREPROXY_PID"

    # 2. Đợi tunnel ổn định (Dynamic Health Check via nc)
    echo "⏳ Waiting for wireproxy to listen on port 1080..."
    MAX_WAIT=10
    COUNT=0
    while ! nc -z 127.0.0.1 1080 2>/dev/null; do
      COUNT=$((COUNT + 1))
      if [ "$COUNT" -ge "$MAX_WAIT" ]; then
        echo "❌ ERROR: wireproxy failed to start in time. Exiting..."
        kill "$WIREPROXY_PID" 2>/dev/null || true
        exit 1
      fi
      sleep 1
    done
    echo "✅ Tunnel is UP and listening!"
else
    echo "ℹ️ No WireGuard credentials found. Running in DIRECT mode."
fi

# 3. Chạy CLIProxyAPI (Background)
echo "🤖 Starting CLIProxyAPI Server..."
./CLIProxyAPI --config config.yaml &
SERVER_PID=$!
echo "✅ CLIProxyAPI started with PID $SERVER_PID"

echo "✅ Industrial Stack is UP. Monitoring processes..."

# 4. Giám sát liên tục - Tự hồi sinh Proxy, thoát nếu App chính chết
RETRIES=0
MAX_PROXY_RETRIES=5

while sleep 5; do
  # Giám sát wireproxy (chỉ khi nó được bật)
  if [ -n "$WIREPROXY_PID" ]; then
    if ! kill -0 "$WIREPROXY_PID" 2>/dev/null; then
      RETRIES=$((RETRIES + 1))
      echo "⚠️ WARNING: wireproxy died (attempt $RETRIES/$MAX_PROXY_RETRIES). Restarting..."

      if [ "$RETRIES" -ge "$MAX_PROXY_RETRIES" ]; then
        echo "❌ CRITICAL: wireproxy failed $MAX_PROXY_RETRIES times. Exiting container..."
        kill "$SERVER_PID" 2>/dev/null || true
        exit 1
      fi

      /usr/local/bin/wireproxy -c /CLIProxyAPI/wireproxy.conf &
      WIREPROXY_PID=$!
      sleep 2
    else
      # Đang chạy ổn định, reset bộ đếm
      RETRIES=0
    fi
  fi

  # Giám sát server chính - nếu chết thì thoát ngay để Railway restart
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "❌ CRITICAL: CLIProxyAPI server died. Exiting container..."
    [ -n "$WIREPROXY_PID" ] && kill "$WIREPROXY_PID" 2>/dev/null || true
    exit 1
  fi
done
