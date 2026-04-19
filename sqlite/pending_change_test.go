package sqlite_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/heyjorgedev/deploykit"
	"github.com/heyjorgedev/deploykit/sqlite"
)

func TestPendingChangeService_AppendAndList(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")

	payload, _ := json.Marshal(deploykit.ProjectUpdatePayload{Name: strPtr("renamed")})

	first, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpProjectUpdate,
		TargetType: deploykit.PendingTargetProject,
		TargetID:   &proj.ID,
		Payload:    payload,
	})
	if err != nil {
		t.Fatalf("append first: %v", err)
	}
	if first.Seq != 1 {
		t.Errorf("first seq: got %d, want 1", first.Seq)
	}

	second, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpProjectUpdate,
		TargetType: deploykit.PendingTargetProject,
		TargetID:   &proj.ID,
		Payload:    payload,
	})
	if err != nil {
		t.Fatalf("append second: %v", err)
	}
	if second.Seq != 2 {
		t.Errorf("second seq: got %d, want 2", second.Seq)
	}

	list, err := svc.List(ctx, proj.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list size: got %d, want 2", len(list))
	}
	if list[0].Seq != 1 || list[1].Seq != 2 {
		t.Errorf("list order: got %d,%d; want 1,2", list[0].Seq, list[1].Seq)
	}
}

func TestPendingChangeService_DiscardAll(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")

	for range 3 {
		if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
			Op:         deploykit.PendingOpProjectUpdate,
			TargetType: deploykit.PendingTargetProject,
			TargetID:   &proj.ID,
			Payload:    json.RawMessage(`{"name":"x"}`),
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.DiscardAll(ctx, proj.ID); err != nil {
		t.Fatalf("discard: %v", err)
	}
	list, err := svc.List(ctx, proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("list after discard: got %d, want 0", len(list))
	}
}

func TestPendingChangeService_Apply_ProjectRename(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	projSvc := sqlite.NewProjectService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")

	payload, _ := json.Marshal(deploykit.ProjectUpdatePayload{Name: strPtr("new name")})
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpProjectUpdate,
		TargetType: deploykit.PendingTargetProject,
		TargetID:   &proj.ID,
		Payload:    payload,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Apply(ctx, proj.ID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.AppliedCount != 1 {
		t.Errorf("applied count: got %d, want 1", res.AppliedCount)
	}

	got, err := projSvc.GetProject(ctx, proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "new name" {
		t.Errorf("project name: got %q, want %q", got.Name, "new name")
	}

	list, _ := svc.List(ctx, proj.ID)
	if len(list) != 0 {
		t.Errorf("log not cleared: got %d", len(list))
	}
}

func TestPendingChangeService_Apply_ServiceCreateWithEnvVar(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	svcSvc := sqlite.NewServiceService(db)
	envSvc := sqlite.NewEnvVarService(db)
	canvasSvc := sqlite.NewCanvasService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")

	// The canvas node has to exist before service.create is applied — it
	// was placed when the user dropped the service. Its ID is the temp id.
	tempID := "node-aaa"
	if _, err := canvasSvc.UpsertNode(ctx, proj.ID, deploykit.CanvasNodeUpsert{
		ID:        tempID,
		Type:      deploykit.CanvasNodeTypeService,
		Label:     "web",
		PositionX: 100,
		PositionY: 200,
	}); err != nil {
		t.Fatal(err)
	}

	createPayload, _ := json.Marshal(deploykit.ServiceCreatePayload{
		Name:  "web",
		Image: "nginx:latest",
	})
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:           deploykit.PendingOpServiceCreate,
		TargetType:   deploykit.PendingTargetService,
		TargetTempID: &tempID,
		Payload:      createPayload,
	}); err != nil {
		t.Fatal(err)
	}

	// env_var.create that references the pending service via parent_temp_id.
	envPayload, _ := json.Marshal(deploykit.EnvVarCreatePayload{
		Scope: deploykit.EnvVarScopeService,
		Key:   "DATABASE_URL",
		Value: "postgres://x",
	})
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:           deploykit.PendingOpEnvVarCreate,
		TargetType:   deploykit.PendingTargetEnvVar,
		ParentTempID: &tempID,
		Payload:      envPayload,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Apply(ctx, proj.ID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.AppliedCount != 2 {
		t.Errorf("applied: got %d, want 2", res.AppliedCount)
	}
	realID, ok := res.TempIDToServiceID[tempID]
	if !ok {
		t.Fatalf("temp id %s not mapped", tempID)
	}
	if len(res.CreatedDeployments) != 1 {
		t.Errorf("deployments: got %d, want 1", len(res.CreatedDeployments))
	}

	svcRec, err := svcSvc.GetService(ctx, realID)
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if svcRec.Name != "web" {
		t.Errorf("service name: got %q", svcRec.Name)
	}
	if svcRec.ActiveDeploymentID == nil {
		t.Error("active deployment not set")
	}

	envs, err := envSvc.ListEnvVars(ctx, deploykit.EnvVarScopeService, realID)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 || envs[0].Key != "DATABASE_URL" {
		t.Errorf("env vars: got %+v", envs)
	}

	// Canvas node should now link to the new service.
	nodes, _, err := canvasSvc.GetCanvasState(ctx, proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes: got %d, want 1", len(nodes))
	}
	if nodes[0].ServiceID == nil || *nodes[0].ServiceID != realID {
		t.Errorf("node service id: got %v, want %s", nodes[0].ServiceID, realID)
	}
}

func TestPendingChangeService_Apply_EnvVarUpdateRedeploys(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	svcSvc := sqlite.NewServiceService(db)
	envSvc := sqlite.NewEnvVarService(db)
	depSvc := sqlite.NewDeploymentService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")

	// Set up a service with an active deployment.
	service, err := svcSvc.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "api"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := depSvc.CreateDeployment(ctx, service.ID, deploykit.DeploymentCreate{Image: "nginx:1"}); err != nil {
		t.Fatal(err)
	}
	ev, err := envSvc.CreateEnvVar(ctx, deploykit.EnvVarScopeService, service.ID, deploykit.EnvVarCreate{
		Key: "FOO", Value: "bar",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Stage an env var value change.
	payload, _ := json.Marshal(deploykit.EnvVarUpdatePayload{Value: "baz"})
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpEnvVarUpdate,
		TargetType: deploykit.PendingTargetEnvVar,
		TargetID:   &ev.ID,
		Payload:    payload,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Apply(ctx, proj.ID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(res.RedeployedServiceIDs) != 1 || res.RedeployedServiceIDs[0] != service.ID {
		t.Errorf("redeployed: %+v", res.RedeployedServiceIDs)
	}
	if len(res.CreatedDeployments) != 1 {
		t.Errorf("created deployments: got %d, want 1", len(res.CreatedDeployments))
	}

	updated, err := envSvc.GetEnvVar(ctx, ev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Value != "baz" {
		t.Errorf("value: got %q, want %q", updated.Value, "baz")
	}
}

func TestPendingChangeService_Apply_AtomicRollback(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	svcSvc := sqlite.NewServiceService(db)
	envSvc := sqlite.NewEnvVarService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")

	// Pre-existing env var.
	service, _ := svcSvc.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "api"})
	if _, err := envSvc.CreateEnvVar(ctx, deploykit.EnvVarScopeService, service.ID, deploykit.EnvVarCreate{
		Key: "FOO", Value: "bar",
	}); err != nil {
		t.Fatal(err)
	}

	// First entry renames the project — this would succeed on its own.
	rename, _ := json.Marshal(deploykit.ProjectUpdatePayload{Name: strPtr("new-name")})
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpProjectUpdate,
		TargetType: deploykit.PendingTargetProject,
		TargetID:   &proj.ID,
		Payload:    rename,
	}); err != nil {
		t.Fatal(err)
	}
	// Second entry tries to create a duplicate FOO env var — this must fail,
	// rolling back the project rename.
	svcID := service.ID
	duplicate, _ := json.Marshal(deploykit.EnvVarCreatePayload{
		Scope: deploykit.EnvVarScopeService,
		Key:   "FOO",
		Value: "other",
	})
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpEnvVarCreate,
		TargetType: deploykit.PendingTargetEnvVar,
		TargetID:   &svcID,
		Payload:    duplicate,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Apply(ctx, proj.ID); err == nil {
		t.Fatal("expected apply to fail")
	}

	// Project name must still be the original — rollback worked.
	projSvc := sqlite.NewProjectService(db)
	got, _ := projSvc.GetProject(ctx, proj.ID)
	if got.Name == "new-name" {
		t.Error("project was renamed despite rollback")
	}

	// The pending log must still contain both entries.
	list, _ := svc.List(ctx, proj.ID)
	if len(list) != 2 {
		t.Errorf("log cleared despite failed apply: got %d", len(list))
	}
}

func TestPendingChangeService_Apply_ServiceDeleteRemovesNode(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	svcSvc := sqlite.NewServiceService(db)
	canvasSvc := sqlite.NewCanvasService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")

	service, _ := svcSvc.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "api"})
	if _, err := canvasSvc.UpsertNode(ctx, proj.ID, deploykit.CanvasNodeUpsert{
		ID:        "node-1",
		Type:      deploykit.CanvasNodeTypeService,
		Label:     "api",
		ServiceID: &service.ID,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpServiceDelete,
		TargetType: deploykit.PendingTargetService,
		TargetID:   &service.ID,
		Payload:    json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Apply(ctx, proj.ID); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if _, err := svcSvc.GetService(ctx, service.ID); deploykit.ErrorCode(err) != deploykit.ENOTFOUND {
		t.Errorf("service should be deleted, got err: %v", err)
	}
	nodes, _, _ := canvasSvc.GetCanvasState(ctx, proj.ID)
	if len(nodes) != 0 {
		t.Errorf("canvas nodes: got %d, want 0", len(nodes))
	}
}

func TestPendingChangeService_RemoveByTempID(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")

	tempID := "node-x"
	createPayload, _ := json.Marshal(deploykit.ServiceCreatePayload{Name: "x", Image: "nginx"})
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:           deploykit.PendingOpServiceCreate,
		TargetType:   deploykit.PendingTargetService,
		TargetTempID: &tempID,
		Payload:      createPayload,
	}); err != nil {
		t.Fatal(err)
	}
	envPayload, _ := json.Marshal(deploykit.EnvVarCreatePayload{
		Scope: deploykit.EnvVarScopeService,
		Key:   "FOO",
		Value: "bar",
	})
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:           deploykit.PendingOpEnvVarCreate,
		TargetType:   deploykit.PendingTargetEnvVar,
		ParentTempID: &tempID,
		Payload:      envPayload,
	}); err != nil {
		t.Fatal(err)
	}

	removed, err := svc.RemoveByTempID(ctx, proj.ID, tempID)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Errorf("removed ids: got %d, want 2", len(removed))
	}

	list, _ := svc.List(ctx, proj.ID)
	if len(list) != 0 {
		t.Errorf("list after remove by temp id: got %d, want 0", len(list))
	}
}

func TestPendingChangeService_Apply_ServiceUpdate(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	svcSvc := sqlite.NewServiceService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")
	service, _ := svcSvc.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "api"})

	payload, _ := json.Marshal(deploykit.ServiceUpdatePayload{
		Name:    strPtr("renamed"),
		IconURL: strPtr("https://example.com/icon.png"),
	})
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpServiceUpdate,
		TargetType: deploykit.PendingTargetService,
		TargetID:   &service.ID,
		Payload:    payload,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Apply(ctx, proj.ID); err != nil {
		t.Fatalf("apply: %v", err)
	}

	got, err := svcSvc.GetService(ctx, service.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "renamed" {
		t.Errorf("name: got %q, want %q", got.Name, "renamed")
	}
	if got.IconURL == nil || *got.IconURL != "https://example.com/icon.png" {
		t.Errorf("icon: got %v", got.IconURL)
	}
}

func TestPendingChangeService_Apply_EnvVarDeleteRedeploys(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	svcSvc := sqlite.NewServiceService(db)
	envSvc := sqlite.NewEnvVarService(db)
	depSvc := sqlite.NewDeploymentService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")
	service, _ := svcSvc.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "api"})
	if _, err := depSvc.CreateDeployment(ctx, service.ID, deploykit.DeploymentCreate{Image: "nginx:1"}); err != nil {
		t.Fatal(err)
	}
	ev, _ := envSvc.CreateEnvVar(ctx, deploykit.EnvVarScopeService, service.ID, deploykit.EnvVarCreate{
		Key: "FOO", Value: "bar",
	})

	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpEnvVarDelete,
		TargetType: deploykit.PendingTargetEnvVar,
		TargetID:   &ev.ID,
		Payload:    json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Apply(ctx, proj.ID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(res.RedeployedServiceIDs) != 1 || res.RedeployedServiceIDs[0] != service.ID {
		t.Errorf("redeployed: %+v", res.RedeployedServiceIDs)
	}
	if _, err := envSvc.GetEnvVar(ctx, ev.ID); deploykit.ErrorCode(err) != deploykit.ENOTFOUND {
		t.Errorf("env var still present: %v", err)
	}
}

func TestPendingChangeService_Apply_ProjectEnvVarFanOut(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	svcSvc := sqlite.NewServiceService(db)
	depSvc := sqlite.NewDeploymentService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")

	// Two deployed services + one service with no active deployment. Only the
	// two deployed services should be redeployed when a project env var changes.
	a, _ := svcSvc.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "a"})
	b, _ := svcSvc.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "b"})
	c, _ := svcSvc.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "c"})
	if _, err := depSvc.CreateDeployment(ctx, a.ID, deploykit.DeploymentCreate{Image: "nginx:1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := depSvc.CreateDeployment(ctx, b.ID, deploykit.DeploymentCreate{Image: "nginx:1"}); err != nil {
		t.Fatal(err)
	}

	projID := proj.ID
	payload, _ := json.Marshal(deploykit.EnvVarCreatePayload{
		Scope: deploykit.EnvVarScopeProject,
		Key:   "SHARED",
		Value: "1",
	})
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpEnvVarCreate,
		TargetType: deploykit.PendingTargetEnvVar,
		TargetID:   &projID,
		Payload:    payload,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Apply(ctx, proj.ID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	got := map[string]bool{}
	for _, id := range res.RedeployedServiceIDs {
		got[id] = true
	}
	if !got[a.ID] || !got[b.ID] {
		t.Errorf("a + b should be redeployed: got %+v", res.RedeployedServiceIDs)
	}
	if got[c.ID] {
		t.Errorf("c has no active deployment but was redeployed")
	}
}

func TestPendingChangeService_DiscardAll_CleansUpPendingCreatedNodes(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	canvasSvc := sqlite.NewCanvasService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")

	// Pre-existing applied service node — should survive discard.
	svcSvc := sqlite.NewServiceService(db)
	existing, _ := svcSvc.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "existing"})
	if _, err := canvasSvc.UpsertNode(ctx, proj.ID, deploykit.CanvasNodeUpsert{
		ID:        "node-existing",
		Type:      deploykit.CanvasNodeTypeService,
		Label:     "existing",
		ServiceID: &existing.ID,
	}); err != nil {
		t.Fatal(err)
	}

	// Pending-added service: canvas node + service.create entry.
	tempID := "node-pending"
	if _, err := canvasSvc.UpsertNode(ctx, proj.ID, deploykit.CanvasNodeUpsert{
		ID:    tempID,
		Type:  deploykit.CanvasNodeTypeService,
		Label: "pending",
	}); err != nil {
		t.Fatal(err)
	}
	createPayload, _ := json.Marshal(deploykit.ServiceCreatePayload{Name: "pending", Image: "nginx"})
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:           deploykit.PendingOpServiceCreate,
		TargetType:   deploykit.PendingTargetService,
		TargetTempID: &tempID,
		Payload:      createPayload,
	}); err != nil {
		t.Fatal(err)
	}

	// A plain label node with no pending change — should also survive.
	if _, err := canvasSvc.UpsertNode(ctx, proj.ID, deploykit.CanvasNodeUpsert{
		ID:    "node-label",
		Type:  deploykit.CanvasNodeTypeLabel,
		Label: "notes",
	}); err != nil {
		t.Fatal(err)
	}

	if err := svc.DiscardAll(ctx, proj.ID); err != nil {
		t.Fatalf("discard: %v", err)
	}

	nodes, _, err := canvasSvc.GetCanvasState(ctx, proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, n := range nodes {
		ids[n.ID] = true
	}
	if ids[tempID] {
		t.Error("pending-added node not cleaned up")
	}
	if !ids["node-existing"] {
		t.Error("existing applied node was wrongly deleted")
	}
	if !ids["node-label"] {
		t.Error("plain label node was wrongly deleted")
	}

	list, _ := svc.List(ctx, proj.ID)
	if len(list) != 0 {
		t.Errorf("pending changes after discard: got %d, want 0", len(list))
	}
}

func strPtr(s string) *string { return &s }
