package sqlite_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/deploykitdev/deploykit"
	"github.com/deploykitdev/deploykit/sqlite"
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

// TestPendingChangeService_Apply_EnvRefAutoEdges exercises the env-ref →
// auto-edge sync end to end: applying a pending env var that references
// another service's host creates a system-managed canvas edge between the
// two service nodes, with the resolved hostname snapshotted into the
// deployment's env var map.
func TestPendingChangeService_Apply_EnvRefAutoEdges(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	svcSvc := sqlite.NewServiceService(db)
	depSvc := sqlite.NewDeploymentService(db)
	canvasSvc := sqlite.NewCanvasService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")

	// Set up two services + their canvas nodes.
	dbSvc, err := svcSvc.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "db"})
	if err != nil {
		t.Fatal(err)
	}
	web, err := svcSvc.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "web"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := depSvc.CreateDeployment(ctx, web.ID, deploykit.DeploymentCreate{Image: "nginx:1"}); err != nil {
		t.Fatal(err)
	}
	dbNode, err := canvasSvc.UpsertNode(ctx, proj.ID, deploykit.CanvasNodeUpsert{
		ID: "node-db", Type: deploykit.CanvasNodeTypeService, Label: "db", ServiceID: &dbSvc.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	webNode, err := canvasSvc.UpsertNode(ctx, proj.ID, deploykit.CanvasNodeUpsert{
		ID: "node-web", Type: deploykit.CanvasNodeTypeService, Label: "web", ServiceID: &web.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Stage an env var on web that references db.
	payload, _ := json.Marshal(deploykit.EnvVarCreatePayload{
		Scope: deploykit.EnvVarScopeService,
		Key:   "DB_HOST",
		Value: "${{db.HOST}}",
	})
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpEnvVarCreate,
		TargetType: deploykit.PendingTargetEnvVar,
		TargetID:   &web.ID,
		Payload:    payload,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Apply(ctx, proj.ID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Verify the new deployment snapshot has the resolved hostname.
	if len(res.CreatedDeployments) != 1 {
		t.Fatalf("deployments: got %d, want 1", len(res.CreatedDeployments))
	}
	wantHost := "dk-" + proj.Slug + "-db-0"
	if got := res.CreatedDeployments[0].EnvVars["DB_HOST"]; got != wantHost {
		t.Errorf("snapshot DB_HOST: got %q, want %q", got, wantHost)
	}

	// Verify the auto-edge exists.
	_, edges, err := canvasSvc.GetCanvasState(ctx, proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	var found *deploykit.CanvasEdge
	for _, e := range edges {
		if e.SourceID == webNode.ID && e.TargetID == dbNode.ID {
			found = e
			break
		}
	}
	if found == nil {
		t.Fatalf("expected auto-edge web -> db; got %d edges: %+v", len(edges), edges)
	}
	if !strings.Contains(found.Data, `"managed":"env-ref"`) {
		t.Errorf("edge data missing managed marker: %q", found.Data)
	}
	if !strings.Contains(found.Data, `"DB_HOST"`) {
		t.Errorf("edge data should list triggering key DB_HOST: %q", found.Data)
	}
}

// TestPendingChangeService_Apply_ServiceRenameRewritesRefs verifies that
// renaming a service updates `${{old.HOST}}` references in consumer env vars
// and refreshes their deployment snapshot to use the new container hostname.
func TestPendingChangeService_Apply_ServiceRenameRewritesRefs(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	svcSvc := sqlite.NewServiceService(db)
	envSvc := sqlite.NewEnvVarService(db)
	depSvc := sqlite.NewDeploymentService(db)
	canvasSvc := sqlite.NewCanvasService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")

	dbSvc, _ := svcSvc.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "db"})
	web, _ := svcSvc.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "web"})
	if _, err := depSvc.CreateDeployment(ctx, web.ID, deploykit.DeploymentCreate{Image: "nginx:1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := envSvc.CreateEnvVar(ctx, deploykit.EnvVarScopeService, web.ID, deploykit.EnvVarCreate{
		Key: "DB_HOST", Value: "${{db.HOST}}",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := canvasSvc.UpsertNode(ctx, proj.ID, deploykit.CanvasNodeUpsert{
		ID: "node-db", Type: deploykit.CanvasNodeTypeService, Label: "db", ServiceID: &dbSvc.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := canvasSvc.UpsertNode(ctx, proj.ID, deploykit.CanvasNodeUpsert{
		ID: "node-web", Type: deploykit.CanvasNodeTypeService, Label: "web", ServiceID: &web.ID,
	}); err != nil {
		t.Fatal(err)
	}

	// Rename db -> database.
	renamePayload, _ := json.Marshal(deploykit.ServiceUpdatePayload{Name: strPtr("database")})
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpServiceUpdate,
		TargetType: deploykit.PendingTargetService,
		TargetID:   &dbSvc.ID,
		Payload:    renamePayload,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Apply(ctx, proj.ID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// web's env var raw value should now reference the new name.
	envs, _ := envSvc.ListEnvVars(ctx, deploykit.EnvVarScopeService, web.ID)
	if len(envs) != 1 || envs[0].Value != "${{database.HOST}}" {
		t.Errorf("rewritten env var: got %+v", envs)
	}

	// web should have been redeployed with the new resolved hostname.
	wantHost := "dk-" + proj.Slug + "-database-0"
	var webDep *deploykit.Deployment
	for _, dep := range res.CreatedDeployments {
		if dep.ServiceID == web.ID {
			webDep = dep
			break
		}
	}
	if webDep == nil {
		t.Fatalf("web was not redeployed; deployments: %+v", res.CreatedDeployments)
	}
	if got := webDep.EnvVars["DB_HOST"]; got != wantHost {
		t.Errorf("DB_HOST snapshot: got %q, want %q", got, wantHost)
	}
}

// TestPendingChangeService_CoalesceServiceReparent verifies the atomic
// list/remove/append flow used by the canvas WS handler when a service is
// dragged in and out of groups.
func TestPendingChangeService_CoalesceServiceReparent(t *testing.T) {
	ctx := context.Background()

	t.Run("first reparent appends with previous applied state", func(t *testing.T) {
		db := sqlite.MustOpenDB(t)
		svc := sqlite.NewPendingChangeService(db)
		proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")
		serviceID := "svc-1"

		removed, appended, err := svc.CoalesceServiceReparent(
			ctx, proj.ID, serviceID, "" /* applied: top-level */, "group-A", nil,
		)
		if err != nil {
			t.Fatalf("coalesce: %v", err)
		}
		if len(removed) != 0 {
			t.Errorf("removed: got %d, want 0", len(removed))
		}
		if appended == nil {
			t.Fatal("expected appended entry")
		}
		var p deploykit.ServiceUpdatePayload
		if err := json.Unmarshal(appended.Payload, &p); err != nil {
			t.Fatal(err)
		}
		if !p.Reparented {
			t.Error("expected Reparented=true")
		}
		if p.PreviousParentID != "" {
			t.Errorf("PreviousParentID: got %q, want empty (top-level)", p.PreviousParentID)
		}
	})

	t.Run("second reparent coalesces, preserves original applied state", func(t *testing.T) {
		db := sqlite.MustOpenDB(t)
		svc := sqlite.NewPendingChangeService(db)
		proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")
		serviceID := "svc-1"

		// First move: top-level → group-A.
		_, first, err := svc.CoalesceServiceReparent(ctx, proj.ID, serviceID, "", "group-A", nil)
		if err != nil {
			t.Fatal(err)
		}
		if first == nil {
			t.Fatal("expected first appended")
		}

		// Second move: group-A → group-B. The handler passes
		// currentParentBeforeUpsert="group-A" (the canvas row's value), but
		// CoalesceServiceReparent should ignore that and use the prior
		// entry's PreviousParentID="" as the truly-applied state.
		removed, second, err := svc.CoalesceServiceReparent(
			ctx, proj.ID, serviceID, "group-A", "group-B", nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(removed) != 1 || removed[0] != first.ID {
			t.Errorf("removed: got %v, want [%s]", removed, first.ID)
		}
		if second == nil {
			t.Fatal("expected new entry after coalesce")
		}
		var p deploykit.ServiceUpdatePayload
		if err := json.Unmarshal(second.Payload, &p); err != nil {
			t.Fatal(err)
		}
		if p.PreviousParentID != "" {
			t.Errorf("PreviousParentID should track original applied state (empty), got %q", p.PreviousParentID)
		}

		// Only one entry remains in the log.
		list, err := svc.List(ctx, proj.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].ID != second.ID {
			t.Errorf("list: got %d entries, want 1 with ID %s", len(list), second.ID)
		}
	})

	t.Run("net-zero move drops staged entry and appends nothing", func(t *testing.T) {
		db := sqlite.MustOpenDB(t)
		svc := sqlite.NewPendingChangeService(db)
		proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")
		serviceID := "svc-1"

		// Stage: top-level → group-A.
		_, first, err := svc.CoalesceServiceReparent(ctx, proj.ID, serviceID, "", "group-A", nil)
		if err != nil {
			t.Fatal(err)
		}

		// Move back to top-level. PreviousParentID was "" so this is a no-op.
		removed, appended, err := svc.CoalesceServiceReparent(
			ctx, proj.ID, serviceID, "group-A", "", nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(removed) != 1 || removed[0] != first.ID {
			t.Errorf("removed: got %v, want [%s]", removed, first.ID)
		}
		if appended != nil {
			t.Errorf("expected no appended entry on net-zero, got %s", appended.ID)
		}

		list, err := svc.List(ctx, proj.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 0 {
			t.Errorf("list: got %d entries, want 0 after net-zero", len(list))
		}
	})

	t.Run("only reparent entries are coalesced (rename entry untouched)", func(t *testing.T) {
		db := sqlite.MustOpenDB(t)
		svc := sqlite.NewPendingChangeService(db)
		proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")
		serviceID := "svc-1"

		renamePayload, _ := json.Marshal(deploykit.ServiceUpdatePayload{Name: strPtr("new-name")})
		rename, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
			Op:         deploykit.PendingOpServiceUpdate,
			TargetType: deploykit.PendingTargetService,
			TargetID:   &serviceID,
			Payload:    renamePayload,
		})
		if err != nil {
			t.Fatal(err)
		}

		removed, appended, err := svc.CoalesceServiceReparent(
			ctx, proj.ID, serviceID, "", "group-A", nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(removed) != 0 {
			t.Errorf("removed: got %d, want 0 (rename should not be touched)", len(removed))
		}
		if appended == nil {
			t.Fatal("expected appended reparent entry")
		}

		list, err := svc.List(ctx, proj.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 2 {
			t.Errorf("list: got %d, want 2 (rename + reparent)", len(list))
		}
		// Rename entry must still be there.
		seen := false
		for _, e := range list {
			if e.ID == rename.ID {
				seen = true
				break
			}
		}
		if !seen {
			t.Error("rename entry was incorrectly removed")
		}
	})

	t.Run("mixed payload (name + reparented) is not coalesced", func(t *testing.T) {
		// Forward-looking guard: today's HTTP layer always stages pure
		// reparent or pure rename entries, but ServiceUpdatePayload permits
		// both flags on the same row. If a future caller stages a mixed
		// entry, CoalesceServiceReparent must NOT delete it — that would
		// silently drop the rename.
		db := sqlite.MustOpenDB(t)
		svc := sqlite.NewPendingChangeService(db)
		proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")
		serviceID := "svc-1"

		mixedPayload, _ := json.Marshal(deploykit.ServiceUpdatePayload{
			Name:             strPtr("renamed-and-moved"),
			Reparented:       true,
			PreviousParentID: "group-A",
		})
		mixed, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
			Op:         deploykit.PendingOpServiceUpdate,
			TargetType: deploykit.PendingTargetService,
			TargetID:   &serviceID,
			Payload:    mixedPayload,
		})
		if err != nil {
			t.Fatal(err)
		}

		// Now stage a pure reparent for the same service.
		removed, appended, err := svc.CoalesceServiceReparent(
			ctx, proj.ID, serviceID, "group-A", "group-B", nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(removed) != 0 {
			t.Errorf("removed: got %d, want 0 (mixed payload must survive)", len(removed))
		}
		if appended == nil {
			t.Fatal("expected appended pure-reparent entry")
		}

		// Mixed entry's PreviousParentID was "group-A", so the new pure
		// reparent should ALSO use the canvas-derived applied state ("group-A")
		// since the mixed entry was skipped during the "find prior" walk.
		var p deploykit.ServiceUpdatePayload
		if err := json.Unmarshal(appended.Payload, &p); err != nil {
			t.Fatal(err)
		}
		if p.PreviousParentID != "group-A" {
			t.Errorf("PreviousParentID: got %q, want %q", p.PreviousParentID, "group-A")
		}

		list, err := svc.List(ctx, proj.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 2 {
			t.Errorf("list: got %d, want 2 (mixed + new pure reparent)", len(list))
		}
		// Mixed entry must still be there.
		seenMixed := false
		for _, e := range list {
			if e.ID == mixed.ID {
				seenMixed = true
				break
			}
		}
		if !seenMixed {
			t.Error("mixed entry was incorrectly removed")
		}
	})
}

// TestPendingChangeService_Apply_ServiceUpdate_AfterDelete verifies that a
// service.update entry whose target was already deleted by an earlier-seq
// service.delete in the same apply is skipped silently — instead of failing
// the entire apply with ENOTFOUND.
func TestPendingChangeService_Apply_ServiceUpdate_AfterDelete(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	svcSvc := sqlite.NewServiceService(db)
	depSvc := sqlite.NewDeploymentService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")

	web, _ := svcSvc.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "web"})
	if _, err := depSvc.CreateDeployment(ctx, web.ID, deploykit.DeploymentCreate{Image: "nginx:1"}); err != nil {
		t.Fatal(err)
	}

	// Seq 1: delete the service.
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpServiceDelete,
		TargetType: deploykit.PendingTargetService,
		TargetID:   &web.ID,
		Payload:    json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	// Seq 2: stage a reparent on the same (now soon-to-be-deleted) service.
	reparentPayload, _ := json.Marshal(deploykit.ServiceUpdatePayload{
		Reparented:       true,
		PreviousParentID: "",
	})
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpServiceUpdate,
		TargetType: deploykit.PendingTargetService,
		TargetID:   &web.ID,
		Payload:    reparentPayload,
	}); err != nil {
		t.Fatal(err)
	}

	// Apply should not error — the reparent is skipped because the service
	// is already gone.
	res, err := svc.Apply(ctx, proj.ID)
	if err != nil {
		t.Fatalf("apply should not fail when reparent target was deleted earlier in the log: %v", err)
	}
	if res.AppliedCount != 2 {
		t.Errorf("applied count: got %d, want 2", res.AppliedCount)
	}
}

// TestPendingChangeService_Apply_PureReparent verifies that applying a
// service.update with Reparented=true (and no field changes) refreshes the
// service's deployment snapshot, picking up env vars from the new parent group.
func TestPendingChangeService_Apply_PureReparent(t *testing.T) {
	ctx := context.Background()
	db := sqlite.MustOpenDB(t)
	svc := sqlite.NewPendingChangeService(db)
	svcSvc := sqlite.NewServiceService(db)
	envSvc := sqlite.NewEnvVarService(db)
	depSvc := sqlite.NewDeploymentService(db)
	canvasSvc := sqlite.NewCanvasService(db)
	proj := sqlite.MustCreateProject(t, sqlite.NewProjectService(db), "proj")

	// Service with an active deployment (so it's eligible for refresh).
	web, _ := svcSvc.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "web"})
	if _, err := depSvc.CreateDeployment(ctx, web.ID, deploykit.DeploymentCreate{Image: "nginx:1"}); err != nil {
		t.Fatal(err)
	}

	// Group canvas node + service canvas node parented to it.
	groupNode, err := canvasSvc.UpsertNode(ctx, proj.ID, deploykit.CanvasNodeUpsert{
		ID: "node-group", Type: deploykit.CanvasNodeTypeGroup, Label: "app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := canvasSvc.UpsertNode(ctx, proj.ID, deploykit.CanvasNodeUpsert{
		ID: "node-web", Type: deploykit.CanvasNodeTypeService, Label: "web",
		ServiceID: &web.ID, ParentID: &groupNode.ID,
	}); err != nil {
		t.Fatal(err)
	}

	// Group has an env var that the service should inherit on refresh.
	if _, err := envSvc.CreateEnvVar(ctx, deploykit.EnvVarScopeGroup, groupNode.ID, deploykit.EnvVarCreate{
		Key: "SHARED", Value: "abc",
	}); err != nil {
		t.Fatal(err)
	}

	// Stage a pure reparent (no name/icon change, just Reparented=true).
	payload, _ := json.Marshal(deploykit.ServiceUpdatePayload{
		Reparented:       true,
		PreviousParentID: "",
	})
	if _, err := svc.Append(ctx, proj.ID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpServiceUpdate,
		TargetType: deploykit.PendingTargetService,
		TargetID:   &web.ID,
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
	if len(res.RedeployedServiceIDs) != 1 || res.RedeployedServiceIDs[0] != web.ID {
		t.Errorf("redeployed: got %v, want [%s]", res.RedeployedServiceIDs, web.ID)
	}
	if len(res.CreatedDeployments) != 1 {
		t.Fatalf("created deployments: got %d, want 1", len(res.CreatedDeployments))
	}
	if got := res.CreatedDeployments[0].EnvVars["SHARED"]; got != "abc" {
		t.Errorf("inherited group var SHARED: got %q, want %q", got, "abc")
	}
}

