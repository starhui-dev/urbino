package api

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAdminOpenAPISourceContract(t *testing.T) {
	b, err := os.ReadFile("admin.openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("OpenAPI YAML invalid: %v", err)
	}
	info, ok := doc["info"].(map[string]any)
	if !ok || info["title"] != "Urbino" {
		t.Fatalf("info.title = %#v, want Urbino", info["title"])
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok || len(paths) < 5 {
		t.Fatalf("OpenAPI paths incomplete: %d", len(paths))
	}
	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI components missing")
	}
	if _, ok := components["securitySchemes"]; !ok {
		t.Fatal("admin security scheme missing")
	}
}
