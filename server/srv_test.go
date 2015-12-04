package server

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/suite"

	"github.com/mondough/mercury"
	"github.com/mondough/mercury/marshaling"
	"github.com/mondough/mercury/testproto"
	"github.com/mondough/mercury/transport"
	"github.com/mondough/terrors"
	pe "github.com/mondough/terrors/proto"
	tmsg "github.com/mondough/typhon/message"
	"github.com/mondough/typhon/mock"
	"github.com/mondough/typhon/rabbit"
)

const testServiceName = "service.server-example"

func TestServerSuite_MockTransport(t *testing.T) {
	suite.Run(t, &serverSuite{
		TransF: func() transport.Transport {
			return mock.NewTransport()
		},
	})
}

func TestServerSuite_RabbitTransport(t *testing.T) {
	suite.Run(t, &serverSuite{
		TransF: func() transport.Transport {
			return rabbit.NewTransport()
		},
	})
}

type serverSuite struct {
	suite.Suite
	TransF func() transport.Transport
	trans  transport.Transport
	server Server
}

func (suite *serverSuite) SetupSuite() {
	// Share a single transport between all tests (which each use a different server). This deliberately tests the
	// underlying transport's ability to connect and disconnect a service while leaving the connection open.
	suite.trans = suite.TransF()
	select {
	case <-suite.trans.Ready():
	case <-time.After(5 * time.Second):
		panic("transport not ready")
	}
}

func (suite *serverSuite) SetupTest() {
	suite.server = NewServer(testServiceName)
	suite.server.Start(suite.trans)
}

func (suite *serverSuite) TearDownTest() {
	suite.server.Stop()
	suite.server = nil
}

func (suite *serverSuite) TearDownSuite() {
	suite.trans.Tomb().Killf("Test ending")
	suite.trans.Tomb().Wait()
	suite.trans = nil
}

// TestRouting verifies a registered endpoint receives messages destined for it, and that responses are sent
// appropriately
func (suite *serverSuite) TestRouting() {
	srv := suite.server
	srv.AddEndpoints(Endpoint{
		Name:     "dummy",
		Request:  new(testproto.DummyRequest),
		Response: new(testproto.DummyResponse),
		Handler: func(req mercury.Request) (mercury.Response, error) {
			request := req.Body().(*testproto.DummyRequest)
			rsp := req.Response(&testproto.DummyResponse{
				Pong: request.Ping,
			})
			rsp.SetHeader("X-Ping-Pong", request.Ping)
			return rsp, nil
		}})

	req := mercury.NewRequest()
	req.SetService(testServiceName)
	req.SetEndpoint("dummy")
	req.SetBody(&testproto.DummyRequest{
		Ping: "routing"})
	suite.Assert().NoError(tmsg.ProtoMarshaler().MarshalBody(req))

	rsp, err := suite.trans.Send(req, time.Second)
	suite.Require().NoError(err)
	suite.Require().NotNil(rsp)

	suite.Require().NoError(tmsg.ProtoUnmarshaler(new(testproto.DummyResponse)).UnmarshalPayload(rsp))
	suite.Require().NotNil(rsp.Body())
	suite.Require().IsType(new(testproto.DummyResponse), rsp.Body())
	response := rsp.Body().(*testproto.DummyResponse)
	suite.Assert().Equal("routing", response.Pong)
	suite.Assert().Equal("routing", rsp.Headers()["X-Ping-Pong"])
}

// TestErrorResponse tests that errors are serialised and returned to callers appropriately (as we are using the
// transport directly here, we actually get a response containing an error, not a transport error)
func (suite *serverSuite) TestErrorResponse() {
	srv := suite.server
	srv.AddEndpoints(Endpoint{
		Name:     "err",
		Request:  new(testproto.DummyRequest),
		Response: new(testproto.DummyResponse),
		Handler: func(req mercury.Request) (mercury.Response, error) {
			request := req.Body().(*testproto.DummyRequest)
			return nil, terrors.NotFound("", request.Ping, nil)
		}})

	req := mercury.NewRequest()
	req.SetService(testServiceName)
	req.SetEndpoint("err")
	req.SetBody(&testproto.DummyRequest{
		Ping: "Foo bar baz"})
	suite.Assert().NoError(tmsg.ProtoMarshaler().MarshalBody(req))

	rsp_, err := suite.trans.Send(req, time.Second)
	suite.Assert().NoError(err)
	suite.Assert().NotNil(rsp_)
	rsp := mercury.FromTyphonResponse(rsp_)
	suite.Assert().True(rsp.IsError())

	errResponse := &pe.Error{}
	suite.Assert().NoError(proto.Unmarshal(rsp.Payload(), errResponse))
	terr := terrors.Unmarshal(errResponse)
	suite.Require().NotNil(terr)
	suite.Assert().Equal("Foo bar baz", terr.Message, string(rsp.Payload()))
	suite.Assert().Equal(terrors.ErrNotFound, terr.Code)
}

// TestPanicResponse tests that panics result in an appropriate internal service error with the panic stack in the
// error context.
func (suite *serverSuite) TestPanicResponse() {
	srv := suite.server
	srv.AddEndpoints(Endpoint{
		Name:     "panic",
		Request:  new(testproto.DummyRequest),
		Response: new(testproto.DummyResponse),
		Handler: func(req mercury.Request) (mercury.Response, error) {
			panic("FOOBARBAZ")
		}})

	req := mercury.NewRequest()
	req.SetService(testServiceName)
	req.SetEndpoint("panic")
	req.SetBody(&testproto.DummyRequest{})
	suite.Assert().NoError(tmsg.ProtoMarshaler().MarshalBody(req))

	rsp_, err := suite.trans.Send(req, time.Second)
	suite.Require().NoError(err)
	suite.Require().NotNil(rsp_)
	rsp := mercury.FromTyphonResponse(rsp_)
	suite.Assert().True(rsp.IsError())

	errResponse := &pe.Error{}
	suite.Assert().NoError(proto.Unmarshal(rsp.Payload(), errResponse))
	terr := terrors.Unmarshal(errResponse)
	suite.Require().NotNil(terr)
	suite.Assert().Equal(fmt.Sprintf("%s.panic", terrors.ErrInternalService), terr.Code)
	suite.Assert().Equal(terr.Params["err"], "FOOBARBAZ")
	suite.Assert().Contains(terr.Params["stack"], "goroutine") // All traces contain "goroutine x:" lines
}

// TestNilResponse tests that a nil response correctly returns a Response with an empty payload to the caller
func (suite *serverSuite) TestNilResponse() {
	srv := suite.server
	srv.AddEndpoints(Endpoint{
		Name:     "nil",
		Request:  new(testproto.DummyRequest),
		Response: new(testproto.DummyResponse),
		Handler: func(req mercury.Request) (mercury.Response, error) {
			return nil, nil
		}})

	req := mercury.NewRequest()
	req.SetService(testServiceName)
	req.SetBody(&testproto.DummyRequest{})
	suite.Assert().NoError(tmsg.ProtoMarshaler().MarshalBody(req))
	req.SetEndpoint("nil")

	rsp, err := suite.trans.Send(req, time.Second)
	suite.Require().NoError(err)
	suite.Require().NotNil(rsp)
	suite.Assert().Len(rsp.Payload(), 0)
}

// TestEndpointNotFound tests that a Bad Request error is correctly returned on receiving an event for an unknown
// endpoing
func (suite *serverSuite) TestEndpointNotFound() {
	req := mercury.NewRequest()
	req.SetService(testServiceName)
	req.SetEndpoint("dummy")
	req.SetBody(&testproto.DummyRequest{
		Ping: "routing"})
	suite.Assert().NoError(tmsg.ProtoMarshaler().MarshalBody(req))

	rsp_, err := suite.trans.Send(req, time.Second)
	rsp := mercury.FromTyphonResponse(rsp_)
	suite.Require().NoError(err)
	suite.Require().NotNil(rsp)
	suite.Assert().True(rsp.IsError())

	suite.Assert().NoError(tmsg.ProtoUnmarshaler(new(pe.Error)).UnmarshalPayload(rsp))
	suite.Assert().IsType(new(pe.Error), rsp.Body())
	terr := terrors.Unmarshal(rsp.Body().(*pe.Error))
	suite.Assert().Equal(terrors.ErrBadRequest+".endpoint_not_found", terr.Code)
	suite.Assert().Contains(terr.Error(), "Endpoint not found")
}

// TestRegisteringInvalidEndpoint tests that appropriate panics are raised when registering invalid Endpoints
func (suite *serverSuite) TestRegisteringInvalidEndpoint() {
	suite.T().Skip("Not implemented") // @TODO
}

// TestRoutingParallel sends a bunch of requests in parallel to different endpoints and checks that the responses match
// correctly. 200 workers, 100 requests each.
func (suite *serverSuite) TestRoutingParallel() {
	if testing.Short() {
		suite.T().Skip("Skipping TestRoutingParallel in short mode")
	}

	names := [...]string{"1", "2", "3"}
	srv := suite.server
	srv.AddEndpoints(
		Endpoint{
			Name:     names[0],
			Request:  new(testproto.DummyRequest),
			Response: new(testproto.DummyResponse),
			Handler: func(req mercury.Request) (mercury.Response, error) {
				return req.Response(&testproto.DummyResponse{Pong: names[0]}), nil
			}},
		Endpoint{
			Name:     names[1],
			Request:  new(testproto.DummyRequest),
			Response: new(testproto.DummyResponse),
			Handler: func(req mercury.Request) (mercury.Response, error) {
				return req.Response(&testproto.DummyResponse{Pong: names[1]}), nil
			}},
		Endpoint{
			Name:     names[2],
			Request:  new(testproto.DummyRequest),
			Response: new(testproto.DummyResponse),
			Handler: func(req mercury.Request) (mercury.Response, error) {
				return req.Response(&testproto.DummyResponse{Pong: names[2]}), nil
			}})

	workers := 200
	wg := sync.WaitGroup{}
	wg.Add(workers)
	unmarshaler := tmsg.ProtoUnmarshaler(new(testproto.DummyResponse))
	work := func(i int) {
		defer wg.Done()
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		ep := names[rng.Int()%len(names)]
		for i := 0; i < 100; i++ {
			req := mercury.NewRequest()
			req.SetService(testServiceName)
			req.SetEndpoint(ep)
			req.SetBody(&testproto.DummyRequest{})
			suite.Assert().NoError(tmsg.ProtoMarshaler().MarshalBody(req))

			rsp, err := suite.trans.Send(req, time.Second)
			suite.Require().NoError(err)
			suite.Require().NotNil(rsp)
			suite.Require().NoError(unmarshaler.UnmarshalPayload(rsp))
			response := rsp.Body().(*testproto.DummyResponse)
			suite.Assert().Equal(ep, response.Pong)
		}
	}

	for i := 0; i < workers; i++ {
		go work(i)
	}
	wg.Wait()
}

// TestJSONResponse verifies that a request sent with JSON content and an accepting JSON response, does in fact yield a
// JSON response (from a proto handler)
func (suite *serverSuite) TestJSONResponse() {
	srv := suite.server
	srv.AddEndpoints(Endpoint{
		Name:     "dummy",
		Request:  new(testproto.DummyRequest),
		Response: new(testproto.DummyResponse),
		Handler: func(req mercury.Request) (mercury.Response, error) {
			request := req.Body().(*testproto.DummyRequest)
			return req.Response(&testproto.DummyResponse{
				Pong: request.Ping,
			}), nil
		}})

	req := mercury.NewRequest()
	req.SetService(testServiceName)
	req.SetEndpoint("dummy")
	req.SetBody(map[string]string{
		"ping": "json"})
	req.SetHeader(marshaling.AcceptHeader, "application/json")
	suite.Assert().NoError(tmsg.JSONMarshaler().MarshalBody(req))

	rsp, err := suite.trans.Send(req, time.Second)
	suite.Require().NoError(err)
	suite.Require().NotNil(rsp)

	suite.Assert().Equal("application/json", rsp.Headers()[marshaling.ContentTypeHeader])
	suite.Assert().Equal(`{"pong":"json"}`, string(rsp.Payload()))
}
