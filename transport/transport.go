package transport

import (
	"sync"

	ttrans "github.com/monzo/typhon/transport"
)

type Transport ttrans.Transport

var (
	defaultTransport  Transport
	defaultTransportM sync.RWMutex
)

// DefaultTransport returns the global default transport, over which servers and clients should run by default
func DefaultTransport() Transport {
	defaultTransportM.RLock()
	defer defaultTransportM.RUnlock()
	return defaultTransport
}

// SetDefaultTransport replaces the global default transport. When replacing, it does not close the prior transport.
func SetDefaultTransport(t Transport) {
	defaultTransportM.Lock()
	defer defaultTransportM.Unlock()
	defaultTransport = t
}
