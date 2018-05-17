package hi

import (
	"context"

	"github.com/tddhit/wox/example/dispatch/internal/api"
)

type printAPI struct {
	req api.HiSayHiReq
	rsp api.HiSayHiRsp
	ctx *Context
}

func (a *printAPI) do(ctx context.Context, req, rsp interface{}) (err error) {
	jsonReq := req.(*api.HiSayHiReq)
	jsonRsp := rsp.(*api.HiSayHiRsp)

	jsonRsp.Code = 200
	jsonRsp.Str = "hi:" + jsonReq.Str
	return
}
