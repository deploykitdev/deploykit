package deploykit_test

import (
	"encoding/json"
	"testing"

	"github.com/heyjorgedev/deploykit"
)

func TestValidationErrors_Empty(t *testing.T) {
	ve := deploykit.NewValidationErrors()
	if ve.HasErrors() {
		t.Fatal("expected no errors")
	}
	if err := ve.Err(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidationErrors_Add(t *testing.T) {
	ve := deploykit.NewValidationErrors()
	ve.Add("name", "Name is required.")
	ve.Add("email", "Email is required.")
	ve.Add("email", "Email must be valid.")

	if !ve.HasErrors() {
		t.Fatal("expected errors")
	}

	err := ve.Err()
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	valErr, ok := err.(*deploykit.ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	if len(valErr.Errors["name"]) != 1 {
		t.Fatalf("expected 1 name error, got %d", len(valErr.Errors["name"]))
	}
	if len(valErr.Errors["email"]) != 2 {
		t.Fatalf("expected 2 email errors, got %d", len(valErr.Errors["email"]))
	}
}

func TestValidationError_ErrorInterface(t *testing.T) {
	ve := deploykit.NewValidationErrors()
	ve.Add("name", "Name is required.")
	err := ve.Err()

	if err.Error() == "" {
		t.Fatal("expected non-empty error string")
	}
}

func TestValidationError_ErrorCode(t *testing.T) {
	ve := deploykit.NewValidationErrors()
	ve.Add("name", "Name is required.")
	err := ve.Err()

	if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
		t.Fatalf("expected %q, got %q", deploykit.EINVALID, code)
	}
}

func TestValidationError_ErrorMessage(t *testing.T) {
	ve := deploykit.NewValidationErrors()
	ve.Add("name", "Name is required.")
	err := ve.Err()

	if msg := deploykit.ErrorMessage(err); msg != "Validation failed." {
		t.Fatalf("expected %q, got %q", "Validation failed.", msg)
	}
}

func TestValidationError_JSON(t *testing.T) {
	ve := deploykit.NewValidationErrors()
	ve.Add("email", "Email is required.")
	ve.Add("password", "Password is required.")
	ve.Add("password", "Password must be at least 8 characters.")
	err := ve.Err()

	valErr := err.(*deploykit.ValidationError)
	data, jsonErr := json.Marshal(valErr)
	if jsonErr != nil {
		t.Fatalf("json.Marshal failed: %v", jsonErr)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if result["error"] != "invalid" {
		t.Fatalf("expected error=%q, got %q", "invalid", result["error"])
	}
	if result["message"] != "Validation failed." {
		t.Fatalf("expected message=%q, got %q", "Validation failed.", result["message"])
	}

	errors, ok := result["errors"].(map[string]any)
	if !ok {
		t.Fatal("expected errors to be a map")
	}
	if len(errors) != 2 {
		t.Fatalf("expected 2 field errors, got %d", len(errors))
	}

	passwordErrs, ok := errors["password"].([]any)
	if !ok {
		t.Fatal("expected password errors to be an array")
	}
	if len(passwordErrs) != 2 {
		t.Fatalf("expected 2 password errors, got %d", len(passwordErrs))
	}
}

func TestUserCreate_Validate_CollectsAllErrors(t *testing.T) {
	req := deploykit.UserCreate{}
	err := req.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}

	valErr, ok := err.(*deploykit.ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	// Should have errors for all three required fields.
	for _, field := range []string{"email", "name", "password"} {
		if _, exists := valErr.Errors[field]; !exists {
			t.Errorf("expected error for field %q", field)
		}
	}
}

func TestUserCreate_Validate_PasswordLength(t *testing.T) {
	req := deploykit.UserCreate{
		Email:    "test@example.com",
		Name:     "Test",
		Password: "short",
	}
	err := req.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}

	valErr, ok := err.(*deploykit.ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	if _, exists := valErr.Errors["password"]; !exists {
		t.Fatal("expected error for password field")
	}
	if valErr.Errors["password"][0] != "Password must be at least 8 characters." {
		t.Fatalf("unexpected message: %s", valErr.Errors["password"][0])
	}
}

func TestDeploymentCreate_Validate_CollectsAllErrors(t *testing.T) {
	req := deploykit.DeploymentCreate{Replicas: -1}
	err := req.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}

	valErr, ok := err.(*deploykit.ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	if _, exists := valErr.Errors["image"]; !exists {
		t.Error("expected error for image field")
	}
	if _, exists := valErr.Errors["replicas"]; !exists {
		t.Error("expected error for replicas field")
	}
}
