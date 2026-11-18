# hdrezka-lampa

Плагин для Lampa и сервер к нему.

## Сборка

Плагин:

```bash
npm ci
npm run build
```

Сервер (Go 1.25+):

```bash
cd server
go build -o hdrezka-server .
./hdrezka-server
```

Docker:

```bash
docker build -f docker/Dockerfile -t hdrezka-server .
```

Настройка — в веб-интерфейсе сервера на порту `8765`.
