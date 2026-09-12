package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	ErrUnknownField   = errors.New("unknown or forbidden json field")
	ErrFieldTooLarge  = errors.New("bounded pass-through field exceeds limit")
	ErrBodyNotAllowed = errors.New("request body is not allowed for endpoint")
)

type FieldMode uint8

const (
	FieldAllowed FieldMode = iota + 1
	FieldForbidden
	FieldBoundedPassThrough
)

type FieldRule struct {
	Mode     FieldMode
	MaxBytes int
}
type EndpointPolicy struct {
	Method, Path string
	Fields       map[string]FieldRule
	MaxBodyBytes int
	Enabled      bool
}

func rule(mode FieldMode, max int) FieldRule { return FieldRule{Mode: mode, MaxBytes: max} }

// PublicEndpointPolicies is deliberately conservative. Every advertised
// endpoint starts disabled until authorization, accounting and live gates pass.
var PublicEndpointPolicies = []EndpointPolicy{
	{Method: http.MethodGet, Path: "/v1/models", Enabled: false},
	{Method: http.MethodPost, Path: "/v1/chat/completions", Enabled: false, MaxBodyBytes: 8 << 20, Fields: chatFields()},
	{Method: http.MethodPost, Path: "/v1/responses", Enabled: false, MaxBodyBytes: 8 << 20, Fields: responsesFields()},
	{Method: http.MethodPost, Path: "/v1/embeddings", Enabled: false, MaxBodyBytes: 8 << 20, Fields: embeddingsFields()},
	{Method: http.MethodPost, Path: "/v1/messages", Enabled: false, MaxBodyBytes: 8 << 20, Fields: messagesFields()},
	{Method: http.MethodPost, Path: "/v1/messages/count_tokens", Enabled: false, MaxBodyBytes: 2 << 20, Fields: countAnthropicFields()},
	{Method: http.MethodPost, Path: "/v1beta/models/{model}:generateContent", Enabled: false, MaxBodyBytes: 8 << 20, Fields: geminiFields()},
	{Method: http.MethodPost, Path: "/v1beta/models/{model}:streamGenerateContent", Enabled: false, MaxBodyBytes: 8 << 20, Fields: geminiFields()},
	{Method: http.MethodPost, Path: "/v1beta/models/{model}:countTokens", Enabled: false, MaxBodyBytes: 2 << 20, Fields: geminiCountFields()},
}

func chatFields() map[string]FieldRule {
	return fields([]string{"model", "messages", "stream", "temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "seed", "response_format", "tools", "tool_choice", "user"}, []string{"n", "modalities", "audio", "images", "image_url", "video", "web_search_options", "service_tier"}, []string{"metadata"})
}
func responsesFields() map[string]FieldRule {
	return fields([]string{"model", "input", "instructions", "stream", "temperature", "top_p", "max_output_tokens", "tools", "tool_choice", "previous_response_id", "store", "include"}, []string{"background", "conversation", "computer", "file_search", "web_search", "code_interpreter", "hosted_tools", "compact"}, []string{"metadata"})
}
func embeddingsFields() map[string]FieldRule {
	return fields([]string{"model", "input", "encoding_format", "dimensions", "user"}, []string{"image", "images", "input_audio"}, nil)
}
func messagesFields() map[string]FieldRule {
	return fields([]string{"model", "messages", "max_tokens", "system", "stream", "temperature", "top_p", "top_k", "stop_sequences", "tools", "tool_choice"}, []string{"thinking", "container", "service_tier", "betas", "cache_control"}, []string{"metadata"})
}
func countAnthropicFields() map[string]FieldRule {
	return fields([]string{"model", "messages", "system", "tools"}, []string{"thinking", "container", "betas"}, nil)
}
func geminiFields() map[string]FieldRule {
	return fields([]string{"contents", "systemInstruction", "generationConfig", "safetySettings", "tools", "toolConfig"}, []string{"cachedContent", "fileData", "inlineData", "codeExecution", "googleSearch", "urlContext"}, []string{"metadata"})
}
func geminiCountFields() map[string]FieldRule {
	return fields([]string{"contents", "systemInstruction", "tools"}, []string{"cachedContent", "fileData", "inlineData", "codeExecution", "googleSearch", "urlContext"}, nil)
}
func fields(allowed, forbidden, bounded []string) map[string]FieldRule {
	m := map[string]FieldRule{}
	for _, k := range allowed {
		m[k] = rule(FieldAllowed, 0)
	}
	for _, k := range forbidden {
		m[k] = rule(FieldForbidden, 0)
	}
	for _, k := range bounded {
		m[k] = rule(FieldBoundedPassThrough, 16<<10)
	}
	return m
}

func FindEndpointPolicy(method, path string) (EndpointPolicy, bool) {
	for _, p := range PublicEndpointPolicies {
		if p.Method != method {
			continue
		}
		if p.Path == path || (strings.Contains(p.Path, "{model}") && matchModelPath(p.Path, path)) {
			return p, true
		}
	}
	return EndpointPolicy{}, false
}
func matchModelPath(pattern, path string) bool {
	pre, post, ok := strings.Cut(pattern, "{model}")
	if !ok || !strings.HasPrefix(path, pre) || !strings.HasSuffix(path, post) || len(path) <= len(pre)+len(post) {
		return false
	}
	model := path[len(pre) : len(path)-len(post)]
	return model != "" && !strings.Contains(model, "/")
}

// ValidateObjectFields rejects undeclared and forbidden fields and returns
// allowed fields and exact RawMessage values for bounded pass-through fields.
func ValidateObjectFields(data []byte, p EndpointPolicy) (map[string]json.RawMessage, error) {
	max := p.MaxBodyBytes
	if max <= 0 {
		max = 8 << 20
	}
	if p.Fields == nil {
		if len(strings.TrimSpace(string(data))) != 0 {
			return nil, ErrBodyNotAllowed
		}
		return map[string]json.RawMessage{}, nil
	}
	obj, err := ParseObject(data, JSONOptions{MaxBytes: max, MaxDepth: DefaultMaxJSONDepth})
	if err != nil {
		return nil, err
	}
	for k, v := range obj {
		r, ok := p.Fields[k]
		if !ok || r.Mode == FieldForbidden {
			return nil, fmt.Errorf("%w: %s", ErrUnknownField, k)
		}
		if r.Mode == FieldBoundedPassThrough && r.MaxBytes > 0 && len(v) > r.MaxBytes {
			return nil, fmt.Errorf("%w: %s", ErrFieldTooLarge, k)
		}
		if containsDangerousNestedField(v) {
			return nil, fmt.Errorf("%w: nested media or hosted tool in %s", ErrUnknownField, k)
		}
	}
	return obj, nil
}

// These names are rejected at any nesting level because they would delegate
// network access, media processing, or execution to an unverified provider.
var dangerousNestedFields = map[string]struct{}{
	"image_url": {}, "input_image": {}, "video": {}, "audio": {},
	"file_data": {}, "inline_data": {}, "fileData": {}, "inlineData": {},
	"computer": {}, "code_interpreter": {}, "web_search": {}, "googleSearch": {}, "urlContext": {},
}

func containsDangerousNestedField(raw []byte) bool {
	v, err := DecodeStrict(raw, JSONOptions{MaxBytes: len(raw), MaxDepth: DefaultMaxJSONDepth})
	if err != nil {
		return true
	}
	var walk func(any) bool
	walk = func(x any) bool {
		switch y := x.(type) {
		case map[string]any:
			for k, v := range y {
				if _, bad := dangerousNestedFields[k]; bad {
					return true
				}
				if walk(v) {
					return true
				}
			}
		case []any:
			for _, v := range y {
				if walk(v) {
					return true
				}
			}
		}
		return false
	}
	return walk(v)
}
