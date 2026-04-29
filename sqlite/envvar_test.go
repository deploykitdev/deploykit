package sqlite

import (
	"context"
	"testing"

	"github.com/deploykitdev/deploykit"
)

// MustCreateEnvVar is a test helper that creates an env var or fails the test.
func MustCreateEnvVar(t *testing.T, s *EnvVarService, scope deploykit.EnvVarScope, scopeID, key, value string) *deploykit.EnvVar {
	t.Helper()
	ev, err := s.CreateEnvVar(context.Background(), scope, scopeID, deploykit.EnvVarCreate{Key: key, Value: value})
	if err != nil {
		t.Fatal("creating seed env var:", err)
	}
	return ev
}

func TestEnvVarService_CreateEnvVar(t *testing.T) {
	t.Run("project scope", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		svc := NewEnvVarService(db)

		ev, err := svc.CreateEnvVar(context.Background(), deploykit.EnvVarScopeProject, project.ID, deploykit.EnvVarCreate{
			Key: "DB_URL", Value: "postgres://localhost/db",
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if ev.ID == "" {
			t.Fatal("expected non-empty ID")
		}
		if ev.Scope != deploykit.EnvVarScopeProject {
			t.Fatalf("got scope %q, want %q", ev.Scope, deploykit.EnvVarScopeProject)
		}
		if ev.ScopeID != project.ID {
			t.Fatalf("got scope_id %q, want %q", ev.ScopeID, project.ID)
		}
		if ev.Key != "DB_URL" || ev.Value != "postgres://localhost/db" {
			t.Fatalf("unexpected key/value: %q=%q", ev.Key, ev.Value)
		}
		if ev.CreatedAt.IsZero() || ev.UpdatedAt.IsZero() {
			t.Fatal("expected non-zero timestamps")
		}
	})

	t.Run("service scope", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		svc := NewEnvVarService(db)

		ev, err := svc.CreateEnvVar(context.Background(), deploykit.EnvVarScopeService, service.ID, deploykit.EnvVarCreate{
			Key: "PORT", Value: "8080",
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if ev.Scope != deploykit.EnvVarScopeService {
			t.Fatalf("got scope %q, want %q", ev.Scope, deploykit.EnvVarScopeService)
		}
		if ev.ScopeID != service.ID {
			t.Fatalf("got scope_id %q, want %q", ev.ScopeID, service.ID)
		}
	})

	t.Run("invalid scope", func(t *testing.T) {
		db := MustOpenDB(t)
		svc := NewEnvVarService(db)

		_, err := svc.CreateEnvVar(context.Background(), deploykit.EnvVarScope("bogus"), "x", deploykit.EnvVarCreate{
			Key: "K", Value: "v",
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got code %q, want %q", code, deploykit.EINVALID)
		}
	})

	t.Run("empty key", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		svc := NewEnvVarService(db)

		_, err := svc.CreateEnvVar(context.Background(), deploykit.EnvVarScopeProject, project.ID, deploykit.EnvVarCreate{
			Key: "", Value: "v",
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got code %q, want %q", code, deploykit.EINVALID)
		}
	})

	t.Run("invalid key format", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		svc := NewEnvVarService(db)

		cases := []string{"1FOO", "FOO-BAR", "FOO BAR", "foo.bar"}
		for _, key := range cases {
			_, err := svc.CreateEnvVar(context.Background(), deploykit.EnvVarScopeProject, project.ID, deploykit.EnvVarCreate{
				Key: key, Value: "v",
			})
			if err == nil {
				t.Fatalf("expected error for key %q", key)
			}
			if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
				t.Fatalf("key %q: got code %q, want %q", key, code, deploykit.EINVALID)
			}
		}
	})

	t.Run("unknown project", func(t *testing.T) {
		db := MustOpenDB(t)
		svc := NewEnvVarService(db)

		_, err := svc.CreateEnvVar(context.Background(), deploykit.EnvVarScopeProject, "nope", deploykit.EnvVarCreate{
			Key: "K", Value: "v",
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})

	t.Run("duplicate key conflict", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		svc := NewEnvVarService(db)

		MustCreateEnvVar(t, svc, deploykit.EnvVarScopeProject, project.ID, "FOO", "bar")

		_, err := svc.CreateEnvVar(context.Background(), deploykit.EnvVarScopeProject, project.ID, deploykit.EnvVarCreate{
			Key: "FOO", Value: "other",
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ECONFLICT {
			t.Fatalf("got code %q, want %q", code, deploykit.ECONFLICT)
		}
	})

	t.Run("same key allowed across scopes", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		svc := NewEnvVarService(db)

		MustCreateEnvVar(t, svc, deploykit.EnvVarScopeProject, project.ID, "FOO", "from-project")
		MustCreateEnvVar(t, svc, deploykit.EnvVarScopeService, service.ID, "FOO", "from-service")
	})
}

func TestEnvVarService_ListEnvVars(t *testing.T) {
	t.Run("ordered by key", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		svc := NewEnvVarService(db)

		MustCreateEnvVar(t, svc, deploykit.EnvVarScopeProject, project.ID, "ZEBRA", "z")
		MustCreateEnvVar(t, svc, deploykit.EnvVarScopeProject, project.ID, "ALPHA", "a")
		MustCreateEnvVar(t, svc, deploykit.EnvVarScopeProject, project.ID, "MIKE", "m")

		got, err := svc.ListEnvVars(context.Background(), deploykit.EnvVarScopeProject, project.ID)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if len(got) != 3 {
			t.Fatalf("got %d env vars, want 3", len(got))
		}
		wantOrder := []string{"ALPHA", "MIKE", "ZEBRA"}
		for i, ev := range got {
			if ev.Key != wantOrder[i] {
				t.Fatalf("position %d: got %q, want %q", i, ev.Key, wantOrder[i])
			}
		}
	})

	t.Run("scoped to target", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		svc := NewEnvVarService(db)

		MustCreateEnvVar(t, svc, deploykit.EnvVarScopeProject, project.ID, "PROJ", "1")
		MustCreateEnvVar(t, svc, deploykit.EnvVarScopeService, service.ID, "SVC", "1")

		projVars, _ := svc.ListEnvVars(context.Background(), deploykit.EnvVarScopeProject, project.ID)
		if len(projVars) != 1 || projVars[0].Key != "PROJ" {
			t.Fatalf("project list unexpected: %+v", projVars)
		}
		svcVars, _ := svc.ListEnvVars(context.Background(), deploykit.EnvVarScopeService, service.ID)
		if len(svcVars) != 1 || svcVars[0].Key != "SVC" {
			t.Fatalf("service list unexpected: %+v", svcVars)
		}
	})

	t.Run("empty", func(t *testing.T) {
		db := MustOpenDB(t)
		svc := NewEnvVarService(db)

		got, err := svc.ListEnvVars(context.Background(), deploykit.EnvVarScopeProject, "whatever")
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if got == nil {
			t.Fatal("expected non-nil slice")
		}
		if len(got) != 0 {
			t.Fatalf("got %d env vars, want 0", len(got))
		}
	})
}

func TestEnvVarService_UpdateEnvVar(t *testing.T) {
	t.Run("update value", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		svc := NewEnvVarService(db)
		original := MustCreateEnvVar(t, svc, deploykit.EnvVarScopeProject, project.ID, "FOO", "before")

		newValue := "after"
		got, err := svc.UpdateEnvVar(context.Background(), original.ID, deploykit.EnvVarUpdate{Value: &newValue})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if got.Value != "after" {
			t.Fatalf("got value %q, want %q", got.Value, "after")
		}
		if got.Key != "FOO" {
			t.Fatalf("key should not change: got %q", got.Key)
		}
	})

	t.Run("nil value no-op", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		svc := NewEnvVarService(db)
		original := MustCreateEnvVar(t, svc, deploykit.EnvVarScopeProject, project.ID, "FOO", "keep")

		got, err := svc.UpdateEnvVar(context.Background(), original.ID, deploykit.EnvVarUpdate{Value: nil})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if got.Value != "keep" {
			t.Fatalf("got value %q, want %q", got.Value, "keep")
		}
	})

	t.Run("not found", func(t *testing.T) {
		db := MustOpenDB(t)
		svc := NewEnvVarService(db)

		v := "x"
		_, err := svc.UpdateEnvVar(context.Background(), "nope", deploykit.EnvVarUpdate{Value: &v})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})
}

func TestEnvVarService_DeleteEnvVar(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		svc := NewEnvVarService(db)
		ev := MustCreateEnvVar(t, svc, deploykit.EnvVarScopeProject, project.ID, "FOO", "bar")

		if err := svc.DeleteEnvVar(context.Background(), ev.ID); err != nil {
			t.Fatal("unexpected error:", err)
		}

		_, err := svc.GetEnvVar(context.Background(), ev.ID)
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})

	t.Run("not found", func(t *testing.T) {
		db := MustOpenDB(t)
		svc := NewEnvVarService(db)

		err := svc.DeleteEnvVar(context.Background(), "nope")
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})
}

func TestEnvVarService_ResolveForService(t *testing.T) {
	t.Run("merges project and service with service winning", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		svc := NewEnvVarService(db)

		// Project has DB_URL and QUEUE_CONNECTION.
		MustCreateEnvVar(t, svc, deploykit.EnvVarScopeProject, project.ID, "DB_URL", "pg://project")
		MustCreateEnvVar(t, svc, deploykit.EnvVarScopeProject, project.ID, "QUEUE_CONNECTION", "database")
		// Service overrides QUEUE_CONNECTION and adds PORT.
		MustCreateEnvVar(t, svc, deploykit.EnvVarScopeService, service.ID, "QUEUE_CONNECTION", "redis")
		MustCreateEnvVar(t, svc, deploykit.EnvVarScopeService, service.ID, "PORT", "8080")

		merged, err := svc.ResolveForService(context.Background(), service.ID)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if merged["DB_URL"] != "pg://project" {
			t.Fatalf("DB_URL: got %q, want %q", merged["DB_URL"], "pg://project")
		}
		if merged["QUEUE_CONNECTION"] != "redis" {
			t.Fatalf("QUEUE_CONNECTION: got %q, want %q (service should win)", merged["QUEUE_CONNECTION"], "redis")
		}
		if merged["PORT"] != "8080" {
			t.Fatalf("PORT: got %q, want %q", merged["PORT"], "8080")
		}
		if len(merged) != 3 {
			t.Fatalf("got %d entries, want 3", len(merged))
		}
	})

	t.Run("empty result is non-nil", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		svc := NewEnvVarService(db)

		merged, err := svc.ResolveForService(context.Background(), service.ID)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if merged == nil {
			t.Fatal("expected non-nil map")
		}
		if len(merged) != 0 {
			t.Fatalf("got %d entries, want 0", len(merged))
		}
	})

	t.Run("resolves ${{name.HOST}} to sibling hostname", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		MustCreateService(t, db, project.ID, "db")
		web := MustCreateService(t, db, project.ID, "web")
		svc := NewEnvVarService(db)

		MustCreateEnvVar(t, svc, deploykit.EnvVarScopeService, web.ID, "DB_HOST", "${{db.HOST}}")
		MustCreateEnvVar(t, svc, deploykit.EnvVarScopeService, web.ID, "DB_URL", "postgres://${{db.HOST}}:5432")

		merged, refs, err := svc.ResolveForServiceWithRefs(context.Background(), web.ID)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		wantHost := "dk-" + project.Slug + "-db-0"
		if merged["DB_HOST"] != wantHost {
			t.Fatalf("DB_HOST: got %q, want %q", merged["DB_HOST"], wantHost)
		}
		want := "postgres://" + wantHost + ":5432"
		if merged["DB_URL"] != want {
			t.Fatalf("DB_URL: got %q, want %q", merged["DB_URL"], want)
		}
		if len(refs["DB_HOST"]) != 1 || refs["DB_HOST"][0] != "db" {
			t.Fatalf("refs[DB_HOST]: got %v, want [db]", refs["DB_HOST"])
		}
		if len(refs["DB_URL"]) != 1 || refs["DB_URL"][0] != "db" {
			t.Fatalf("refs[DB_URL]: got %v, want [db]", refs["DB_URL"])
		}
	})

	t.Run("leaves unresolved refs to unknown services as literal", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		web := MustCreateService(t, db, project.ID, "web")
		svc := NewEnvVarService(db)

		MustCreateEnvVar(t, svc, deploykit.EnvVarScopeService, web.ID, "X", "${{ghost.HOST}}")

		merged, refs, err := svc.ResolveForServiceWithRefs(context.Background(), web.ID)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if merged["X"] != "${{ghost.HOST}}" {
			t.Fatalf("X: got %q, want literal placeholder", merged["X"])
		}
		// Refs are still recorded so the UI can warn.
		if len(refs["X"]) != 1 || refs["X"][0] != "ghost" {
			t.Fatalf("refs[X]: got %v, want [ghost]", refs["X"])
		}
	})

	t.Run("unknown service", func(t *testing.T) {
		db := MustOpenDB(t)
		svc := NewEnvVarService(db)

		_, err := svc.ResolveForService(context.Background(), "nope")
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})
}

func TestEnvVarService_CascadeDelete(t *testing.T) {
	t.Run("project delete cascades", func(t *testing.T) {
		db := MustOpenDB(t)
		projectSvc := NewProjectService(db)
		project := MustCreateProject(t, projectSvc, "p")
		svc := NewEnvVarService(db)

		ev := MustCreateEnvVar(t, svc, deploykit.EnvVarScopeProject, project.ID, "FOO", "bar")

		if err := projectSvc.DeleteProject(context.Background(), project.ID); err != nil {
			t.Fatal("deleting project:", err)
		}

		_, err := svc.GetEnvVar(context.Background(), ev.ID)
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("env var should be cascaded: got code %q", code)
		}
	})

	t.Run("service delete cascades", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		serviceSvc := NewServiceService(db)
		service, err := serviceSvc.CreateService(context.Background(), project.ID, deploykit.ServiceCreate{Name: "web"})
		if err != nil {
			t.Fatal("creating service:", err)
		}
		svc := NewEnvVarService(db)

		ev := MustCreateEnvVar(t, svc, deploykit.EnvVarScopeService, service.ID, "FOO", "bar")

		if err := serviceSvc.DeleteService(context.Background(), service.ID); err != nil {
			t.Fatal("deleting service:", err)
		}

		_, err = svc.GetEnvVar(context.Background(), ev.ID)
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("env var should be cascaded: got code %q", code)
		}
	})

}
