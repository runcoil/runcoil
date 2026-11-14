package swagger

import (
	"errors"
	"fmt"
	"net/http"

	"go.yaml.in/yaml/v3"
)

const (
	pagePath = "/docs"
	specPath = "/docs/openapi.yaml"
)

const page = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Runcoil API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui.css">
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui-bundle.js" crossorigin></script>
<script>
  window.onload = () => {
    window.ui = SwaggerUIBundle({
      url: "` + specPath + `",
      dom_id: "#swagger-ui",
      deepLinking: true,
      showExtensions: true,
      showCommonExtensions: true,
    });
  };
</script>
</body>
</html>
`

// Register serves the docs page at /docs and spec, the bundled OpenAPI document, at
// /docs/openapi.yaml.
func Register(mux *http.ServeMux, spec []byte) {
	mux.HandleFunc("GET "+pagePath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})

	mux.HandleFunc("GET "+specPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(spec)
	})
}

// WithVersion returns spec with info.version set to version.
func WithVersion(spec []byte, version string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(spec, &doc); err != nil {
		return nil, fmt.Errorf("decode spec: %w", err)
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, errors.New("spec is empty")
	}

	info := mappingValue(doc.Content[0], "info")
	if info == nil {
		return nil, errors.New("spec has no info")
	}

	versionNode := mappingValue(info, "version")
	if versionNode == nil {
		return nil, errors.New("spec has no info.version")
	}

	versionNode.SetString(version)

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("encode spec: %w", err)
	}

	return out, nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}

	return nil
}
