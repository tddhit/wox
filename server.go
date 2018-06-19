package wox

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	etcd "github.com/coreos/etcd/clientv3"

	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox/confcenter"
	"github.com/tddhit/wox/naming"
	"github.com/tddhit/wox/option"
)

const (
	FORK = "FORK"
)

var (
	gEtcdClient *etcd.Client
)

func setGlobalEtcdClient(ec *etcd.Client) {
	gEtcdClient = ec
}

func GlobalEtcdClient() *etcd.Client {
	return gEtcdClient
}

type WoxServer struct {
	registry      string
	targets       []string
	pidPath       string
	masterAddr    string
	workerAddr    string
	transportAddr string
	httpServer    *HTTPServer
}

func NewServer(etcdAddrs, confKey, confPath string, conf confcenter.Conf) *WoxServer {
	// init etcd client
	endpoints := strings.Split(etcdAddrs, ",")
	cfg := etcd.Config{
		Endpoints:   endpoints,
		DialTimeout: 2 * time.Second,
	}
	ec, err := etcd.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	setGlobalEtcdClient(ec)

	// get conf
	if confKey != "" {
		r := &confcenter.Resolver{
			Client:  GlobalEtcdClient(),
			Timeout: 2 * time.Second,
		}
		if err := r.Save(confKey, confPath); err != nil {
			log.Fatal(err)
		}
	}
	if err := confcenter.NewConf(confPath, conf); err != nil {
		log.Fatal(err)
	}

	s := &WoxServer{
		pidPath:    conf.Server().PIDPath,
		registry:   conf.Server().Registry,
		httpServer: NewHTTPServer(conf.Server()),
	}
	s.AddWatchTarget(confKey)

	if s.pidPath == "" {
		name := strings.Split(os.Args[0], "/")
		if len(name) == 0 {
			log.Fatal("get pidPath fail:", os.Args[0])
		}
		s.pidPath = fmt.Sprintf("/var/%s.pid", name[len(name)-1])
	}

	s.transportAddr = naming.GetLocalAddr(conf.Server().TransportAddr)
	if conf.Server().MasterAddr != "" {
		s.masterAddr = naming.GetLocalAddr(conf.Server().MasterAddr)
	} else {
		s.masterAddr = getDefaultAddr(s.transportAddr, 1)
	}
	if conf.Server().WorkerAddr != "" {
		s.workerAddr = naming.GetLocalAddr(conf.Server().WorkerAddr)
	} else {
		s.workerAddr = getDefaultAddr(s.transportAddr, 2)
	}
	log.Info(s.masterAddr, s.workerAddr, s.transportAddr)
	return s
}

func (s *WoxServer) AddWatchTarget(target string) {
	if target != "" {
		s.targets = append(s.targets, target)
	}
}

func (s *WoxServer) AddHandler(pattern string, req, rsp interface{},
	h HandlerFunc, contentType string) {
	s.httpServer.AddHandler(pattern, req, rsp, h, contentType)
}

func (s *WoxServer) AddProxyUpstream(opt *option.Upstream) error {
	return s.httpServer.AddProxyUpstream(opt)
}

func (s *WoxServer) TransportAddr() string {
	return s.transportAddr
}

func (s *WoxServer) Go() {
	if os.Getenv(FORK) == "1" {
		newWorker(s.workerAddr, s.transportAddr, s.registry, s.httpServer).run()
	} else {
		newMaster(s.masterAddr, s.targets, s.pidPath).run()
	}
}

// get default masterAddr/workerAddr.
// eg. transportAddr:80, masterAddr:81, workerAddr:82
func getDefaultAddr(addr string, n int) string {
	a := strings.Split(addr, ":")
	port, _ := strconv.Atoi(a[len(a)-1])
	a[len(a)-1] = strconv.Itoa(port + n)
	return strings.Join(a, ":")
}
