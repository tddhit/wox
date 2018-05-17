package hello

import (
	"context"

	"github.com/tddhit/wox/example/dispatch/internal/api"
)

type printAPI struct {
	req api.HelloSayHelloReq
	rsp api.HelloSayHelloRsp
	ctx *Context
}

func (a *printAPI) do(ctx context.Context, req, rsp interface{}) (err error) {
	jsonReq := req.(*api.HelloSayHelloReq)
	jsonRsp := rsp.(*api.HelloSayHelloRsp)

	jsonRsp.Code = 200
	jsonRsp.Str = "hello:" + jsonReq.Str
	return
}
