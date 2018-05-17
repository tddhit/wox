package hello

import (
	"strings"
	"time"

	etcd "github.com/coreos/etcd/clientv3"

	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox"
)

type Hello struct {
	woxServer *wox.WoxServer
}

func New(etcdAddrs, confKey, confPath string) *Hello {
	endpoints := strings.Split(etcdAddrs, ",")
	cfg := etcd.Config{
		Endpoints:   endpoints,
		DialTimeout: 2 * time.Second,
	}
	etcdClient, err := etcd.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	conf, err := NewConf(confPath)
	if err != nil {
		log.Fatal(err)
	}
	log.Init(conf.LogPath, conf.LogLevel)
	n := &Hello{}
	httpServer := wox.NewHTTPServer(conf.HTTPServer)
	n.woxServer = &wox.WoxServer{
		Client:   etcdClient,
		Registry: conf.HTTPServer.Registry,
		Server:   httpServer,
	}
	n.woxServer.AddWatchTarget(confKey)
	ctx := &Context{n}
	handler := &printAPI{ctx: ctx}
	httpServer.AddHandler("/sayHello", &handler.req, &handler.rsp, handler.do)
	return n
}

func (n *Hello) Go() {
	go n.woxServer.Go()
}