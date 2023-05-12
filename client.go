package vermouth

import (
	"context"
	"errors"
	"os"
	"os/signal"

	"google.golang.org/grpc"
)

const (
	Success = "success"
)

type RpcClient struct {
	addr    string
	conn    *grpc.ClientConn
	service VermouthGrpcClient

	port      int64
	prefix    string
	localAddr string
}

func NewVermouthRpcClient(remoteAddr string) *RpcClient {
	client := &RpcClient{
		addr: remoteAddr,
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

func (client *RpcClient) Close() {
	client.conn.Close()
}
