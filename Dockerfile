FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.BuildDate=${BUILD_DATE}'" -o ./CLIProxyAPI ./cmd/server/

FROM alpine:3.23

RUN apk add --no-cache tzdata netcat-openbsd

RUN mkdir /CLIProxyAPI

# Install wireproxy (Userspace WireGuard to SOCKS5 proxy sidecar)
ADD https://github.com/octeep/wireproxy/releases/download/v1.0.7/wireproxy_linux_amd64.tar.gz /tmp/wireproxy.tar.gz
RUN tar -xzf /tmp/wireproxy.tar.gz -C /usr/local/bin/ wireproxy && chmod +x /usr/local/bin/wireproxy && rm /tmp/wireproxy.tar.gz

COPY --from=builder ./app/CLIProxyAPI /CLIProxyAPI/CLIProxyAPI

COPY config.example.yaml /CLIProxyAPI/config.example.yaml
COPY wireproxy.conf /CLIProxyAPI/wireproxy.conf
COPY entrypoint.sh /CLIProxyAPI/entrypoint.sh
RUN chmod +x /CLIProxyAPI/entrypoint.sh

WORKDIR /CLIProxyAPI

EXPOSE 8317

ENV TZ=Asia/Shanghai

RUN cp /usr/share/zoneinfo/${TZ} /etc/localtime && echo "${TZ}" > /etc/timezone

CMD ["./entrypoint.sh"]
