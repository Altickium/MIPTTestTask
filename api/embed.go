package apispec

import _ "embed"

// OpenAPI is the public API contract served by the application.
//
//go:embed openapi.yaml
var OpenAPI []byte
