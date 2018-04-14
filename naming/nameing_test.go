package naming

import (
	"testing"
	"time"

	etcd "github.com/coreos/etcd/clientv3"

	"github.com/tddhit/tools/log"
)

func TestRegistry(t *testing.T) {
	etcdClient, err := etcd.New(etcd.Config{
		Endpoints: []string{"localhost:2379"},
	})
	if err != nil {
		log.Fatal(err)
	}
	r := &Registry{
		Client:     etcdClient,
		Timeout:    2000,
		TTL:        1,
		Target:     "/test",
		ListenAddr: ":12345",
	}
	r.Register()
	time.Sleep(5 * time.Second)
	r.Close()
}

func TestResolver(t *testing.T) {
	etcdClient, err := etcd.New(etcd.Config{
		Endpoints: []string{"localhost:2379"},
	})
	if err != nil {
		log.Fatal(err)
	}
	r := &Resolver{
		Client:  etcdClient,
		Timeout: 2000,
	}
	addrs := r.Resolve("/test/192.168.0.5:12345")
	log.Debug(addrs)
}

func TestWatcher(t *testing.T) {
	etcdClient, err := etcd.New(etcd.Config{
		Endpoints: []string{"localhost:2379"},
	})
	if err != nil {
		log.Fatal(err)
	}
	w := &Watcher{
		Client:  etcdClient,
		Timeout: 2000,
	}
	ch, err := w.Watch("/test/192.168.0.5:12345")
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		time.Sleep(5 * time.Second)
		w.Close()
	}()
	for rsp := range ch {
		log.Debug(rsp.Canceled, rsp.Created, rsp.Err())
	}
}
