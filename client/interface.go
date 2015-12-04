package client

import (
	"time"

	"github.com/mondough/mercury"
	"github.com/mondough/mercury/transport"
	"github.com/mondough/terrors"
)

// A Client is a convenient way to make Requests (potentially in parallel) and access their Responses/Errors.
type Client interface {
	// Add a Call to the internal request set.
	Add(Call) Client
	// Add a Request to the internal request set (requests added this way will not benefit from automatic body
	// unmarshaling).
	AddRequest(uid string, req mercury.Request) Client
	// SetTimeout sets a timeout within which all requests must be received. Any response not received within this
	// window will result in an error being added to the error set.
	SetTimeout(timeout time.Duration) Client
	// Go fires off the requests. It does not wait until the requests have completed to return.
	Go() Client
	// Wait blocks until all requests have finished executing.
	Wait() Client
	// Execute fires off all requests and waits until all requests have completed before returning.
	Execute() Client
	// SetTransport configures a Transport to use with this Client. By default, it uses the default transport.
	SetTransport(t transport.Transport) Client

	// WaitC returns a channel which will be closed when all requests have finished.
	WaitC() <-chan struct{}
	// Errors returns an ErrorSet of all errors generated during execution (if any).
	Errors() ErrorSet
	// Response retrieves the Response for the request given by its uid. If no such uid is known, returns nil.
	Response(uid string) mercury.Response

	// Middleware returns the ClientMiddleware stack currently installed. This is not a copy, so it's advisable not to
	// f*ck around with it.
	//
	// Client middleware is used to act upon or transform an RPC request or its response. Middleware is applied in order
	// during the request phase, and in reverse order during the response phase.
	//
	// Beware: client middleware can cause the timeouts to be exceeded. They must be fast, and certainly should not
	// make any remote calls themselves.
	Middleware() []ClientMiddleware
	// SetMiddleware replaces the client's Client stack.
	SetMiddleware([]ClientMiddleware) Client
	// AddMiddleware appends the given ClientMiddleware to the stack.
	AddMiddleware(ClientMiddleware) Client
}

type ClientMiddleware interface {
	// ProcessClientRequest is called on every outbound request, before it is sent to a transport.
	//
	// The middleware may mutate the request, or by returning nil, prevent the request from being sent entirely.
	ProcessClientRequest(req mercury.Request) mercury.Request
	// ProcessClientResponse is called on responses before they are available to the caller. If a call fails, or
	// returns an error, ProcessClientError is invoked instead of this method for that request.
	//
	// Note that response middleware are applied in reverse order.
	ProcessClientResponse(rsp mercury.Response, req mercury.Request) mercury.Response
	// ProcessClientError is called whenever a remote call results in an error (either local or remote).
	//
	// Note that error middleware are applied in reverse order.
	ProcessClientError(err *terrors.Error, req mercury.Request)
}
