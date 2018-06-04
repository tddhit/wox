package hello

import (
	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox"
)

type Hello struct {
	woxServer *wox.WoxServer
}

func New(etcdAddrs, confKey, confPath string) *Hello {
	n := &Hello{}
	conf := &Conf{}
	n.woxServer = wox.NewServer(etcdAddrs, confKey, confPath, conf)
	log.Init(conf.LogPath, conf.LogLevel)
	ctx := &Context{n}
	handler := &printAPI{ctx: ctx}
	n.woxServer.AddHandler("/sayHello", &handler.req, &handler.rsp, handler.do, "application/json")
	return n
}

func (n *Hello) Go() {
	go n.woxServer.Go()
}
