package main

import (
	"flag"

	"github.com/tddhit/wox/example/dispatch/hello"
)

var (
	etcdAddrs string
	confKey   string
	confPath  string
)

func init() {
	flag.StringVar(&etcdAddrs, "etcd-addrs", "127.0.0.1:2379", "etcd addrs")
	flag.StringVar(&confKey, "conf-key", "/config/service/hello", "config key")
	flag.StringVar(&confPath, "conf-path", "hello.yml", "config file")
	flag.Parse()
}

type program struct {
	hello *hello.Hello
}

func newProgram() *program {
	p := &program{
		hello: hello.New(etcdAddrs, confKey, confPath),
	}
	return p
}

func (p *program) Go() {
	p.hello.Go()
	select {}
}

func main() {
	p := newProgram()
	p.Go()
}
