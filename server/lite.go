package main

// Источник в формате lampac-family для нативного приложения Apple TV (tvos_native/SourceConnect).
//
//	GET /api/capabilities        описание источника (без входа — luxo-source/1 без блока auth)
//	GET /lite/withsearch         ["hdrezka"]
//	GET /lite/events?…           [{"name","url","balanser"}]
//	GET /lite/hdrezka?…          варианты поиска / переводы / сезоны / серии
//	GET /lite/hdrezka/video?…    ссылка на поток (method "play"); &play=true — сразу редирект на видео
//
// Формат ответов повторяет шаблоны lampac (immisterio/Lampac, Shared/Models/Templates: MovieTpl,
// SeasonTpl, EpisodeTpl, VoiceTpl, SimilarTpl, VideoTpl): JSON при rjson=true, иначе HTML-блоки
// videos__item с data-json. Код lampac не используется — только формат данных.
//
// Токен здесь не нужен: сервер для домашней сети (как и /hdrezka.js с вшитым токеном).

import (
	"bytes"
	"encoding/json"
	"errors"
	"html"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const liteBalanser = "hdrezka"

// hlsManifestTimeout — lampac пишет его в каждый элемент воспроизведения и в ответ VideoTpl
// (по умолчанию 30 с). Клиенты lampac-family могут считать поле обязательным.
const hlsManifestTimeout = 30000

// ---------- DTO (поля и порядок как в lampac) ----------

type liteVoice struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	Active bool   `json:"active,omitempty"`
	Name   string `json:"name"`
}

type liteSimilar struct {
	Method  string `json:"method"`
	URL     string `json:"url"`
	Similar bool   `json:"similar"`
	Year    int    `json:"year,omitempty"`
	Details string `json:"details,omitempty"`
	Title   string `json:"title"`
	Img     string `json:"img,omitempty"`
}

type liteSeason struct {
	Method string `json:"method"`
	ID     int    `json:"id"`
	URL    string `json:"url"`
	Name   string `json:"name"`
}

// Stream — прямая ссылка для воспроизведения (наш /video с play=true → редирект на видео).
// lampac кладёт её рядом с method "call" (Modules/OnlinePaid/Rezka/Service.cs: stream / streamlink),
// и нативные клиенты играют именно по ней, а url вызывают только для получения качеств и субтитров.
type liteMovie struct {
	Method     string `json:"method"`
	URL        string `json:"url"`
	Stream     string `json:"stream,omitempty"`
	Translate  string `json:"translate"`
	MaxQuality string `json:"maxquality,omitempty"`
	VoiceName  string `json:"voice_name,omitempty"`
	Year       int    `json:"year,omitempty"`
	Title      string `json:"title"`
	HLSTimeout int    `json:"hls_manifest_timeout"`
}

type liteEpisode struct {
	Method     string `json:"method"`
	URL        string `json:"url"`
	Stream     string `json:"stream,omitempty"`
	S          int    `json:"s"`
	E          int    `json:"e"`
	Details    string `json:"details,omitempty"`
	Name       string `json:"name"`
	Title      string `json:"title"`
	HLSTimeout int    `json:"hls_manifest_timeout"`
}

type liteSubtitle struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	Label  string `json:"label"`
}

type liteVideo struct {
	Title      string            `json:"title,omitempty"`
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Quality    map[string]string `json:"quality,omitempty"`
	Subtitles  []liteSubtitle    `json:"subtitles,omitempty"`
	HLSTimeout int               `json:"hls_manifest_timeout"`
}

// ---------- вывод ----------

// dataJSON — JSON для атрибута data-json='…' (одинарные кавычки и HTML-символы экранированы)
func dataJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
	return strings.ReplaceAll(strings.TrimSpace(buf.String()), "'", `'`)
}

func writeLiteHTML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write([]byte(body))
}

func voiceHTML(voices []liteVoice) string {
	if len(voices) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="videos__line">`)
	for _, v := range voices {
		b.WriteString(`<div class="videos__button selector `)
		if v.Active {
			b.WriteString("active")
		}
		b.WriteString(`" data-json='` + dataJSON(v) + `'>` + html.EscapeString(v.Name) + `</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// folderHTML — плитки сезонов / вариантов поиска (videos__season)
func folderHTML(items []any, titles []string) string {
	var b strings.Builder
	b.WriteString(`<div class="videos__line">`)
	for i, it := range items {
		b.WriteString(`<div class="videos__item videos__season selector `)
		if i == 0 {
			b.WriteString("focused")
		}
		b.WriteString(`" data-json='` + dataJSON(it) + `'><div class="videos__season-layers"></div>` +
			`<div class="videos__item-imgbox videos__season-imgbox"><div class="videos__item-title videos__season-title">` +
			html.EscapeString(titles[i]) + `</div></div></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// itemHTML — элементы для воспроизведения (videos__movie); attrs — доп. атрибуты (s/e у серий)
func itemHTML(items []any, titles, attrs []string) string {
	var b strings.Builder
	b.WriteString(`<div class="videos__line">`)
	for i, it := range items {
		b.WriteString(`<div class="videos__item videos__movie selector `)
		if i == 0 {
			b.WriteString("focused")
		}
		b.WriteString(`" media="" ` + attrs[i] + `data-json='` + dataJSON(it) + `'>` +
			`<div class="videos__item-imgbox videos__movie-imgbox"></div><div class="videos__item-title">` +
			html.EscapeString(titles[i]) + `</div></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// ---------- адреса ----------

type liteCtx struct {
	r     *http.Request
	base  string // http://host:port
	rjson bool
	title string // название для заголовков (как прислало приложение)
	orig  string
}

func newLiteCtx(r *http.Request) *liteCtx {
	q := r.URL.Query()
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return &liteCtx{
		r:     r,
		base:  scheme + "://" + r.Host,
		rjson: q.Get("rjson") == "true" || q.Get("rjson") == "1",
		title: q.Get("title"),
		orig:  q.Get("original_title"),
	}
}

// link строит адрес на наш эндпоинт, сохраняя rjson/title/original_title
func (c *liteCtx) link(path string, params map[string]string) string {
	v := url.Values{}
	if c.rjson {
		v.Set("rjson", "true")
	}
	if c.title != "" {
		v.Set("title", c.title)
	}
	if c.orig != "" {
		v.Set("original_title", c.orig)
	}
	for k, val := range params {
		if val != "" {
			v.Set(k, val)
		}
	}
	return c.base + path + "?" + v.Encode()
}

func (c *liteCtx) displayTitle(info *VideoInfo) string {
	if c.title != "" {
		return c.title
	}
	if c.orig != "" {
		return c.orig
	}
	return info.Title
}

// пустой ответ — как IsEmpty у шаблонов lampac
func (c *liteCtx) empty(w http.ResponseWriter) {
	if c.rjson {
		writeJSON(w, 200, map[string]any{})
		return
	}
	writeLiteHTML(w, "")
}

func (c *liteCtx) fail(w http.ResponseWriter, err error) {
	// приложение показывает текст ошибки; формат как у lampac для закрытого контента
	var ae *apiError
	msg := err.Error()
	if errors.As(err, &ae) {
		msg = ae.Message
	}
	writeJSON(w, 200, map[string]any{"accsdb": false, "msg": "HDRezka: " + msg})
}

// ---------- журнал запросов приложения ----------

type statusRecorder struct {
	http.ResponseWriter
	status int
	size   int
	body   []byte // начало тела ответа — для журнала
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = 200
	}
	if room := 4096 - len(s.body); room > 0 {
		if len(b) < room {
			room = len(b)
		}
		s.body = append(s.body, b[:room]...)
	}
	n, err := s.ResponseWriter.Write(b)
	s.size += n
	return n, err
}

var reLongURL = regexp.MustCompile(`(https?://[^"\s]{30})[^"\s]*`)

// bodyPreview — тело ответа одной строкой, длинные ссылки укорочены
func bodyPreview(b []byte) string {
	s := reLongURL.ReplaceAllString(string(b), "$1…")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 600 {
		s = s[:600] + "…"
	}
	return s
}

// withLiteLog пишет в консоль запросы приложения Apple TV (/lite/*, /api/capabilities):
// метод, путь с параметрами, статус, размер и время — чтобы видеть, что именно запрашивает клиент.
func withLiteLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/lite/") && r.URL.Path != "/api/capabilities" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		uri := r.URL.Path
		if q, err := url.QueryUnescape(r.URL.RawQuery); err == nil && q != "" {
			uri += "?" + q
		}
		loc := ""
		if l := rec.Header().Get("Location"); l != "" {
			loc = " → " + l
		}
		log.Printf("app %s %s %s → %d %dB %s%s", r.RemoteAddr, r.Method, uri, rec.status, rec.size, time.Since(start).Round(time.Millisecond), loc)
		// ответ на получение ссылки — целиком (с укороченными адресами), чтобы видеть, что получил клиент
		if r.URL.Path == "/lite/hdrezka/video" && loc == "" {
			log.Printf("app   ↳ %s", bodyPreview(rec.body))
		}
	})
}

// ---------- обработчики ----------

func liteRoutes(mux *http.ServeMux, svc *Service) {
	mux.HandleFunc("GET /api/capabilities", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"lampac":   false,
			"features": map[string]int{"online": 1},
			"source": map[string]any{
				"protocol": "luxo-source/1",
				"id":       liteBalanser,
				"title":    "HDRezka",
				"family":   "lampac",
				"ttl":      3600,
			},
		})
	})

	mux.HandleFunc("GET /lite/withsearch", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, []string{liteBalanser})
	})

	mux.HandleFunc("GET /lite/events", func(w http.ResponseWriter, r *http.Request) {
		c := newLiteCtx(r)
		writeJSON(w, 200, []map[string]string{{
			"name":     "HDRezka",
			"url":      c.base + "/lite/" + liteBalanser,
			"balanser": liteBalanser,
		}})
	})

	mux.HandleFunc("GET /lite/hdrezka", func(w http.ResponseWriter, r *http.Request) {
		liteIndex(w, r, svc)
	})

	mux.HandleFunc("GET /lite/hdrezka/video", func(w http.ResponseWriter, r *http.Request) {
		liteVideoHandler(w, r, svc)
	})
}

// liteIndex: без href — поиск (уверенное совпадение открывается сразу, иначе варианты);
// с href — фильм (переводы) или сериал (переводы + сезоны / серии сезона s).
func liteIndex(w http.ResponseWriter, r *http.Request, svc *Service) {
	c := newLiteCtx(r)
	q := r.URL.Query()
	href := q.Get("href")

	if href == "" {
		year, _ := strconv.Atoi(q.Get("year"))
		query := SearchQuery{Title: c.title, OriginalTitle: c.orig, Year: year}
		if v := q.Get("serial"); v == "0" || v == "1" {
			serial := v == "1"
			query.Serial = &serial
		}
		// clarification=1 — пользователь уточнил запрос вручную: ищем как есть, без автовыбора
		clarification := q.Get("clarification") == "1"
		if clarification {
			query = SearchQuery{Title: c.title, Raw: true}
		}
		if strings.TrimSpace(query.Title+query.OriginalTitle) == "" {
			c.empty(w)
			return
		}

		items, err := svc.Search(query)
		if err != nil {
			c.fail(w, err)
			return
		}
		if len(items) == 0 {
			c.empty(w)
			return
		}
		if !clarification && confident(items) {
			href = items[0].URL
		} else {
			liteSimilarList(w, c, items)
			return
		}
	}

	info, err := svc.Info(href)
	if err != nil {
		c.fail(w, err)
		return
	}
	if len(info.Translators) == 0 {
		c.empty(w)
		return
	}

	if !info.Serial {
		liteMovieList(w, c, href, info)
		return
	}

	// Сериал: активный перевод — из t, иначе по умолчанию (не премиум)
	t := pickTranslator(info, q.Get("t"))
	s, _ := strconv.Atoi(q.Get("s"))
	if q.Get("s") == "" {
		s = -1
	}

	seasons, err := svc.Episodes(href, t.ID)
	if err != nil {
		c.fail(w, err)
		return
	}
	if len(seasons) == 0 {
		c.empty(w)
		return
	}

	// Кнопки переводов ведут на тот же сезон (если выбран) — как у lampac
	voices := make([]liteVoice, 0, len(info.Translators))
	for _, tr := range info.Translators {
		params := map[string]string{"href": href, "t": tr.ID}
		if s >= 0 {
			params["s"] = strconv.Itoa(s)
		}
		voices = append(voices, liteVoice{Method: "link", URL: c.link("/lite/hdrezka", params), Active: tr.ID == t.ID, Name: translatorName(tr)})
	}

	if s < 0 {
		liteSeasonList(w, c, href, t, seasons, voices)
		return
	}

	var season *Season
	for i := range seasons {
		if seasons[i].Season == s {
			season = &seasons[i]
		}
	}
	if season == nil {
		// у этого перевода нет такого сезона — показываем список сезонов
		liteSeasonList(w, c, href, t, seasons, voices)
		return
	}
	liteEpisodeList(w, c, href, info, t, season, voices)
}

func translatorName(t TranslatorInfo) string {
	if t.Premium {
		return t.Name + " (премиум)"
	}
	return t.Name
}

func pickTranslator(info *VideoInfo, id string) TranslatorInfo {
	list := info.Translators
	for _, t := range list {
		if id != "" && t.ID == id {
			return t
		}
	}
	for _, t := range list {
		if t.Default && !t.Premium {
			return t
		}
	}
	for _, t := range list {
		if !t.Premium {
			return t
		}
	}
	return list[0]
}

func liteSimilarList(w http.ResponseWriter, c *liteCtx, items []SearchItem) {
	data := make([]liteSimilar, 0, len(items))
	for _, it := range items {
		details := []string{}
		if len(it.OriginalTitles) > 0 {
			details = append(details, strings.Join(it.OriginalTitles, " / "))
		}
		if it.Details != "" {
			details = append(details, it.Details)
		}
		data = append(data, liteSimilar{
			Method:  "link",
			URL:     c.link("/lite/hdrezka", map[string]string{"href": it.URL}),
			Similar: true,
			Year:    it.Year,
			Details: strings.Join(details, " · "),
			Title:   it.Title,
			Img:     it.Poster,
		})
	}

	if c.rjson {
		writeJSON(w, 200, map[string]any{"type": "similar", "data": data})
		return
	}
	items2 := make([]any, len(data))
	titles := make([]string, len(data))
	for i, d := range data {
		items2[i] = d
		titles[i] = d.Title
	}
	writeLiteHTML(w, folderHTML(items2, titles))
}

func liteMovieList(w http.ResponseWriter, c *liteCtx, href string, info *VideoInfo) {
	title := c.displayTitle(info)
	year, _ := strconv.Atoi(info.Year)

	// Сначала обычные переводы, премиум — в конце (без премиум-аккаунта сайт их не отдаёт)
	ordered := []TranslatorInfo{}
	for _, t := range info.Translators {
		if !t.Premium {
			ordered = append(ordered, t)
		}
	}
	for _, t := range info.Translators {
		if t.Premium {
			ordered = append(ordered, t)
		}
	}

	data := make([]liteMovie, 0, len(ordered))
	for _, t := range ordered {
		name := translatorName(t)
		link := c.link("/lite/hdrezka/video", map[string]string{"href": href, "t": t.ID})
		data = append(data, liteMovie{
			Method:     "call",
			URL:        link,
			Stream:     link + "&play=true",
			Translate:  name,
			VoiceName:  name,
			Year:       year,
			Title:      title + " (" + name + ")",
			HLSTimeout: hlsManifestTimeout,
		})
	}

	if c.rjson {
		writeJSON(w, 200, map[string]any{"type": "movie", "data": data})
		return
	}
	items := make([]any, len(data))
	titles := make([]string, len(data))
	attrs := make([]string, len(data))
	for i, d := range data {
		items[i] = d
		titles[i] = d.Translate
	}
	writeLiteHTML(w, itemHTML(items, titles, attrs))
}

func liteSeasonList(w http.ResponseWriter, c *liteCtx, href string, t TranslatorInfo, seasons []Season, voices []liteVoice) {
	data := make([]liteSeason, 0, len(seasons))
	for _, s := range seasons {
		data = append(data, liteSeason{
			Method: "link",
			ID:     s.Season,
			URL:    c.link("/lite/hdrezka", map[string]string{"href": href, "t": t.ID, "s": strconv.Itoa(s.Season)}),
			Name:   strconv.Itoa(s.Season) + " сезон",
		})
	}

	if c.rjson {
		writeJSON(w, 200, map[string]any{"type": "season", "voice": voices, "data": data})
		return
	}
	items := make([]any, len(data))
	titles := make([]string, len(data))
	for i, d := range data {
		items[i] = d
		titles[i] = d.Name
	}
	writeLiteHTML(w, voiceHTML(voices)+folderHTML(items, titles))
}

func liteEpisodeList(w http.ResponseWriter, c *liteCtx, href string, info *VideoInfo, t TranslatorInfo, season *Season, voices []liteVoice) {
	title := c.displayTitle(info)

	data := make([]liteEpisode, 0, len(season.Episodes))
	for _, e := range season.Episodes {
		link := c.link("/lite/hdrezka/video", map[string]string{
			"href": href, "t": t.ID, "s": strconv.Itoa(season.Season), "e": strconv.Itoa(e),
		})
		data = append(data, liteEpisode{
			Method:     "call",
			URL:        link,
			Stream:     link + "&play=true",
			S:          season.Season,
			E:          e,
			Details:    translatorName(t),
			Name:       strconv.Itoa(e) + " серия",
			Title:      title + " (" + strconv.Itoa(e) + " серия)",
			HLSTimeout: hlsManifestTimeout,
		})
	}

	if c.rjson {
		writeJSON(w, 200, map[string]any{"type": "episode", "voice": voices, "data": data})
		return
	}
	items := make([]any, len(data))
	titles := make([]string, len(data))
	attrs := make([]string, len(data))
	for i, d := range data {
		items[i] = d
		titles[i] = d.Name
		attrs[i] = `s="` + strconv.Itoa(d.S) + `" e="` + strconv.Itoa(d.E) + `" `
	}
	writeLiteHTML(w, voiceHTML(voices)+itemHTML(items, titles, attrs))
}

// liteVideoHandler отдаёт поток как VideoTpl lampac: {"method":"play","url","title","quality","subtitles"}.
// play=true — сразу редирект на видео (RedirectToPlay у lampac).
func liteVideoHandler(w http.ResponseWriter, r *http.Request, svc *Service) {
	c := newLiteCtx(r)
	q := r.URL.Query()
	href := q.Get("href")
	if href == "" {
		c.fail(w, errors.New("нет href"))
		return
	}
	s, _ := strconv.Atoi(q.Get("s"))
	e, _ := strconv.Atoi(q.Get("e"))
	if (s > 0) != (e > 0) {
		s, e = 0, 0
	}

	st, err := svc.Stream(href, q.Get("t"), s, e)
	if err != nil {
		c.fail(w, err)
		return
	}

	// Только MP4: Infuse не воспроизводит HLS как поток (поддержка M3U8 у него частичная)
	quality := map[string]string{}
	for _, ql := range st.Qualities {
		quality[ql.Label] = ql.MP4
	}

	// Выбор по умолчанию — лучшее качество (сервер отдаёт его первым); приложение может сменить по quality
	best := st.Qualities[0].MP4

	if q.Get("play") == "true" {
		http.Redirect(w, r, best, http.StatusFound)
		return
	}

	subs := make([]liteSubtitle, 0, len(st.Subtitles))
	for _, sb := range st.Subtitles {
		subs = append(subs, liteSubtitle{Method: "link", URL: sb.URL, Label: sb.Label})
	}

	title := c.title
	if title == "" {
		title = c.orig
	}
	if s > 0 {
		title += " (" + strconv.Itoa(e) + " серия)"
	}

	writeJSON(w, 200, liteVideo{
		Title:      title,
		Method:     "play",
		URL:        best,
		Quality:    quality,
		Subtitles:  subs,
		HLSTimeout: hlsManifestTimeout,
	})
}
