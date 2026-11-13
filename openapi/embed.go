// Package openapi holds the dashboard API's OpenAPI document.
package openapi

import _ "embed"

// Spec is the bundled OpenAPI document.
//
//go:embed openapi.gen.yaml
var Spec []byte
