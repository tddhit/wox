package wox

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"reflect"
	"time"

	"golang.org/x/time/rate"

	"github.com/tddhit/tools/log"
)

type HandlerFunc func(req, rsp interface{}) (err error)

type Decorator func(HandlerFunc) HandlerFunc

func Decorate(h HandlerFunc, decorators ...Decorator) HandlerFunc {
	for _, d := range decorators {
		h = d(h)
	}
	return h
}

func withLimit(limit rate.Limit, burst int, do HandlerFunc) HandlerFunc {
	return func(req, rsp interface{}) (err error) {
		l := rate.NewLimiter(limit, burst)
		if l.Allow() {
			err = do(req, rsp)
			return
		}
		return Err503Rsp
	}
}

func withJsonParse(s *HTTPServer, pattern string, jsonReq, jsonRsp interface{}, do HandlerFunc) http.HandlerFunc {
	return func(rsp http.ResponseWriter, req *http.Request) {
		s.requests.Lock()
		s.requests.Data[pattern]++
		s.requests.Unlock()
		var output []byte
		start := time.Now()
		reqType := reflect.TypeOf(jsonReq).Elem()
		rspType := reflect.TypeOf(jsonRsp).Elem()
		newJsonReq := reflect.New(reqType).Interface()
		newJsonRsp := reflect.New(rspType).Interface()
		body, _ := ioutil.ReadAll(req.Body)
		err := json.Unmarshal(body, newJsonReq)
		if err != nil {
			log.Error(err)
			rsp.Write([]byte(Err400Rsp.Error()))
			return
		}
		input, _ := json.Marshal(newJsonReq)
		log.Infof("type=http\treq=%s\n", input)
		err = do(newJsonReq, newJsonRsp)
		if err != nil {
			output = []byte(err.Error())
		} else {
			output, _ = json.Marshal(newJsonRsp)
		}
		rsp.Header().Set("Content-Type", "application/json; charset=utf-8")
		rsp.Header().Set("Access-Control-Allow-Origin", "*")
		rsp.Write(output)
		end := time.Now()
		elapsed := end.Sub(start)
		log.Infof("type=http\treq=%s\trsp=%s\telapsed=%d\n", input, output, elapsed/1000000)
	}
}
