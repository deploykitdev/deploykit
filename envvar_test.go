package deploykit

import (
	"reflect"
	"testing"
)

func TestResolveServiceRefs(t *testing.T) {
	lookup := func(name string) (string, bool) {
		switch name {
		case "db":
			return "dk-acme-db-0", true
		case "api":
			return "dk-acme-api-0", true
		}
		return "", false
	}

	tests := []struct {
		name      string
		value     string
		wantValue string
		wantRefs  []string
	}{
		{
			name:      "no placeholders",
			value:     "postgres://localhost:5432/x",
			wantValue: "postgres://localhost:5432/x",
			wantRefs:  nil,
		},
		{
			name:      "single ref resolves",
			value:     "${{db.HOST}}",
			wantValue: "dk-acme-db-0",
			wantRefs:  []string{"db"},
		},
		{
			name:      "ref embedded in URL",
			value:     "postgres://${{db.HOST}}:5432/app",
			wantValue: "postgres://dk-acme-db-0:5432/app",
			wantRefs:  []string{"db"},
		},
		{
			name:      "multiple refs",
			value:     "primary=${{db.HOST}}; api=${{api.HOST}}",
			wantValue: "primary=dk-acme-db-0; api=dk-acme-api-0",
			wantRefs:  []string{"db", "api"},
		},
		{
			name:      "duplicate refs collapse",
			value:     "${{db.HOST}}/${{db.HOST}}",
			wantValue: "dk-acme-db-0/dk-acme-db-0",
			wantRefs:  []string{"db"},
		},
		{
			name:      "unknown service stays literal",
			value:     "${{ghost.HOST}}",
			wantValue: "${{ghost.HOST}}",
			wantRefs:  []string{"ghost"},
		},
		{
			name:      "shell-style ${VAR} ignored",
			value:     "${PATH}:${{db.HOST}}",
			wantValue: "${PATH}:dk-acme-db-0",
			wantRefs:  []string{"db"},
		},
		{
			name:      "whitespace inside braces",
			value:     "${{ db.HOST }}",
			wantValue: "dk-acme-db-0",
			wantRefs:  []string{"db"},
		},
		{
			name:      "wrong field name not matched",
			value:     "${{db.PORT}}",
			wantValue: "${{db.PORT}}",
			wantRefs:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotValue, gotRefs := ResolveServiceRefs(tc.value, lookup)
			if gotValue != tc.wantValue {
				t.Errorf("value: got %q, want %q", gotValue, tc.wantValue)
			}
			if !reflect.DeepEqual(gotRefs, tc.wantRefs) {
				t.Errorf("refs: got %v, want %v", gotRefs, tc.wantRefs)
			}
		})
	}
}
