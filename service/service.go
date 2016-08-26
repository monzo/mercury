package service

import (
	"sync"

	"github.com/monzo/mercury/client"
	"github.com/monzo/mercury/requesttree"
	"github.com/monzo/mercury/server"
	"github.com/monzo/mercury/transport"
)

var (
	defaultService  Service
	defaultServiceM sync.RWMutex
)

func init() {
	client.SetDefaultMiddleware(DefaultClientMiddleware())
	server.SetDefaultMiddleware(DefaultServerMiddleware())
}

type Service interface {
	Server() server.Server
	Run()
	Transport() transport.Transport
}

type svc struct {
	srv    server.Server
	config Config
}

func (s *svc) Server() server.Server {
	return s.srv
}

func (s *svc) Run() {
	s.srv.Run(s.config.Transport)
}

func (s *svc) Transport() transport.Transport {
	return s.config.Transport
}

// DefaultServerMiddleware returns the complement of server middleware provided by Mercury
func DefaultServerMiddleware() []server.ServerMiddleware {
	return []server.ServerMiddleware{
		requesttree.Middleware()}
}

// DefaultClientMiddleware returns the complement of client middleware provided by Mercury
func DefaultClientMiddleware() []client.ClientMiddleware {
	return []client.ClientMiddleware{
		requesttree.Middleware()}
}

// DefaultService returns the global default Service.
func DefaultService() Service {
	defaultServiceM.RLock()
	defer defaultServiceM.RUnlock()
	return defaultService
}

// New creates a new service with default middleware
func New(cfg Config) Service {
	if cfg.Transport == nil {
		cfg.Transport = transport.DefaultTransport()
	}

	srv := server.NewServer(cfg.Name)
	srv.SetMiddleware(DefaultServerMiddleware())

	return &svc{
		srv:    srv,
		config: cfg,
	}
}

// Init performs any global initialisation that is usually required for Mercury services. Namely it:
//
// * Sets up a server with middleware (request tree)
// * Sets the created service as the default service
func Init(cfg Config) Service {
	impl := New(cfg)

	defaultServiceM.Lock()
	defaultService = impl
	defaultServiceM.Unlock()

	<-impl.Transport().Ready()

	return impl
}
