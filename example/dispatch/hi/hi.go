package hi

import (
	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox"
)

type Hi struct {
	woxServer *wox.WoxServer
}

func New(etcdAddrs, confKey, confPath string) *Hi {
	conf, err := NewConf(confPath)
	if err != nil {
		log.Fatal(err)
	}
	log.Init(conf.LogPath, conf.LogLevel)
	n := &Hi{}
	ctx := &Context{n}
	handler := &printAPI{ctx: ctx}
	n.woxServer = wox.NewServer(conf.HTTPServer, etcdAddrs)
	n.woxServer.AddWatchTarget(confKey)
	n.woxServer.AddHandler("/sayHi", &handler.req, &handler.rsp, handler.do)
	return n
}

func (n *Hi) Go() {
	go n.woxServer.Go()
}
