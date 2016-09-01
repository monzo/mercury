package server

import (
	"github.com/monzo/mercury"
	"github.com/monzo/mercury/transport"
)

// A Server provides Endpoint RPC functionality atop a typhon Transport.
type Server interface {
	// Name returns the service name. It must be set at construction time and is immutable.
	Name() string
	// AddEndpoints registers new Endpoint. If any name conflicts with an existing endpoint, the old endpoint(s) will be
	// removed. Errors raised as panics.
	AddEndpoints(eps ...Endpoint)
	// RemoveEndpoints removes the Endpoints given (if they are registered).
	RemoveEndpoints(eps ...Endpoint)
	// Endpoint returns a registered endpoint (if there is one) for the given Name.
	Endpoint(name string) (Endpoint, bool)
	// Endpoints returns all Endpoints registered.
	Endpoints() []Endpoint
	// Start starts the server on the given transport, and returns once the server is ready for work. The server will
	// continue until purposefully stopped, or until a terminal error occurs. The transport should be pre-initialised.
	Start(trans transport.Transport) error
	// Run starts the server and blocks until it stops. As this function is intended to support the main run loop of a
	// service, an error results in a panic.
	Run(trans transport.Transport)
	// Stop forcefully stops the server. It does not terminate the underlying transport.
	Stop()

	// Middleware returns a copy of the ServerMiddleware stack currently installed.
	//
	// Server middleware is used to act upon or transform a handler's input or output. Middleware is applied in order
	// during the request phase, and in reverse order during the response phase.
	Middleware() []ServerMiddleware
	// SetMiddleware replaces the server's ServerMiddleware stack.
	SetMiddleware([]ServerMiddleware)
	// AddMiddleware appends the given ServerMiddleware to the stack.
	AddMiddleware(ServerMiddleware)
}

type ServerMiddleware interface {
	// ProcessServerRequest is called on each inbound request, before it is routed to an Endpoint. If a response or an
	// error is returned, Mercury does not bother calling any other request middleware. It will apply response
	// middleware and respond to the caller with the result.
	//
	// If an error is to be returned, use `ErrorResponse`.
	ProcessServerRequest(req mercury.Request) (mercury.Request, mercury.Response)
	// ProcessServerResponse is called on all responses before they are returned to a caller. Unlike request middleware,
	// response middleware is always called. If an error is returned, it will be marshaled to a response and will
	// continue to other response middleware.
	//
	// Nil responses MUST be handled. If an error is to be returned, use `ErrorResponse`.
	//
	// Note that response middleware are applied in reverse order.
	ProcessServerResponse(rsp mercury.Response, req mercury.Request) mercury.Response
}
