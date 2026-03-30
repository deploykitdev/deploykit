package deploykit

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"unicode"
)

// GenerateSlug creates a URL-safe, Docker-safe slug from a project name
// with a random 6-character hex suffix for uniqueness.
// The result contains only lowercase alphanumeric characters and hyphens.
func GenerateSlug(name string) string {
	base := slugify(name)
	if base == "" {
		base = "project"
	}
	if len(base) > 40 {
		base = base[:40]
		base = strings.TrimRight(base, "-")
	}
	return base + "-" + randomHex(3)
}

// slugify converts a string to a lowercase, hyphen-separated slug.
func slugify(s string) string {
	s = strings.ToLower(s)

	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen {
			b.WriteByte('-')
			prevHyphen = true
		}
	}

	return strings.Trim(b.String(), "-")
}

// randomHex returns n random bytes encoded as a 2*n character hex string.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
