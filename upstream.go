package wox

import (
	"context"
	"encoding/json"
	rhttp "net/http"
	"sync/atomic"
	"time"

	"github.com/sony/gobreaker"

	"github.com/tddhit/tools/consistent"
	"github.com/tddhit/tools/log"
	"github.com/tddhit/wox/naming"
	"github.com/tddhit/wox/option"
)

const (
	RoundRobin = iota
	ConsistentHash
)

type Upstream struct {
	Api      map[string]*api
	cs       []*client
	c        map[string]*client
	counter  uint64
	policy   uint8
	hashRing *consistent.HashRing
	cb       *gobreaker.CircuitBreaker
}

func NewUpstream(
	opt option.Upstream,
	policy uint8) (*Upstream, error) {

	u := &Upstream{
		Api:    make(map[string]*api),
		c:      make(map[string]*client),
		policy: policy,
	}
	for k, v := range opt.Api {
		u.Api[k] = &api{
			method: v.Method,
			header: v.Header,
			path:   v.Path,
		}
	}
	r := &naming.Resolver{
		Client:  GlobalEtcdClient(),
		Timeout: 2 * time.Second,
	}
	addrs := r.Resolve(opt.Registry)
	rnodes := make([]consistent.RNode, len(addrs))
	for i, addr := range addrs {
		c := NewClient(opt.Client, addr)
		u.cs = append(u.cs, c)
		u.c[addr] = c
		rnodes[i] = consistent.RNode{addr, 1}
	}
	u.hashRing = consistent.NewHashRing(rnodes, 10)
	st := gobreaker.Settings{
		Name:        opt.Registry,
		MaxRequests: 10,
		Interval:    60 * time.Second,
		Timeout:     10 * time.Second,
	}
	var (
		requests uint32  = 60
		ratio    float64 = 0.6
	)
	if opt.CircuitBreaker.MaxRequests > 0 {
		st.MaxRequests = opt.CircuitBreaker.MaxRequests
	}
	if opt.CircuitBreaker.Interval > 0 {
		st.Interval = time.Duration(opt.CircuitBreaker.Interval) * time.Millisecond
	}
	if opt.CircuitBreaker.Timeout > 0 {
		st.Timeout = time.Duration(opt.CircuitBreaker.Timeout) * time.Millisecond
	}
	if opt.CircuitBreaker.TotalRequests > 0 {
		if opt.CircuitBreaker.FailureRatio > 0 {
			requests = opt.CircuitBreaker.TotalRequests
			ratio = opt.CircuitBreaker.FailureRatio
		}
	}
	st.ReadyToTrip = func(counts gobreaker.Counts) bool {
		failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
		return counts.Requests >= requests && failureRatio >= ratio
	}
	u.cb = gobreaker.NewCircuitBreaker(st)
	return u, nil
}

func (u *Upstream) NewRequest(ctx context.Context, a *api,
	req, rsp interface{}, hashKey string) error {

	f := func() (interface{}, error) {
		if u.policy == ConsistentHash && len(u.c) == 0 {
			return nil, errUnavailableUpstream
		} else if len(u.cs) == 0 {
			return nil, errUnavailableUpstream
		}
		var c *client
		if u.policy == ConsistentHash {
			rnode := u.hashRing.GetNode(hashKey)
			if _, ok := u.c[rnode.Id]; !ok {
				return nil, errUnavailableUpstream
			}
			c = u.c[rnode.Id]
		} else {
			counter := atomic.AddUint64(&u.counter, 1)
			index := counter % uint64(len(u.cs))
			c = u.cs[index]
		}
		err := a.newRequest(ctx, c, req, rsp)
		return rsp, err
	}
	_, err := u.cb.Execute(f)
	return err
}

type api struct {
	path   string
	method string
	header rhttp.Header
}

func (a *api) newRequest(ctx context.Context, c *client, req, rsp interface{}) (err error) {
	body, err := a.buildBody(req)
	if err != nil {
		return
	}
	rspBody, err := a.send(ctx, c, body)
	if err != nil {
		return
	}
	err = a.parseResponse(rspBody, rsp)

	return
}

func (a *api) buildBody(req interface{}) (body []byte, err error) {
	if _, ok := a.header["Content-Type"]; !ok {
		err = errUnsupportedContentType
		return
	}
	if len(a.header["Content-Type"]) == 0 {
		err = errUnsupportedContentType
		return
	}
	if a.header["Content-Type"][0] == "application/json" {
		body, err = json.Marshal(req)
	} else { //todo:支持更多Content-Type
		err = errUnsupportedContentType
	}
	return
}

func (a *api) send(ctx context.Context, c *client, reqBody []byte) (rspBody []byte, err error) {
	start := time.Now()
	rspBody, err = c.Request(ctx, a.method, a.path, a.header, reqBody)
	end := time.Now()
	elapsed := end.Sub(start)
	log.Infof("type=upstream\taddr=%s\tpath=%s\treq=%s\trsp=%s\telapsed=%d\n",
		c.addr, a.path, string(reqBody), string(rspBody), elapsed/1000000)
	return
}

func (a *api) parseResponse(body []byte, rsp interface{}) (err error) {
	err = json.Unmarshal(body, rsp)
	return
}
