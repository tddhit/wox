package wox

import (
	"bytes"
	"encoding/gob"
	"net"
	"os"

	etcd "github.com/coreos/etcd/clientv3"
	"github.com/tddhit/tools/log"
)

const (
	FORK = "FORK"
)

const (
	msgWorkerStats    = 1 + iota // worker->master
	msgWorkerTakeover            // worker->master
	msgWorkerQuit                // master->worker
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

type Server interface {
	serve() error
	ListenAddr() string
	statusAddr() [2]string
	close(chan struct{})
	stats() <-chan []byte
}

type WoxServer struct {
	Client   *etcd.Client
	Targets  []string
	Registry string
	Server   Server
}

func (s *WoxServer) AddWatchTarget(target string) {
	s.Targets = append(s.Targets, target)
}

func (s *WoxServer) Go() {
	if os.Getenv(FORK) == "1" {
		newWorker(s.Client, s.Registry, s.Server).run()
	} else {
		newMaster(s.Client, s.Targets, s.Server.statusAddr()).run()
	}
}
