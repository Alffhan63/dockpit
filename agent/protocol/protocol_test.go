package protocol

import (
	"strings"
	"testing"
)

func TestValidContainerRef(t *testing.T) {
	for _, ref := range []string{"web", "245cdae5c677", "funpos-funpos-1", "my_app.v2"} {
		if !ValidContainerRef(ref) {
			t.Errorf("ValidContainerRef(%q) = false", ref)
		}
	}
	for _, ref := range []string{"", "-x", ".x", "a/b", "../etc", "a b", "a?b", "a%2Fb", strings.Repeat("a", 129)} {
		if ValidContainerRef(ref) {
			t.Errorf("ValidContainerRef(%q) = true", ref)
		}
	}
}

func TestValidImageID(t *testing.T) {
	for _, id := range []string{"sha256:" + strings.Repeat("a", 64), strings.Repeat("0", 64), "abcdef123456"} {
		if !ValidImageID(id) {
			t.Errorf("ValidImageID(%q) = false", id)
		}
	}
	for _, id := range []string{"", "redis:7", "abc", "sha256:", "sha256:XYZ123456789", "../x", strings.Repeat("a", 65)} {
		if ValidImageID(id) {
			t.Errorf("ValidImageID(%q) = true", id)
		}
	}
}

func TestValidVolumeName(t *testing.T) {
	for _, n := range []string{"demo_db", "my-data.v2", strings.Repeat("ab", 32)} {
		if !ValidVolumeName(n) {
			t.Errorf("ValidVolumeName(%q) = false", n)
		}
	}
	for _, n := range []string{"", "-x", "../x", "a/b", "a b", strings.Repeat("a", 256)} {
		if ValidVolumeName(n) {
			t.Errorf("ValidVolumeName(%q) = true", n)
		}
	}
}
