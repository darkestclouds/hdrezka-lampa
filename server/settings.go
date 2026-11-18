package main

// Настройки, которые задаются в веб-интерфейсе (не в config.json): data/settings.json.

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Settings struct {
	// Mirror — зеркало HDRezka (схема и хост, например https://example.com). Задаёт пользователь;
	// встроенных адресов в сервере нет. Пусто — сервер не обращается к сайту.
	Mirror string `json:"mirror"`
}

type SettingsStore struct {
	path string
	mu   sync.RWMutex
	cur  Settings
}

func NewSettingsStore(dataDir string) *SettingsStore {
	st := &SettingsStore{path: filepath.Join(dataDir, "settings.json")}
	if raw, err := os.ReadFile(st.path); err == nil {
		var s Settings
		if json.Unmarshal(raw, &s) == nil {
			if m, err := normalizeMirror(s.Mirror); err == nil {
				s.Mirror = m
			} else {
				s.Mirror = ""
			}
			st.cur = s
		}
	}
	return st
}

// normalizeMirror: "example.com", "https://example.com/путь" → "https://example.com"
func normalizeMirror(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return "", newError(400, "bad_request", errors.New("адрес зеркала вида https://example.com"))
	}
	return u.Scheme + "://" + strings.ToLower(u.Host), nil
}

func (st *SettingsStore) Get() Settings {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.cur
}

func (st *SettingsStore) save(next Settings) error {
	raw, _ := json.MarshalIndent(next, "", "  ")
	if err := os.MkdirAll(filepath.Dir(st.path), 0o700); err != nil {
		return err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, st.path)
}

// SetMirror сохраняет нормализованный адрес зеркала и возвращает его.
func (st *SettingsStore) SetMirror(raw string) (string, error) {
	mirror, err := normalizeMirror(raw)
	if err != nil {
		return "", err
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	next := st.cur
	next.Mirror = mirror
	if err := st.save(next); err != nil {
		return "", err
	}
	st.cur = next
	return mirror, nil
}
