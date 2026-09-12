package protocol

import "errors"

// PublicError is safe to serialize to a client. Internal causes are kept out
// of this structure; request IDs are assigned by the HTTP boundary.
type PublicError struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id,omitempty"`
	Retryable bool              `json:"retryable"`
	Details   map[string]string `json:"details,omitempty"`
}

// StableCode maps capability/authentication outcomes to the public envelope;
// callers must not expose Go error text as a protocol message.
func StableCode(err error) string {
	if errors.Is(err, ErrUnauthorized) {
		return "unauthorized"
	}
	if errors.Is(err, ErrUnsupportedCapability) {
		return "unsupported_capability"
	}
	return "invalid_request"
}
