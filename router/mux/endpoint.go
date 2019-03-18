package mux

import (
	"context"
	"fmt"
	"net/http"
	"net/textproto"
	"regexp"
	"strings"

	"github.com/vm-affekt/krakend/config"
	"github.com/vm-affekt/krakend/core"
	"github.com/vm-affekt/krakend/proxy"
	"github.com/vm-affekt/krakend/router"
)

const requestParamsAsterisk string = "*"

// HandlerFactory creates a handler function that adapts the mux router with the injected proxy
type HandlerFactory func(*config.EndpointConfig, proxy.Proxy) http.HandlerFunc

// EndpointHandler is a HandlerFactory that adapts the mux router with the injected proxy
// and the default RequestBuilder
var EndpointHandler = CustomEndpointHandler(NewRequest)

// CustomEndpointHandler returns a HandlerFactory with the received RequestBuilder using the default ToHTTPError function
func CustomEndpointHandler(rb RequestBuilder) HandlerFactory {
	return CustomEndpointHandlerWithHTTPError(rb, router.DefaultToHTTPError)
}

// CustomEndpointHandlerWithHTTPError returns a HandlerFactory with the received RequestBuilder
func CustomEndpointHandlerWithHTTPError(rb RequestBuilder, errF router.ToHTTPError) HandlerFactory {
	return func(configuration *config.EndpointConfig, prxy proxy.Proxy) http.HandlerFunc {
		cacheControlHeaderValue := fmt.Sprintf("public, max-age=%d", int(configuration.CacheTTL.Seconds()))
		isCacheEnabled := configuration.CacheTTL.Seconds() != 0
		render := getRender(configuration)

		headersToSend := configuration.HeadersToPass
		if len(headersToSend) == 0 {
			headersToSend = router.HeadersToSend
		}
		method := strings.ToTitle(configuration.Method)

		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(core.KrakendHeaderName, core.KrakendHeaderValue)
			if r.Method != method {
				w.Header().Set(router.CompleteResponseHeaderName, router.HeaderIncompleteResponseValue)
				http.Error(w, "", http.StatusMethodNotAllowed)
				return
			}

			requestCtx, cancel := context.WithTimeout(r.Context(), configuration.Timeout)

			response, err := prxy(requestCtx, rb(r, configuration.QueryString, headersToSend))

			select {
			case <-requestCtx.Done():
				if err == nil {
					err = router.ErrInternalError
				}
			default:
			}

			if response != nil && len(response.Data) > 0 {
				if response.IsComplete {
					w.Header().Set(router.CompleteResponseHeaderName, router.HeaderCompleteResponseValue)
					if isCacheEnabled {
						w.Header().Set("Cache-Control", cacheControlHeaderValue)
					}
				} else {
					w.Header().Set(router.CompleteResponseHeaderName, router.HeaderIncompleteResponseValue)
				}

				for k, vs := range response.Metadata.Headers {
					for _, v := range vs {
						w.Header().Add(k, v)
					}
				}
			} else {
				w.Header().Set(router.CompleteResponseHeaderName, router.HeaderIncompleteResponseValue)
				if err != nil {
					if t, ok := err.(responseError); ok {
						http.Error(w, err.Error(), t.StatusCode())
					} else {
						http.Error(w, err.Error(), errF(err))
					}
					cancel()
					return
				}
			}

			render(w, response)
			cancel()
		}
	}
}

// RequestBuilder is a function that creates a proxy.Request from the received http request
type RequestBuilder func(r *http.Request, queryString, headersToSend []string) *proxy.Request

// ParamExtractor is a function that extracts query params from the requested uri
type ParamExtractor func(r *http.Request) map[string]string

// NewRequest is a RequestBuilder that creates a proxy request from the received http request without
// processing the uri params
var NewRequest = NewRequestBuilder(func(_ *http.Request) map[string]string {
	return map[string]string{}
})

// NewRequestBuilder gets a RequestBuilder with the received ParamExtractor as a query param
// extraction mechanism
func NewRequestBuilder(paramExtractor ParamExtractor) RequestBuilder {
	var re = regexp.MustCompile(`^\[?([\d.:]+)\]?(:[\d]*)$`)
	return func(r *http.Request, queryString, headersToSend []string) *proxy.Request {
		params := paramExtractor(r)
		headers := make(map[string][]string, 2+len(headersToSend))

		for _, k := range headersToSend {
			if k == requestParamsAsterisk {
				headers = r.Header

				break
			}

			if h, ok := r.Header[textproto.CanonicalMIMEHeaderKey(k)]; ok {
				headers[k] = h
			}
		}

		matches := re.FindAllStringSubmatch(r.RemoteAddr, -1)

		if len(matches) > 0 && len(matches[0]) > 1 {
			headers["X-Forwarded-For"] = []string{matches[0][1]}
		} else {
			headers["X-Forwarded-For"] = []string{r.RemoteAddr}
		}
		headers["User-Agent"] = router.UserAgentHeaderValue

		query := make(map[string][]string, len(queryString))
		queryValues := r.URL.Query()
		for i := range queryString {
			if queryString[i] == requestParamsAsterisk {
				query = queryValues

				break
			}

			if v, ok := queryValues[queryString[i]]; ok && len(v) > 0 {
				query[queryString[i]] = v
			}
		}

		return &proxy.Request{
			Method:  r.Method,
			Query:   query,
			Body:    r.Body,
			Params:  params,
			Headers: headers,
		}
	}
}

type responseError interface {
	error
	StatusCode() int
}
