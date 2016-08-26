package requesttree

import (
	"testing"

	"github.com/monzo/typhon/mock"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"

	"github.com/monzo/mercury"
	"github.com/monzo/mercury/client"
	"github.com/monzo/mercury/server"
	"github.com/monzo/mercury/testproto"
	"github.com/monzo/mercury/transport"
)

const testOriginServiceName = "service.requesttree-origin"
const testServiceName = "service.requesttree-example"

func TestParentRequestIdMiddlewareSuite(t *testing.T) {
	suite.Run(t, new(parentRequestIdMiddlewareSuite))
}

type parentRequestIdMiddlewareSuite struct {
	suite.Suite
	trans transport.Transport
	srv   server.Server
}

func (suite *parentRequestIdMiddlewareSuite) SetupTest() {
	suite.trans = mock.NewTransport()
	suite.srv = server.NewServer(testServiceName)
	suite.srv.AddMiddleware(Middleware())

	suite.srv.AddEndpoints(
		server.Endpoint{
			Name:     "foo",
			Request:  &testproto.DummyRequest{},
			Response: &testproto.DummyResponse{},
			Handler: func(req mercury.Request) (mercury.Response, error) {

				// Assert first call has correct origin
				suite.Assert().Equal(testOriginServiceName, OriginServiceFor(req))
				suite.Assert().Equal("e2etest", OriginEndpointFor(req))

				// Assert first call has updated to the current service
				suite.Assert().Equal(testServiceName, CurrentServiceFor(req))
				suite.Assert().Equal("foo", CurrentEndpointFor(req))

				cl := client.NewClient().
					SetTransport(suite.trans).
					SetMiddleware([]client.ClientMiddleware{Middleware()}).
					Add(client.Call{
						Uid:      "call",
						Service:  testServiceName,
						Endpoint: "foo-2",
						Body:     &testproto.DummyRequest{},
						Response: &testproto.DummyResponse{},
						Context:  req,
					}).
					Execute()
				return cl.Response("call"), cl.Errors().Combined()
			}},
		server.Endpoint{
			Name:     "foo-2",
			Request:  &testproto.DummyRequest{},
			Response: &testproto.DummyResponse{},
			Handler: func(req mercury.Request) (mercury.Response, error) {

				// Assert origin headers were set correctly as previous service
				suite.Assert().Equal(testServiceName, OriginServiceFor(req))
				suite.Assert().Equal("foo", OriginEndpointFor(req))

				// And that our current service's headers were set
				suite.Assert().Equal(testServiceName, CurrentServiceFor(req))
				suite.Assert().Equal("foo-2", CurrentEndpointFor(req))

				return req.Response(&testproto.DummyResponse{
					Pong: ParentRequestIdFor(req)}), nil
			}})
	suite.srv.Start(suite.trans)
}

func (suite *parentRequestIdMiddlewareSuite) TearDownTest() {
	suite.srv.Stop()
	suite.srv = nil
	suite.trans.Tomb().Killf("test ending")
	suite.trans.Tomb().Wait()
	suite.trans = nil
}

// TestE2E verifies parent request IDs are properly set on child requests
func (suite *parentRequestIdMiddlewareSuite) TestE2E() {
	cli := client.
		NewClient().
		SetTransport(suite.trans).
		SetMiddleware([]client.ClientMiddleware{Middleware()})

	dummyOrigin := mercury.NewRequest()
	dummyOrigin.SetId("foobarbaz")
	ctx := context.WithValue(dummyOrigin.Context(), "Current-Service", testOriginServiceName)
	ctx = context.WithValue(ctx, "Current-Endpoint", "e2etest")
	dummyOrigin.SetContext(ctx)

	cli.Add(client.Call{
		Uid:      "call",
		Service:  testServiceName,
		Endpoint: "foo",
		Context:  dummyOrigin,
		Response: &testproto.DummyResponse{},
		Body:     &testproto.DummyRequest{}})
	cli.Execute()

	suite.Assert().NoError(cli.Errors().Combined())
	rsp := cli.Response("call")
	response := rsp.Body().(*testproto.DummyResponse)
	suite.Assert().NotEmpty(response.Pong)
	suite.Assert().Equal(response.Pong, rsp.Headers()[parentIdHeader])
}
