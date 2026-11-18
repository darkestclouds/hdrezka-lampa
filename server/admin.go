package main

// Веб-интерфейс настройки сервера: http://localhost:<порт>/
//
// Доступ: по умолчанию только с этого компьютера (loopback); admin_lan=true — со всей локальной сети
// (в Docker/Umbrel запросы приходят с частного адреса шлюза или прокси).
// Токен здесь не нужен — вместо него проверяются адрес клиента, заголовок Host (только localhost, IP или имена
// из admin_hosts — защита от DNS rebinding) и для POST — JSON + Origin своего же хоста (защита от CSRF).
// Прокси Umbrel передаёт исходный Host (changeOrigin: false), поэтому проверки работают и за ним.

import (
	_ "embed"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed web/index.html
var adminPage []byte

func isAdminPath(path string) bool {
	return path == "/" || strings.HasPrefix(path, "/admin/")
}

// withAdminGuard пропускает к интерфейсу только «своих».
func withAdminGuard(allowLAN bool, allowHosts []string, next http.Handler) http.Handler {
	allowed := map[string]bool{"localhost": true}
	for _, h := range allowHosts {
		allowed[strings.ToLower(h)] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAdminPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		deny := func(msg string) {
			writeError(w, newError(403, "forbidden", errors.New(msg)))
		}

		host, _, err := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if err != nil || ip == nil {
			deny("неизвестный адрес клиента")
			return
		}
		if !ip.IsLoopback() && !(allowLAN && ip.IsPrivate()) {
			deny("интерфейс открыт только с этого компьютера (admin_lan в config.json)")
			return
		}

		// Host — localhost, IP или имя из admin_hosts: чужое доменное имя здесь означает DNS rebinding
		reqHost := hostOnly(r.Host)
		if !allowed[reqHost] && net.ParseIP(reqHost) == nil {
			deny("откройте интерфейс по IP-адресу, localhost или имени из admin_hosts")
			return
		}

		if r.Method == http.MethodPost {
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				deny("ожидается JSON")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host {
					deny("запрос с чужой страницы")
					return
				}
			}
		}

		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func hostOnly(hostport string) string {
	h := hostport
	if hh, _, err := net.SplitHostPort(hostport); err == nil {
		h = hh
	}
	return strings.ToLower(strings.Trim(h, "[]"))
}

func isLoopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// serverBases — адреса сервера для ТВ: сначала тот, по которому открыта страница (если это не localhost) —
// за прокси Umbrel это umbrel.local:порт или IP устройства; затем IPv4 сетевых интерфейсов (если list_interfaces).
func serverBases(r *http.Request, cfg Config) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(base string) {
		if !seen[base] {
			seen[base] = true
			out = append(out, base)
		}
	}

	if !isLoopbackHost(hostOnly(r.Host)) {
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		add(scheme + "://" + r.Host)
	}
	if cfg.ListInterfaces {
		for _, b := range interfaceBases(cfg.Listen) {
			add(b)
		}
	}
	return out
}

// interfaceBases — http://IP:порт по IPv4 этого компьютера (локальные сети первыми).
func interfaceBases(listen string) []string {
	port := listenPort(listen)
	if port == "" {
		port = ":80"
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}

	type entry struct {
		url     string
		private bool
	}
	list := []entry{}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		list = append(list, entry{url: "http://" + ip.String() + port, private: ip.IsPrivate()})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].private && !list[j].private })

	out := []string{}
	for _, e := range list {
		out = append(out, e.url)
	}
	return out
}

type adminStatus struct {
	*Status
	Error       string   `json:"error,omitempty"`
	PluginBuilt bool     `json:"plugin_built"`
	PluginURLs  []string `json:"plugin_urls"`
	AppURLs     []string `json:"app_urls"` // адрес подключения для приложения Apple TV
	AdminLAN    bool     `json:"admin_lan"`
	// Зеркало из настроек (есть, даже если сайт сейчас недоступен — тогда Status пустой)
	ConfiguredMirror string `json:"configured_mirror"`
}

func adminRoutes(mux *http.ServeMux, svc *Service, cfg Config) {
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'none'")
		_, _ = w.Write(adminPage)
	})

	status := func(r *http.Request, check bool) adminStatus {
		out := adminStatus{
			PluginURLs:       []string{},
			AppURLs:          []string{},
			AdminLAN:         cfg.AdminLAN,
			ConfiguredMirror: svc.settings.Get().Mirror,
		}
		for _, base := range serverBases(r, cfg) {
			out.PluginURLs = append(out.PluginURLs, base+"/hdrezka.js")
			// приложению Apple TV достаточно host:порт; https оставляем явно
			out.AppURLs = append(out.AppURLs, strings.TrimPrefix(base, "http://"))
		}
		if cfg.PluginDir != "" {
			_, err := os.Stat(filepath.Join(cfg.PluginDir, "hdrezka.js"))
			out.PluginBuilt = err == nil
		}
		st, err := svc.Status(check)
		if err != nil {
			out.Error = err.Error()
			var ae *apiError
			if errors.As(err, &ae) {
				out.Error = ae.Message
			}
			// статус без проверки сайта всё равно покажем
			if check {
				if st2, err2 := svc.Status(false); err2 == nil {
					st = st2
				}
			}
		}
		out.Status = st
		return out
	}

	mux.HandleFunc("GET /admin/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, status(r, r.URL.Query().Get("check") != ""))
	})

	mux.HandleFunc("POST /admin/login", func(w http.ResponseWriter, r *http.Request) {
		body, err := readBody(r)
		if err != nil {
			writeError(w, err)
			return
		}
		login, password := strings.TrimSpace(body["login"]), body["password"]
		if login == "" || password == "" {
			writeError(w, newError(400, "bad_request", errors.New("нужны логин и пароль")))
			return
		}
		reply(w, map[string]bool{"ok": true}, svc.Login(login, password))
	})

	mux.HandleFunc("POST /admin/cookies", func(w http.ResponseWriter, r *http.Request) {
		body, err := readBody(r)
		if err != nil {
			writeError(w, err)
			return
		}
		reply(w, map[string]bool{"ok": true}, svc.SetCookies(body["cookies"]))
	})

	// Зеркало сохраняется, даже если сейчас недоступно: ответ {mirror, error?}
	mux.HandleFunc("POST /admin/mirror", func(w http.ResponseWriter, r *http.Request) {
		body, err := readBody(r)
		if err != nil {
			writeError(w, err)
			return
		}
		mirror, err := svc.SetMirror(body["mirror"])
		if mirror == "" && err != nil {
			writeError(w, err)
			return
		}
		out := map[string]string{"mirror": mirror}
		if err != nil {
			var ae *apiError
			out["error"] = err.Error()
			if errors.As(err, &ae) {
				out["error"] = ae.Message
			}
		}
		writeJSON(w, 200, out)
	})

	mux.HandleFunc("POST /admin/logout", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]bool{"ok": true}, svc.Logout())
	})
}
