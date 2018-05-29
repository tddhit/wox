package wox

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"time"

	"golang.org/x/time/rate"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/tddhit/tools/log"
)

type HandlerFunc func(ctx context.Context, req, rsp interface{}) (err error)

type Decorator func(HandlerFunc) HandlerFunc

func Decorate(h HandlerFunc, decorators ...Decorator) HandlerFunc {
	for _, d := range decorators {
		h = d(h)
	}
	return h
}

func withLimit(limit rate.Limit, burst int, do HandlerFunc) HandlerFunc {
	return func(ctx context.Context, req, rsp interface{}) (err error) {
		l := rate.NewLimiter(limit, burst)
		if l.Allow() {
			err = do(ctx, req, rsp)
			return
		}
		return Err503Rsp
	}
}

func withParse(
	s *HTTPServer,
	pattern string,
	realReq interface{},
	realRsp interface{},
	do HandlerFunc,
	contentType string) http.HandlerFunc {

	return func(rsp http.ResponseWriter, req *http.Request) {
		// stats
		s.requests.Lock()
		s.requests.Data[pattern]++
		s.requests.Unlock()

		// tracing
		var span opentracing.Span
		spanCtx, _ := s.tracer.Extract(opentracing.HTTPHeaders,
			opentracing.HTTPHeadersCarrier(req.Header))
		span = s.tracer.StartSpan(pattern, ext.RPCServerOption(spanCtx))
		defer span.Finish()

		var output []byte
		start := time.Now()
		reqType := reflect.TypeOf(realReq).Elem()
		rspType := reflect.TypeOf(realRsp).Elem()
		newRealReq := reflect.New(reqType).Interface()
		newRealRsp := reflect.New(rspType).Interface()
		body, _ := ioutil.ReadAll(req.Body)
		var err error
		if contentType == "application/json" {
			err = json.Unmarshal(body, newRealReq)
		} else if contentType == "application/x-www-form-urlencoded" {
			newRealReq, err = url.ParseQuery(string(body))
		}
		if err != nil {
			log.Error(err)
			rsp.Write([]byte(Err400Rsp.Error()))
			return
		}
		input, _ := json.Marshal(newRealReq)
		log.Infof("type=http\treq=%s\n", input)
		ctx := opentracing.ContextWithSpan(context.Background(), span)
		err = do(ctx, newRealReq, newRealRsp)
		if err != nil {
			output = []byte(err.Error())
		} else {
			output, _ = json.Marshal(newRealRsp)
		}
		rsp.Header().Set("Content-Type", "application/json; charset=utf-8")
		rsp.Header().Set("Access-Control-Allow-Origin", "*")
		rsp.Write(output)

		end := time.Now()
		elapsed := end.Sub(start)
		log.Infof("type=http\treq=%s\trsp=%s\telapsed=%d\n", input, output, elapsed/1000000)
	}
}
