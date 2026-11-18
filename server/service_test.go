package main

import (
	"errors"
	"sort"
	"testing"
)

func TestHostAllowed(t *testing.T) {
	extra := map[string]bool{"media.example.com": true}
	for _, h := range []string{"umbrel", "nas", "localhost", "umbrel.local", "nas.lan", "box.home.arpa",
		"srv.internal", "192.168.1.10", "10.21.21.9", "::1", "media.example.com"} {
		if !hostAllowed(h, extra) {
			t.Errorf("hostAllowed(%q) = false, want true", h)
		}
	}
	for _, h := range []string{"evil.example", "attacker.com", "umbrel.local.evil.com", "lan.evil.org", ""} {
		if hostAllowed(h, extra) {
			t.Errorf("hostAllowed(%q) = true, want false", h)
		}
	}
}

func TestNormalizeMirror(t *testing.T) {
	ok := map[string]string{
		"example.com":                    "https://example.com",
		"https://Example.com/films/x":    "https://example.com",
		"http://mirror.example.org:8080": "http://mirror.example.org:8080",
		"  https://example.com/  ":       "https://example.com",
		"":                               "",
	}
	for in, want := range ok {
		got, err := normalizeMirror(in)
		if err != nil || got != want {
			t.Errorf("normalizeMirror(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"ftp://example.com", "https://", "https://user:pass@example.com", "://x"} {
		if _, err := normalizeMirror(bad); err == nil {
			t.Errorf("normalizeMirror(%q): want error", bad)
		}
	}
}

func TestQualityOrder(t *testing.T) {
	labels := []string{"360p", "4K", "1080p", "2K", "1080p Ultra", "720p", "480p"}
	for i := range labels {
		labels[i] = qualityLabel(labels[i])
	}
	sort.Slice(labels, func(a, b int) bool { return qualityRank(labels[a]) > qualityRank(labels[b]) })

	want := []string{"2160p (4K)", "1440p (2K)", "1080p Ultra", "1080p", "720p", "480p", "360p"}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("got %v, want %v", labels, want)
		}
	}
}

func TestLoginErrorText(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			// реальный ответ сайта при неверном пароле (message — объект)
			name: "object message",
			in:   `login response is not JSON (HTTP 200, content-type "application/json; charset=utf-8"): {"success":false,"message":{"email":"Введен неверный email или пароль"}}`,
			want: "Введен неверный email или пароль",
		},
		{
			name: "string message",
			in:   `login response is not JSON (HTTP 200): {"success":false,"message":"Слишком много попыток"}`,
			want: "Слишком много попыток",
		},
		{
			name: "plain library error",
			in:   "login failed: unknown error",
			want: "login failed: unknown error",
		},
		{
			name: "truncated body",
			in:   `login response is not JSON (HTTP 200): {"success":false,"message":{"email":"Введен...`,
			want: `login response is not JSON (HTTP 200): {"success":false,"message":{"email":"Введен...`,
		},
	}
	for _, c := range cases {
		if got := loginErrorText(errors.New(c.in)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
