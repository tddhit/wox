package wox

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/time/rate"

	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox/naming"
	"github.com/tddhit/wox/option"
)

type requests struct {
	sync.Mutex
	Data map[string]int
}

type HTTPServer struct {
	opt      option.Server
	mux      *http.ServeMux
	h1Server *http.Server
	h2Server *http2.Server
	listener net.Listener
	statsCh  chan []byte
	quitCh   chan struct{}
	requests *requests

	closeHandler sync.Once
}

func NewHTTPServer(opt option.Server) *HTTPServer {
	if os.Getenv(FORK) != "1" {
		return &HTTPServer{opt: opt}
	}
	mux := http.NewServeMux()
	mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	writeTimeout := 5 * time.Second
	if opt.WriteTimeout > 0 {
		writeTimeout = time.Duration(opt.WriteTimeout)
	}
	timeoutHandler := http.TimeoutHandler(mux, writeTimeout*time.Millisecond, Err503Rsp.Error())
	s := &HTTPServer{
		opt:     opt,
		mux:     mux,
		statsCh: make(chan []byte, 100),
		quitCh:  make(chan struct{}),
		requests: &requests{
			Data: make(map[string]int),
		},
	}
	addr := naming.GetLocalAddr(opt.Addr)
	if opt.HTTPVersion == "2.0" {
		s.h1Server = &http.Server{
			Addr:        addr,
			Handler:     timeoutHandler,
			ReadTimeout: time.Duration(opt.ReadTimeout) * time.Millisecond,
		}
		s.h2Server = &http2.Server{
			IdleTimeout: time.Duration(opt.IdleTimeout) * time.Millisecond,
		}
	} else {
		s.h1Server = &http.Server{
			Addr:         addr,
			Handler:      timeoutHandler,
			ReadTimeout:  time.Duration(opt.ReadTimeout) * time.Millisecond,
			WriteTimeout: time.Duration(opt.WriteTimeout) * time.Millisecond,
			IdleTimeout:  time.Duration(opt.IdleTimeout) * time.Millisecond,
		}
	}
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
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
	s.listener = listener
	s.mux.HandleFunc("/status", func(rsp http.ResponseWriter, req *http.Request) {
		rsp.Write([]byte(`{"code":200}`))
		return
	})

	go s.calcQPS()
	return s
}

func (s *HTTPServer) stats() <-chan []byte {
	return s.statsCh
}

func (s *HTTPServer) calcQPS() {
	tick := time.Tick(1 * time.Second)
	for range tick {
		s.requests.Lock()
		out, _ := json.Marshal(s.requests.Data)
		s.requests.Unlock()
		s.statsCh <- out
		s.requests.Lock()
		for k, _ := range s.requests.Data {
			s.requests.Data[k] = 0
		}
		s.requests.Unlock()
	}
}

func (s *HTTPServer) AddHandler(pattern string, req, rsp interface{},
	h HandlerFunc, chans ...chan *message) {
	if os.Getenv(FORK) != "1" {
		return
	}
	if api, ok := s.opt.Api[pattern]; ok {
		if api.Limit > 0 && api.Burst > 0 {
			h = withLimit(rate.Limit(api.Limit), api.Burst, h)
		}
	}
	f := withJsonParse(s, pattern, req, rsp, h)
	s.mux.Handle(pattern, f)
}

func (h *HTTPServer) serve() (err error) {
	defer func() {
		if err := recover(); err != nil {
			if err == http.ErrServerClosed {
				log.Info("worker server closed.", os.Getpid())
			} else {
				log.Error(err, os.Getpid())
			}
			close(h.quitCh)
		}
	}()
	if h.opt.HTTPVersion == "2.0" {
		for {
			conn, err := h.listener.Accept()
			if err != nil {
				panic(err)
			}
			go func() {
				h.h2Server.ServeConn(conn, &http2.ServeConnOpts{BaseConfig: h.h1Server})
			}()
		}
	} else {
		if err := h.h1Server.Serve(h.listener); err != nil {
			panic(err)
		}
	}
	return
}

func (h *HTTPServer) ListenAddr() string {
	return naming.GetLocalAddr(h.opt.Addr)
}

func (h *HTTPServer) statusAddr() (addrs [2]string) {
	s := strings.Split(h.opt.Addr, ":")
	port, _ := strconv.Atoi(s[len(s)-1])
	s[len(s)-1] = strconv.Itoa(port + 1)
	defaultAddr := strings.Join(s, ":")
	if h.opt.StatusAddr != "" {
		addrs[0] = h.opt.StatusAddr
	} else {
		addrs[0] = defaultAddr
	}
	addrs[1] = defaultAddr
	return
}

func (h *HTTPServer) close(quitCh chan struct{}) {
	h.closeHandler.Do(func() {
		if h.h2Server == nil {
			ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
			h.h1Server.Shutdown(ctx)
		} else {
			h.listener.Close()
		}
		go func() {
			select {
			case <-h.quitCh:
				close(quitCh)
			}
		}()
	})
}
