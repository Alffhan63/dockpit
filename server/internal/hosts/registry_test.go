package hosts

import (
	"strings"
	"testing"
)

func TestValidID(t *testing.T) {
	for _, id := range []string{"local", "mac-mini", "srv1", "a"} {
		if !ValidID(id) {
			t.Errorf("ValidID(%q) = false", id)
		}
	}
	for _, id := range []string{"", "-lead", "UPPER", "has_underscore", "../etc", "a/b", "a b", string(make([]byte, 64))} {
		if ValidID(id) {
			t.Errorf("ValidID(%q) = true", id)
		}
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Mac mini":           "mac-mini",
		"  Linux Server #1 ": "linux-server-1",
		"STB":                "stb",
		"!!!":                "host",
		"日本":                 "host",
	}
	for in, want := range cases {
		if got := Slug(in); got != want || !ValidID(got) {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Slug(strings.Repeat("ab-", 30)); len(got) > 40 || !ValidID(got) {
		t.Errorf("long slug %q", got)
	}
}
