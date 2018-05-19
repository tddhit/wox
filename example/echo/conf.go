package main

import (
	httpopt "github.com/tddhit/wox/option"
)

type Conf struct {
	LogPath    string          `yaml:"logpath"`
	LogLevel   int             `yaml:"loglevel"`
	HTTPServer *httpopt.Server `yaml:"httpServer"`
}

func (c *Conf) Server() *httpopt.Server {
	return c.HTTPServer
}
