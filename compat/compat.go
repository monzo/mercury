package mercurycompat

import (
	"fmt"
	"strings"

	"github.com/mondough/mercury/server"
	"github.com/mondough/typhon"
)

func CompatServer(srv server.Server) typhon.Filter {
	return func(req typhon.Request, svc typhon.Service) typhon.Response {
		eps := []string{
			fmt.Sprintf("%s %s", req.Method, req.URL.Path),
			fmt.Sprintf("%s %s", req.Method, strings.TrimPrefix(req.URL.Path, "/"))}
		if req.Method == "POST" {
			eps = append(eps, fmt.Sprintf("%s", strings.TrimPrefix(req.URL.Path, "/")))
		}

		for _, epName := range eps {
			ep, ok := srv.Endpoint(epName)
			if ok {
				oldRsp, err := ep.Handle(new2OldRequest(req))
				if err != nil {
					return typhon.Response{
						Error: err}
				}
				return old2NewResponse(req, oldRsp)
			}
		}

		// No matching endpoint found; send it to the lower-level service
		return svc(req)
	}
}
