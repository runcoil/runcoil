package api

import "github.com/runcoil/runcoil/internal/api/gen"

var _ gen.StrictServerInterface = (*handler)(nil)

type handler struct {
	*authHandler
}
