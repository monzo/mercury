package mercury

import (
	"sync"
	"time"

	log "github.com/mondough/slog"
	tmsg "github.com/mondough/typhon/message"
	"golang.org/x/net/context"

	"github.com/mondough/mercury/marshaling"
)

const (
	errHeader = "Content-Error"
)

// A Request is a representation of an RPC call (inbound or outbound). It extends Typhon's Request to provide a
// Context, and also helpers for constructing a response.
type Request interface {
	tmsg.Request
	context.Context

	// Response constructs a response to this request, with the (optional) given body. The response will share
	// the request's ID, and be destined for the originator.
	Response(body interface{}) Response
	// A Context for the Request.
	Context() context.Context
	// SetContext replaces the Request's Context.
	SetContext(ctx context.Context)
}

func responseFromRequest(req Request, body interface{}) Response {
	rsp := NewResponse()
	rsp.SetId(req.Id())
	if body != nil {
		rsp.SetBody(body)

		ct := req.Headers()[marshaling.AcceptHeader]
		marshaler := marshaling.Marshaler(ct)
		if marshaler == nil { // Fall back to proto
			marshaler = marshaling.Marshaler(marshaling.ProtoContentType)
		}
		if marshaler == nil {
			log.Error(req, "[Mercury] No marshaler for response %s: %s", rsp.Id(), ct)
		} else if err := marshaler.MarshalBody(rsp); err != nil {
			log.Error(req, "[Mercury] Failed to marshal response %s: %v", rsp.Id(), err)
		}
	}
	return rsp
}

type request struct {
	sync.RWMutex
	tmsg.Request
	ctx context.Context
}

func (r *request) Response(body interface{}) Response {
	return responseFromRequest(r, body)
}

func (r *request) Context() context.Context {
	if r == nil {
		return nil
	}
	r.RLock()
	defer r.RUnlock()
	return r.ctx
}

func (r *request) SetContext(ctx context.Context) {
	r.Lock()
	defer r.Unlock()
	r.ctx = ctx
}

func (r *request) Copy() tmsg.Request {
	r.RLock()
	defer r.RUnlock()
	return &request{
		Request: r.Request.Copy(),
		ctx:     r.ctx,
	}
}

// Context implementation

func (r *request) Deadline() (time.Time, bool) {
	return r.Context().Deadline()
}

func (r *request) Done() <-chan struct{} {
	return r.Context().Done()
}

func (r *request) Err() error {
	return r.Context().Err()
}

func (r *request) Value(key interface{}) interface{} {
	return r.Context().Value(key)
}

func NewRequest() Request {
	return FromTyphonRequest(tmsg.NewRequest())
}

func FromTyphonRequest(req tmsg.Request) Request {
	return &request{
		Request: req,
		ctx:     context.Background(),
	}
}
