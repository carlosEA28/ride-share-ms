package tracing

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func WrapperHandlerFunc(handler http.HandlerFunc, operation string) http.Handler {
	return otelhttp.NewHandler(handler, operation)
}
