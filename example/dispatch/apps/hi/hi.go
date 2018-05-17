package main

import (
	"flag"

	"github.com/tddhit/wox/example/dispatch/hi"
)

var (
	etcdAddrs string
	confKey   string
	confPath  string
)

func init() {
	flag.StringVar(&etcdAddrs, "etcd-addrs", "127.0.0.1:2379", "etcd addrs")
	flag.StringVar(&confKey, "conf-key", "/config/service/hi", "config key")
	flag.StringVar(&confPath, "conf-path", "hi.yml", "config file")
	flag.Parse()
}

type program struct {
	hi *hi.Hi
}

func newProgram() *program {
	p := &program{
		hi: hi.New(etcdAddrs, confKey, confPath),
	}
	return p
}

func (p *program) Go() {
	p.hi.Go()
	select {}
}

func main() {
	p := newProgram()
	p.Go()
}
