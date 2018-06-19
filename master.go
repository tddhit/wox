package wox

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	etcd "github.com/coreos/etcd/clientv3"

	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox/naming"
)

type workerState int

const (
	workerAlive workerState = iota
	workerReload
	workerQuit
	workerCrash
)

func (s workerState) String() string {
	switch s {
	case workerAlive:
		return "alive"
	case workerReload:
		return "reload"
	case workerQuit:
		return "quit"
	case workerCrash:
		return "crash"
	default:
		return fmt.Sprintf("unknown worker state:%d", s)
	}
}

const (
	reasonStart  = "start"
	reasonReload = "reload"
	reasonCrash  = "crash"

	workerNum = 1
)

type master struct {
	addr     string
	targets  []string // for watch
	pid      int
	pidPath  string
	uc       sync.Map // key: pid, value:UnixConn
	children sync.Map // key: pid, value:state
	exitC    chan struct{}
}

func newMaster(addr string, targets []string, pidPath string) *master {
	m := &master{
		addr:    addr,
		targets: targets,
		pid:     os.Getpid(),
		pidPath: pidPath,
		exitC:   make(chan struct{}),
	}
	return m
}

func (m *master) run() {
	m.savePID()
	if err := os.Setenv(FORK, "1"); err != nil {
		log.Fatal(err)
	}
	for i := 0; i < workerNum; i++ {
		if _, err := m.fork(reasonStart); err != nil {
			log.Fatal(err)
		}
	}
	go m.watchTarget()
	go m.watchChildren()
	go m.listenAndServe()

	log.Infof("StartMaster\tPid=%d\n", m.pid)
	signalC := make(chan os.Signal)
	signal.Notify(signalC)
	for {
		select {
		case sig := <-signalC:
			log.Infof("WatchSignal\tPid=%d\tSig=%s\n", m.pid, sig.String())
			switch sig {
			case syscall.SIGHUP:
				m.reload()
			case syscall.SIGINT:
				fallthrough
			case syscall.SIGQUIT:
				m.graceful()
				goto exit
			case syscall.SIGTERM:
				m.rough()
				goto exit
			}
		}
	}
exit:
}

func (m *master) savePID() {
	f, err := os.OpenFile(m.pidPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0666)
	if err != nil {
		log.Fatal(err)
	}
	f.WriteString(strconv.Itoa(m.pid) + "\n")
	f.Sync()
	f.Close()
}

func (m *master) removePID() {
	err := os.Remove(m.pidPath)
	if err != nil {
		log.Error(err)
	}
}

func (m *master) watchTarget() {
	for _, target := range m.targets {
		log.Infof("WatchTarget\tTarget=%s\n", target)
		w := &naming.Watcher{
			Client:  GlobalEtcdClient(),
			Timeout: 2 * time.Second,
		}
		if ch, err := w.Watch(target); err != nil {
			log.Fatal(err)
		} else {
			go func(ch etcd.WatchChan, target string) {
				for rsp := range ch {
					log.Infof("WatchEvent\t%s\n", target)
					for _, event := range rsp.Events {
						log.Infof("WatchEvent\tType=%d\tKey=%s\tValue=%s\n",
							event.Type, string(event.Kv.Key), string(event.Kv.Value))
					}
					m.reload()
				}
			}(ch, target)
		}
	}
}

func (m *master) watchChildren() {
	f := func(key, value interface{}) bool {
		pid := key.(int)
		state := value.(workerState)
		switch state {
		case workerCrash:
			if _, err := m.fork(reasonCrash); err != nil {
				log.Error(err)
			}
			fallthrough
		case workerQuit:
			m.children.Delete(pid)
			m.uc.Delete(pid)
		}
		return true
	}
	tick := time.Tick(1 * time.Second)
	for range tick {
		m.children.Range(f)
	}
}

func (m *master) fork(reason string) (pid int, err error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		log.Error(err)
		return
	}
	execSpec := &syscall.ProcAttr{
		Env: append(os.Environ(), "REASON="+reason, "PPID="+strconv.Itoa(m.pid)),
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd(),
			uintptr(fds[1])},
	}
	pid, err = syscall.ForkExec(os.Args[0], os.Args, execSpec)
	if err != nil {
		log.Error(err)
		return
	}
	file := os.NewFile(uintptr(fds[0]), "")
	conn, _ := net.FileConn(file)
	uc, _ := conn.(*net.UnixConn)
	m.uc.Store(pid, uc)
	m.children.Store(pid, workerAlive)
	syscall.Close(fds[1])

	go m.waitWorker(pid)
	go m.readMsg(pid, uc)
	return
}

func (m *master) waitWorker(pid int) {
	p, _ := os.FindProcess(pid)
	state, _ := p.Wait()
	status := state.Sys().(syscall.WaitStatus)
	if status.ExitStatus() != 0 {
		m.children.Store(pid, workerCrash)
		log.Errorf("WorkerCrash\tPid=%d\tStatus=%d\n", pid, status.ExitStatus())
	} else {
		m.children.Store(pid, workerQuit)
		log.Infof("WorkerQuit\tPid=%d\n", pid)
	}
	/*
		log.Error(status.Exited())
		log.Error(status.ExitStatus())
		log.Error(status.Signaled())
		log.Error(status.Signal())
		log.Error(status.CoreDump())
		log.Error(status.Stopped())
		log.Error(status.Continued())
		log.Error(status.StopSignal())
	*/
}

func (m *master) readMsg(pid int, uc *net.UnixConn) {
	for {
		msg, err := readMsg(uc, "master", m.pid)
		if err != nil {
			break
		}
		switch msg.Typ {
		case msgTakeover:
			m.notifyWorker(&message{Typ: msgQuit}, workerReload)
			log.Infof("ReadMsg\tPid=%d\tMsg=%s\n", pid, msg.Typ)
		}
	}
}

func (m *master) reload() {
	f := func(key, value interface{}) bool {
		pid := key.(int)
		state := value.(workerState)
		if state == workerAlive {
			m.children.Store(pid, workerReload)
		}
		return true
	}
	m.children.Range(f)
	for i := 0; i < workerNum; i++ {
		if _, err := m.fork(reasonReload); err != nil {
			log.Error(err)
		}
	}
}

func (m *master) notifyWorker(msg *message, states ...workerState) {
	f := func(key, value interface{}) bool {
		pid := key.(int)
		curState := value.(workerState)
		match := false
		for _, state := range states {
			if curState == state {
				match = true
				break
			}
		}
		if !match {
			return true
		}
		if uc, ok := m.uc.Load(pid); !ok {
			log.Errorf("NotInUnixConn\tPid=%d\n", pid)
			return true
		} else {
			var buf bytes.Buffer
			gob.NewEncoder(&buf).Encode(msg)
			if _, _, err := uc.(*net.UnixConn).WriteMsgUnix(buf.Bytes(), nil, nil); err != nil {
				log.Warnf("WriteMsg\tPid=%d\tErr=%s\n", pid, err.Error())
			} else {
				log.Infof("WriteMsg\tPid=%d\tMsg=%s\n", pid, msg.Typ)
			}
		}
		return true
	}
	m.children.Range(f)
}

func (m *master) graceful() {
	m.notifyWorker(&message{Typ: msgQuit}, workerAlive, workerReload)
	select {
	case <-time.After(5 * time.Second):
		f := func(key, value interface{}) bool {
			uc := value.(*net.UnixConn)
			uc.Close()
			return true
		}
		m.uc.Range(f)
		m.removePID()
	}
}

func (m *master) rough() {
	f := func(key, value interface{}) bool {
		pid := key.(int)
		state := value.(workerState)
		if state == workerAlive || state == workerReload {
			log.Infof("SIGKILL\tPid=%d\tState=%s\n", pid, state)
			if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
				log.Error(err)
			}
		}
		return true
	}
	m.children.Range(f)
	m.removePID()
}

func (m *master) listenAndServe() {
	http.HandleFunc("/stats", m.doStats)
	if err := http.ListenAndServe(m.addr, nil); err != nil {
		log.Fatal(err)
	}
}

func (m *master) doStats(rsp http.ResponseWriter, req *http.Request) {
	var jsonRsp struct {
		Code    int    `json:"code"`
		Error   string `json:"error"`
		Version string `json:"version"`
	}
	count := 0
	f := func(key, value interface{}) bool {
		state := value.(workerState)
		if state == workerAlive {
			count++
		}
		return true
	}
	m.children.Range(f)
	if count != workerNum {
		jsonRsp.Code = 207
		jsonRsp.Error = fmt.Sprintf("WorkersNum is %d, not %d\n", count, workerNum)
	} else {
		jsonRsp.Code = 200
	}
	jsonRsp.Version = VERSION
	out, _ := json.Marshal(jsonRsp)
	rsp.Write(out)
}
