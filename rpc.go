package main

import (
	"fmt"
	l "github.com/inconshreveable/log15"
	//"io/ioutil"
	"sort"
	"sync"
	"time"

	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
)

var log = l.New("module", "rpc")

type msg struct {
	w       http.ResponseWriter
	r       *http.Request
	msgDone chan struct{}
}
type proxyInfo struct {
	proxy    *httputil.ReverseProxy
	url      string
	count    uint64
	errCount uint64
	start    time.Time
}

type JsonrpcServer struct {
	master     bool
	listener   net.Listener
	nodeServ   []*proxyInfo
	proxyNodes []*proxyInfo
	msgChan    chan *msg
	lock       sync.Mutex
	cfg        *Config
}

func (server *JsonrpcServer) DeleteNode(index int) {
	server.lock.Lock()
	defer server.lock.Unlock()
	server.nodeServ = append(server.nodeServ[:index], server.nodeServ[index+1:]...)
	return
}

func (server *JsonrpcServer) DeleteProxy(index int) {
	server.lock.Lock()
	defer server.lock.Unlock()
	server.proxyNodes = append(server.proxyNodes[:index], server.proxyNodes[index+1:]...)
	return
}
func (server *JsonrpcServer) CheckProxy(url string) bool {
	server.lock.Lock()
	defer server.lock.Unlock()
	for _, proxy := range server.proxyNodes {
		if proxy.url == url {
			return true
		}
	}
	return false
}
func (server *JsonrpcServer) CheckNodes(url string) bool {
	server.lock.Lock()
	defer server.lock.Unlock()
	for _, node := range server.nodeServ {
		if node.url == url {
			return true
		}
	}
	return false
}

func (server *JsonrpcServer) GetNodes() []*proxyInfo {
	server.lock.Lock()
	defer server.lock.Unlock()
	return server.nodeServ
}

func (server *JsonrpcServer) GetProxy() []*proxyInfo {
	server.lock.Lock()
	defer server.lock.Unlock()
	return server.proxyNodes
}

func (server *JsonrpcServer) Sort() []*proxyInfo {
	server.lock.Lock()
	defer server.lock.Unlock()
	sort.Slice(server.proxyNodes, func(i, j int) bool {
		return server.proxyNodes[i].count < server.proxyNodes[j].count
	})
	return server.proxyNodes
}

func (server *JsonrpcServer) SetMaster() {
	server.master = true
}
func (server *JsonrpcServer) Close() {
	server.listener.Close()
}

func (server *JsonrpcServer) NewServer(listenAddr string, thirdNodes []string, proxyNodes []string) {

	if server.msgChan == nil {
		server.msgChan = make(chan *msg, 256)
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		panic(err)
	}

	server.listener = listener
	//第三方公共节点
	if len(thirdNodes) != 0 {
		for _, node := range thirdNodes {
			remote, err := url.Parse(node)
			if err != nil {
				panic(err)
			}
			proxy := httputil.NewSingleHostReverseProxy(remote)
			server.nodeServ = append(server.nodeServ, &proxyInfo{proxy: proxy, url: node, start: time.Now()})
		}
	}

	//代理服务
	if len(proxyNodes) != 0 {
		for _, proxNode := range proxyNodes {
			proxy, err := url.Parse(proxNode)
			if err != nil {
				panic(err)
			}
			server.proxyNodes = append(server.proxyNodes, &proxyInfo{start: time.Now(), url: proxNode, proxy: httputil.NewSingleHostReverseProxy(proxy)})
		}
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ping" {
			fmt.Fprintf(w, "pong")
		} else {
			//fmt.Println(" r.URL.Path:", r.URL.Path)
			if server.master {
				done := make(chan struct{})
				server.msgChan <- &msg{w, r, done}
				<-done
			} else {
				if len(server.GetNodes()) > 0 {
					remote, _ := url.Parse(server.GetNodes()[0].url)
					r.Host = remote.Host
					server.GetNodes()[0].proxy.ServeHTTP(w, r)
					return
				} else {
					log.Error("no node service")
					RespError("no node service", w)
				}

			}
		}

		return

	})

	go server.msgChanProcess()
	go server.manageNodes()
	go server.manageProxy()
	go server.waitHealthNodes()
	go server.waitHealthServProx()
	go http.Serve(listener, handler)

}

func (server *JsonrpcServer) msgChanProcess() {
	var totalCount uint64
	var start time.Time
	var goroutineNum int
	var processStart = time.Now()
	sproxys := server.GetProxy()
	for smsg := range server.msgChan {
		totalCount++
		cost := time.Now().Sub(start).Seconds()
		goroutineNum++
		go func(msg *msg, proxys []*proxyInfo) {
			defer func() {
				if err := recover(); err != nil {
					fmt.Println("recover err:", err)
				}
				server.lock.Lock()
				goroutineNum--
				server.lock.Unlock()

			}()

			remote, _ := url.Parse(proxys[0].url)
			msg.r.Host = remote.Host
			//reqs := time.Now()
			proxys[0].proxy.ServeHTTP(msg.w, msg.r)
			//fmt.Println("msgChanProcess cost Milliseconds:", time.Now().Sub(reqs).Milliseconds())
			proxys[0].count++
			tick := time.NewTicker(time.Second)

			select {
			case msg.msgDone <- struct{}{}:
			case <-tick.C:
				return
			}

		}(smsg, sproxys)

		if goroutineNum > 256 {
			time.Sleep(time.Millisecond * 500)
		}

		if cost > 30 && time.Now().Sub(processStart).Seconds() > 30 {
			tps := totalCount / uint64(cost)
			log.Info("msgChanProcess", "tps by seconds:", tps, "totalcount:", totalCount, "cost:", cost,
				"msgChan capcity:", len(server.msgChan), "goroutineNum:", goroutineNum, "proxys:", len(sproxys), "callcount:", sproxys[0].count, "url:", sproxys[0].url)
			log.Info("msgChanProcess", "tatalcost:", time.Now().Sub(processStart).Minutes(), "average count per seconds:", sproxys[0].count/uint64(time.Now().Sub(processStart).Seconds()))
			sproxys = server.Sort()
			if len(sproxys) == 0 {
				RespError("no proxy service", smsg.w)
				return
			}
			start = time.Now()
			totalCount = 0
		}

	}
}

func RespError(msg string, w http.ResponseWriter) {
	fmt.Fprintf(w, msg)
}
