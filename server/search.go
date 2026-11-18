package main

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/n0madic/go-hdrezka"
)

// Поиск на сайте двумя способами одновременно:
//   - быстрый (/engine/ajax/search.php): оригинальные названия, пометка «сериал», рейтинг;
//   - полный (/search/): постер, год, страна и жанр, «N сезон, M серия».
// Результаты объединяются по адресу страницы.

type SearchItem struct {
	URL            string   `json:"url"`
	Title          string   `json:"title"`
	OriginalTitles []string `json:"original_titles"`
	Year           int      `json:"year,omitempty"`
	Serial         bool     `json:"serial"`
	Category       string   `json:"category"`
	Rating         string   `json:"rating,omitempty"`
	Poster         string   `json:"poster,omitempty"`
	Details        string   `json:"details,omitempty"`  // "США, Драмы" из полного поиска
	Progress       string   `json:"progress,omitempty"` // "1 сезон, 10 серия"
	Score          int      `json:"score"`
}

type SearchQuery struct {
	Title         string
	OriginalTitle string
	Year          int
	Serial        *bool
	// Raw — запрос как есть (ручное уточнение в плагине), без подстановки года
	Raw bool
}

// fullSearchLimit — сколько карточек брать из полного поиска (одна страница сайта)
const fullSearchLimit = 20

var reYear = regexp.MustCompile(`^\d{4}`)

func pathKey(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		return u.Path
	}
	return raw
}

func categoryOf(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return strings.Split(strings.Trim(u.Path, "/"), "/")[0]
	}
	return ""
}

// parseQuickItem разбирает быстрый поиск: "Сёгун (Shôgun / Shogun, сериал, 2024 - ...)".
func parseQuickItem(c *hdrezka.CoverItem) SearchItem {
	item := SearchItem{
		URL:    c.URL,
		Title:  strings.TrimSpace(c.Title),
		Rating: strings.TrimSpace(c.Info),
	}

	extra := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(c.Description), item.Title))
	extra = strings.TrimSuffix(strings.TrimPrefix(extra, "("), ")")

	names := []string{}
	for _, part := range strings.Split(extra, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case reYear.MatchString(part):
			item.Year, _ = strconv.Atoi(part[:4])
		case strings.EqualFold(part, "сериал"):
			item.Serial = true
		default:
			names = append(names, part)
		}
	}
	for _, n := range strings.Split(strings.Join(names, ", "), " / ") {
		if n = strings.TrimSpace(n); n != "" {
			item.OriginalTitles = append(item.OriginalTitles, n)
		}
	}

	item.Category = categoryOf(c.URL)
	if item.Category == "series" {
		item.Serial = true
	}
	return item
}

// parseFullItem разбирает полный поиск: Description "2024, США, Драмы", Info "1 сезон, 10 серия".
func parseFullItem(c *hdrezka.CoverItem) SearchItem {
	item := SearchItem{
		URL:      c.URL,
		Title:    strings.TrimSpace(c.Title),
		Poster:   strings.TrimSpace(c.Cover),
		Progress: strings.TrimSpace(c.Info),
		Category: categoryOf(c.URL),
	}

	parts := strings.Split(c.Description, ",")
	rest := []string{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if item.Year == 0 && reYear.MatchString(p) {
			item.Year, _ = strconv.Atoi(p[:4])
			continue
		}
		rest = append(rest, p)
	}
	item.Details = strings.Join(rest, ", ")
	item.Serial = item.Category == "series" || strings.Contains(strings.ToLower(item.Progress), "сезон")
	return item
}

// merge дополняет элемент быстрого поиска данными полного (и наоборот).
func merge(dst *SearchItem, src SearchItem) {
	if dst.Poster == "" {
		dst.Poster = src.Poster
	}
	if dst.Details == "" {
		dst.Details = src.Details
	}
	if dst.Progress == "" {
		dst.Progress = src.Progress
	}
	if dst.Year == 0 {
		dst.Year = src.Year
	}
	if len(dst.OriginalTitles) == 0 {
		dst.OriginalTitles = src.OriginalTitles
	}
	if dst.Rating == "" {
		dst.Rating = src.Rating
	}
	dst.Serial = dst.Serial || src.Serial
}

// searchOnce: быстрый и полный поиск параллельно. Ошибка полного поиска не критична (останутся без постеров).
func searchOnce(hd *hdrezka.HDRezka, query string) ([]SearchItem, error) {
	var (
		wg          sync.WaitGroup
		quick, full []*hdrezka.CoverItem
		quickErr    error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		quick, quickErr = hd.QuickSearch(query)
	}()
	go func() {
		defer wg.Done()
		full, _ = hd.Search(query, fullSearchLimit)
	}()
	wg.Wait()

	if quickErr != nil && len(full) == 0 {
		return nil, newError(502, "upstream", quickErr)
	}

	index := map[string]int{}
	items := []SearchItem{}
	for _, c := range quick {
		if c.URL == "" {
			continue
		}
		key := pathKey(c.URL)
		if _, ok := index[key]; ok {
			continue
		}
		index[key] = len(items)
		items = append(items, parseQuickItem(c))
	}
	for _, c := range full {
		if c.URL == "" {
			continue
		}
		fi := parseFullItem(c)
		key := pathKey(c.URL)
		if i, ok := index[key]; ok {
			merge(&items[i], fi)
			continue
		}
		index[key] = len(items)
		items = append(items, fi)
	}
	return items, nil
}

var reNonWord = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func normalizeTitle(s string) string {
	s = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, "ё", "е"), "Ё", "Е"))
	return strings.TrimSpace(reNonWord.ReplaceAllString(s, " "))
}

// score: совпадение названия, год (±1), тип (фильм/сериал). 90+ — уверенное совпадение (название + год).
func score(it *SearchItem, want map[string]bool, q SearchQuery) {
	it.Score = 0
	names := []string{normalizeTitle(it.Title)}
	for _, o := range it.OriginalTitles {
		names = append(names, normalizeTitle(o))
	}

	exact, partial := false, false
	for _, n := range names {
		if n == "" {
			continue
		}
		for w := range want {
			if n == w {
				exact = true
			} else if strings.Contains(n, w) || strings.Contains(w, n) {
				partial = true
			}
		}
	}
	switch {
	case exact:
		it.Score += 50
	case partial:
		it.Score += 20
	}

	if q.Year > 0 && it.Year > 0 {
		switch d := q.Year - it.Year; {
		case d == 0:
			it.Score += 40
		case d == 1 || d == -1:
			it.Score += 15
		default:
			it.Score -= 30
		}
	}
	if q.Serial != nil && it.Serial == *q.Serial {
		it.Score += 10
	}
}

func confident(items []SearchItem) bool {
	return len(items) > 0 && items[0].Score >= 90 && (len(items) == 1 || items[1].Score < items[0].Score)
}

// Search: сначала «название год» (сайт так находит точнее всего), затем «оригинальное название год»,
// и только если уверенного совпадения нет — названия без года. Останавливаемся на первом уверенном.
func (s *Service) Search(q SearchQuery) ([]SearchItem, error) {
	hd, err := s.client()
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(q.Title)
	original := strings.TrimSpace(q.OriginalTitle)
	if strings.EqualFold(original, title) {
		original = ""
	}

	queries := []string{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		for _, e := range queries {
			if strings.EqualFold(e, v) {
				return
			}
		}
		if v != "" {
			queries = append(queries, v)
		}
	}
	if q.Raw {
		add(title)
	} else {
		if q.Year > 0 {
			y := " " + strconv.Itoa(q.Year)
			if title != "" {
				add(title + y)
			}
			if original != "" {
				add(original + y)
			}
		}
		add(title)
		add(original)
	}

	want := map[string]bool{}
	if !q.Raw {
		for _, t := range []string{title, original} {
			if n := normalizeTitle(t); n != "" {
				want[n] = true
			}
		}
	}

	index := map[string]int{}
	items := []SearchItem{}
	var lastErr error

	for _, query := range queries {
		found, err := searchOnce(hd, query)
		if err != nil {
			lastErr = err
			continue
		}
		for _, it := range found {
			key := pathKey(it.URL)
			if i, ok := index[key]; ok {
				merge(&items[i], it)
				continue
			}
			index[key] = len(items)
			items = append(items, it)
		}

		for i := range items {
			score(&items[i], want, q)
		}
		sort.SliceStable(items, func(a, b int) bool { return items[a].Score > items[b].Score })
		for i := range items {
			index[pathKey(items[i].URL)] = i
		}

		if !q.Raw && confident(items) {
			break
		}
	}

	if len(items) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return items, nil
}
