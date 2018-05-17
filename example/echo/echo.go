package main

import (
	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox"
	"github.com/tddhit/wox/option"
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

func (a *echoAPI) do(req, rsp interface{}) (err error) {
	jsonReq := req.(*echoReq)
	jsonRsp := rsp.(*echoRsp)
	jsonRsp.Str = jsonReq.Str
	return
}

func main() {
	log.Init("echo.log", log.INFO)
	httpServer := wox.NewHTTPServer(option.Server{Addr: ":18860", StatusAddr: ":8018", Registry: "/nlpservice/echo"})
	s := &wox.WoxServer{
		Server:    httpServer,
		WorkerNum: 1,
	}
	handler := &echoAPI{}
	httpServer.AddHandler("/echo", &handler.req, &handler.rsp, handler.do)
	httpServer.AddHandler("/echo2", &handler.req, &handler.rsp, handler.do)
	s.Go()
}
