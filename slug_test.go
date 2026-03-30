package deploykit

import (
	"regexp"
	"testing"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

func TestGenerateSlug(t *testing.T) {
	t.Run("normal name", func(t *testing.T) {
		slug := GenerateSlug("My Cool App")
		if !slugPattern.MatchString(slug) {
			t.Fatalf("slug %q does not match expected pattern", slug)
		}
		if len(slug) < len("my-cool-app-")+6 {
			t.Fatalf("slug %q is too short", slug)
		}
	})

	t.Run("special characters", func(t *testing.T) {
		slug := GenerateSlug("Hello, World!!!")
		if !slugPattern.MatchString(slug) {
			t.Fatalf("slug %q does not match expected pattern", slug)
		}
	})

	t.Run("already slugified", func(t *testing.T) {
		slug := GenerateSlug("my-app")
		if !slugPattern.MatchString(slug) {
			t.Fatalf("slug %q does not match expected pattern", slug)
		}
	})

	t.Run("long name truncated", func(t *testing.T) {
		long := "this-is-a-very-long-project-name-that-should-be-truncated-to-fit-within-limits"
		slug := GenerateSlug(long)
		// 40 chars base + 1 hyphen + 6 hex = 47 max
		if len(slug) > 47 {
			t.Fatalf("slug %q is too long (%d chars)", slug, len(slug))
		}
		if !slugPattern.MatchString(slug) {
			t.Fatalf("slug %q does not match expected pattern", slug)
		}
	})

	t.Run("all special chars uses fallback", func(t *testing.T) {
		slug := GenerateSlug("!!!")
		if !slugPattern.MatchString(slug) {
			t.Fatalf("slug %q does not match expected pattern", slug)
		}
		if len(slug) < len("project-")+6 {
			t.Fatalf("slug %q should start with project- prefix", slug)
		}
	})

	t.Run("unique slugs for same name", func(t *testing.T) {
		a := GenerateSlug("my-app")
		b := GenerateSlug("my-app")
		if a == b {
			t.Fatalf("expected different slugs, got %q both times", a)
		}
	})
}
