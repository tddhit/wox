package wox

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net"
	"os"
	"runtime"
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

type msgType int

const (
	msgWorkerStats    msgType = 1 + iota // worker->master
	msgWorkerTakeover                    // worker->master
	msgWorkerQuit                        // master->worker
)

func (m msgType) String() string {
	switch m {
	case msgWorkerStats:
		return "stats"
	case msgWorkerTakeover:
		return "takeover"
	case msgWorkerQuit:
		return "quit"
	default:
		return fmt.Sprintf("unknown msg type:%d", m)
	}
}

const (
	DefaultWorkerNum = 2
)

var (
	globalEtcdClient *etcd.Client
)

type message struct {
	Typ   msgType
	Value interface{}
}

func readMsg(conn *net.UnixConn, id string, pid int) (*message, error) {
	buf := make([]byte, 1024)
	msg := &message{}
	if _, _, _, _, err := conn.ReadMsgUnix(buf, nil); err != nil {
		log.Errorf("ReadMsg\tId=%s\tPid=%d\tErr=%s\n", id, pid, err.Error())
		return nil, err
	}
	if err := gob.NewDecoder(bytes.NewBuffer(buf)).Decode(msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func setGlobalEtcdClient(ec *etcd.Client) {
	globalEtcdClient = ec
}

func GlobalEtcdClient() *etcd.Client {
	return globalEtcdClient
}

type WoxServer struct {
	pidPath    string
	registry   string
	masterAddr string
	workerAddr string
	targets    []string
	workerNum  int
	httpServer *HTTPServer
}

func NewServer(etcdAddrs, confKey, confPath string, conf confcenter.Conf) *WoxServer {
	// init etcd client
	endpoints := strings.Split(etcdAddrs, ",")
	cfg := etcd.Config{
		Endpoints:   endpoints,
		DialTimeout: 2000 * time.Millisecond,
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
			Timeout: 2000 * time.Millisecond,
		}
		if err := r.Save(confKey, confPath); err != nil {
			log.Fatal(err)
		}
	}
	if err := confcenter.NewConf(confPath, conf); err != nil {
		log.Fatal(err)
	}

	httpServer := NewHTTPServer(conf.Server())
	s := &WoxServer{
		pidPath:    conf.Server().PIDPath,
		registry:   conf.Server().Registry,
		workerNum:  conf.Server().WorkerNum,
		httpServer: httpServer,
	}
	s.AddWatchTarget(confKey)

	if s.pidPath == "" {
		name := strings.Split(os.Args[0], "/")
		if len(name) == 0 {
			log.Fatal("get pidPath fail:", os.Args[0])
		}
		s.pidPath = fmt.Sprintf("/var/%s.pid", name[len(name)-1])
	}

	if s.workerNum == 0 {
		s.workerNum = DefaultWorkerNum
	} else {
		cpuNum := runtime.NumCPU()
		if s.workerNum > cpuNum {
			s.workerNum = cpuNum
		}
	}

	s.workerAddr = naming.GetLocalAddr(conf.Server().Addr)
	if conf.Server().StatusAddr != "" {
		s.masterAddr = naming.GetLocalAddr(conf.Server().StatusAddr)
	} else {
		addr := strings.Split(conf.Server().Addr, ":")
		port, _ := strconv.Atoi(addr[len(addr)-1])
		addr[len(addr)-1] = strconv.Itoa(port + 1)
		statusAddr := strings.Join(addr, ":")
		s.masterAddr = naming.GetLocalAddr(statusAddr)
	}

	return s
}

func (s *WoxServer) AddWatchTarget(target string) {
	if target != "" {
		s.targets = append(s.targets, target)
	}
}

func (s *WoxServer) ListenAddr() string {
	return s.workerAddr
}

func (s *WoxServer) statusAddr() string {
	return s.masterAddr
}

func (s *WoxServer) AddHandler(pattern string, req, rsp interface{},
	h HandlerFunc, contentType string) {
	s.httpServer.AddHandler(pattern, req, rsp, h, contentType)
}

func (s *WoxServer) AddProxyUpstream(opt *option.Upstream) error {
	return s.httpServer.AddProxyUpstream(opt)
}

func (s *WoxServer) Go() {
	if os.Getenv(FORK) == "1" {
		newWorker(s.registry, s.workerAddr, s.pidPath, s.httpServer).run()
	} else {
		newMaster(s.targets, s.masterAddr, s.pidPath, s.workerNum).run()
	}
}
