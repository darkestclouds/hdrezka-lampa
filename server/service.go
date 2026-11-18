package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/n0madic/go-hdrezka"
)

// apiError — ошибка с HTTP-статусом и кодом, который плагин переводит в понятный текст.
type apiError struct {
	Status  int    `json:"-"`
	Code    string `json:"error"`
	Message string `json:"message"`
}

func (e *apiError) Error() string { return e.Code + ": " + e.Message }

func newError(status int, code string, err error) *apiError {
	msg := code
	if err != nil {
		msg = err.Error()
	}
	return &apiError{Status: status, Code: code, Message: msg}
}

const (
	videoTTL    = 30 * time.Minute
	episodesTTL = 10 * time.Minute
)

type cachedVideo struct {
	video *hdrezka.Video
	at    time.Time
}

type cachedEpisodes struct {
	episodes hdrezka.Episodes
	err      error
	at       time.Time
}

// Service держит клиент go-hdrezka, сессию (куки) и кэши.
// Клиент пересоздаётся при входе/выходе/смене кук; кэш при этом сбрасывается,
// потому что объекты Translation привязаны к конкретному клиенту.
type Service struct {
	cfg         Config
	cookiesPath string
	settings    *SettingsStore

	mu       sync.RWMutex
	hd       *hdrezka.HDRezka
	ready    bool
	videos   map[string]cachedVideo
	episodes map[string]cachedEpisodes
}

func NewService(cfg Config) *Service {
	s := &Service{
		cfg:         cfg,
		cookiesPath: filepath.Join(cfg.DataDir, "cookies.txt"),
		settings:    NewSettingsStore(cfg.DataDir),
	}
	s.reset(s.loadCookies())
	return s
}

// ---------- сессия ----------

func (s *Service) loadCookies() string {
	raw, err := os.ReadFile(s.cookiesPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func (s *Service) saveCookies(cookies string) error {
	if err := os.MkdirAll(filepath.Dir(s.cookiesPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.cookiesPath, []byte(cookies+"\n"), 0o600)
}

// reset создаёт новый клиент с заданными куками. Вызывать под s.mu.Lock (или в конструкторе).
func (s *Service) reset(cookies string) {
	hd := hdrezka.New().WithPersistentSession()
	// Зеркало — только заданное пользователем в веб-интерфейсе. Без него клиент не инициализируется
	// (initLocked), поэтому встроенный список зеркал библиотеки никогда не используется.
	if mirror := s.settings.Get().Mirror; mirror != "" {
		hd.WithMirrors(mirror)
	}
	if s.cfg.Proxy != "" {
		hd.WithProxy(s.cfg.Proxy)
	}
	if s.cfg.Resolver != "" {
		hd.WithResolver(s.cfg.Resolver)
	}
	if cookies != "" {
		hd.WithCookies(cookies)
	}
	s.hd = hd
	s.ready = false
	s.videos = map[string]cachedVideo{}
	s.episodes = map[string]cachedEpisodes{}
}

// initLocked проверяет зеркала (и проходит Anubis). Вызывать под s.mu.Lock.
func (s *Service) initLocked() error {
	if s.ready {
		return nil
	}
	if s.settings.Get().Mirror == "" {
		return errNoMirror
	}
	if err := s.hd.Init(); err != nil {
		return newError(503, "unavailable", err)
	}
	s.ready = true
	log.Printf("зеркало: %s", s.hd.URL)
	return nil
}

var errNoMirror = newError(503, "no_mirror", errors.New("зеркало HDRezka не задано — укажите его в веб-интерфейсе сервера"))

// SetMirror сохраняет зеркало и пересоздаёт клиент. Возвращает сохранённый адрес и результат проверки
// (зеркало сохраняется, даже если сейчас недоступно).
func (s *Service) SetMirror(raw string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	mirror, err := s.settings.SetMirror(raw)
	if err != nil {
		return "", err
	}
	s.reset(s.loadCookies())
	if mirror == "" {
		return "", nil
	}
	log.Printf("зеркало задано: %s", mirror)
	if err := s.initLocked(); err != nil {
		return mirror, err
	}
	// Init проверяет только, что страница открывается. Похоже ли это на HDRezka — по меню категорий,
	// которое библиотека разбирает с главной: у чужого сайта его нет.
	if len(s.hd.Categories[hdrezka.Films]) == 0 && len(s.hd.Categories[hdrezka.Series]) == 0 {
		return mirror, newError(502, "not_rezka", errors.New("страница по этому адресу открывается, но не похожа на HDRezka"))
	}
	return mirror, nil
}

// client возвращает готовый клиент, при необходимости инициализируя его.
func (s *Service) client() (*hdrezka.HDRezka, error) {
	s.mu.RLock()
	hd, ready := s.hd, s.ready
	s.mu.RUnlock()
	if ready {
		return hd, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.initLocked(); err != nil {
		return nil, err
	}
	return s.hd, nil
}

func loggedIn(hd *hdrezka.HDRezka) bool {
	if hd.URL == nil {
		return false
	}
	for _, c := range hd.Client.Jar.Cookies(hd.URL) {
		if c.Name == "dle_user_id" && c.Value != "" && c.Value != "deleted" {
			return true
		}
	}
	return false
}

type Status struct {
	Mirror   string `json:"mirror"`
	LoggedIn bool   `json:"logged_in"`
	Premium  *bool  `json:"premium,omitempty"`
}

// Status: check=true дополнительно открывает главную (проверяет доступ и премиум).
func (s *Service) Status(check bool) (*Status, error) {
	hd, err := s.client()
	if err != nil {
		return nil, err
	}
	st := &Status{Mirror: hd.URL.String(), LoggedIn: loggedIn(hd)}
	if check {
		premium, err := hd.IsPremiumUser()
		if err != nil {
			return nil, newError(502, "upstream", err)
		}
		st.Premium = &premium
	}
	return st, nil
}

// Login входит логином/паролем. Пароль нигде не сохраняется — только полученные куки.
func (s *Service) Login(login, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Чистый клиент, чтобы старая сессия не смешалась с новой
	s.reset("")
	if err := s.initLocked(); err != nil {
		return err
	}
	if err := s.hd.Login(login, password); err != nil {
		s.reset(s.loadCookies())
		return newError(401, "login_failed", errors.New(loginErrorText(err)))
	}
	if !loggedIn(s.hd) {
		s.reset(s.loadCookies())
		return newError(401, "login_failed", errors.New("сайт не выдал куки сессии"))
	}
	return s.saveCookies(s.hd.ExportCookies())
}

// SetCookies принимает строку кук из браузера ("name=value; name2=value2").
func (s *Service) SetCookies(raw string) error {
	cookies := normalizeCookieString(raw)
	if cookies == "" {
		return newError(400, "bad_request", errors.New(`нужна строка вида "dle_user_id=…; dle_password=…"`))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Куки сохраняем и без зеркала — применятся, когда зеркало будет задано
	if err := s.saveCookies(cookies); err != nil {
		return err
	}
	s.reset(cookies)
	if s.settings.Get().Mirror == "" {
		return nil
	}
	return s.initLocked()
}

func (s *Service) Logout() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.cookiesPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	s.reset("")
	return nil
}

// loginErrorText достаёт текст ошибки сайта из ошибки go-hdrezka.
// Сайт отвечает {"success":false,"message":{"email":"Введен неверный email или пароль"}} — message бывает
// объектом, а библиотека ждёт строку и падает с "login response is not JSON …: <тело ответа>".
// Библиотеку не правим — разбираем тело из текста ошибки.
func loginErrorText(err error) string {
	text := err.Error()
	i := strings.Index(text, "{")
	if i < 0 {
		return text
	}
	var resp struct {
		Message any `json:"message"`
	}
	if json.Unmarshal([]byte(text[i:]), &resp) != nil {
		return text
	}

	parts := []string{}
	switch m := resp.Message.(type) {
	case string:
		parts = append(parts, m)
	case map[string]any:
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if s, ok := m[k].(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
	case []any:
		for _, v := range m {
			if s, ok := v.(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
	}
	if len(parts) == 0 {
		return text
	}
	return strings.Join(parts, "; ")
}

func normalizeCookieString(raw string) string {
	parts := []string{}
	for _, p := range strings.Split(raw, ";") {
		p = strings.TrimSpace(p)
		if i := strings.Index(p, "="); i > 0 {
			parts = append(parts, strings.TrimSpace(p[:i])+"="+strings.TrimSpace(p[i+1:]))
		}
	}
	return strings.Join(parts, ";")
}

// ---------- повтор при устаревшем пропуске Anubis ----------

// AJAX-запросы библиотеки (get_cdn_series) не проходят анти-бот сами. Если кука-пропуск
// истекла, вместо JSON приходит HTML и декодер падает с SyntaxError. Тогда открываем
// главную (getDoc решит Anubis и обновит куку) и повторяем запрос один раз.
func withAntibotRetry(hd *hdrezka.HDRezka, call func() error) error {
	err := call()
	var syntax *json.SyntaxError
	if err == nil || !errors.As(err, &syntax) {
		return err
	}
	if _, perr := hd.IsPremiumUser(); perr != nil {
		return err
	}
	return call()
}

// ---------- видео ----------

type TranslatorInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Premium bool   `json:"premium"`
	Default bool   `json:"default"`
}

type VideoInfo struct {
	ID            string           `json:"id"`
	URL           string           `json:"url"`
	Title         string           `json:"title"`
	TitleOriginal string           `json:"title_original,omitempty"`
	Year          string           `json:"year,omitempty"`
	Category      string           `json:"category"`
	Serial        bool             `json:"serial"`
	Translators   []TranslatorInfo `json:"translators"`
}

// pageURL приводит ссылку к адресу на текущем зеркале и пропускает только страницы фильмов.
func pageURL(hd *hdrezka.HDRezka, raw string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.HasSuffix(u.Path, ".html") || strings.Count(strings.Trim(u.Path, "/"), "/") < 1 {
		return "", "", newError(400, "bad_request", errors.New("url должен быть страницей фильма HDRezka"))
	}
	full := hd.URL.ResolveReference(&url.URL{Path: u.Path})
	return full.String(), u.Path, nil
}

func (s *Service) video(raw string) (*hdrezka.HDRezka, *hdrezka.Video, error) {
	hd, err := s.client()
	if err != nil {
		return nil, nil, err
	}
	full, key, err := pageURL(hd, raw)
	if err != nil {
		return nil, nil, err
	}

	s.mu.RLock()
	c, ok := s.videos[key]
	s.mu.RUnlock()
	if ok && time.Since(c.at) < videoTTL {
		return hd, c.video, nil
	}

	v, err := hd.GetVideo(full)
	if err != nil {
		if strings.Contains(err.Error(), "sign in required") {
			return nil, nil, newError(401, "login_required", err)
		}
		return nil, nil, newError(502, "upstream", err)
	}

	s.mu.Lock()
	if s.hd == hd { // клиент мог смениться, пока шёл запрос
		s.videos[key] = cachedVideo{video: v, at: time.Now()}
	}
	s.mu.Unlock()
	return hd, v, nil
}

func findTranslation(v *hdrezka.Video, id string) *hdrezka.Translation {
	var def *hdrezka.Translation
	for _, t := range v.Translation {
		if id != "" && t.ID == id {
			return t
		}
		if t.IsDefault && def == nil {
			def = t
		}
	}
	if id != "" {
		return nil
	}
	if def != nil {
		return def
	}
	if len(v.Translation) > 0 {
		return v.Translation[0]
	}
	return nil
}

func (s *Service) loadEpisodes(hd *hdrezka.HDRezka, v *hdrezka.Video, t *hdrezka.Translation) (hdrezka.Episodes, error) {
	key := v.ID + ":" + t.ID

	s.mu.RLock()
	c, ok := s.episodes[key]
	s.mu.RUnlock()
	if ok && time.Since(c.at) < episodesTTL {
		return c.episodes, c.err
	}

	var eps hdrezka.Episodes
	err := withAntibotRetry(hd, func() error {
		var e error
		eps, e = t.GetEpisodes()
		return e
	})

	s.mu.Lock()
	if s.hd == hd {
		s.episodes[key] = cachedEpisodes{episodes: eps, err: err, at: time.Now()}
	}
	s.mu.Unlock()
	return eps, err
}

// Info: данные страницы + список переводов + признак сериала.
// Признак сериала берём по факту: есть ли серии у перевода по умолчанию
// (категория в URL не помогает — мультфильмы и аниме бывают и фильмами, и сериалами).
func (s *Service) Info(raw string) (*VideoInfo, error) {
	hd, v, err := s.video(raw)
	if err != nil {
		return nil, err
	}

	info := &VideoInfo{
		ID:            v.ID,
		URL:           raw,
		Title:         v.Title,
		TitleOriginal: v.TitleOriginal,
		Year:          v.Year,
		Category:      string(v.Type),
		Translators:   []TranslatorInfo{},
	}
	for _, t := range v.Translation {
		info.Translators = append(info.Translators, TranslatorInfo{
			ID: t.ID, Name: t.Name, Premium: t.IsPremium, Default: t.IsDefault,
		})
	}

	if t := findTranslation(v, ""); t != nil {
		if eps, err := s.loadEpisodes(hd, v, t); err == nil && len(eps) > 0 {
			info.Serial = true
		}
	}
	return info, nil
}

type Season struct {
	Season   int   `json:"season"`
	Episodes []int `json:"episodes"`
}

func (s *Service) Episodes(raw, translatorID string) ([]Season, error) {
	hd, v, err := s.video(raw)
	if err != nil {
		return nil, err
	}
	t := findTranslation(v, translatorID)
	if t == nil {
		return nil, newError(404, "not_found", errors.New("перевод не найден"))
	}
	eps, err := s.loadEpisodes(hd, v, t)
	if err != nil {
		return nil, newError(502, "upstream", err)
	}

	seasons := []Season{}
	for _, sn := range eps.ListSeasons() {
		seasons = append(seasons, Season{Season: sn, Episodes: eps.ListEpisodes(sn)})
	}
	return seasons, nil
}

// ---------- поток ----------

type Quality struct {
	Label string `json:"label"`
	MP4   string `json:"mp4"`
	HLS   string `json:"hls,omitempty"`
}

type Subtitle struct {
	Label string `json:"label"`
	Lang  string `json:"lang,omitempty"`
	URL   string `json:"url"`
}

type StreamInfo struct {
	Qualities []Quality  `json:"qualities"`
	Subtitles []Subtitle `json:"subtitles"`
}

var reDigits = regexp.MustCompile(`\d+`)

// qualityLabel приводит метку сайта к виду, который понимает плеер Lampa: он выбирает и сортирует качества
// по parseInt(метки) (interaction/player.js → getUrlQuality), а parseInt("2K") = 2, parseInt("4K") = 4.
func qualityLabel(label string) string {
	switch strings.ToUpper(strings.TrimSpace(label)) {
	case "2K":
		return "1440p (2K)"
	case "4K":
		return "2160p (4K)"
	}
	return label
}

// qualityRank: 2160p > 1440p > 1080p Ultra > 1080p > 720p … (метка уже после qualityLabel)
func qualityRank(label string) int {
	l := strings.ToLower(label)
	n := 0
	if m := reDigits.FindString(l); m != "" {
		n, _ = strconv.Atoi(m)
	}
	n *= 10
	if strings.Contains(l, "ultra") {
		n += 5
	}
	return n
}

func (s *Service) Stream(raw, translatorID string, season, episode int) (*StreamInfo, error) {
	hd, v, err := s.video(raw)
	if err != nil {
		return nil, err
	}
	t := findTranslation(v, translatorID)
	if t == nil {
		return nil, newError(404, "not_found", errors.New("перевод не найден"))
	}

	var st *hdrezka.Stream
	err = withAntibotRetry(hd, func() error {
		var e error
		if season > 0 {
			st, e = t.GetStream(season, episode)
		} else {
			st, e = t.GetStream()
		}
		return e
	})
	if err != nil {
		return nil, newError(502, "upstream", err)
	}

	out := &StreamInfo{Qualities: []Quality{}, Subtitles: []Subtitle{}}
	premiumOnly := 0
	for label, f := range st.Formats {
		// Премиум-качество без премиум-аккаунта: CDN отдаёт короткий тизер вместо фильма — не показываем
		if f.IsPremium {
			premiumOnly++
			continue
		}
		if f.MP4 == "" && f.HLS == "" {
			continue
		}
		mp4 := f.MP4
		if mp4 == "" {
			mp4 = f.HLS
		}
		out.Qualities = append(out.Qualities, Quality{Label: qualityLabel(label), MP4: mp4, HLS: f.HLS})
	}
	sort.Slice(out.Qualities, func(a, b int) bool {
		return qualityRank(out.Qualities[a].Label) > qualityRank(out.Qualities[b].Label)
	})

	if len(out.Qualities) == 0 {
		msg := "сайт не отдал ссылок на видео"
		if premiumOnly > 0 {
			msg = "доступно только с премиум-аккаунтом"
		}
		return nil, newError(404, "no_stream", errors.New(msg))
	}

	for label, link := range st.Subtitles {
		if link == "" {
			continue
		}
		lang, _ := hdrezka.SubtitleLanguageCode(label)
		out.Subtitles = append(out.Subtitles, Subtitle{Label: label, Lang: lang, URL: link})
	}
	sort.Slice(out.Subtitles, func(a, b int) bool { return out.Subtitles[a].Label < out.Subtitles[b].Label })

	return out, nil
}
