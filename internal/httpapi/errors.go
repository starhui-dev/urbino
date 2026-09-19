package httpapi

import (
	"encoding/json"
	"net/http"

	"example.com/urbino/internal/domain"
)

type publicError struct {
	Code      domain.Code       `json:"code"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id"`
	Retryable bool              `json:"retryable"`
	Details   map[string]string `json:"details"`
}

// WriteDomainError maps a safe domain error to one stable public response.
// It never serializes internal error values or credential material.
func WriteDomainError(w http.ResponseWriter, requestID domain.UUID, err *domain.Error) {
	status := http.StatusInternalServerError
	if err != nil {
		switch err.Class {
		case domain.ClassUnsupported:
			status = http.StatusNotImplemented
		case domain.ClassUnauthorized:
			status = http.StatusUnauthorized
		}
	}
	response := publicError{Code: domain.CodeUnknown, Message: "unknown error", RequestID: requestID.String(), Details: map[string]string{}}
	if err != nil {
		response.Code = err.Code
		response.Message = err.Message
		response.Retryable = err.Retryable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

// WriteUnsupported is the explicit response for an undeployed capability.
func WriteUnsupported(w http.ResponseWriter, requestID domain.UUID, capability string) {
	WriteDomainError(w, requestID, domain.Unsupported("capability is not enabled: "+capability))
}
