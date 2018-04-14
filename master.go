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
	"sync"
	"syscall"
	"time"

	etcd "github.com/coreos/etcd/clientv3"

	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox/naming"
)

const (
	workerAlive  = "alive"
	workerReload = "reload"
	workerQuit   = "quit"
	workerCrash  = "crash"

	reasonStart  = "start"
	reasonReload = "reload"
	reasonCrash  = "crash"
)

type master struct {
	c          *etcd.Client
	targets    []string
	num        int
	children   sync.Map // key: pid, value:state
	uc         sync.Map // key: pid, value:UnixConn
	stats      *stats
	listenAddr string
	pid        int
}

func newMaster(c *etcd.Client, targets []string, statusAddr string) *master {
	m := &master{
		c:       c,
		targets: targets,
		num:     2,
		pid:     os.Getpid(),
	}
	m.listenAddr = naming.GetLocalAddr(statusAddr)
	m.stats = newStats(m.listenAddr)
	return m
}

func (m *master) run() {
	if err := os.Setenv(FORK, "1"); err != nil {
		log.Fatal(err)
	}
	for i := 0; i < m.num; i++ {
		if _, err := m.fork(reasonStart); err != nil {
			log.Fatal(err)
		}
	}
	go m.watchTarget()
	go m.watchChildren()
	go m.watchSignal()
	go m.listenAndServe()
	log.Infof("StartMaster\tPid=%d\n", m.pid)
	select {}
}

func (m *master) watchTarget() {
	for _, target := range m.targets {
		log.Infof("WatchTarget\tTarget=%s\n", target)
		w := &naming.Watcher{
			Client:  m.c,
			Timeout: 2000,
		}
		if ch, err := w.Watch(target); err != nil {
			log.Fatal(err)
		} else {
			go func(ch etcd.WatchChan) {
				for rsp := range ch {
					for _, event := range rsp.Events {
						log.Infof("WatchEvent\tType=%d\tKey=%s\tValue=%s\n",
							event.Type, string(event.Kv.Key), string(event.Kv.Value))
					}
					m.reload()
				}
			}(ch)
		}
	}
}

func (m *master) watchChildren() {
	f := func(key, value interface{}) bool {
		pid := key.(int)
		state := value.(string)
		switch state {
		case workerAlive:
		case workerReload:
		case workerCrash:
			if _, err := m.fork(reasonCrash); err != nil {
				log.Error(err)
			}
			fallthrough
		case workerQuit:
			m.children.Delete(pid)
			m.uc.Delete(pid)
			m.stats.Lock()
			delete(m.stats.Worker, pid)
			m.stats.Unlock()
		}
		return true
	}
	tick := time.Tick(1 * time.Second)
	for range tick {
		m.children.Range(f)
	}
}

func (m *master) watchSignal() {
	c := make(chan os.Signal)
	signal.Notify(c)
	for {
		sig := <-c
		log.Infof("WatchSignal\tPid=%d\tSig=%s\n", m.pid, sig.String())
		switch sig {
		case syscall.SIGHUP:
			m.reload()
		case syscall.SIGINT:
			fallthrough
		case syscall.SIGQUIT:
			m.graceful()
		case syscall.SIGTERM:
			m.rough()
		}
	}
}

func (m *master) fork(reason string) (pid int, err error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		log.Error(err)
		return
	}
	execSpec := &syscall.ProcAttr{
		Env:   append(os.Environ(), "REASON="+reason),
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd(), uintptr(fds[1])},
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
	m.stats.Lock()
	m.stats.Worker[pid] = &processStats{
		Method: make(map[string]int),
	}
	m.stats.Unlock()
	go m.waitWorker(pid)
	go m.readMsg(pid, uc)
	return
}

func (m *master) waitWorker(pid int) {
	p, _ := os.FindProcess(pid)
	state, _ := p.Wait()
	status := state.Sys().(syscall.WaitStatus)
	if status.Signaled() {
		m.children.Store(pid, workerCrash)
		log.Errorf("WorkerCrash\tPid=%d\n", pid)
	} else {
		m.children.Store(pid, workerQuit)
		log.Infof("WorkerQuit\tPid=%d\n", pid)
	}
}

func (m *master) readMsg(pid int, uc *net.UnixConn) {
	for {
		msg, err := readMsg(uc, "master", m.pid)
		if err != nil {
			break
		}
		switch msg.Typ {
		case msgWorkerStats:
			var stats map[string]int
			json.Unmarshal(msg.Value.([]byte), &stats)
			m.stats.Lock()
			m.stats.resetWorker(pid)
			for k, v := range stats {
				m.stats.Worker[pid].Id = pid
				m.stats.Worker[pid].QPS += v
				m.stats.Worker[pid].Method[k] = v
			}
			m.stats.Unlock()
		case msgWorkerTakeover:
			if m.aliveWorkers() >= m.num {
				m.notifyWorker(&message{Typ: msgWorkerQuit}, workerReload)
			}
			log.Infof("ReadMsg\tPid=%d\tMsg=%d\n", pid, msg.Typ)
		}
	}
}

func (m *master) reload() {
	f := func(key, value interface{}) bool {
		pid := key.(int)
		state := value.(string)
		if state == workerAlive {
			m.children.Store(pid, workerReload)
		}
		return true
	}
	m.children.Range(f)
	for i := 0; i < m.num; i++ {
		if _, err := m.fork(reasonReload); err != nil {
			log.Error(err)
		}
	}
}

func (m *master) notifyWorker(msg *message, states ...string) {
	f := func(key, value interface{}) bool {
		pid := key.(int)
		curState := value.(string)
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
				log.Errorf("WriteMsg\tPid=%d\tErr=%s\n", pid, err.Error())
			} else {
				log.Infof("WriteMsg\tPid=%d\tMsg=%d\n", pid, msg.Typ)
			}
		}
		return true
	}
	m.children.Range(f)
}

func (m *master) graceful() {
	m.notifyWorker(&message{Typ: msgWorkerQuit}, workerAlive, workerReload)
	select {
	case <-time.After(2 * time.Second):
		f := func(key, value interface{}) bool {
			uc := value.(*net.UnixConn)
			uc.Close()
			return true
		}
		m.uc.Range(f)
		if m.c != nil {
			m.c.Close()
		}
		os.Exit(0)
	}
}

func (m *master) rough() {
	f := func(key, value interface{}) bool {
		pid := key.(int)
		state := value.(string)
		if state == workerAlive || state == workerReload {
			log.Infof("SIGKILL\tPid=%d\tState=%s\n", pid, state)
			if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
				log.Error(err)
			}
		}
		return true
	}
	m.children.Range(f)
	os.Exit(1)
}

func (m *master) listenAndServe() {
	http.HandleFunc("/stats", m.doStats)
	http.HandleFunc("/status", m.doStatus)
	http.HandleFunc("/stats.html", m.doStatsHTML)
	if err := http.ListenAndServe(m.listenAddr, nil); err != nil {
		log.Fatal(err)
	}
}

func (m *master) aliveWorkers() int {
	count := 0
	f := func(key, value interface{}) bool {
		state := value.(string)
		if state == workerAlive || state == workerReload {
			count++
		}
		return true
	}
	m.children.Range(f)
	return count
}

func (m *master) doStatus(rsp http.ResponseWriter, req *http.Request) {
	var jsonRsp struct {
		Code    int    `json:"code"`
		Error   string `json:"error"`
		Version string `json:"version"`
	}
	count := 0
	f := func(key, value interface{}) bool {
		state := value.(string)
		if state == workerAlive {
			count++
		}
		return true
	}
	m.children.Range(f)
	if count != m.num {
		jsonRsp.Code = 207
		jsonRsp.Error = fmt.Sprintf("WorkersNum is %d, not %d\n", count, m.num)
	} else {
		jsonRsp.Code = 200
	}
	jsonRsp.Version = VERSION
	out, _ := json.Marshal(jsonRsp)
	rsp.Write(out)
}

func (m *master) doStats(rsp http.ResponseWriter, req *http.Request) {
	m.stats.Lock()
	m.stats.resetMaster()
	m.stats.Master.Id = m.pid
	for _, workerStats := range m.stats.Worker {
		for name, qps := range workerStats.Method {
			m.stats.Master.QPS += qps
			m.stats.Master.Method[name] += qps
		}
	}
	out, _ := json.Marshal(m.stats)
	m.stats.Unlock()
	rsp.Header().Set("Content-Type", "application/json; charset=utf-8")
	rsp.Header().Set("Access-Control-Allow-Origin", "*")
	rsp.Write(out)
}

func (m *master) doStatsHTML(rsp http.ResponseWriter, req *http.Request) {
	rsp.Header().Set("Content-Type", "text/html; charset=utf-8")
	rsp.Write([]byte(m.stats.html))
}
