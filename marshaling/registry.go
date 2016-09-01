package marshaling

import (
	"sync"

	tmsg "github.com/monzo/typhon/message"
)

const (
	ContentTypeHeader = "Content-Type"
	AcceptHeader      = "Accept"
	JSONContentType   = tmsg.JSONContentType
)

type MarshalerFactory func() tmsg.Marshaler
type UnmarshalerFactory func(interface{}) tmsg.Unmarshaler

type marshalerPair struct {
	m MarshalerFactory
	u UnmarshalerFactory
}

var (
	marshalerRegistryM sync.RWMutex
	marshalerRegistry  = map[string]marshalerPair{
		JSONContentType: {
			m: tmsg.JSONMarshaler,
			u: tmsg.JSONUnmarshaler},
	}
)

func Register(contentType string, mc MarshalerFactory, uc UnmarshalerFactory) {
	if contentType == "" || mc == nil || uc == nil {
		return
	}

	marshalerRegistryM.Lock()
	defer marshalerRegistryM.Unlock()
	marshalerRegistry[contentType] = marshalerPair{
		m: mc,
		u: uc,
	}
}

func Marshaler(contentType string) tmsg.Marshaler {
	marshalerRegistryM.RLock()
	defer marshalerRegistryM.RUnlock()
	if mp, ok := marshalerRegistry[contentType]; ok {
		return mp.m()
	}
	return nil
}

func Unmarshaler(contentType string, protocol interface{}) tmsg.Unmarshaler {
	marshalerRegistryM.RLock()
	defer marshalerRegistryM.RUnlock()
	if mp, ok := marshalerRegistry[contentType]; ok {
		return mp.u(protocol)
	}
	return nil
}
