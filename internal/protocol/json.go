package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

const (
	// DefaultMaxJSONBytes is the largest public JSON request accepted by this
	// package. Callers may choose a smaller limit for a specific endpoint.
	DefaultMaxJSONBytes = 8 << 20
	DefaultMaxJSONDepth = 64
)

var (
	ErrJSONTooLarge    = errors.New("json body exceeds limit")
	ErrJSONTooDeep     = errors.New("json nesting exceeds limit")
	ErrDuplicateKey    = errors.New("duplicate json object key")
	ErrTrailingContent = errors.New("trailing json content")
	ErrInvalidUTF8     = errors.New("json is not valid UTF-8")
)

// JSONOptions bounds resource use while decoding an untrusted JSON body.
type JSONOptions struct{ MaxBytes, MaxDepth int }

func (o JSONOptions) normalized() JSONOptions {
	if o.MaxBytes <= 0 {
		o.MaxBytes = DefaultMaxJSONBytes
	}
	if o.MaxDepth <= 0 {
		o.MaxDepth = DefaultMaxJSONDepth
	}
	return o
}

// DecodeStrict decodes JSON while rejecting duplicate keys (after unescaping),
// malformed/invalid UTF-8 input, excessive depth and any second top-level value.
// json.Number is used so validation does not silently round large numbers.
func DecodeStrict(data []byte, opts JSONOptions) (any, error) {
	o := opts.normalized()
	if len(data) > o.MaxBytes {
		return nil, ErrJSONTooLarge
	}
	if !utf8.Valid(data) {
		return nil, ErrInvalidUTF8
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeValue(dec, 0, o.MaxDepth)
	if err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, ErrTrailingContent
		}
		return nil, fmt.Errorf("%w: %v", ErrTrailingContent, err)
	}
	return v, nil
}

func decodeValue(dec *json.Decoder, depth, maxDepth int) (any, error) {
	if depth > maxDepth {
		return nil, ErrJSONTooDeep
	}
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch x := t.(type) {
	case json.Delim:
		switch x {
		case '{':
			m := make(map[string]any)
			for dec.More() {
				key, err := dec.Token()
				if err != nil {
					return nil, err
				}
				ks, ok := key.(string)
				if !ok {
					return nil, errors.New("json object key is not string")
				}
				if _, exists := m[ks]; exists {
					return nil, fmt.Errorf("%w: %q", ErrDuplicateKey, ks)
				}
				value, err := decodeValue(dec, depth+1, maxDepth)
				if err != nil {
					return nil, err
				}
				m[ks] = value
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return m, nil
		case '[':
			var a []any
			for dec.More() {
				value, err := decodeValue(dec, depth+1, maxDepth)
				if err != nil {
					return nil, err
				}
				a = append(a, value)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return a, nil
		default:
			return nil, errors.New("unexpected json delimiter")
		}
	default:
		return t, nil
	}
}

// ParseObject returns each top-level value as RawMessage, preserving exact
// bytes for explicitly bounded pass-through fields. It applies the same
// duplicate/depth/UTF-8/trailing checks as DecodeStrict.
func ParseObject(data []byte, opts JSONOptions) (map[string]json.RawMessage, error) {
	o := opts.normalized()
	if len(data) > o.MaxBytes {
		return nil, ErrJSONTooLarge
	}
	if !utf8.Valid(data) {
		return nil, ErrInvalidUTF8
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return nil, errors.New("json value must be an object")
	}
	result := make(map[string]json.RawMessage)
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, err
		}
		ks, ok := key.(string)
		if !ok {
			return nil, errors.New("json object key is not string")
		}
		if _, exists := result[ks]; exists {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateKey, ks)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		if _, err := DecodeStrict(raw, JSONOptions{MaxBytes: len(raw), MaxDepth: o.MaxDepth}); err != nil {
			return nil, err
		}
		result[ks] = append(json.RawMessage(nil), raw...)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, ErrTrailingContent
		}
		return nil, fmt.Errorf("%w: %v", ErrTrailingContent, err)
	}
	return result, nil
}
