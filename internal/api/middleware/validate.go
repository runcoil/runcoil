package middleware

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/api/httperr"
	"github.com/runcoil/runcoil/internal/apierr"
)

const invalidRequestMessage = "request is not valid"

// ValidateRequest refuses a request that does not match spec, whose paths sit under
// prefix.
func ValidateRequest(spec *openapi3.T, prefix string) func(http.Handler) http.Handler {
	return nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		Options: openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
		ErrorHandlerWithOpts: handleValidationError,
		DoNotValidateServers: true,
		Prefix:               prefix,
	})
}

func handleValidationError(_ context.Context, err error, w http.ResponseWriter, r *http.Request, _ nethttpmiddleware.ErrorHandlerOpts) {
	RecordError(r.Context(), err)

	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		httperr.Write(w, http.StatusRequestEntityTooLarge, httperr.New(apierr.CodePayloadTooLarge, "request body is too large"))
		return
	}

	if errors.Is(err, routers.ErrPathNotFound) {
		httperr.Write(w, http.StatusNotFound, httperr.New(apierr.CodeNotFound, "no such operation"))
		return
	}

	if errors.Is(err, routers.ErrMethodNotAllowed) {
		httperr.Write(w, http.StatusMethodNotAllowed, httperr.New(apierr.CodeInvalidRequest, "method not allowed"))
		return
	}

	var violation *jsonschema.ValidationError
	if errors.As(err, &violation) {
		httperr.Write(w, http.StatusBadRequest, invalidRequest(violationParams(violation)))
		return
	}

	var requestErr *openapi3filter.RequestError
	if errors.As(err, &requestErr) && requestErr.Parameter != nil {
		param := gen.ErrorParam{Field: requestErr.Parameter.Name, Message: requestErrorText(requestErr)}
		httperr.Write(w, http.StatusBadRequest, invalidRequest([]gen.ErrorParam{param}))

		return
	}

	if errors.As(err, &requestErr) && requestErrorText(requestErr) != "" {
		httperr.Write(w, http.StatusBadRequest, httperr.New(apierr.CodeInvalidRequest, requestErrorText(requestErr)))
		return
	}

	httperr.Write(w, http.StatusBadRequest, httperr.New(apierr.CodeInvalidRequest, invalidRequestMessage))
}

func requestErrorText(requestErr *openapi3filter.RequestError) string {
	if requestErr.Reason != "" || requestErr.Err == nil {
		return requestErr.Reason
	}

	return requestErr.Err.Error()
}

func invalidRequest(params []gen.ErrorParam) gen.ErrorResponse {
	text := invalidRequestMessage
	if len(params) == 1 {
		text = params[0].Message
	}

	return httperr.New(apierr.CodeInvalidRequest, text, params...)
}

func violationParams(violation *jsonschema.ValidationError) []gen.ErrorParam {
	if len(violation.Causes) > 0 {
		var params []gen.ErrorParam
		for _, cause := range violation.Causes {
			params = append(params, violationParams(cause)...)
		}

		return params
	}

	location := violation.InstanceLocation

	switch k := violation.ErrorKind.(type) {
	case *kind.Required:
		params := make([]gen.ErrorParam, 0, len(k.Missing))
		for _, name := range k.Missing {
			field := fieldPath(location, name)
			params = append(params, gen.ErrorParam{Field: field, Message: field + " is required"})
		}

		return params
	case *kind.AdditionalProperties:
		params := make([]gen.ErrorParam, 0, len(k.Properties))
		for _, name := range k.Properties {
			field := fieldPath(location, name)
			params = append(params, gen.ErrorParam{Field: field, Message: field + " is not allowed"})
		}

		return params
	default:
		field := fieldPath(location, "")
		text := field + " is not valid: " + k.LocalizedString(message.NewPrinter(language.English))

		return []gen.ErrorParam{{Field: field, Message: text}}
	}
}

func fieldPath(location []string, name string) string {
	if name == "" {
		return strings.Join(location, ".")
	}

	return strings.Join(append(slices.Clone(location), name), ".")
}
