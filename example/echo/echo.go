package main

import (
	"context"
	"net/url"

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

type echo2API struct {
	req url.Values
	rsp echo2Rsp
}

type echo2Rsp struct {
	Str string `json:"str"`
}

func (a *echo2API) do(ctx context.Context, req, rsp interface{}) (err error) {
	realReq := req.(url.Values)
	realRsp := rsp.(*echo2Rsp)
	if str, ok := realReq["Str"]; ok {
		if len(str) > 0 {
			realRsp.Str = str[0]
		}
	}
	return
}

func main() {
	conf := &Conf{}
	s := wox.NewServer("172.17.32.101:2379", "", "echo.yml", conf)
	log.Init(conf.LogPath, conf.LogLevel)
	handler := &echoAPI{}
	s.AddHandler("/echo", &handler.req, &handler.rsp, handler.do, "application/json")
	handler2 := &echo2API{}
	s.AddHandler("/echo/ab", &handler2.req, &handler2.rsp, handler2.do, "application/x-www-form-urlencoded")
	s.Go()
}
