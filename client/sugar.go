package client

import (
	"golang.org/x/net/context"
)

// Req sends a synchronous request to a service using a new client, and unmarshals the response into the supplied
// protobuf
func Req(ctx context.Context, service, endpoint string, req, res interface{}) error {
	return NewClient().
		Add(Call{
			Uid:      "1",
			Service:  service,
			Endpoint: endpoint,
			Body:     req,
			Response: res,
			Context:  ctx,
		}).
		Execute().
		Errors().
		Combined()
}
