package contracts_test

// P01-T03 (checklists/test-matrix.csv: OpenAPI 生成/验证): the shipped
// management contract keeps info.title=Urbino, declares the common error
// envelope, and prepares a stable unsupported error for every operation
// instead of faking success. Regeneration diff-freeness of api/admin_gen.go
// is verified separately by the main agent's generate step.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"example.com/urbino/internal/domain"
)

func loadOpenAPI(t *testing.T) map[string]any {
	t.Helper()
	path := filepath.Join(findRepoRoot(t), "api", "admin.openapi.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if doc == nil {
		t.Fatalf("%s parsed to an empty document", path)
	}
	return doc
}

func mapOf(t *testing.T, v any, what string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s: expected mapping, got %T", what, v)
	}
	return m
}

// resolveRef follows local "#/..." references to their target mapping.
func resolveRef(t *testing.T, doc map[string]any, node map[string]any) map[string]any {
	t.Helper()
	for i := 0; i < 16; i++ {
		raw, ok := node["$ref"]
		if !ok {
			return node
		}
		ref, ok := raw.(string)
		if !ok || !strings.HasPrefix(ref, "#/") {
			t.Fatalf("unsupported $ref %v", raw)
		}
		cur := any(doc)
		for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			m, ok := cur.(map[string]any)
			if !ok {
				t.Fatalf("cannot resolve $ref %q at segment %q", ref, part)
			}
			cur = m[part]
		}
		node, ok = cur.(map[string]any)
		if !ok {
			t.Fatalf("$ref %q does not resolve to a mapping", ref)
		}
	}
	t.Fatal("$ref chain does not terminate")
	return nil
}

func TestP01T03OpenAPIHeaderContract(t *testing.T) {
	doc := loadOpenAPI(t)
	if version, _ := doc["openapi"].(string); !strings.HasPrefix(version, "3.") {
		t.Fatalf("openapi version = %q, want 3.x", version)
	}
	info := mapOf(t, doc["info"], "info")
	if info["title"] != "Urbino" {
		t.Fatalf("info.title = %v, want Urbino", info["title"])
	}
	if v, _ := info["version"].(string); v == "" {
		t.Fatal("info.version must be set")
	}
	paths := mapOf(t, doc["paths"], "paths")
	if len(paths) == 0 {
		t.Fatal("management contract must declare paths")
	}
}

func errorEnvelopeProperties(t *testing.T) map[string]bool {
	t.Helper()
	return map[string]bool{"code": true, "message": true, "request_id": true, "retryable": true, "details": true}
}

func TestP01T03CommonErrorSchema(t *testing.T) {
	doc := loadOpenAPI(t)
	components := mapOf(t, doc["components"], "components")
	schemas := mapOf(t, components["schemas"], "components.schemas")
	errSchema := mapOf(t, schemas["Error"], "components.schemas.Error")

	gotRequired := map[string]bool{}
	required, ok := errSchema["required"].([]any)
	if !ok {
		t.Fatal("components.schemas.Error.required must be a list")
	}
	for _, r := range required {
		name, ok := r.(string)
		if !ok {
			t.Fatalf("required entries must be strings, got %T", r)
		}
		gotRequired[name] = true
	}
	want := errorEnvelopeProperties(t)
	for name := range want {
		if !gotRequired[name] {
			t.Errorf("Error schema must require %q", name)
		}
	}
	for name := range gotRequired {
		if !want[name] {
			t.Errorf("Error schema requires unexpected field %q", name)
		}
	}

	props := mapOf(t, errSchema["properties"], "Error.properties")
	for name := range want {
		if _, ok := props[name]; !ok {
			t.Errorf("Error schema must declare property %q", name)
		}
	}
	for name := range props {
		if !want[name] {
			t.Errorf("Error schema declares undeclared envelope field %q", name)
		}
	}

	if got := mapOf(t, props["retryable"], "retryable")["type"]; got != "boolean" {
		t.Errorf("retryable.type = %v, want boolean", got)
	}
	requestID := mapOf(t, props["request_id"], "request_id")
	if requestID["type"] != "string" || requestID["format"] != "uuid" {
		t.Errorf("request_id = %v, want string with uuid format", requestID)
	}
	details := mapOf(t, props["details"], "details")
	if details["type"] != "object" {
		t.Errorf("details.type = %v, want object", details["type"])
	}
	additional := mapOf(t, details["additionalProperties"], "details.additionalProperties")
	if additional["type"] != "string" {
		t.Errorf("details values must be strings, got %v", additional["type"])
	}

	// Cross-artifact consistency: the codes enumerated by the OpenAPI error
	// schema are exactly the public codes of internal/domain.
	enum, ok := mapOf(t, props["code"], "code")["enum"].([]any)
	if !ok {
		t.Fatal("Error.code must enumerate its stable values")
	}
	gotCodes := map[string]bool{}
	for _, c := range enum {
		name, ok := c.(string)
		if !ok {
			t.Fatalf("code enum entries must be strings, got %T", c)
		}
		gotCodes[name] = true
	}
	wantCodes := map[string]bool{
		string(domain.CodeBadRequest):            true,
		string(domain.CodeUnauthorized):          true,
		string(domain.CodePermissionDenied):      true,
		string(domain.CodeResourceNotFound):      true,
		string(domain.CodeVersionConflict):       true,
		string(domain.CodeUnsupportedCapability): true,
		string(domain.CodeUnknown):               true,
	}
	for code := range wantCodes {
		if !gotCodes[code] {
			t.Errorf("OpenAPI Error.code enum misses public domain code %q", code)
		}
	}
	for code := range gotCodes {
		if !wantCodes[code] {
			t.Errorf("OpenAPI Error.code enum declares unknown code %q", code)
		}
	}
}

func TestP01T03NoFakeSuccessOperations(t *testing.T) {
	doc := loadOpenAPI(t)
	paths := mapOf(t, doc["paths"], "paths")
	operations := 0
	for path, itemAny := range paths {
		item := mapOf(t, itemAny, "path item "+path)
		for _, method := range []string{"get", "post", "put", "patch", "delete", "head", "options", "trace"} {
			opAny, ok := item[method]
			if !ok {
				continue
			}
			operations++
			op := mapOf(t, opAny, path+" "+method)
			responses := mapOf(t, op["responses"], path+" "+method+" responses")
			if len(responses) == 0 {
				t.Errorf("%s %s: operation without responses", method, path)
			}

			// Every undeployed operation must be able to answer with the
			// stable unsupported error instead of a fake success.
			unsupAny, ok := responses["501"]
			if !ok {
				t.Errorf("%s %s: no 501 unsupported response declared", method, path)
			} else {
				unsup := resolveRef(t, doc, mapOf(t, unsupAny, path+" "+method+" 501"))
				content := mapOf(t, unsup["content"], path+" "+method+" 501 content")
				jsonContent := mapOf(t, content["application/json"], path+" "+method+" 501 json")
				schema := resolveRef(t, doc, mapOf(t, jsonContent["schema"], path+" "+method+" 501 schema"))
				if _, ok := schema["properties"]; !ok {
					t.Errorf("%s %s: 501 response does not use the common Error schema", method, path)
				}
			}

			// Declared successes must carry a concrete JSON schema; a schema-
			// less 2XX would be a fake-success placeholder.
			for code, respAny := range responses {
				if !strings.HasPrefix(code, "2") || code == "204" {
					continue
				}
				resp := resolveRef(t, doc, mapOf(t, respAny, path+" "+method+" "+code))
				content, ok := resp["content"].(map[string]any)
				if !ok {
					t.Errorf("%s %s %s: success response without content", method, path, code)
					continue
				}
				jsonAny, ok := content["application/json"]
				if !ok {
					t.Errorf("%s %s %s: success response without application/json", method, path, code)
					continue
				}
				if _, ok := mapOf(t, jsonAny, path+" "+method+" "+code+" json")["schema"]; !ok {
					t.Errorf("%s %s %s: success response without schema", method, path, code)
				}
			}
		}
	}
	if operations == 0 {
		t.Fatal("management contract declares no operations")
	}
	t.Logf("validated %d operations across %d paths", operations, len(paths))
}
