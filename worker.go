package wox

import (
	"bytes"
	"encoding/gob"
	"net"
	"os"
	"os/signal"
	"time"

	etcd "github.com/coreos/etcd/clientv3"

	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox/naming"
)

type worker struct {
	c        *etcd.Client
	registry string
	server   Server
	quitCh   chan struct{}
	uc       *net.UnixConn
	pid      int
}

func newWorker(c *etcd.Client, registry string, server Server) *worker {
	w := &worker{
		c:        c,
		registry: registry,
		server:   server,
		quitCh:   make(chan struct{}),
		pid:      os.Getpid(),
	}
	file := os.NewFile(3, "")
	if conn, err := net.FileConn(file); err != nil {
		log.Fatal(err)
	} else {
		if uc, ok := conn.(*net.UnixConn); ok {
			w.uc = uc
		} else {
			log.Fatal(err)
		}
	}
	return w
}

func (w *worker) run() {
	if len(w.registry) > 0 {
		w.register()
	}
	//go w.watchMaster()
	go w.watchSignal()
	go w.readMsg()
	go w.listenServer()
	go w.watchStats()
	go w.server.serve()
	reason := os.Getenv("REASON")
	if reason == reasonReload {
		if err := w.notifyMaster(&message{Typ: msgWorkerTakeover}); err == nil {
			log.Infof("WriteMsg\tPid=%d\tMsg=%d\n", w.pid, msgWorkerTakeover)
		}
	}
	log.Infof("StartWorker\tPid=%d\n", w.pid)
	select {}
}

func (w *worker) register() {
	r := &naming.Registry{
		Client:     w.c,
		Timeout:    2000,
		TTL:        1,
		Target:     w.registry,
		ListenAddr: w.server.listenAddr(),
	}
	r.Register()
}

func (w *worker) watchMaster() {
	tick := time.Tick(1 * time.Second)
	for range tick {
		if os.Getppid() == 1 {
			os.Exit(1)
		}
	}
}

func (w *worker) watchSignal() {
	c := make(chan os.Signal)
	signal.Notify(c)
	for {
		sig := <-c
		log.Infof("WatchSignal\tPid=%d\tSig=%s\n", w.pid, sig.String())
	}
}

func (w *worker) readMsg() {
	for {
		msg, err := readMsg(w.uc, "worker", w.pid)
		if err != nil {
			if w.uc == nil {
				break
			} else {
				log.Fatal(err)
			}
		}
		switch msg.Typ {
		case msgWorkerQuit:
			log.Infof("ReadMsg\tPid=%d\tMsg=%d\n", w.pid, msg.Typ)
			w.server.close(w.quitCh)
		}
	}
}

func (w *worker) listenServer() {
	select {
	case <-w.quitCh:
		if w.c != nil {
			w.c.Close()
		}
		w.uc.Close()
		w.uc = nil
		os.Exit(0)
	}
}

func (w *worker) watchStats() {
	for stats := range w.server.stats() {
		w.notifyMaster(&message{Typ: msgWorkerStats, Value: stats})
	}
}

func (w *worker) notifyMaster(msg *message) (err error) {
	var buf bytes.Buffer
	gob.NewEncoder(&buf).Encode(msg)
	if _, _, err = w.uc.WriteMsgUnix(buf.Bytes(), nil, nil); err != nil {
		log.Errorf("WriteMsg\tPid=%d\tErr=%s\n", w.pid, err.Error())
		return
	}
	return
}
