package openapi

import _ "embed"

// Spec is the bundled OpenAPI document.
//
//go:embed openapi.gen.yaml
var Spec []byte
