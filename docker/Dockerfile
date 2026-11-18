# HDRezka server for Lampa / Apple TV.
#
# Контекст сборки — корень репозитория (нужны и src/ плагина, и server/):
#   docker build -f docker/Dockerfile -t hdrezka-server:0.1.0 .
# Мультиархитектурный образ (amd64 + arm64, нужен для Umbrel):
#   docker buildx build -f docker/Dockerfile --platform linux/amd64,linux/arm64 -t <registry>/hdrezka-server:0.1.0 --push .
#
# Сборочные стадии выполняются на платформе сборщика (BUILDPLATFORM): Go кросс-компилирует без эмуляции,
# а JS плагина от архитектуры не зависит.

# ---------- плагин Lampa: src/ → dist/hdrezka.js ----------
FROM --platform=$BUILDPLATFORM node:22-alpine AS plugin
WORKDIR /src
COPY package.json package-lock.json build.mjs modules.json ./
RUN npm ci --no-audit --no-fund
COPY src ./src
RUN node build.mjs

# ---------- сервер: Go ----------
# Зависимости скачиваются по go.mod/go.sum: go.mod фиксирует точный коммит go-hdrezka, go.sum — контрольную сумму
# его кода. Если скачанное отличается хоть на байт, сборка падает (checksum mismatch); go mod verify проверяет ещё раз.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS server
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS=-mod=readonly
COPY server/go.mod server/go.sum ./
RUN go mod download && go mod verify
COPY server/ ./
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/hdrezka-server .

# ---------- рабочий образ: только бинарник, сборка плагина и корневые сертификаты ----------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/hdrezka-server /app/hdrezka-server
COPY --from=plugin /src/dist/ /app/plugin/

# Всё состояние — в /data: config.json (API-токен), cookies.txt (сессия), settings.json (зеркало).
# Зеркало HDRezka задаётся в веб-интерфейсе; встроенных адресов нет.
ENV HDREZKA_LISTEN=:8765 \
    HDREZKA_DATA_DIR=/data \
    HDREZKA_PLUGIN_DIR=/app/plugin \
    HDREZKA_ADMIN_LAN=true \
    HDREZKA_LIST_INTERFACES=false

# UID/GID 1000 — как у данных приложений Umbrel; для обычного Docker папку данных нужно отдать 1000:1000
USER 1000:1000
EXPOSE 8765
VOLUME ["/data"]
ENTRYPOINT ["/app/hdrezka-server", "-config", "/data/config.json"]
