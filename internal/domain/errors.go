package domain

import "fmt"

type ErrorCode string

const (
	ErrUnauthorized      ErrorCode = "unauthorized"
	ErrPermissionDenied  ErrorCode = "permission_denied"
	ErrUnknown           ErrorCode = "unknown"
	ErrUnsupported       ErrorCode = "unsupported_capability"
	ErrInvalidArgument   ErrorCode = "invalid_argument"
	ErrInvalidTransition ErrorCode = "invalid_state_transition"
	ErrCurrencyMismatch  ErrorCode = "currency_mismatch"
	ErrOverflow          ErrorCode = "overflow"
)

// Error is safe to expose to clients. Internal causes are deliberately not included in Error().
type Error struct {
	Code      ErrorCode
	Message   string
	Retryable bool
	Cause     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
func (e *Error) Unwrap() error { return e.Cause }

func SafeError(code ErrorCode, message string) *Error { return &Error{Code: code, Message: message} }

type Scope string

func NewScope(value string) (Scope, error) {
	if value == "" {
		return "", &Error{Code: ErrInvalidArgument, Message: "empty scope"}
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != ':' && r != '_' && r != '-' {
			return "", &Error{Code: ErrInvalidArgument, Message: "invalid scope"}
		}
	}
	return Scope(value), nil
}

func ParseScope(value string) (Scope, error) {
	s, err := NewScope(value)
	if err != nil {
		return "", err
	}
	switch s {
	case ScopeTenantRead, ScopeTenantWrite, ScopeProjectRead, ScopeProjectWrite, ScopeModelInvoke, ScopeUsageRead:
		return s, nil
	default:
		return "", &Error{Code: ErrUnsupported, Message: "unsupported permission scope"}
	}
}

const (
	ScopeTenantRead   Scope = "tenant:read"
	ScopeTenantWrite  Scope = "tenant:write"
	ScopeProjectRead  Scope = "project:read"
	ScopeProjectWrite Scope = "project:write"
	ScopeModelInvoke  Scope = "model:invoke"
	ScopeUsageRead    Scope = "usage:read"
)

type Capability string

const (
	CapabilityChatCompletions   Capability = "chat.completions"
	CapabilityResponses         Capability = "responses"
	CapabilityEmbeddings        Capability = "embeddings"
	CapabilityAnthropicMessages Capability = "anthropic.messages"
	CapabilityGeminiGenerate    Capability = "gemini.generate_content"
)

func ParseCapability(value string) (Capability, error) {
	c := Capability(value)
	switch c {
	case CapabilityChatCompletions, CapabilityResponses, CapabilityEmbeddings, CapabilityAnthropicMessages, CapabilityGeminiGenerate:
		return c, nil
	default:
		return "", &Error{Code: ErrUnsupported, Message: "unsupported capability"}
	}
}

type CapabilityDescriptor struct {
	Name    Capability
	Version string
	Enabled bool
}
