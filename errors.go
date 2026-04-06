package deploykit

import (
	"fmt"
	"strings"
)

// Error codes returned by the application.
const (
	ECONFLICT     = "conflict"
	EFORBIDDEN    = "forbidden"
	EINTERNAL     = "internal"
	EINVALID      = "invalid"
	ENOTFOUND     = "not_found"
	EUNAUTHORIZED = "unauthorized"
)

// Error represents a domain-level error with an application error code
// and a human-readable message.
type Error struct {
	Code    string `json:"error"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Errorf creates a new Error with the given code and formatted message.
func Errorf(code string, format string, args ...any) *Error {
	return &Error{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// ValidationErrors is a builder for collecting field-level validation errors.
type ValidationErrors map[string][]string

// NewValidationErrors creates an empty ValidationErrors builder.
func NewValidationErrors() ValidationErrors {
	return make(ValidationErrors)
}

// Add appends a validation message for the given field.
func (ve ValidationErrors) Add(field, message string) {
	ve[field] = append(ve[field], message)
}

// HasErrors returns true if any field errors have been collected.
func (ve ValidationErrors) HasErrors() bool {
	return len(ve) > 0
}

// Err returns a *ValidationError if any errors were collected, or nil otherwise.
// Use this as the return value from Validate() methods.
func (ve ValidationErrors) Err() error {
	if !ve.HasErrors() {
		return nil
	}
	return &ValidationError{
		Code:    EINVALID,
		Message: "Validation failed.",
		Errors:  map[string][]string(ve),
	}
}

// ValidationError represents a structured validation failure with per-field error messages.
type ValidationError struct {
	Code    string              `json:"error"`
	Message string              `json:"message"`
	Errors  map[string][]string `json:"errors"`
}

func (e *ValidationError) Error() string {
	fields := make([]string, 0, len(e.Errors))
	for field, msgs := range e.Errors {
		fields = append(fields, fmt.Sprintf("%s: %s", field, strings.Join(msgs, "; ")))
	}
	return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, strings.Join(fields, ", "))
}

// ErrorCode unwraps an error and returns its code.
// Non-application errors return EINTERNAL.
func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if e, ok := err.(*Error); ok {
		return e.Code
	}
	if e, ok := err.(*ValidationError); ok {
		return e.Code
	}
	return EINTERNAL
}

// ErrorMessage unwraps an error and returns its human-readable message.
// Non-application errors return "Internal error."
func ErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	if e, ok := err.(*Error); ok {
		return e.Message
	}
	if e, ok := err.(*ValidationError); ok {
		return e.Message
	}
	return "Internal error."
}
