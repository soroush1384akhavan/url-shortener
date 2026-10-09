package docs

import "testing"

func TestSwaggerDocsInitialized(t *testing.T) {
	if SwaggerInfo.Title == "" {
		t.Error("expected SwaggerInfo.Title to not be empty")
	}

	if SwaggerInfo.Version == "" {
		t.Error("expected SwaggerInfo.Version to not be empty")
	}
}