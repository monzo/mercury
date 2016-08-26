package server

import (
	log "github.com/mondough/slog"
	"github.com/mondough/terrors"
	tmsg "github.com/mondough/typhon/message"

	"github.com/mondough/mercury"
	"github.com/mondough/mercury/marshaling"
)

type Handler func(req mercury.Request) (mercury.Response, error)

// An Endpoint represents a handler function bound to a particular endpoint name.
type Endpoint struct {
	// Name is the Endpoint's unique name, and is used to route requests to it.
	Name string
	// Handler is a function to be invoked upon receiving a request, to generate a response.
	Handler Handler
	// Request is a "template" object for the Endpoint's request format.
	Request interface{}
	// Response is a "template" object for the Endpoint's response format.
	Response interface{}
}

func (e Endpoint) unmarshaler(req mercury.Request) tmsg.Unmarshaler {
	result := marshaling.Unmarshaler(req.Headers()[marshaling.ContentTypeHeader], e.Request)
	if result == nil { // Default to json
		result = marshaling.Unmarshaler(marshaling.JSONContentType, e.Request)
	}
	return result
}

// Handle takes an inbound Request, unmarshals it, dispatches it to the handler, and serialises the result as a
// Response. Note that the response may be nil.
func (e Endpoint) Handle(req mercury.Request) (mercury.Response, error) {
	// Unmarshal the request body (unless there already is one)
	if req.Body() == nil && e.Request != nil {
		if um := e.unmarshaler(req); um != nil {
			if werr := terrors.Wrap(um.UnmarshalPayload(req), nil); werr != nil {
				log.Warn(req, "[Mercury:Server] Cannot unmarshal request payload: %v", werr)
				terr := werr.(*terrors.Error)
				terr.Code = terrors.ErrBadRequest
				return nil, terr
			}
		}
	}

	return e.Handler(req)
}
