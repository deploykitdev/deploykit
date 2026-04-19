package http

import (
	"encoding/json"
	"testing"

	"github.com/heyjorgedev/deploykit"
)

func TestEnvVarKeyTaken(t *testing.T) {
	svcID := "svc-1"
	projectID := "proj-1"
	otherSvcID := "svc-2"

	appliedFoo := &deploykit.EnvVar{
		ID:      "ev-foo",
		Scope:   deploykit.EnvVarScopeService,
		ScopeID: svcID,
		Key:     "FOO",
		Value:   "bar",
	}
	appliedOther := &deploykit.EnvVar{
		ID:      "ev-baz",
		Scope:   deploykit.EnvVarScopeService,
		ScopeID: otherSvcID,
		Key:     "FOO",
		Value:   "different",
	}

	// Helper to build a pending env_var.create entry.
	pendingCreate := func(scope deploykit.EnvVarScope, scopeID, key string) *deploykit.PendingChange {
		payload, _ := json.Marshal(deploykit.EnvVarCreatePayload{
			Scope: scope,
			Key:   key,
			Value: "v",
		})
		id := scopeID
		return &deploykit.PendingChange{
			Op:         deploykit.PendingOpEnvVarCreate,
			TargetType: deploykit.PendingTargetEnvVar,
			TargetID:   &id,
			Payload:    payload,
		}
	}

	pendingDelete := func(envVarID string) *deploykit.PendingChange {
		id := envVarID
		return &deploykit.PendingChange{
			Op:         deploykit.PendingOpEnvVarDelete,
			TargetType: deploykit.PendingTargetEnvVar,
			TargetID:   &id,
			Payload:    json.RawMessage(`{}`),
		}
	}

	tests := []struct {
		name    string
		applied []*deploykit.EnvVar
		changes []*deploykit.PendingChange
		key     string
		want    bool
	}{
		{
			name: "free",
			key:  "NEW_KEY",
			want: false,
		},
		{
			name:    "applied hit",
			applied: []*deploykit.EnvVar{appliedFoo},
			key:     "FOO",
			want:    true,
		},
		{
			name:    "applied row on a different scope does not count",
			applied: []*deploykit.EnvVar{appliedOther},
			key:     "FOO",
			want:    false,
		},
		{
			name: "pending create hit",
			changes: []*deploykit.PendingChange{
				pendingCreate(deploykit.EnvVarScopeService, svcID, "FOO"),
			},
			key:  "FOO",
			want: true,
		},
		{
			name: "pending create on different scope does not count",
			changes: []*deploykit.PendingChange{
				pendingCreate(deploykit.EnvVarScopeService, otherSvcID, "FOO"),
			},
			key:  "FOO",
			want: false,
		},
		{
			name:    "applied + staged delete = free to re-add",
			applied: []*deploykit.EnvVar{appliedFoo},
			changes: []*deploykit.PendingChange{pendingDelete("ev-foo")},
			key:     "FOO",
			want:    false,
		},
		{
			name:    "staged delete targeting a different env var doesn't unblock",
			applied: []*deploykit.EnvVar{appliedFoo},
			changes: []*deploykit.PendingChange{pendingDelete("ev-other")},
			key:     "FOO",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := envVarKeyTaken(
				tt.applied, tt.changes,
				deploykit.EnvVarScopeService, svcID, tt.key,
			)
			if got != tt.want {
				t.Errorf("envVarKeyTaken: got %v, want %v", got, tt.want)
			}
		})
	}

	// Sanity: project scope routing.
	t.Run("project-scope applied blocks project-scope add", func(t *testing.T) {
		applied := []*deploykit.EnvVar{{
			ID:      "ev-p",
			Scope:   deploykit.EnvVarScopeProject,
			ScopeID: projectID,
			Key:     "SHARED",
		}}
		if !envVarKeyTaken(applied, nil, deploykit.EnvVarScopeProject, projectID, "SHARED") {
			t.Error("expected project-scope applied row to block")
		}
	})
}
