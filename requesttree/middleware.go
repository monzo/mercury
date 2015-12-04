package requesttree

import (
	"golang.org/x/net/context"

	"github.com/mondough/mercury"
	"github.com/mondough/terrors"
)

const (
	parentIdHeader = "Parent-Request-ID"
	reqIdCtxKey    = "Request-ID"

	currentServiceHeader  = "Current-Service"
	currentEndpointHeader = "Current-Endpoint"
	originServiceHeader   = "Origin-Service"
	originEndpointHeader  = "Origin-Endpoint"
)

type requestTreeMiddleware struct{}

func (m requestTreeMiddleware) ProcessClientRequest(req mercury.Request) mercury.Request {
	if req.Headers()[parentIdHeader] == "" { // Don't overwrite an exiting header
		if parentId, ok := req.Context().Value(reqIdCtxKey).(string); ok && parentId != "" {
			req.SetHeader(parentIdHeader, parentId)
		}
	}

	// Pass through the current service and endpoint as the origin of this request
	req.SetHeader(originServiceHeader, CurrentServiceFor(req))
	req.SetHeader(originEndpointHeader, CurrentEndpointFor(req))

	return req
}

func (m requestTreeMiddleware) ProcessClientResponse(rsp mercury.Response, req mercury.Request) mercury.Response {
	return rsp
}

func (m requestTreeMiddleware) ProcessClientError(err *terrors.Error, req mercury.Request) {
}

func (m requestTreeMiddleware) ProcessServerRequest(req mercury.Request) (mercury.Request, mercury.Response) {
	req.SetContext(context.WithValue(req.Context(), reqIdCtxKey, req.Id()))
	if v := req.Headers()[parentIdHeader]; v != "" {
		req.SetContext(context.WithValue(req.Context(), parentIdCtxKey, v))
	}

	// Set the current service and endpoint into the context
	req.SetContext(context.WithValue(req.Context(), currentServiceHeader, req.Service()))
	req.SetContext(context.WithValue(req.Context(), currentEndpointHeader, req.Endpoint()))

	// Set the originator into the context
	req.SetContext(context.WithValue(req.Context(), originServiceHeader, req.Headers()[originServiceHeader]))
	req.SetContext(context.WithValue(req.Context(), originEndpointHeader, req.Headers()[originEndpointHeader]))

	return req, nil
}

func (m requestTreeMiddleware) ProcessServerResponse(rsp mercury.Response, req mercury.Request) mercury.Response {
	if v, ok := req.Value(parentIdCtxKey).(string); ok && v != "" && rsp != nil {
		rsp.SetHeader(parentIdHeader, v)
	}
	return rsp
}

func Middleware() requestTreeMiddleware {
	return requestTreeMiddleware{}
}

// OriginServiceFor returns the originating service for this context
func OriginServiceFor(ctx context.Context) string {
	if s, ok := ctx.Value(originServiceHeader).(string); ok {
		return s
	}
	return ""
}

// OriginEndpointFor returns the originating endpoint for this context
func OriginEndpointFor(ctx context.Context) string {
	if e, ok := ctx.Value(originEndpointHeader).(string); ok {
		return e
	}
	return ""
}

// CurrentServiceFor returns the current service that this context is executing within
func CurrentServiceFor(ctx context.Context) string {
	if s, ok := ctx.Value(currentServiceHeader).(string); ok {
		return s
	}
	return ""
}

// CurrentEndpointFor returns the current endpoint that this context is executing within
func CurrentEndpointFor(ctx context.Context) string {
	if e, ok := ctx.Value(currentEndpointHeader).(string); ok {
		return e
	}
	return ""
}
