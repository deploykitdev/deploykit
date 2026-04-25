package presets

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// passwordAlphabet is the character set used by password_24. Alphanumeric
// only — keeps generated passwords copy-paste safe in any shell, env file,
// or compose file the user might reach for.
const passwordAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// generators maps the typed YAML `generate:` value to a function that
// produces a fresh random string. Keep names stable — they're part of the
// preset YAML schema users may author.
var generators = map[string]func() (string, error){
	"random_hex_32":    randomHex32,
	"random_base64_24": randomBase64URL24,
	"password_24":      password24,
}

// generate runs the named generator. Returns ErrUnknownGenerator if the
// name isn't registered.
func generate(name string) (string, error) {
	fn, ok := generators[name]
	if !ok {
		return "", fmt.Errorf("unknown generator %q", name)
	}
	return fn()
}

// hasGenerator reports whether the given name is registered.
func hasGenerator(name string) bool {
	_, ok := generators[name]
	return ok
}

func randomHex32() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func randomBase64URL24() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func password24() (string, error) {
	const length = 24
	out := make([]byte, length)
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := range length {
		out[i] = passwordAlphabet[int(buf[i])%len(passwordAlphabet)]
	}
	return string(out), nil
}
