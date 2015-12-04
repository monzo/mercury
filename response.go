package mercury

import (
	"github.com/mondough/terrors"
	tperrors "github.com/mondough/terrors/proto"
	tmsg "github.com/mondough/typhon/message"

	"github.com/mondough/mercury/marshaling"
)

type Response interface {
	tmsg.Response
	// IsError returns whether this response contains an error
	IsError() bool
	// SetIsError modifies whether the flag specifying whether this response contains an error
	SetIsError(bool)
	// Error returns the error contained within the response
	Error() error
}

type response struct {
	tmsg.Response
}

func (r *response) IsError() bool {
	return r.Headers()[errHeader] == "1"
}

func (r *response) SetIsError(v bool) {
	r.SetHeader(errHeader, "1")
}

func (r *response) Error() error {
	if !r.IsError() {
		return nil
	}
	r2 := r.Copy()
	um := marshaling.Unmarshaler(r2.Headers()[marshaling.ContentTypeHeader], &tperrors.Error{})
	if um == nil {
		um = marshaling.Unmarshaler(marshaling.ProtoContentType, &tperrors.Error{})
	}
	if umErr := um.UnmarshalPayload(r2); umErr != nil {
		return umErr
	}
	if err := terrors.Unmarshal(r2.Body().(*tperrors.Error)); err != nil {
		return err // Don't return a nil but typed result
	}
	return nil
}

func NewResponse() Response {
	return FromTyphonResponse(tmsg.NewResponse())
}

func FromTyphonResponse(rsp tmsg.Response) Response {
	return &response{
		Response: rsp,
	}
}
