package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/api/httperr"
	"github.com/runcoil/runcoil/internal/api/middleware"
	"github.com/runcoil/runcoil/internal/apierr"
)

func (s *Service) handleRequestError(w http.ResponseWriter, r *http.Request, err error) {
	middleware.RecordError(r.Context(), err)
	httperr.Write(w, http.StatusBadRequest, httperr.New(apierr.CodeInvalidRequest, err.Error()))
}

const statusClientClosedRequest = 499

func (s *Service) handleResponseError(w http.ResponseWriter, r *http.Request, err error) {
	middleware.RecordError(r.Context(), err)

	if errors.Is(err, context.Canceled) {
		w.WriteHeader(statusClientClosedRequest)
		return
	}

	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		httperr.Write(w, http.StatusInternalServerError, httperr.Internal)
		return
	}

	var params []gen.ErrorParam
	if field := apiErr.Details["field"]; field != "" {
		params = append(params, gen.ErrorParam{Field: field, Message: apiErr.Message})
	}

	httperr.Write(w, apiErr.Kind.HTTPStatus(), httperr.New(apiErr.Code, apiErr.Message, params...))
}
