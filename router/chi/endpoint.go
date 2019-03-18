package chi

import (
	"net/http"
	"strings"

	"github.com/vm-affekt/krakend/config"
	"github.com/vm-affekt/krakend/proxy"
	"github.com/vm-affekt/krakend/router/mux"
	"github.com/go-chi/chi"
)

// HandlerFactory creates a handler function that adapts the chi router with the injected proxy
type HandlerFactory func(*config.EndpointConfig, proxy.Proxy) http.HandlerFunc

// NewEndpointHandler implements the HandleFactory interface using the default ToHTTPError function
func NewEndpointHandler(cfg *config.EndpointConfig, prxy proxy.Proxy) http.HandlerFunc {
	hf := mux.CustomEndpointHandler(
		mux.NewRequestBuilder(func(r *http.Request) map[string]string {
			return extractParamsFromEndpoint(r)
		}),
	)
	return hf(cfg, prxy)
}

func extractParamsFromEndpoint(r *http.Request) map[string]string {
	ctx := r.Context()
	rctx := chi.RouteContext(ctx)

	params := map[string]string{}
	if len(rctx.URLParams.Keys) > 0 {
		for _, param := range rctx.URLParams.Keys {
			params[strings.Title(param)] = chi.URLParam(r, param)
		}
	}
	return params
}
