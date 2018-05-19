package main

import (
	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox"
)

func main() {
	conf := &Conf{}
	s := wox.NewServer("127.0.0.1:2379", "", "proxy.yml", conf)
	log.Init(conf.LogPath, conf.LogLevel)
	s.AddWatchTarget(conf.Upstream.Registry)
	err := s.AddProxyUpstream(conf.Upstream)
	if err != nil {
		log.Error(err)
	}
	s.Go()
}
