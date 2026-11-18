package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Config — содержимое config.json. Любое поле можно переопределить переменной окружения HDREZKA_* (см. applyEnv) —
// так настраивается контейнер. Зеркало HDRezka здесь не задаётся: только в веб-интерфейсе (data/settings.json).
type Config struct {
	// Адрес, на котором слушает сервер. ":8765" — все интерфейсы (нужно, чтобы ТВ в сети его видел).
	Listen string `json:"listen"`
	// Токен доступа: плагин шлёт его в заголовке X-Api-Key. Создаётся при первом запуске.
	APIToken string `json:"api_token"`
	// Прокси для запросов к сайту: http://, https://, socks5://, socks5h://. Пусто — напрямую.
	Proxy string `json:"proxy"`
	// DNS-сервер для запросов к сайту (IP без порта). Пусто — системный.
	Resolver string `json:"resolver"`
	// Каталог для сессии и настроек (cookies.txt, settings.json). Относительный путь — от каталога config.json.
	DataDir string `json:"data_dir"`
	// Каталог со сборкой плагина (hdrezka.js, hdrezka.min.js), которую сервер отдаёт для установки в Lampa.
	// Относительный путь — от каталога config.json. Файлы читаются при каждом запросе — после npm run build
	// перезапуск сервера не нужен.
	PluginDir string `json:"plugin_dir"`
	// Веб-интерфейс настройки (/) открывается только с этого компьютера. true — со всей локальной сети
	// (и из Docker-сети: в контейнере запросы приходят с адреса шлюза Docker или прокси Umbrel).
	AdminLAN bool `json:"admin_lan"`
	// Имена хоста, по которым можно открыть веб-интерфейс, кроме localhost и IP-адресов
	// (например umbrel.local). Остальные имена отклоняются — защита от DNS rebinding.
	AdminHosts []string `json:"admin_hosts"`
	// Показывать в веб-интерфейсе адреса сетевых интерфейсов этой машины. В контейнере это адреса
	// Docker-сети, бесполезные для ТВ, — там выключено; адрес берётся из того, по которому открыта страница.
	ListInterfaces bool `json:"list_interfaces"`
}

func defaultConfig() Config {
	return Config{
		Listen:         ":8765",
		DataDir:        "data",
		PluginDir:      "../dist",
		ListInterfaces: true,
	}
}

func randomToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("token: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// loadConfig читает config.json; если файла или токена нет — создаёт и сохраняет.
// Переменные окружения применяются после и в файл не записываются.
func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()

	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return cfg, errors.New("config.json: " + err.Error())
		}
	case os.IsNotExist(err):
		// первый запуск
	default:
		return cfg, err
	}

	if cfg.APIToken == "" && os.Getenv("HDREZKA_API_TOKEN") == "" {
		cfg.APIToken = randomToken()
		out, _ := json.MarshalIndent(cfg, "", "  ")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return cfg, err
		}
		if err := os.WriteFile(path, append(out, '\n'), 0o600); err != nil {
			return cfg, err
		}
		log.Printf("создан %s с новым API-токеном", path)
	}

	applyEnv(&cfg)

	if !filepath.IsAbs(cfg.DataDir) {
		cfg.DataDir = filepath.Join(filepath.Dir(path), cfg.DataDir)
	}
	if cfg.PluginDir != "" && !filepath.IsAbs(cfg.PluginDir) {
		cfg.PluginDir = filepath.Join(filepath.Dir(path), cfg.PluginDir)
	}
	return cfg, nil
}

// applyEnv: HDREZKA_LISTEN, HDREZKA_API_TOKEN, HDREZKA_PROXY, HDREZKA_RESOLVER, HDREZKA_DATA_DIR,
// HDREZKA_PLUGIN_DIR, HDREZKA_ADMIN_LAN (true/false), HDREZKA_ADMIN_HOSTS (через запятую),
// HDREZKA_LIST_INTERFACES (true/false).
func applyEnv(cfg *Config) {
	str := func(name string, dst *string) {
		if v, ok := os.LookupEnv(name); ok {
			*dst = strings.TrimSpace(v)
		}
	}
	boolean := func(name string, dst *bool) {
		if v, ok := os.LookupEnv(name); ok {
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "1", "true", "yes", "on":
				*dst = true
			case "0", "false", "no", "off":
				*dst = false
			}
		}
	}

	str("HDREZKA_LISTEN", &cfg.Listen)
	str("HDREZKA_API_TOKEN", &cfg.APIToken)
	str("HDREZKA_PROXY", &cfg.Proxy)
	str("HDREZKA_RESOLVER", &cfg.Resolver)
	str("HDREZKA_DATA_DIR", &cfg.DataDir)
	str("HDREZKA_PLUGIN_DIR", &cfg.PluginDir)
	boolean("HDREZKA_ADMIN_LAN", &cfg.AdminLAN)
	boolean("HDREZKA_LIST_INTERFACES", &cfg.ListInterfaces)

	if v, ok := os.LookupEnv("HDREZKA_ADMIN_HOSTS"); ok {
		cfg.AdminHosts = nil
		for _, h := range strings.Split(v, ",") {
			if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
				cfg.AdminHosts = append(cfg.AdminHosts, h)
			}
		}
	}
}
