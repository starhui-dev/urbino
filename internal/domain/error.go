package domain

import "fmt"

// ErrorClass is the stable class used when mapping internal failures to public errors.
type ErrorClass string

const (
	ClassUnsupported  ErrorClass = "unsupported"
	ClassUnauthorized ErrorClass = "unauthorized"
	ClassUnknown      ErrorClass = "unknown"
)

// Code is the public-safe error code.
type Code string

const (
	CodeBadRequest            Code = "bad_request"
	CodeUnauthorized          Code = "unauthorized"
	CodePermissionDenied      Code = "permission_denied"
	CodeResourceNotFound      Code = "resource_not_found"
	CodeVersionConflict       Code = "version_conflict"
	CodeUnsupportedCapability Code = "unsupported_capability"
	CodeUnknown               Code = "unknown"
)

// Error is a typed domain error safe to map to the public error envelope.
type Error struct {
	Class     ErrorClass
	Code      Code
	Message   string
	Retryable bool
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func NewError(class ErrorClass, code Code, message string, retryable bool) *Error {
	return &Error{Class: class, Code: code, Message: message, Retryable: retryable}
}

func Unsupported(message string) *Error {
	return NewError(ClassUnsupported, CodeUnsupportedCapability, message, false)
}

func Unauthorized(message string) *Error {
	return NewError(ClassUnauthorized, CodeUnauthorized, message, false)
}

func Unknown(message string) *Error {
	return NewError(ClassUnknown, CodeUnknown, message, false)
}

// PublicError is the stable error envelope. Details are safe, non-secret strings.
type PublicError struct {
	Code      Code              `json:"code"`
	Message   string            `json:"message"`
	RequestID UUID              `json:"request_id"`
	Retryable bool              `json:"retryable"`
	Details   map[string]string `json:"details"`
}
