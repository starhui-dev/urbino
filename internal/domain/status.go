package domain

// Request lifecycle is monotonic; terminal states cannot be reopened.
type RequestStatus string

const (
	RequestPending    RequestStatus = "pending"
	RequestAdmitted   RequestStatus = "admitted"
	RequestDispatched RequestStatus = "dispatched"
	RequestStreaming  RequestStatus = "streaming"
	RequestSucceeded  RequestStatus = "succeeded"
	RequestFailed     RequestStatus = "failed"
	RequestCancelled  RequestStatus = "cancelled"
	RequestUnknown    RequestStatus = "unknown"
)

func (s RequestStatus) Valid() bool {
	switch s {
	case RequestPending, RequestAdmitted, RequestDispatched, RequestStreaming, RequestSucceeded, RequestFailed, RequestCancelled, RequestUnknown:
		return true
	}
	return false
}

func RequestTransition(from, to RequestStatus) error {
	if !from.Valid() || !to.Valid() {
		return &Error{Code: ErrInvalidTransition, Message: "unknown request status"}
	}
	allowed := map[RequestStatus][]RequestStatus{
		RequestPending:    {RequestAdmitted, RequestCancelled, RequestFailed},
		RequestAdmitted:   {RequestDispatched, RequestCancelled, RequestFailed},
		RequestDispatched: {RequestStreaming, RequestSucceeded, RequestFailed, RequestCancelled, RequestUnknown},
		RequestStreaming:  {RequestSucceeded, RequestFailed, RequestCancelled, RequestUnknown},
		RequestUnknown:    {}, RequestSucceeded: {}, RequestFailed: {}, RequestCancelled: {},
	}
	for _, x := range allowed[from] {
		if x == to {
			return nil
		}
	}
	return &Error{Code: ErrInvalidTransition, Message: "invalid request status transition"}
}

type AttemptStatus string

const (
	AttemptCreated  AttemptStatus = "created"
	AttemptStarted  AttemptStatus = "started"
	AttemptFinished AttemptStatus = "finished"
	AttemptFailed   AttemptStatus = "failed"
	AttemptUnknown  AttemptStatus = "unknown"
)

func (s AttemptStatus) Valid() bool {
	switch s {
	case AttemptCreated, AttemptStarted, AttemptFinished, AttemptFailed, AttemptUnknown:
		return true
	}
	return false
}
func AttemptTransition(from, to AttemptStatus) error {
	if !from.Valid() || !to.Valid() {
		return &Error{Code: ErrInvalidTransition, Message: "unknown attempt status"}
	}
	allowed := map[AttemptStatus][]AttemptStatus{AttemptCreated: {AttemptStarted, AttemptFailed}, AttemptStarted: {AttemptFinished, AttemptFailed, AttemptUnknown}, AttemptFinished: {}, AttemptFailed: {}, AttemptUnknown: {}}
	for _, x := range allowed[from] {
		if x == to {
			return nil
		}
	}
	return &Error{Code: ErrInvalidTransition, Message: "invalid attempt status transition"}
}

type CredentialStatus string

const (
	CredentialActive         CredentialStatus = "active"
	CredentialExpired        CredentialStatus = "expired"
	CredentialRevoked        CredentialStatus = "revoked"
	CredentialRequiresReauth CredentialStatus = "requires_reauth"
)

func (s CredentialStatus) Valid() bool {
	switch s {
	case CredentialActive, CredentialExpired, CredentialRevoked, CredentialRequiresReauth:
		return true
	}
	return false
}

func CredentialTransition(from, to CredentialStatus) error {
	if !from.Valid() || !to.Valid() {
		return &Error{Code: ErrInvalidTransition, Message: "unknown credential status"}
	}
	allowed := map[CredentialStatus][]CredentialStatus{
		CredentialActive:         {CredentialExpired, CredentialRevoked, CredentialRequiresReauth},
		CredentialExpired:        {CredentialRequiresReauth, CredentialRevoked},
		CredentialRequiresReauth: {CredentialActive, CredentialRevoked, CredentialExpired},
		CredentialRevoked:        {},
	}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return nil
		}
	}
	return &Error{Code: ErrInvalidTransition, Message: "invalid credential status transition"}
}

type CredentialVersion uint64

func NewCredentialVersion(v uint64) (CredentialVersion, error) {
	if v == 0 {
		return 0, &Error{Code: ErrInvalidArgument, Message: "credential version must be positive"}
	}
	return CredentialVersion(v), nil
}
