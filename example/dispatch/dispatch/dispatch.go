package dispatch

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	etcd "github.com/coreos/etcd/clientv3"

	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox"

	"github.com/tddhit/wox/example/dispatch/internal/api"
)

var (
	errRequestHello   = errors.New("request hello service fail.")
	errRequestHi      = errors.New("request hi service fail.")
	errAbnormalResult = errors.New("abnormal result.")
)

type Dispatch struct {
	woxServer *wox.WoxServer
	upstream  map[string]*wox.Upstream
}

func New(etcdAddrs, confKey, confPath string) *Dispatch {
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
	n := &Dispatch{
		upstream: make(map[string]*wox.Upstream),
	}
	var targets []string
	for _, up := range conf.Upstream {
		if up.Enable {
			targets = append(targets, up.Registry)
		}
	}
	httpServer := wox.NewHTTPServer(conf.HTTPServer)
	n.woxServer = &wox.WoxServer{
		Client:   etcdClient,
		Targets:  targets,
		Registry: conf.HTTPServer.Registry,
		Server:   httpServer,
	}
	n.woxServer.AddWatchTarget(confKey)
	ctx := &Context{n}
	handler := &greetAPI{ctx: ctx}
	httpServer.AddHandler("/greet", &handler.req, &handler.rsp, wox.Decorate(handler.do, checkParams))
	if os.Getenv(wox.FORK) == "1" {
		for k, v := range conf.Upstream {
			if v.Enable {
				if upstream, err := wox.NewUpstream(etcdClient, v, wox.RoundRobin); err != nil {
					log.Fatal(err)
				} else {
					n.upstream[k] = upstream
				}
			}
		}
	}
	return n
}

func (n *Dispatch) Go() {
	go n.woxServer.Go()
}

func (n *Dispatch) greet(ctx context.Context, str string) (string, error) {
	helloReq := &api.HelloSayHelloReq{
		Str: str,
	}
	hiReq := &api.HiSayHiReq{
		Str: str,
	}
	var (
		wg       sync.WaitGroup
		helloRsp api.HelloSayHelloRsp
		hiRsp    api.HiSayHiRsp
	)
	wg.Add(2)
	go func() {
		err := n.upstream["hello"].NewRequest(ctx, n.upstream["hello"].Api["sayHello"], helloReq, &helloRsp, "")
		if err != nil {
			log.Error("hello", err)
		}
		wg.Done()
	}()
	go func() {
		err := n.upstream["hi"].NewRequest(ctx, n.upstream["hi"].Api["sayHi"], hiReq, &hiRsp, "")
		if err != nil {
			log.Error("hi", err)
		}
		wg.Done()
	}()
	wg.Wait()
	return n.processResult(&helloRsp, &hiRsp)
}

func (n *Dispatch) processResult(helloRsp *api.HelloSayHelloRsp, hiRsp *api.HiSayHiRsp) (str string, err error) {
	if helloRsp.Code != 200 {
		err = errRequestHello
		return
	}
	if hiRsp.Code != 200 {
		err = errRequestHi
		return
	}
	str = helloRsp.Str + "-" + hiRsp.Str
	return
}
