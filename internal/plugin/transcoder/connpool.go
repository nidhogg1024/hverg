package transcoder

import (
	"fmt"
	"log/slog"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ConnPool 管理到各个 gRPC 后端的连接，避免同一个 target 创建重复连接。
// 线程安全，适用于多个 TranscoderPlugin 实例共享。
type ConnPool struct {
	mu    sync.RWMutex
	conns map[string]*grpc.ClientConn
}

var globalConnPool = &ConnPool{
	conns: make(map[string]*grpc.ClientConn),
}

// GetConn 获取到指定 target 的 gRPC 连接。
// 如果连接不存在则创建一个新的。
func (p *ConnPool) GetConn(target string) (*grpc.ClientConn, error) {
	// 先读锁快速检查
	p.mu.RLock()
	if conn, ok := p.conns[target]; ok {
		p.mu.RUnlock()
		return conn, nil
	}
	p.mu.RUnlock()

	// 升级为写锁，double-check 后创建
	p.mu.Lock()
	defer p.mu.Unlock()

	if conn, ok := p.conns[target]; ok {
		return conn, nil
	}

	slog.Info("Creating new gRPC connection", "target", target)
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(16*1024*1024)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create grpc connection to %s: %w", target, err)
	}

	p.conns[target] = conn
	return conn, nil
}

// CloseAll 关闭池中所有连接，用于网关优雅停机。
func (p *ConnPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for target, conn := range p.conns {
		slog.Info("Closing gRPC connection", "target", target)
		conn.Close()
	}
	p.conns = make(map[string]*grpc.ClientConn)
}
