package deploykit

import "fmt"

// Error codes returned by the application.
const (
	ECONFLICT      = "conflict"
	EINTERNAL      = "internal"
	EINVALID       = "invalid"
	ENOTFOUND      = "not_found"
	EUNAUTHORIZED  = "unauthorized"
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

// ErrorCode unwraps an error and returns its code.
// Non-application errors return EINTERNAL.
func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if e, ok := err.(*Error); ok {
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
	return "Internal error."
}
