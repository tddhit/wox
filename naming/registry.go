package naming

import (
	"context"
	"net"
	"strings"
	"time"

	etcd "github.com/coreos/etcd/clientv3"

	"github.com/tddhit/tools/log"
)

type Registry struct {
	Client     *etcd.Client
	Timeout    time.Duration
	TTL        int64
	Target     string
	ListenAddr string
	ctx        context.Context
	cancel     context.CancelFunc
}

func (r *Registry) Register() {
	addr := GetLocalAddr(r.ListenAddr)
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		rsp, err := r.Client.Grant(ctx, r.TTL)
		if err != nil {
			log.Fatal(err)
		}
		ch, err := r.Client.KeepAlive(ctx, rsp.ID)
		if err != nil || ch == nil {
			log.Fatal(err)
		}
		if _, err = r.Client.Put(ctx, r.Target+"/"+addr, addr,
			etcd.WithLease(rsp.ID)); err != nil {
			log.Fatal(err)
		}
		go func() {
			for range ch {
				//log.Debug("KeepAlive")
			}
			log.Info("registry keepalive close.")
		}()
		done <- struct{}{}
	}()
	select {
	case <-time.After(r.Timeout * time.Millisecond):
		log.Fatalf("registry %s/%s timeout.\n", r.Target, addr)
	case <-done:
		log.Infof("registry success:%s/%s\n", r.Target, addr)
	}
	r.ctx, r.cancel = ctx, cancel
}

func (r *Registry) Close() {
	r.cancel()
}

func GetLocalAddr(listenAddr string) string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	s := strings.Split(conn.LocalAddr().String(), ":")
	if len(s) < 2 {
		log.Fatalf("invalid localAddr:%s\n", conn.LocalAddr().String())
	}
	host := s[0]
	s = strings.Split(listenAddr, ":")
	if len(s) < 2 {
		log.Fatalf("invalid listenAddr:%s\n", listenAddr)
	}
	port := s[len(s)-1]
	return host + ":" + port
}
