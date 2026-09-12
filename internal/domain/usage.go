package domain

import "fmt"

// Count distinguishes an omitted provider field from an observed zero.
type Count struct {
	Value int64
	Known bool
}

func KnownCount(v int64) (Count, error) {
	if v < 0 {
		return Count{}, fmt.Errorf("negative count")
	}
	return Count{Value: v, Known: true}, nil
}
func UnknownCount() Count { return Count{} }

type UsageCompleteness string

const (
	UsageUnknown  UsageCompleteness = "unknown"
	UsagePartial  UsageCompleteness = "partial"
	UsageComplete UsageCompleteness = "complete"
)

func (c UsageCompleteness) Valid() bool {
	switch c {
	case UsageUnknown, UsagePartial, UsageComplete:
		return true
	}
	return false
}

type UsageSource string

const (
	UsageSourceProvider UsageSource = "provider"
	UsageSourceEstimate UsageSource = "estimate"
	UsageSourceGateway  UsageSource = "gateway"
)

type Usage struct {
	InputTotal    Count
	InputUncached Count
	CacheRead     Count
	CacheWrite    Count
	OutputTotal   Count
	Reasoning     Count
	Completeness  UsageCompleteness
	Source        UsageSource
	IsEstimate    bool
}

func (u Usage) Validate() error {
	if !u.Completeness.Valid() {
		return &Error{Code: ErrInvalidArgument, Message: "invalid usage completeness"}
	}
	for _, c := range []Count{u.InputTotal, u.InputUncached, u.CacheRead, u.CacheWrite, u.OutputTotal, u.Reasoning} {
		if c.Known && c.Value < 0 {
			return &Error{Code: ErrInvalidArgument, Message: "negative usage"}
		}
	}
	if u.Completeness == UsageComplete && !u.InputTotal.Known && !u.OutputTotal.Known {
		return &Error{Code: ErrInvalidArgument, Message: "complete usage missing totals"}
	}
	return nil
}
