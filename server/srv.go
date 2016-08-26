package server

import (
	"fmt"
	"strings"
	"sync"
	"time"

	log "github.com/monzo/slog"
	"github.com/monzo/terrors"
	tmsg "github.com/monzo/typhon/message"
	ttrans "github.com/monzo/typhon/transport"
	"golang.org/x/net/context"
	"gopkg.in/tomb.v2"

	"github.com/monzo/mercury"
	"github.com/monzo/mercury/transport"
)

const (
	connectTimeout = 30 * time.Second
)

var (
	ErrAlreadyRunning   error = terrors.InternalService("", "Server is already running", nil) // empty dotted code so impl details don't leak outside
	ErrTransportClosed  error = terrors.InternalService("", "Transport closed", nil)
	errEndpointNotFound       = terrors.BadRequest("endpoint_not_found", "Endpoint not found", nil)
	defaultMiddleware   []ServerMiddleware
	defaultMiddlewareM  sync.RWMutex
)

func NewServer(name string) Server {
	defaultMiddlewareM.RLock()
	middleware := defaultMiddleware
	defaultMiddlewareM.RUnlock()

	return &server{
		name:       name,
		middleware: middleware,
	}
}

func SetDefaultMiddleware(middleware []ServerMiddleware) {
	defaultMiddlewareM.Lock()
	defer defaultMiddlewareM.Unlock()
	defaultMiddleware = middleware
}

type server struct {
	name        string              // server name (registered with the transport; immutable)
	endpoints   map[string]Endpoint // endpoint name: Endpoint
	endpointsM  sync.RWMutex        // protects endpoints
	workerTomb  *tomb.Tomb          // runs as long as there is a worker consuming Requests
	workerTombM sync.RWMutex        // protects workerTomb
	middleware  []ServerMiddleware  // applied in-order for requests, reverse-order for responses
	middlewareM sync.RWMutex        // protects middleware
}

func (s *server) Name() string {
	return s.name
}

func (s *server) AddEndpoints(eps ...Endpoint) {
	// Check the endpoint is valid (panic if not)
	for _, ep := range eps {
		if ep.Handler == nil {
			panic(fmt.Sprintf("Endpoint %s has no handler function", ep.Name))
		}
	}

	s.endpointsM.Lock()
	defer s.endpointsM.Unlock()
	if s.endpoints == nil {
		s.endpoints = make(map[string]Endpoint, len(eps))
	}
	for _, e := range eps {
		// if e.Request == nil || e.Response == nil {
		// 	panic(fmt.Sprintf("Endpoint \"%s\" must have Request and Response defined", e.Name))
		// }
		s.endpoints[e.Name] = e
	}
}

func (s *server) RemoveEndpoints(eps ...Endpoint) {
	s.endpointsM.Lock()
	defer s.endpointsM.Unlock()
	for _, e := range eps {
		delete(s.endpoints, e.Name)
	}
}
func (s *server) Endpoints() []Endpoint {
	s.endpointsM.RLock()
	defer s.endpointsM.RUnlock()
	result := make([]Endpoint, 0, len(s.endpoints))
	for _, ep := range s.endpoints {
		result = append(result, ep)
	}
	return result
}

func (s *server) Endpoint(path string) (Endpoint, bool) {
	s.endpointsM.RLock()
	defer s.endpointsM.RUnlock()
	ep, ok := s.endpoints[path]
	if !ok && strings.HasPrefix(path, "/") { // Try looking for a "legacy" match without the leading slash
		ep, ok = s.endpoints[strings.TrimPrefix(path, "/")]
	}
	return ep, ok
}

func (s *server) start(trans transport.Transport) (*tomb.Tomb, error) {
	ctx := context.Background()

	s.workerTombM.Lock()
	if s.workerTomb != nil {
		s.workerTombM.Unlock()
		return nil, ErrAlreadyRunning
	}
	tm := new(tomb.Tomb)
	s.workerTomb = tm
	s.workerTombM.Unlock()

	stop := func() {
		trans.StopListening(s.Name())
		s.workerTombM.Lock()
		s.workerTomb = nil
		s.workerTombM.Unlock()
	}

	var inbound chan tmsg.Request
	connect := func() error {
		select {
		case <-trans.Ready():
			inbound = make(chan tmsg.Request, 500)
			return trans.Listen(s.Name(), inbound)

		case <-time.After(connectTimeout):
			log.Warn(ctx, "[Mercury:Server] Timed out after %v waiting for transport readiness", connectTimeout)
			return ttrans.ErrTimeout
		}
	}

	// Block here purposefully (deliberately not in the goroutine below, because we want to report a connection error
	// to the caller)
	if err := connect(); err != nil {
		stop()
		return nil, err
	}

	tm.Go(func() error {
		defer stop()
		for {
			select {
			case req, ok := <-inbound:
				if !ok {
					// Received because the channel closed; try to reconnect
					log.Warn(ctx, "[Mercury:Server] Inbound channel closed; trying to reconnect…")
					if err := connect(); err != nil {
						log.Critical(ctx, "[Mercury:Server] Could not reconnect after channel close: %s", err)
						return err
					}
				} else {
					go s.handle(trans, req)
				}

			case <-tm.Dying():
				return tomb.ErrDying
			}
		}
	})
	return tm, nil
}

func (s *server) Start(trans transport.Transport) error {
	_, err := s.start(trans)
	return err
}

func (s *server) Run(trans transport.Transport) {
	if tm, err := s.start(trans); err != nil || tm == nil {
		panic(err)
	} else if err := tm.Wait(); err != nil {
		panic(err)
	}
}

func (s *server) Stop() {
	s.workerTombM.RLock()
	tm := s.workerTomb
	s.workerTombM.RUnlock()
	if tm != nil {
		tm.Killf("Stop() called")
		tm.Wait()
	}
}

func (s *server) applyRequestMiddleware(req mercury.Request) (mercury.Request, mercury.Response) {
	s.middlewareM.RLock()
	mws := s.middleware
	s.middlewareM.RUnlock()
	for _, mw := range mws {
		if req_, rsp := mw.ProcessServerRequest(req); rsp != nil {
			return req_, rsp
		} else {
			req = req_
		}
	}
	return req, nil
}

func (s *server) applyResponseMiddleware(rsp mercury.Response, req mercury.Request) mercury.Response {
	s.middlewareM.RLock()
	mws := s.middleware
	s.middlewareM.RUnlock()
	for i := len(mws) - 1; i >= 0; i-- { // reverse order
		mw := mws[i]
		rsp = mw.ProcessServerResponse(rsp, req)
	}
	return rsp
}

func (s *server) handle(trans transport.Transport, req_ tmsg.Request) {
	req := mercury.FromTyphonRequest(req_)
	req, rsp := s.applyRequestMiddleware(req)

	if rsp == nil {
		if ep, ok := s.Endpoint(req.Endpoint()); !ok {
			log.Warn(req, "[Mercury:Server] Received request %s for unknown endpoint %s", req.Id(), req.Endpoint())
			rsp = ErrorResponse(req, errEndpointNotFound)
		} else {
			if rsp_, err := ep.Handle(req); err != nil {
				rsp = ErrorResponse(req, err)
				log.Info(req, "[Mercury:Server] Error from endpoint %s for %v: %v", ep.Name, req, err, map[string]string{
					"request_payload": string(req.Payload())})
			} else if rsp_ == nil {
				rsp = req.Response(nil)
			} else {
				rsp = rsp_
			}
		}
	}
	rsp = s.applyResponseMiddleware(rsp, req)
	if rsp != nil {
		trans.Respond(req, rsp)
	}
}

func (s *server) Middleware() []ServerMiddleware {
	// Note that no operation exists that mutates a particular element; this is very deliberate and means we do not
	// need to hold a read lock when iterating over the middleware slice, only when getting a reference to the slice.
	s.middlewareM.RLock()
	mws := s.middleware
	s.middlewareM.RUnlock()
	result := make([]ServerMiddleware, len(mws))
	copy(result, mws)
	return result
}

func (s *server) SetMiddleware(mws []ServerMiddleware) {
	s.middlewareM.Lock()
	defer s.middlewareM.Unlock()
	s.middleware = mws
}

func (s *server) AddMiddleware(mw ServerMiddleware) {
	s.middlewareM.Lock()
	defer s.middlewareM.Unlock()
	s.middleware = append(s.middleware, mw)
}

// ErrorResponse constructs a response for the given request, with the given error as its contents. Mercury clients
// know how to unmarshal these errors.
func ErrorResponse(req mercury.Request, err error) mercury.Response {
	rsp := req.Response(nil)
	var terr *terrors.Error
	if err != nil {
		terr = terrors.Wrap(err, nil).(*terrors.Error)
	}
	rsp.SetBody(terrors.Marshal(terr))
	if err := tmsg.JSONMarshaler().MarshalBody(rsp); err != nil {
		log.Error(req, "[Mercury:Server] Failed to marshal error response: %v", err)
		return nil // Not much we can do here
	}
	rsp.SetIsError(true)
	return rsp
}
