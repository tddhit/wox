package naming

import (
	"context"
	"errors"
	"time"

	etcd "github.com/coreos/etcd/clientv3"

	"github.com/tddhit/tools/log"
)

var (
	errTimeout = errors.New("timeout")
)

type Watcher struct {
	Client  *etcd.Client
	Timeout time.Duration
	ctx     context.Context
	cancel  context.CancelFunc
}

func (w *Watcher) Watch(target string) (watchCh etcd.WatchChan, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		watchCh = w.Client.Watch(ctx, target, etcd.WithPrefix())
		done <- struct{}{}
	}()
	select {
	case <-time.After(w.Timeout * time.Millisecond):
		err = errTimeout
		log.Errorf("watch %s timeout.\n", target)
	case <-done:
		log.Infof("watch %s success.\n", target)
	}
	w.ctx, w.cancel = ctx, cancel
	return
}

func (w *Watcher) Close() {
	w.cancel()
}
