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
	"github.com/tddhit/wox/naming"
	"github.com/tddhit/wox/option"
)

const (
	FORK = "FORK"
)

const (
	msgWorkerStats    = 1 + iota // worker->master
	msgWorkerTakeover            // worker->master
	msgWorkerQuit                // master->worker
)

const (
	DefaultWorkerNum = 2
)

var (
	globalEtcdClient *etcd.Client
)

type message struct {
	Typ   int
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

func SetGlobalEtcdClient(ec *etcd.Client) {
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

func NewServer(opt option.Server, etcdAddrs string) *WoxServer {
	httpServer := NewHTTPServer(opt)
	s := &WoxServer{
		pidPath:    opt.PIDPath,
		registry:   opt.Registry,
		workerNum:  opt.WorkerNum,
		httpServer: httpServer,
	}
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

	// init etcd client
	if etcdAddrs != "" {
		endpoints := strings.Split(etcdAddrs, ",")
		cfg := etcd.Config{
			Endpoints:   endpoints,
			DialTimeout: 2000 * time.Millisecond,
		}
		ec, err := etcd.New(cfg)
		if err != nil {
			log.Fatal(err)
		}
		SetGlobalEtcdClient(ec)
	}

	s.workerAddr = naming.GetLocalAddr(opt.Addr)
	if opt.StatusAddr != "" {
		s.masterAddr = naming.GetLocalAddr(opt.StatusAddr)
	} else {
		addr := strings.Split(opt.Addr, ":")
		port, _ := strconv.Atoi(addr[len(addr)-1])
		addr[len(addr)-1] = strconv.Itoa(port + 1)
		s.masterAddr = strings.Join(addr, ":")
	}

	return s
}

func (s *WoxServer) AddWatchTarget(target string) {
	s.targets = append(s.targets, target)
}

func (s *WoxServer) ListenAddr() string {
	return s.workerAddr
}

func (s *WoxServer) statusAddr() string {
	return s.masterAddr
}

func (s *WoxServer) AddHandler(pattern string, req, rsp interface{},
	h HandlerFunc) {
	s.httpServer.AddHandler(pattern, req, rsp, h)
}

func (s *WoxServer) Go() {
	if os.Getenv(FORK) == "1" {
		newWorker(s.registry, s.workerAddr, s.pidPath, s.httpServer).run()
	} else {
		newMaster(s.targets, s.masterAddr, s.pidPath, s.workerNum).run()
	}
}
