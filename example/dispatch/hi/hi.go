package hi

import (
	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox"
)

type Hi struct {
	woxServer *wox.WoxServer
}

func New(etcdAddrs, confKey, confPath string) *Hi {
	n := &Hi{}
	conf := &Conf{}
	n.woxServer = wox.NewServer(etcdAddrs, confKey, confPath, conf)
	log.Init(conf.LogPath, conf.LogLevel)
	ctx := &Context{n}
	handler := &printAPI{ctx: ctx}
	n.woxServer.AddHandler("/sayHi", &handler.req, &handler.rsp, handler.do, "application/json")
	return n
}

func (n *Hi) Go() {
	go n.woxServer.Go()
}
