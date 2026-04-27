package presets

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/deploykitdev/deploykit"
)

func TestNewLoadsAllEmbeddedPresets(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	wantIDs := map[string]bool{
		"postgres": false,
		"mysql":    false,
		"mariadb":  false,
		"redis":    false,
	}
	for _, p := range got {
		if _, ok := wantIDs[p.ID]; ok {
			wantIDs[p.ID] = true
		}
	}
	for id, seen := range wantIDs {
		if !seen {
			t.Errorf("expected preset %q to load, missing", id)
		}
	}
}

func TestNewRejectsUnknownGenerator(t *testing.T) {
	bad := &deploykit.Preset{
		ID:    "x",
		Name:  "X",
		Image: "x:1",
		EnvVars: []deploykit.PresetEnvVar{
			{Key: "PASSWORD", Generate: "totally_not_real"},
		},
	}
	if err := validatePreset(bad, "test.yaml"); err == nil {
		t.Fatal("expected error for unknown generator, got nil")
	} else if !strings.Contains(err.Error(), "unknown generator") {
		t.Errorf("error %v should mention unknown generator", err)
	}
}

func TestNewRejectsBothValueAndGenerate(t *testing.T) {
	bad := &deploykit.Preset{
		ID:    "x",
		Name:  "X",
		Image: "x:1",
		EnvVars: []deploykit.PresetEnvVar{
			{Key: "X", Value: "literal", Generate: "password_24"},
		},
	}
	if err := validatePreset(bad, "test.yaml"); err == nil {
		t.Fatal("expected error when env var has both value and generate, got nil")
	}
}

func TestGetMaterializesGenerators(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p, err := svc.Get(context.Background(), "postgres")
	if err != nil {
		t.Fatalf("Get(postgres): %v", err)
	}
	for _, ev := range p.EnvVars {
		if ev.Generate != "" {
			t.Errorf("env var %s still has Generate=%q after Get", ev.Key, ev.Generate)
		}
		if ev.Value == "" {
			t.Errorf("env var %s has empty Value after Get", ev.Key)
		}
	}
}

func TestGetReturnsFreshRandomsEachCall(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a, _ := svc.Get(context.Background(), "postgres")
	b, _ := svc.Get(context.Background(), "postgres")
	aPwd := envValue(a.EnvVars, "POSTGRES_PASSWORD")
	bPwd := envValue(b.EnvVars, "POSTGRES_PASSWORD")
	if aPwd == "" || bPwd == "" {
		t.Fatal("missing POSTGRES_PASSWORD")
	}
	if aPwd == bPwd {
		t.Errorf("two Get calls returned identical password %q — generators should be fresh", aPwd)
	}
}

func TestGetUnknownIDReturnsNotFound(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := svc.Get(context.Background(), "does-not-exist"); deploykit.ErrorCode(err) != deploykit.ENOTFOUND {
		t.Errorf("expected ENOTFOUND, got %v", err)
	}
}

func TestRandomHex32Length(t *testing.T) {
	v, err := randomHex32()
	if err != nil {
		t.Fatalf("randomHex32: %v", err)
	}
	if len(v) != 64 {
		t.Errorf("randomHex32: got len %d, want 64", len(v))
	}
	if _, err := hex.DecodeString(v); err != nil {
		t.Errorf("randomHex32 output is not valid hex: %v", err)
	}
}

func TestRandomBase64URL24Length(t *testing.T) {
	v, err := randomBase64URL24()
	if err != nil {
		t.Fatalf("randomBase64URL24: %v", err)
	}
	if len(v) != 32 {
		t.Errorf("randomBase64URL24: got len %d, want 32", len(v))
	}
	if _, err := base64.RawURLEncoding.DecodeString(v); err != nil {
		t.Errorf("randomBase64URL24 output is not valid base64url: %v", err)
	}
}

func TestPassword24ShapeAndAlphabet(t *testing.T) {
	v, err := password24()
	if err != nil {
		t.Fatalf("password24: %v", err)
	}
	if len(v) != 24 {
		t.Errorf("password24: got len %d, want 24", len(v))
	}
	for i, c := range v {
		if !strings.ContainsRune(passwordAlphabet, c) {
			t.Errorf("password24[%d]=%q outside alphabet", i, c)
		}
	}
}

func envValue(evs []deploykit.PresetEnvVar, key string) string {
	for _, ev := range evs {
		if ev.Key == key {
			return ev.Value
		}
	}
	return ""
}
