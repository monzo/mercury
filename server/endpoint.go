package server

import (
	"bytes"
	"fmt"
	"runtime"

	log "github.com/cihub/seelog"
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
	return marshaling.Unmarshaler(req.Headers()[marshaling.ContentTypeHeader], e.Request)
}

// Handle takes an inbound Request, unmarshals it, dispatches it to the handler, and serialises the result as a
// Response. Note that the response may be nil.
func (e Endpoint) Handle(req mercury.Request) (rsp mercury.Response, err error) {
	// Unmarshal the request body (unless there already is one)
	if req.Body() == nil && e.Request != nil {
		if um := e.unmarshaler(req); um != nil {
			if werr := terrors.Wrap(um.UnmarshalPayload(req), nil); werr != nil {
				log.Warnf("[Mercury:Server] Cannot unmarshal request payload: %v", werr)
				terr := werr.(*terrors.Error)
				terr.Code = terrors.ErrBadRequest
				rsp, err = nil, terr
				return
			}
		}
	}

	defer func() {
		if v := recover(); v != nil {
			traceVerbose := make([]byte, 8000)
			runtime.Stack(traceVerbose, true)
			traceVerbose = bytes.TrimRight(traceVerbose, "\x00") // Remove trailing nuls (runtime.Stack is derpy)
			log.Criticalf("[Mercury:Server] Recovered from handler panic for request %s: %v\n\n%s", req.Id(), v,
				string(traceVerbose))
			rsp, err = nil, terrors.InternalService(
				"panic",
				fmt.Sprintf("Panic in handler %s: %v\n\n%s", req.Endpoint(), v, string(traceVerbose)),
				map[string]string{
					"err":   fmt.Sprintf("%v", v),
					"stack": string(traceVerbose)})
		}
	}()
	rsp, err = e.Handler(req)
	return
}
