// HDRezka API для плагина Lampa.
//
// Плагин ходит сюда, а сервер — на сайт HDRezka через библиотеку go-hdrezka
// (github.com/n0madic/go-hdrezka, MIT): зеркала, анти-бот Anubis, вход, поиск, переводы, потоки.
// Видео ТВ берёт напрямую с CDN — через сервер идут только короткие JSON-запросы.
//
//	hdrezka-server [-config path/to/config.json]
package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	exe, _ := os.Executable()
	defaultConfig := filepath.Join(filepath.Dir(exe), "config.json")
	if wd, err := os.Getwd(); err == nil {
		// go run . — конфиг рядом с исходниками
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			defaultConfig = filepath.Join(wd, "config.json")
		}
	}
	configPath := flag.String("config", defaultConfig, "путь к config.json")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("конфиг: %v", err)
	}

	svc := NewService(cfg)

	// Проверяем зеркало сразу, чтобы проблемы с доступом были видны в консоли при старте.
	if svc.settings.Get().Mirror == "" {
		log.Printf("зеркало HDRezka не задано — укажите его в веб-интерфейсе")
	} else if st, err := svc.Status(false); err != nil {
		log.Printf("сайт пока недоступен: %v (повторю при первом запросе)", err)
	} else if !st.LoggedIn {
		log.Printf("сессии нет: войдите из настроек плагина или передайте куки (см. README)")
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           withLiteLog(withCORS(withAdminGuard(cfg.AdminLAN, cfg.AdminHosts, withAuth(cfg.APIToken, routes(svc, cfg))))),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if cfg.PluginDir != "" {
		if _, err := os.Stat(filepath.Join(cfg.PluginDir, "hdrezka.js")); err != nil {
			log.Printf("плагин не найден в %s — соберите его: npm run build", cfg.PluginDir)
		} else {
			log.Printf("плагин для Lampa: http://<адрес сервера>%s/hdrezka.js", listenPort(cfg.Listen))
		}
	}
	log.Printf("настройка: http://<адрес сервера>%s/ (на этом компьютере — http://localhost%s/)", listenPort(cfg.Listen), listenPort(cfg.Listen))
	log.Printf("слушаю %s", cfg.Listen)
	log.Fatal(srv.ListenAndServe())
}

// ---------- middleware ----------

// Lampa работает с любого origin (браузер, file://, WebView ТВ) — CORS открыт всем,
// а доступ закрыт токеном в заголовке X-Api-Key. Веб-интерфейс (/ и /admin/) CORS не получает:
// он работает только со своего же origin.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAdminPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "X-Api-Key, Content-Type")
		h.Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /api/capabilities — описание источника для приложения Apple TV, открыто (как и /lite/*)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/capabilities" {
			got := r.Header.Get("X-Api-Key")
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				writeError(w, newError(401, "unauthorized", errors.New("неверный API-токен")))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	var ae *apiError
	if !errors.As(err, &ae) {
		ae = newError(500, "internal", err)
	}
	if ae.Status >= 500 {
		log.Printf("ошибка: %v", ae)
	}
	writeJSON(w, ae.Status, ae)
}

func reply(w http.ResponseWriter, v any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, v)
}

// readBody: Lampa.Reguest шлёт POST как form-urlencoded, curl и скрипты — часто JSON. Принимаем оба.
func readBody(r *http.Request) (map[string]string, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if ct == "application/json" {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, newError(400, "bad_request", err)
		}
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
		return out, nil
	}
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil, newError(400, "bad_request", err)
	}
	for k := range values {
		out[k] = values.Get(k)
	}
	return out, nil
}

func intParam(r *http.Request, name string) (int, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, newError(400, "bad_request", errors.New("неверный параметр "+name))
	}
	return n, nil
}

func required(r *http.Request, name string) (string, error) {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return "", newError(400, "bad_request", errors.New("нужен параметр "+name))
	}
	return v, nil
}

// listenPort: ":8765" / "0.0.0.0:8765" → ":8765" (для подсказки в логе)
func listenPort(listen string) string {
	if i := strings.LastIndex(listen, ":"); i >= 0 {
		return listen[i:]
	}
	return ""
}

// tokenPlaceholder — строка в src/config.js плагина, вместо которой сервер вписывает hex(api_token).
const tokenPlaceholder = "{{HDREZKA_TOKEN}}"

// servePlugin отдаёт файл сборки плагина без проверки токена: Lampa ставит плагины по URL и заголовков не шлёт.
// В отдаваемый файл вшивается API-токен, поэтому в Lampa его вводить не нужно. Следствие: кто может скачать
// /hdrezka.js, тот может пользоваться API — сервер рассчитан на домашнюю сеть, наружу его не открывать.
// Отдаются только эти два имени — никакого листинга каталога.
func servePlugin(dir, name, token string) http.HandlerFunc {
	injected := []byte(hex.EncodeToString([]byte(token)))

	return func(w http.ResponseWriter, r *http.Request) {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			writeError(w, newError(404, "not_found", errors.New("плагин не собран: npm run build")))
			return
		}
		body = bytes.ReplaceAll(body, []byte(tokenPlaceholder), injected)

		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache") // после пересборки ТВ сразу получит новую версию
		w.WriteHeader(200)
		_, _ = w.Write(body)
	}
}

// ---------- routes ----------

func routes(svc *Service, cfg Config) http.Handler {
	pluginDir, token := cfg.PluginDir, cfg.APIToken
	mux := http.NewServeMux()

	adminRoutes(mux, svc, cfg)
	liteRoutes(mux, svc)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]bool{"ok": true})
	})

	if pluginDir != "" {
		mux.HandleFunc("GET /hdrezka.js", servePlugin(pluginDir, "hdrezka.js", token))
		mux.HandleFunc("GET /hdrezka.min.js", servePlugin(pluginDir, "hdrezka.min.js", token))
	}

	// --- сессия ---

	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		st, err := svc.Status(r.URL.Query().Get("check") != "")
		reply(w, st, err)
	})

	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		body, err := readBody(r)
		if err != nil {
			writeError(w, err)
			return
		}
		login, password := strings.TrimSpace(body["login"]), body["password"]
		if login == "" || password == "" {
			writeError(w, newError(400, "bad_request", errors.New("нужны login и password")))
			return
		}
		reply(w, map[string]bool{"ok": true}, svc.Login(login, password))
	})

	mux.HandleFunc("POST /api/cookies", func(w http.ResponseWriter, r *http.Request) {
		body, err := readBody(r)
		if err != nil {
			writeError(w, err)
			return
		}
		reply(w, map[string]bool{"ok": true}, svc.SetCookies(body["cookies"]))
	})

	mux.HandleFunc("POST /api/logout", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]bool{"ok": true}, svc.Logout())
	})

	// --- контент ---

	mux.HandleFunc("GET /api/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		query := SearchQuery{Title: q.Get("title"), OriginalTitle: q.Get("original_title"), Raw: q.Get("raw") != ""}
		if strings.TrimSpace(query.Title+query.OriginalTitle) == "" {
			writeError(w, newError(400, "bad_request", errors.New("нужен параметр title")))
			return
		}
		year, err := intParam(r, "year")
		if err != nil {
			writeError(w, err)
			return
		}
		query.Year = year
		if v := q.Get("serial"); v != "" {
			serial := v == "1" || v == "true"
			query.Serial = &serial
		}
		items, err := svc.Search(query)
		reply(w, map[string]any{"items": items}, err)
	})

	mux.HandleFunc("GET /api/info", func(w http.ResponseWriter, r *http.Request) {
		u, err := required(r, "url")
		if err != nil {
			writeError(w, err)
			return
		}
		info, err := svc.Info(u)
		reply(w, info, err)
	})

	mux.HandleFunc("GET /api/episodes", func(w http.ResponseWriter, r *http.Request) {
		u, err := required(r, "url")
		if err != nil {
			writeError(w, err)
			return
		}
		seasons, err := svc.Episodes(u, r.URL.Query().Get("translator_id"))
		reply(w, map[string]any{"seasons": seasons}, err)
	})

	mux.HandleFunc("GET /api/stream", func(w http.ResponseWriter, r *http.Request) {
		u, err := required(r, "url")
		if err != nil {
			writeError(w, err)
			return
		}
		season, err := intParam(r, "season")
		if err != nil {
			writeError(w, err)
			return
		}
		episode, err := intParam(r, "episode")
		if err != nil {
			writeError(w, err)
			return
		}
		if (season > 0) != (episode > 0) {
			writeError(w, newError(400, "bad_request", errors.New("season и episode передаются вместе")))
			return
		}
		st, err := svc.Stream(u, r.URL.Query().Get("translator_id"), season, episode)
		reply(w, st, err)
	})

	return mux
}
