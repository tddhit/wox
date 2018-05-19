package confcenter

import (
	"io/ioutil"

	yaml "gopkg.in/yaml.v2"

	"github.com/tddhit/wox/option"
)

type Conf interface {
	Server() *option.Server
}

func NewConf(path string, conf Conf) error {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(file, conf)
}
