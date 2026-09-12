package protocol

import "errors"

var (
	ErrUnauthorized          = errors.New("unauthorized")
	ErrUnsupportedCapability = errors.New("unsupported capability")
)

// CapabilityDescriptor is the versioned public description of one protocol
// capability. Disabled is the safe default and cannot be inferred from auth.
type CapabilityDescriptor struct {
	Name, Version, Protocol string
	Enabled                 bool
	RequiresAuthorization   bool
}

func DefaultCapabilities() []CapabilityDescriptor {
	result := make([]CapabilityDescriptor, 0, len(PublicEndpointPolicies))
	for _, p := range PublicEndpointPolicies {
		result = append(result, CapabilityDescriptor{Name: p.Method + " " + p.Path, Version: "v1", Protocol: p.Path, Enabled: false, RequiresAuthorization: true})
	}
	return result
}

// CapabilityForEndpoint returns a disabled descriptor for a declared route;
// unknown routes intentionally return false so callers can map them to the
// same stable unsupported capability response without guessing a protocol.
func CapabilityForEndpoint(method, path string) (CapabilityDescriptor, bool) {
	p, ok := FindEndpointPolicy(method, path)
	if !ok {
		return CapabilityDescriptor{}, false
	}
	return CapabilityDescriptor{Name: p.Method + " " + p.Path, Version: "v1", Protocol: p.Path, Enabled: p.Enabled, RequiresAuthorization: true}, true
}

// CheckCapability intentionally distinguishes a valid principal without an
// enabled feature from a request lacking authentication.
func CheckCapability(c CapabilityDescriptor, authenticated bool) error {
	if !authenticated {
		return ErrUnauthorized
	}
	if !c.Enabled {
		return ErrUnsupportedCapability
	}
	return nil
}
