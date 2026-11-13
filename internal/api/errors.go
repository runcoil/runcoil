package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/api/httperr"
	"github.com/runcoil/runcoil/internal/api/middleware"
	"github.com/runcoil/runcoil/internal/apperr"
)

func (s *Service) handleRequestError(w http.ResponseWriter, r *http.Request, err error) {
	middleware.RecordError(r.Context(), err)
	httperr.Write(w, http.StatusBadRequest, httperr.New(apperr.CodeInvalidRequest, err.Error()))
}

const statusClientClosedRequest = 499

func (s *Service) handleResponseError(w http.ResponseWriter, r *http.Request, err error) {
	middleware.RecordError(r.Context(), err)

	if errors.Is(err, context.Canceled) {
		w.WriteHeader(statusClientClosedRequest)
		return
	}

	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		httperr.Write(w, http.StatusInternalServerError, httperr.Internal)
		return
	}

	var params []gen.ErrorParam
	if field := appErr.Details["field"]; field != "" {
		params = append(params, gen.ErrorParam{Field: field, Message: appErr.Message})
	}

	httperr.Write(w, appErr.Kind.HTTPStatus(), httperr.New(appErr.Code, appErr.Message, params...))
}
