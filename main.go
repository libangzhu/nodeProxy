package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"
)

var (
	quit       = make(chan bool)
	signalChan = make(chan os.Signal, 1)
	nodeAddr   = flag.String("n", "", "the third chain node")
	listenAddr = flag.String("l", "", "listen addr")
	cfgPath    = flag.String("conf", "./proxy.toml", "proxy config file")
)

func main() {

	flag.Parsed()
	runtime.GOMAXPROCS(runtime.NumCPU())
	go func() {
		for range signalChan {
			fmt.Println("即将退出服务,after 5 seconds...")
			time.Sleep(time.Second * 5)
			quit <- true
		}
	}()

	cfg := InitCfg(*cfgPath)
	log.Debug("main", "show cfg", cfg)

	jrpcProxy := new(JsonrpcServer)
	if cfg.Title == "master" {
		jrpcProxy.SetMaster()
	}
	jrpcProxy.cfg = cfg
	jrpcProxy.NewServer(cfg.Server.ListenAddr, cfg.Node.Nodes, cfg.ProxySever.Servers)

	<-quit

}
