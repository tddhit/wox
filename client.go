package wox

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"

	"github.com/tddhit/wox/option"
)

type client struct {
	*http.Client
	addr string
}

func NewClient(opt option.Client, addr string) *client {
	var transport http.RoundTripper
	if opt.HTTPVersion == "2.0" {
		transport = &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
				d := &net.Dialer{
					Timeout:   time.Duration(opt.ConnectTimeout) * time.Millisecond,
					KeepAlive: time.Duration(opt.KeepAlive) * time.Millisecond,
				}
				return d.Dial(network, addr)
			},
		}
	} else {
		transport = &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   time.Duration(opt.ConnectTimeout) * time.Millisecond,
				KeepAlive: time.Duration(opt.KeepAlive) * time.Millisecond,
			}).Dial,
			MaxIdleConns:    opt.MaxIdleConns,
			IdleConnTimeout: time.Duration(opt.IdleConnTimeout) * time.Millisecond,
		}
	}
	c := &client{
		Client: &http.Client{
			Transport: transport,
			Timeout:   time.Duration(opt.ReadTimeout) * time.Millisecond,
		},
		addr: addr,
	}
	return c
}

func (c *client) Request(method, path string, header http.Header, body []byte) (rspBody []byte, err error) {
	if method != "POST" {
		err = errUnsupportedMethod
		return
	}
	var (
		req *http.Request
		rsp *http.Response
	)
	bodyBytes := bytes.NewReader(body)
	url := fmt.Sprintf("http://%s%s", c.addr, path)
	if req, err = http.NewRequest(method, url, bodyBytes); err != nil {
		return
	}
	req.Header = header
	if rsp, err = c.Do(req); err != nil {
		return
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != 200 {
		err = errors.New(rsp.Status)
		return
	}
	if rspBody, err = ioutil.ReadAll(rsp.Body); err != nil {
		return
	}
	return
}
