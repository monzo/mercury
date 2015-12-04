package client

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/mondough/mercury"
	"github.com/mondough/mercury/marshaling"
	"github.com/mondough/mercury/testproto"
	"github.com/mondough/mercury/transport"
	"github.com/mondough/terrors"
	tmsg "github.com/mondough/typhon/message"
	"github.com/mondough/typhon/mock"
	"github.com/mondough/typhon/rabbit"
)

const testServiceName = "service.client-example"

func TestClientSuite_MockTransport(t *testing.T) {
	suite.Run(t, &clientSuite{
		TransF: func() transport.Transport {
			return mock.NewTransport()
		},
	})
}

func TestClientSuite_RabbitTransport(t *testing.T) {
	suite.Run(t, &clientSuite{
		TransF: func() transport.Transport {
			return rabbit.NewTransport()
		},
	})
}

type clientSuite struct {
	suite.Suite
	TransF func() transport.Transport
	trans  transport.Transport
}

func (suite *clientSuite) SetupSuite() {
	trans := suite.TransF()
	select {
	case <-trans.Ready():
	case <-time.After(2 * time.Second):
		panic("transport not ready")
	}
	suite.trans = trans

	// Add a listener that responds blindly to all messages
	inboundChan := make(chan tmsg.Request, 10)
	trans.Listen(testServiceName, inboundChan)
	go func() {
		for {
			select {
			case _req := <-inboundChan:
				req := mercury.FromTyphonRequest(_req)
				switch req.Endpoint() {
				case "timeout":
					continue

				case "invalid-payload":
					// Wrong proto here
					rsp := req.Response(nil)
					rsp.SetPayload([]byte("†HÎß ßHøÜ¬∂ÑT ∑ø®K"))
					suite.Require().NoError(trans.Respond(req, rsp))

				case "error":
					err := terrors.BadRequest("", "foo bar", nil)
					rsp := req.Response(terrors.Marshal(err))
					rsp.SetHeaders(req.Headers())
					rsp.SetIsError(true)
					suite.Require().NoError(trans.Respond(req, rsp))

				case "bulls--t":
					rsp := req.Response(map[string]string{})
					rsp.SetHeaders(req.Headers())
					rsp.SetHeader(marshaling.ContentTypeHeader, "application/bulls--t")
					suite.Require().NoError(trans.Respond(req, rsp))

				default:
					rsp := req.Response(&testproto.DummyResponse{
						Pong: "Pong"})
					rsp.SetHeaders(req.Headers())
					suite.Require().NoError(tmsg.ProtoMarshaler().MarshalBody(rsp))
					suite.Require().NoError(trans.Respond(req, rsp))
				}

			case <-trans.Tomb().Dying():
				return
			}
		}
	}()
}

func (suite *clientSuite) TearDownSuite() {
	trans := suite.trans
	trans.Tomb().Killf("Test ending")
	trans.Tomb().Wait()
	suite.trans = nil
}

// TestExecuting tests an end-to-end flow of one request
func (suite *clientSuite) TestExecuting() {
	response := new(testproto.DummyResponse)
	client := NewClient().Add(Call{
		Uid:      "call1",
		Service:  testServiceName,
		Endpoint: "foo",
		Response: response,
	}).SetTransport(suite.trans).Execute()

	rsp := client.Response("call1")

	suite.Assert().Empty(client.Errors())
	suite.Require().NotNil(rsp)
	suite.Assert().Equal("Pong", response.Pong)
	suite.Assert().Equal(response, rsp.Body())
	suite.Assert().Equal("Pong", rsp.Body().(*testproto.DummyResponse).Pong)
}

// TestTimeout verifies the timeout functionality of the client behaves as expected (especially with multiple calls,
// some of which succeed and some of which fail).
func (suite *clientSuite) TestTimeout() {
	client := NewClient().Add(Call{
		Uid:      "call1",
		Service:  testServiceName,
		Endpoint: "timeout",
		Response: new(testproto.DummyResponse),
	}).SetTransport(suite.trans).SetTimeout(time.Second).Go()

	select {
	case <-client.WaitC():
	case <-time.After(time.Second + 50*time.Millisecond):
		suite.Fail("Should have timed out")
	}

	suite.Assert().Len(client.Errors(), 1)
	err := client.Errors().ForUid("call1")
	suite.Assert().Error(err)
	suite.Assert().Equal(terrors.ErrTimeout, err.Code, err.Message)
}

// TestRawRequest verifies that adding raw requests (rather than Calls) works as expected.

// TestResponseUnmarshalingError verifies that unmarshaling errors are handled appropriately (in this case by expecting
// a different response protocol to what is received).
//
// This also conveniently verifies that Clients use custom transports appropriately.
func (suite *clientSuite) TestResponseUnmarshalingError() {
	client := NewClient().Add(Call{
		Uid:      "call1",
		Service:  testServiceName,
		Endpoint: "invalid-payload",
		Response: new(testproto.DummyResponse),
	}).
		SetTimeout(time.Second).
		SetTransport(suite.trans).
		Execute()

	suite.Assert().Len(client.Errors(), 1)
	err := client.Errors().ForUid("call1")
	suite.Assert().Equal(terrors.ErrBadResponse, err.Code)

	rsp := client.Response("call1")
	suite.Require().NotNil(rsp)
	response := rsp.Body().(*testproto.DummyResponse)
	suite.Assert().Equal("", response.Pong)
}

type testMw struct {
	err *terrors.Error
}

func (m *testMw) ProcessClientRequest(req mercury.Request) mercury.Request {
	req.SetHeader("X-Foo", "X-Bar")
	return req
}

func (m *testMw) ProcessClientResponse(rsp mercury.Response, req mercury.Request) mercury.Response {
	rsp.SetHeader("X-Boop", "Boop")
	return rsp
}

func (m *testMw) ProcessClientError(err *terrors.Error, req mercury.Request) {
	m.err = err
}

// TestMiddleware verifies client middleware methods are executed as expected
func (suite *clientSuite) TestMiddleware() {
	mw := &testMw{}
	client := NewClient().
		AddMiddleware(mw).
		Add(
		Call{
			Uid:      "call1",
			Service:  testServiceName,
			Endpoint: "ping",
			Response: new(testproto.DummyResponse),
		}).
		SetTimeout(time.Second).
		SetTransport(suite.trans).
		Execute()

	suite.Assert().Empty(client.Errors())
	rsp := client.Response("call1")
	suite.Require().NotNil(rsp)
	// ProcessClientRequest should have set X-Foo: Bar (and ping echoes the headers)
	suite.Assert().Equal("X-Bar", rsp.Headers()["X-Foo"])
	// ProcessClientResponse should have set X-Boop: Boop
	suite.Assert().Equal("Boop", rsp.Headers()["X-Boop"])
	suite.Assert().Nil(mw.err)
	client = NewClient().
		AddMiddleware(mw).
		Add(
		Call{
			Uid:      "call1",
			Service:  testServiceName,
			Endpoint: "error",
			Response: new(testproto.DummyResponse),
		}).
		SetTimeout(time.Second).
		SetTransport(suite.trans).
		Execute()

	rsp = client.Response("call1")
	suite.Require().NotNil(rsp)
	suite.Assert().Len(client.Errors(), 1)
	err := client.Errors().ForUid("call1")
	suite.Require().Error(err)
	// ProcessClientError should have stored the error
	suite.Assert().Equal(err, mw.err)
	// ProcessClientRequest should have set X-Foo: Bar (and ping echoes the headers)
	suite.Assert().Equal("X-Bar", rsp.Headers()["X-Foo"])
	// ProcessClientResponse should not have run
	suite.Assert().Empty(rsp.Headers()["X-Boop"])
}

// TestParallelCalls verifies that many calls made in parallel are routed correctly, and their responses/errors are
// available in the proper places.
func (suite *clientSuite) TestParallelCalls() {
	client := NewClient().
		SetTimeout(5 * time.Second).
		SetTransport(suite.trans)

	for i := 0; i < 100; i++ {
		uid := fmt.Sprintf("call%d", i)
		client = client.Add(Call{
			Uid:      uid,
			Service:  testServiceName,
			Endpoint: "foo",
			Response: new(testproto.DummyResponse),
			Headers: map[string]string{
				"Iteration": uid}})
	}

	client.Execute()
	suite.Require().Empty(client.Errors())

	for i := 0; i < 100; i++ {
		uid := fmt.Sprintf("call%d", i)
		rsp := client.Response(uid)
		suite.Assert().Equal(uid, rsp.Headers()["Iteration"])
	}
}

type bsMarshaler struct{}

func (m bsMarshaler) MarshalBody(msg tmsg.Message) error {
	msg.SetPayload([]byte("total garbage"))
	return nil
}

func (m bsMarshaler) UnmarshalPayload(msg tmsg.Message) error {
	msg.SetBody(map[string]string{
		"1": "2",
	})
	return nil
}

// TestCustomMarshaler registers a custom marshaler and then checks a request can be made using it
func (suite *clientSuite) TestCustomMarshaler() {
	marshaling.Register(
		"application/bulls--t",
		func() tmsg.Marshaler { return bsMarshaler{} },
		func(_ interface{}) tmsg.Unmarshaler { return bsMarshaler{} },
	)

	cl := NewClient().
		Add(Call{
		Uid:      "foo",
		Service:  testServiceName,
		Endpoint: "bulls--t",
		Body:     map[string]string{},
		Response: map[string]string{},
		Headers: map[string]string{
			marshaling.ContentTypeHeader: "application/bulls--t",
			marshaling.AcceptHeader:      "application/bulls--t"}}).
		SetTransport(suite.trans).
		SetTimeout(time.Second)

	suite.Require().NoError(cl.Execute().Errors().Combined())
	rsp := cl.Response("foo")
	suite.Require().NotNil(rsp)
	suite.Require().IsType(map[string]string{}, rsp.Body())
	suite.Require().Equal(map[string]string{
		"1": "2",
	}, rsp.Body().(map[string]string))
}

type invalidBodyT struct{}

// TestInvalidBody verifies that an incorrect type passed as the `Body` returns a "bad request" error
func (suite *clientSuite) TestInvalidBody() {
	cl := NewClient().Add(Call{
		Uid:      "call",
		Service:  "notathing", // We would get a timeout if the service *did* exist
		Endpoint: "reallynotathing",
		Body:     invalidBodyT{},
		Response: &testproto.DummyResponse{},
	}).
		SetTransport(suite.trans)

	err := cl.Execute().Errors().ForUid("call")
	suite.Require().Error(err)
	suite.Assert().Equal(terrors.ErrBadRequest, err.Code)
}

// TestEmpty verifies that an empty call-set results in no errors
func (suite *clientSuite) TestEmpty() {
	cl := NewClient()
	suite.Require().Empty(cl.Execute().Errors())
}
