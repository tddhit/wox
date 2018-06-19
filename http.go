package wox

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/julienschmidt/httprouter"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/http2"
	"golang.org/x/time/rate"

	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox/naming"
	"github.com/tddhit/wox/option"
	"github.com/tddhit/wox/tracing"
)

var (
	httpQPS *prometheus.GaugeVec
)

func init() {
	httpQPS = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "http_qps",
			Help: "http qps",
		},
		[]string{"endpoint"},
	)
	prometheus.MustRegister(httpQPS)
}

type HTTPServer struct {
	opt           *option.Server
	mux           *httprouter.Router
	h1Server      *http.Server
	h2Server      *http2.Server
	listener      net.Listener
	closeHandler  sync.Once
	tracer        opentracing.Tracer
	tracingCloser io.Closer
}

func NewHTTPServer(opt *option.Server) *HTTPServer {
	if os.Getenv(FORK) != "1" {
		return &HTTPServer{opt: opt}
	}
	mux := httprouter.New()
	writeTimeout := 5 * time.Second
	if opt.WriteTimeout > 0 {
		writeTimeout = time.Duration(opt.WriteTimeout)
	}
	timeoutHandler := http.TimeoutHandler(mux, writeTimeout*time.Millisecond, Err503Rsp.Error())
	s := &HTTPServer{
		opt: opt,
		mux: mux,
	}
	addr := naming.GetLocalAddr(opt.TransportAddr)
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
	s.mux.Handler("GET", "/metrics", promhttp.Handler())

	tracer, closer, err := tracing.Init(opt.Registry, opt.TracingAgentAddr)
	if err != nil {
		log.Fatal(err, opt.Registry)
	}
	opentracing.SetGlobalTracer(tracer)
	s.tracer = tracer
	s.tracingCloser = closer

	go s.calcQPS()
	return s
}

func (s *HTTPServer) calcQPS() {
	tick := time.Tick(1 * time.Second)
	for range tick {
		globalStats().calculate()
	}
}

func (s *HTTPServer) AddHandler(pattern string, req, rsp interface{},
	h HandlerFunc, contentType string) {

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

			pattern := req.URL.Path
			// metrics
			httpRequestCount.WithLabelValues(pattern).Inc()

			// stats
			globalStats().Lock()
			globalStats().Method[pattern]++
			globalStats().Unlock()

			// tracing
			var span opentracing.Span
			spanCtx, _ := s.tracer.Extract(opentracing.HTTPHeaders,
				opentracing.HTTPHeadersCarrier(req.Header))
			span = s.tracer.StartSpan(pattern, ext.RPCServerOption(spanCtx))
			defer span.Finish()

			ext.SpanKindRPCClient.Set(span)
			ext.HTTPMethod.Set(span, "POST")
			span.Tracer().Inject(
				span.Context(),
				opentracing.HTTPHeaders,
				opentracing.HTTPHeadersCarrier(req.Header),
			)
		},
	}
	for _, location := range opt.Locations {
		log.Debug(location)
		s.mux.Handler(location.Method, location.Pattern, proxy)
	}
	return nil
}

func (h *HTTPServer) Serve(startC chan struct{}) (err error) {
	close(startC)
	defer func() {
		if err := recover(); err != nil {
			if err == http.ErrServerClosed {
				log.Info("worker server closed.", os.Getpid())
			} else {
				log.Error(err, os.Getpid())
			}
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

func (h *HTTPServer) Close() {
	h.closeHandler.Do(func() {
		if h.h2Server == nil {
			ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
			h.h1Server.Shutdown(ctx)
		} else {
			h.listener.Close()
		}
		h.tracingCloser.Close()
	})
}
