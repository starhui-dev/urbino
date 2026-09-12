package protocol

import (
	"errors"
	"testing"
)

func TestDecodeStrictRejectsDuplicateEscapedKey(t *testing.T) {
	_, err := DecodeStrict([]byte(`{"a":1,"\u0061":2}`), JSONOptions{})
	if !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("expected duplicate, got %v", err)
	}
}
func TestDecodeStrictBoundsAndTrailing(t *testing.T) {
	if _, err := DecodeStrict([]byte(`{"a":`), JSONOptions{}); err == nil {
		t.Fatal("malformed JSON accepted")
	}
	if _, err := DecodeStrict([]byte(`{"a":{"b":1}}`), JSONOptions{MaxDepth: 1}); !errors.Is(err, ErrJSONTooDeep) {
		t.Fatalf("depth: %v", err)
	}
	if _, err := DecodeStrict([]byte(`{"a":1} {"b":2}`), JSONOptions{}); !errors.Is(err, ErrTrailingContent) {
		t.Fatalf("trailing: %v", err)
	}
	if _, err := DecodeStrict([]byte{0xff}, JSONOptions{}); !errors.Is(err, ErrInvalidUTF8) {
		t.Fatalf("utf8: %v", err)
	}
	if _, err := DecodeStrict([]byte(`{"a":123}`), JSONOptions{MaxBytes: 5}); !errors.Is(err, ErrJSONTooLarge) {
		t.Fatalf("size: %v", err)
	}
}
func TestValidateObjectFieldsPreservesBoundedRawAndRejectsNestedMedia(t *testing.T) {
	p, _ := FindEndpointPolicy("POST", "/v1/chat/completions")
	obj, err := ValidateObjectFields([]byte(`{"model":"m","metadata":{"x":[1,true]}}`), p)
	if err != nil {
		t.Fatal(err)
	}
	if string(obj["metadata"]) != `{"x":[1,true]}` {
		t.Fatalf("raw value changed: %s", obj["metadata"])
	}
	if _, err := ValidateObjectFields([]byte(`{"model":"m","messages":[{"content":[{"type":"image_url","image_url":{"url":"x"}}]}]}`), p); !errors.Is(err, ErrUnknownField) {
		t.Fatalf("nested media accepted: %v", err)
	}
	if _, err := ValidateObjectFields([]byte(`{"model":"m","n":2}`), p); !errors.Is(err, ErrUnknownField) {
		t.Fatalf("forbidden accepted: %v", err)
	}
}
