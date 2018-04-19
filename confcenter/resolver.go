package confcenter

import (
	"context"
	"errors"
	"os"
	"time"

	etcd "github.com/coreos/etcd/clientv3"
	yaml "gopkg.in/yaml.v2"

	"github.com/tddhit/tools/log"
)

type Resolver struct {
	Client  *etcd.Client
	Timeout time.Duration
}

func (r *Resolver) Save(target, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.Timeout)
	defer cancel()
	if rsp, err := r.Client.Get(ctx, target); err != nil {
		log.Error(err)
		return err
	} else {
		if len(rsp.Kvs) == 0 || rsp.Kvs[0].Value == nil {
			log.Error("empty config.")
			return errors.New("empty config")
		}
		m := make(map[string]interface{})
		if err := yaml.Unmarshal(rsp.Kvs[0].Value, &m); err != nil {
			log.Error(err)
			return err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_RDWR|os.O_SYNC, 0666)
		if err != nil {
			log.Error(err)
			return err
		}
		out, _ := yaml.Marshal(m)
		file.Write(out)
		file.Sync()
		file.Close()
		time.Sleep(10 * time.Millisecond)
		log.Infof("resolver target:%s\n", target)
	}
	return nil
}
