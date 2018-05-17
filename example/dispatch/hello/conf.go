package hello

import (
	"io/ioutil"

	"gopkg.in/yaml.v2"

	httpopt "github.com/tddhit/wox/option"
)

type Conf struct {
	LogPath    string         `yaml:"logpath"`
	LogLevel   int            `yaml:"loglevel"`
	Etcd       []string       `yaml:"etcd"`
	HTTPServer httpopt.Server `yaml:"httpServer"`
}

func NewConf(path string) (*Conf, error) {
	c := &Conf{}
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(file, c)
	if err != nil {
		return nil, err
	}
	return c, nil
}
