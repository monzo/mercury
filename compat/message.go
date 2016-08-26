package mercurycompat

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/monzo/mercury"
	"github.com/monzo/typhon"
)

const legacyIdHeader = "Legacy-Id"

func toHeader(m map[string]string) http.Header {
	h := make(http.Header, len(m))
	for k, v := range m {
		h.Set(k, v)
	}
	return h
}

func fromHeader(h http.Header) map[string]string {
	m := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) < 1 {
			continue
		}
		m[k] = v[0]
	}
	return m
}

func old2NewRequest(oldReq mercury.Request) typhon.Request {
	ep := oldReq.Endpoint()
	if !strings.HasPrefix(ep, "/") {
		ep = "/" + ep
	}
	v := typhon.Request{
		Context: oldReq.Context(),
		Request: http.Request{
			Method: "POST",
			URL: &url.URL{
				Scheme: "http",
				Host:   oldReq.Service(),
				Path:   ep},
			Proto:         "HTTP/1.1",
			ProtoMajor:    1,
			ProtoMinor:    1,
			Header:        toHeader(oldReq.Headers()),
			Host:          oldReq.Service(),
			Body:          ioutil.NopCloser(bytes.NewReader(oldReq.Payload())),
			ContentLength: int64(len(oldReq.Payload()))}}
	v.Header.Set(legacyIdHeader, oldReq.Id())
	return v
}

func new2OldRequest(newReq typhon.Request) mercury.Request {
	req := mercury.NewRequest()
	req.SetService(newReq.Host)
	req.SetEndpoint(newReq.URL.Path)
	req.SetHeaders(fromHeader(newReq.Header))
	b, _ := newReq.BodyBytes(true)
	req.SetPayload(b)
	req.SetId(newReq.Header.Get(legacyIdHeader))
	req.SetContext(newReq)
	return req
}

func old2NewResponse(req typhon.Request, oldRsp mercury.Response) typhon.Response {
	rsp := typhon.NewResponse(req)
	rsp.Header = toHeader(oldRsp.Headers())
	rsp.Write(oldRsp.Payload())
	rsp.Error = oldRsp.Error()
	return rsp
}
