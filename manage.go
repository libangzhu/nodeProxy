package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/ethclient"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

type touchNodes interface {
	FetchNodeBlockNum() (uint64, error)
}

type EthSeries struct {
	url string
}

func (e *EthSeries) FetchNodeBlockNum() (uint64, error) {
	cli, err := ethclient.Dial(e.url)
	if err != nil {
		return 0, err
	}
	return cli.BlockNumber(context.Background())
}

type Trx struct {
	url string
}

type BlockHeight struct {
	Header Block_header `json:"block_header"`
}
type Block_header struct {
	Raw_data          RawData `json:"raw_data"`
	Witness_signature string  `json:"witness_signature,omitempty"`
}

type RawData struct {
	Number          uint64 `json:"number,omitempty"`
	Timestamp       uint64 `json:"timestamp,omitempty"`
	TxTrieRoot      string `json:"txTrieRoot,omitempty"`
	Witness_address string `json:"witness_address,omitempty"`
	ParentHash      string `json:"parentHash,omitempty"`
	Version         int    `json:"version,omitempty"`
}

func (t *Trx) FetchNodeBlockNum() (uint64, error) {
	urlpath := "/wallet/getnowblock"
	hpReq, err := http.NewRequest("POST", t.url+urlpath, bytes.NewBuffer(nil))
	if err != nil {
		return 0, err
	}
	cli := http.DefaultClient
	dresp, err := cli.Do(hpReq)
	if err != nil {
		return 0, err
	}
	defer dresp.Body.Close()
	b, err := ioutil.ReadAll(dresp.Body)
	if err != nil {
		return 0, err
	}

	var blockHeader BlockHeight
	err = json.Unmarshal(b, &blockHeader)
	return blockHeader.Header.Raw_data.Number, err
}

func NewNodeCli(url string, cointype string) touchNodes {
	switch cointype {
	case "ETH", "BSC":
		e := &EthSeries{
			url: url,
		}
		return e
	case "TRX":
		t := &Trx{url: url}
		return t
	default:
		panic(fmt.Sprintf("no support cointype:%v", cointype))
	}
}
func (server *JsonrpcServer) manageNodes() {
	if server.master {
		log.Info("manageNodes", "break ,server.master:", server.master)
		return
	}

	for {
		time.Sleep(time.Second * 5)
		ndoes := server.GetNodes()
		for i, node := range ndoes {
			if server.cfg.Cointype == "" {
				server.cfg.Cointype = "ETH"
			}
			ncli := NewNodeCli(node.url, server.cfg.Cointype)
			blockNum, err := ncli.FetchNodeBlockNum()
			if err != nil || blockNum == 0 {
				log.Error("manageNodes", "blockNum:", err)
				server.DeleteNode(i) // 删除有问题的节点
				continue
			}
			node.count++
			log.Info("manageNodes", "blockNum:", blockNum, "url:", node.url, "index:", i, "count:", node.count, "cost minutes time:", time.Now().Sub(node.start).Minutes(), "cointype:", server.cfg.Cointype)

		}

	}
}

func (server *JsonrpcServer) waitHealthNodes() {
	if server.master {
		log.Info("waitHealthNodes", "break ,server.master:", server.master)
		return
	}
	for {
		for _, node := range server.cfg.Node.Nodes {
			if server.CheckNodes(node) {
				continue
			}
			ncli := NewNodeCli(node, server.cfg.Cointype)
			blockNum, err := ncli.FetchNodeBlockNum()
			if err != nil {
				log.Error("waitHealthServNodes", "blockNum:", err)
				continue
			}

			log.Info("waitHealthServNodes", "blockNum:", blockNum, "cointype:", server.cfg.Cointype)
			proxy, err := url.Parse(node)
			if err == nil {
				server.nodeServ = append(server.nodeServ, &proxyInfo{start: time.Now(), url: node, proxy: httputil.NewSingleHostReverseProxy(proxy)})
			}
			time.Sleep(time.Millisecond * 500)
		}
	}
}
func (server *JsonrpcServer) waitHealthServProx() {

	if !server.master {
		log.Info("waitHealthServNodes", "break ,server.master:", server.master)
		return
	}

	for {
		time.Sleep(time.Second * 2)
		for _, info := range server.cfg.ProxySever.Servers {
			if server.CheckProxy(info) {
				continue
			}
			//fmt.Println("info", info)
			pong, err := pingServer(info)
			if err != nil {
				log.Error("waitHealthServProx", "err:", err)
				continue
			}

			if pong == "pong" {
				proxy, err := url.Parse(info)
				if err == nil {
					server.lock.Lock()
					server.proxyNodes = append(server.proxyNodes, &proxyInfo{start: time.Now(), url: info, proxy: httputil.NewSingleHostReverseProxy(proxy)})
					server.lock.Unlock()
				}
			}

		}
	}
}

func pingServer(url string) (string, error) {
	url = url + "/ping"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(nil))
	if err != nil {
		log.Error("pingServer", " connect err", err)
		return "", err
	}
	defer req.Body.Close()
	cli := http.DefaultClient
	resp, err := cli.Do(req)
	if err != nil {
		return "", err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error("pingServer", "read err", err)
		return "", err
	}

	return string(body), nil
}

func (server *JsonrpcServer) manageProxy() {
	if !server.master {
		log.Info("manageProxy", "break ,server.master:", server.master)
		return
	}

	for {
		proxys := server.GetProxy()
		for i, proxy := range proxys {
			time.Sleep(time.Second)
			resp, err := pingServer(proxy.url)
			if err != nil {
				server.DeleteProxy(i)
				log.Error("manageProxy", "err:", err)
				continue
			}
			//fmt.Println("ping proxy:", resp)
			if resp == "pong" {
				continue
			}

		}
	}
}
