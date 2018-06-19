package wox

import (
	"bytes"
	"encoding/gob"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox/naming"
	"github.com/tddhit/wox/transport"
)

type worker struct {
	addr          string
	transportAddr string
	registry      string
	pid           int
	ppid          int
	uc            *net.UnixConn
	exitCh        chan struct{}
	wg            sync.WaitGroup
	r             *naming.Registry
	server        transport.Server
}

func newWorker(addr, transportAddr, registry string,
	server transport.Server) *worker {

	w := &worker{
		addr:          addr,
		transportAddr: transportAddr,
		registry:      registry,
		pid:           os.Getpid(),
		exitCh:        make(chan struct{}),
		server:        server,
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
	ppid := os.Getenv("PPID")
	w.ppid, _ = strconv.Atoi(ppid)
	return w
}

func (w *worker) run() {
	w.register()
	go w.watchMaster()
	go w.watchSignal()
	go w.listenAndServe()

	startC := make(chan struct{})
	w.wg.Add(1)
	go func() {
		w.server.Serve(startC)
		w.wg.Done()
	}()
	<-startC
	time.Sleep(100 * time.Millisecond)
	go w.readMsg()

	reason := os.Getenv("REASON")
	if reason == reasonReload {
		if err := w.notifyMaster(&message{Typ: msgTakeover}); err == nil {
			log.Infof("WriteMsg\tPid=%d\tMsg=%s\n", w.pid, msgTakeover)
		}
	}
	log.Infof("WorkerStart\tPid=%d\tReason=%s\n", w.pid, reason)
	w.wg.Wait()
	log.Infof("WorkerEnd\tPid=%d\n", w.pid)
}

func (w *worker) register() {
	if len(w.registry) == 0 {
		return
	}
	w.r = &naming.Registry{
		Client:     GlobalEtcdClient(),
		Timeout:    2 * time.Second,
		TTL:        3,
		Target:     w.registry,
		ListenAddr: w.transportAddr,
	}
	w.r.Register()
}

func (w *worker) watchMaster() {
	tick := time.Tick(1 * time.Second)
	for range tick {
		if os.Getppid() != w.ppid {
			log.Fatal("MasterWorker is dead:", os.Getppid(), w.ppid)
		}
	}
}

func (w *worker) watchSignal() {
	signalC := make(chan os.Signal)
	signal.Notify(signalC)
	for {
		sig := <-signalC
		log.Infof("WatchSignal\tPid=%d\tSig=%s\n", w.pid, sig.String())
	}
}

func (w *worker) readMsg() {
	for {
		msg, err := readMsg(w.uc, "worker", w.pid)
		if err != nil {
			log.Fatal(err)
		}
		switch msg.Typ {
		case msgQuit:
			log.Infof("ReadMsg\tPid=%d\tMsg=%s\n", w.pid, msg.Typ)
			goto exit
		}
	}
exit:
	w.close()
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

func (w *worker) listenAndServe() {
	http.HandleFunc("/stats", w.doStats)
	http.HandleFunc("/stats.html", w.doStatsHTML)
	http.Handle("/metrics", promhttp.Handler())
	tcpAddr, err := net.ResolveTCPAddr("tcp", w.addr)
	if err != nil {
		log.Fatal(err)
	}
	fd, err := Listen(tcpAddr)
	if err != nil {
		log.Fatal(err)
	}
	file := os.NewFile(uintptr(fd), "")
	listener, err := net.FileListener(file)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Handler: http.DefaultServeMux}
	if err := srv.Serve(listener); err != nil {
		log.Fatal(err)
	}
}

func (w *worker) doStats(rsp http.ResponseWriter, req *http.Request) {
	rsp.Header().Set("Content-Type", "application/json; charset=utf-8")
	rsp.Header().Set("Access-Control-Allow-Origin", "*")
	rsp.Write(globalStats().bytes())
}

func (w *worker) doStatsHTML(rsp http.ResponseWriter, req *http.Request) {
	if err := req.ParseForm(); err != nil {
		rsp.Write([]byte(err.Error()))
	}
	var (
		html string
		addr = req.FormValue("addr")
	)
	if addr != "" {
		html = strings.Replace(globalStats().html, "##ListenAddr##", addr, 1)
	} else {
		html = strings.Replace(globalStats().html, "##ListenAddr##", w.addr, 1)
	}
	rsp.Header().Set("Content-Type", "text/html; charset=utf-8")
	rsp.Write([]byte(html))
}

func (w *worker) close() {
	w.r.Unregister()
	log.Info(time.Duration(w.r.TTL+1) * time.Second)
	time.AfterFunc(time.Duration(w.r.TTL+1)*time.Second, func() {
		w.uc.Close()
		close(w.exitCh)
		w.server.Close()
	})
}
