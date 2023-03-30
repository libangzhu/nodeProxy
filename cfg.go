package main

import (
	"fmt"
	tml "github.com/BurntSushi/toml"
)

type Config struct {
	Title      string     `toml:"title"`
	Cointype   string     `toml:"cointype"`
	Server     server     `toml:"server"`
	ProxySever proxySever `toml:"proxySevers"`
	Node       NodeCfg    `toml:"node"`
}
type proxySever struct {
	Servers []string `toml:"servers"`
}
type server struct {
	ListenAddr string `toml:"listenAddr"`
}

type NodeCfg struct {
	Nodes []string `toml:"nodes"`
}

func InitCfg(path string) *Config {
	var cfg Config
	if _, err := tml.DecodeFile(path, &cfg); err != nil {
		fmt.Println("DecodeFile:", err)
		panic(err)

	}

	return &cfg
}
