package hello

import (
	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox"
)

type Hello struct {
	woxServer *wox.WoxServer
}

func New(etcdAddrs, confKey, confPath string) *Hello {
	conf, err := NewConf(confPath)
	if err != nil {
		log.Fatal(err)
	}
	log.Init(conf.LogPath, conf.LogLevel)
	n := &Hello{}
	ctx := &Context{n}
	handler := &printAPI{ctx: ctx}
	n.woxServer = wox.NewServer(conf.HTTPServer, etcdAddrs)
	n.woxServer.AddWatchTarget(confKey)
	n.woxServer.AddHandler("/sayHello", &handler.req, &handler.rsp, handler.do)
	return n
}

func (n *Hello) Go() {
	go n.woxServer.Go()
}
