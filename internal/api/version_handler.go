package api

import (
	"context"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/build"
)

type versionHandler struct{}

func newVersionHandler() *versionHandler {
	return &versionHandler{}
}

func (h *versionHandler) GetVersion(_ context.Context, _ gen.GetVersionRequestObject) (gen.GetVersionResponseObject, error) {
	info := build.Get()

	return gen.GetVersion200JSONResponse{
		Version:   info.Version,
		Commit:    info.Commit,
		BuildDate: info.BuildDate,
		GoVersion: info.GoVersion,
	}, nil
}
