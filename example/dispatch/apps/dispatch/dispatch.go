package main

import (
	"flag"

	"github.com/tddhit/wox/example/dispatch/dispatch"
)

var (
	etcdAddrs string
	confKey   string
	confPath  string
)

func init() {
	flag.StringVar(&etcdAddrs, "etcd-addrs", "127.0.0.1:2379", "etcd addrs")
	flag.StringVar(&confKey, "conf-key", "/config/service/dispatch", "config key")
	flag.StringVar(&confPath, "conf-path", "dispatch.yml", "config file")
	flag.Parse()
}

type program struct {
	dispatch *dispatch.Dispatch
}

func newProgram() *program {
	p := &program{
		dispatch: dispatch.New(etcdAddrs, confKey, confPath),
	}
	return p
}

func (p *program) Go() {
	p.dispatch.Go()
	select {}
}

func main() {
	p := newProgram()
	p.Go()
}
