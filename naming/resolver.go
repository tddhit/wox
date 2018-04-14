package naming

import (
	"context"
	"time"

	etcd "github.com/coreos/etcd/clientv3"

	"github.com/tddhit/tools/log"
)

type Resolver struct {
	Client  *etcd.Client
	Timeout time.Duration
}

func (r *Resolver) Resolve(target string) (addrs []string) {
	ctx, cancel := context.WithTimeout(context.Background(), r.Timeout*time.Millisecond)
	defer cancel()
	if rsp, err := r.Client.Get(ctx, target, etcd.WithPrefix()); err != nil {
		log.Fatal(err)
	} else {
		for _, kv := range rsp.Kvs {
			addrs = append(addrs, string(kv.Value))
		}
	}
	log.Infof("resolver target:%s\taddrs:%v\n", target, addrs)
	return
}
