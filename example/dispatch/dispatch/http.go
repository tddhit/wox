package dispatch

import (
	"context"

	"github.com/tddhit/wox"
	"github.com/tddhit/wox/example/dispatch/internal/api"
)

type greetAPI struct {
	req api.DispatchGreetReq
	rsp api.DispatchGreetRsp
	ctx *Context
}

func (a *greetAPI) do(ctx context.Context, req, rsp interface{}) (err error) {
	jsonReq := req.(*api.DispatchGreetReq)
	jsonRsp := rsp.(*api.DispatchGreetRsp)
	str, err := a.ctx.dispatch.greet(ctx, jsonReq.Str)
	if err != nil {
		return wox.Err500Rsp
	}
	jsonRsp.Code = 200
	jsonRsp.Str = str
	return
}

func checkParams(h wox.HandlerFunc) wox.HandlerFunc {
	return func(ctx context.Context, req, rsp interface{}) (err error) {
		if _, ok := req.(*api.DispatchGreetReq); ok {
			err = h(ctx, req, rsp)
			return
		}
		return wox.Err400Rsp
	}
}
