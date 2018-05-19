package main

import (
	"context"

	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox"
)

type echoAPI struct {
	req echoReq
	rsp echoRsp
}

type echoReq struct {
	Str string `json:"str"`
}

type echoRsp struct {
	Str string `json:"str"`
}

func (a *echoAPI) do(ctx context.Context, req, rsp interface{}) (err error) {
	jsonReq := req.(*echoReq)
	jsonRsp := rsp.(*echoRsp)
	jsonRsp.Str = jsonReq.Str
	return
}

func main() {
	conf := &Conf{}
	s := wox.NewServer("127.0.0.1:2379", "", "echo.yml", conf)
	log.Init(conf.LogPath, conf.LogLevel)
	handler := &echoAPI{}
	s.AddHandler("/echo", &handler.req, &handler.rsp, handler.do)
	s.AddHandler("/echo/ab", &handler.req, &handler.rsp, handler.do)
	s.Go()
}
