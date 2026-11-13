package swagger

import (
	"net/http"
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
