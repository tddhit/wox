package wox

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httputil"
	"net/http/pprof"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/julienschmidt/httprouter"
	//opentracing "github.com/opentracing/opentracing-go"
	"golang.org/x/net/http2"
	"golang.org/x/time/rate"

	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox/naming"
	"github.com/tddhit/wox/option"
	//"github.com/tddhit/wox/tracing"
)

type requests struct {
	sync.Mutex
	Data map[string]int
}

type HTTPServer struct {
	opt      *option.Server
	mux      *httprouter.Router
	h1Server *http.Server
	h2Server *http2.Server
	listener net.Listener
	statsCh  chan []byte
	quitCh   chan struct{}
	requests *requests

	//tracer        opentracing.Tracer
	//tracingCloser io.Closer

	closeHandler sync.Once
}

func NewHTTPServer(opt *option.Server) *HTTPServer {
	if os.Getenv(FORK) != "1" {
		return &HTTPServer{opt: opt}
	}
	mux := httprouter.New()
	mux.HandlerFunc("GET", "/debug/pprof/", pprof.Index)
	mux.HandlerFunc("GET", "/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandlerFunc("GET", "/debug/pprof/profile", pprof.Profile)
	mux.HandlerFunc("GET", "/debug/pprof/symbol", pprof.Symbol)
	mux.HandlerFunc("GET", "/debug/pprof/trace", pprof.Trace)
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
	s.mux.HandlerFunc("GET", "/status",
		func(rsp http.ResponseWriter, req *http.Request) {
			rsp.Write([]byte(`{"code":200}`))
			return
		})

	//tracer, closer, err := tracing.Init(opt.Registry, opt.TracingAgentAddr)
	//if err != nil {
	//	log.Fatal(err, opt.Registry)
	//}
	//opentracing.SetGlobalTracer(tracer)
	//s.tracer = tracer
	//s.tracingCloser = closer

	go s.calcQPS()
	return s
}

func (s *HTTPServer) Stats() <-chan []byte {
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

func (s *HTTPServer) AddHandler(
	pattern string,
	req, rsp interface{},
	h HandlerFunc,
	contentType string) {

	if os.Getenv(FORK) != "1" {
		return
	}
	if api, ok := s.opt.Api[pattern]; ok {
		if api.Limit > 0 && api.Burst > 0 {
			h = withLimit(rate.Limit(api.Limit), api.Burst, h)
		}
	}
	f := withParse(s, pattern, req, rsp, h, contentType)
	s.mux.Handler("POST", pattern, f)
}

func (s *HTTPServer) AddProxyUpstream(opt *option.Upstream) error {
	if os.Getenv(FORK) != "1" {
		return nil
	}
	r := &naming.Resolver{
		Client:  GlobalEtcdClient(),
		Timeout: 2 * time.Second,
	}
	addrs := r.Resolve(opt.Registry)
	var urls []*url.URL
	for _, addr := range addrs {
		url := &url.URL{
			Scheme: "http",
			Host:   addr,
		}
		urls = append(urls, url)
	}
	if len(urls) == 0 {
		return errUnavailableUpstream
	}
	var counter uint64
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			counter := atomic.AddUint64(&counter, 1)
			index := counter % uint64(len(urls))
			remote := urls[index]
			req.URL.Scheme = remote.Scheme
			req.URL.Host = remote.Host
		},
	}
	for _, location := range opt.Locations {
		log.Debug(location)
		s.mux.Handler(location.Method, location.Pattern, proxy)
	}
	return nil
}

func (h *HTTPServer) Serve() (err error) {
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

func (h *HTTPServer) Close(quitCh chan struct{}) {
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
		//h.tracingCloser.Close()
	})
}
