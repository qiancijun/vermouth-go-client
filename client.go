package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	jsoniter "github.com/json-iterator/go"
	"google.golang.org/grpc"
)

const (
	Success = "success"
	GetMethod = "GET"
	PostMethod = "POST"
)

type RpcClient struct {
	addr       string
	conn       *grpc.ClientConn
	service    VermouthGrpcClient
	httpClient *http.Client

	port      int64
	prefix    string
	localAddr string
}

func NewVermouthRpcClient(remoteAddr string) *RpcClient {
	client := &RpcClient{
		addr: remoteAddr,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				Dial: (&net.Dialer{
					Timeout:   10 * time.Second, //限制建立TCP连接的时间
					KeepAlive: 10 * time.Second,
				}).Dial,
				ForceAttemptHTTP2:     true,
				MaxIdleConnsPerHost:   10,
				MaxConnsPerHost:       10,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
	}
	con, err := grpc.Dial(remoteAddr, grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	client.conn = con
	client.service = NewVermouthGrpcClient(con)
	return client
}

func (client *RpcClient) RegisterToProxy(port int64, prefix, addr string, opt ...string) error {
	req := &RegisterToProxyReq{
		Port:        port,
		Prefix:      prefix,
		BalanceMode: "round-robin",
		LocalAddr:   addr,
		Static:      false,
	}
	if len(opt) > 0 {
		req.BalanceMode = opt[0]
	}
	res, err := client.service.RegisterToProxy(context.Background(), req)
	if err != nil {
		return err
	}
	if res.GetMessage() != Success {
		return errors.New("server has encounter some error, please check server log")
	}
	// 注册成功，记录信息
	client.port, client.prefix, client.localAddr = port, prefix, addr

	// 当程序收到中断指令会自动触发注销
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt)

	go func() {
		<-interruptChan
		if err := client.Cancel(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}()
	return nil
}

func (client *RpcClient) Cancel() error {
	req := &CancalReq{
		Port:      client.port,
		Prefix:    client.prefix,
		LocalAddr: client.localAddr,
	}
	res, err := client.service.Cancel(context.Background(), req)
	if err != nil {
		return err
	}
	if res.GetMessage() != Success {
		return errors.New("server has encounter some error, please check server log")
	}
	return nil
}

func (client *RpcClient) JoinCluster(tcpAddress, name string) error {
	req := &JoinClusterReq{
		NodeName:   name,
		TcpAddress: tcpAddress,
	}
	res, err := client.service.JoinCluster(context.Background(), req)
	if err != nil {
		return err
	}
	if res.GetMessage() != Success {
		return errors.New("server has encounter some error, please check server log")
	}
	return nil
}

func (client *RpcClient) loadBalance(port int64, prefix, fact string) (string, error) {
	req := &LoadBalanceReq{
		Port:   port,
		Prefix: prefix,
		Fact:   fact,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := client.service.LoadBalance(ctx, req)
	if err != nil {
		return "", err
	}
	if res.Message == "" {
		return "", errors.New("can't find any host")
	}
	return res.GetMessage(), nil
}

func (client *RpcClient) sendHttpRequest(method, url string, data []byte) ([]byte, error) {
	buf := bytes.NewBuffer(data)
	req, err := http.NewRequest(method, url, buf)
	if err != nil {
		return nil, err
	}
	res, err := client.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	
	body := res.Body
	defer body.Close()
	data, err = io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (client *RpcClient) InvokeMethod(port int64, prefix, url, method string, body interface{}) []byte {
	host, err := client.loadBalance(port, prefix, client.localAddr)
	if err != nil {
		fmt.Println(err.Error())
		return nil
	}
	reqUrl := fmt.Sprintf("http://%s%s", host, url)
	var buf bytes.Buffer
	if err := jsoniter.NewEncoder(&buf).Encode(body); err != nil {
		fmt.Println(err.Error())
		return nil
	}
	res, err := client.sendHttpRequest(method, reqUrl, buf.Bytes())
	if err != nil {
		fmt.Println(err.Error())
		return nil
	}
	return res
}

func (client *RpcClient) SetHttpClient(hc *http.Client) {
	client.httpClient = hc
}

func (client *RpcClient) Close() {
	client.conn.Close()
}
