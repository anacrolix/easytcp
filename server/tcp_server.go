package server

import (
	"fmt"
	"github.com/DarthPestilane/easytcp/logger"
	"github.com/DarthPestilane/easytcp/packet"
	"github.com/DarthPestilane/easytcp/router"
	"github.com/DarthPestilane/easytcp/session"
	"github.com/sirupsen/logrus"
	"net"
	"os"
	"os/signal"
	"syscall"
)

type TcpServer struct {
	rwBufferSize int
	listener     *net.TCPListener
	log          *logrus.Entry
	msgPacker    packet.Packer
	msgCodec     packet.Codec
}

type TcpOption struct {
	RWBufferSize int           // socket 读写 buffer
	MsgPacker    packet.Packer // 消息封包/拆包器
	MsgCodec     packet.Codec  // 消息编码/解码器
}

func NewTcp(opt TcpOption) *TcpServer {
	if opt.MsgPacker == nil {
		opt.MsgPacker = &packet.DefaultPacker{}
	}
	if opt.MsgCodec == nil {
		opt.MsgCodec = &packet.StringCodec{}
	}
	return &TcpServer{
		listener:     nil,
		log:          logger.Default.WithField("scope", "server.TcpServer"),
		rwBufferSize: opt.RWBufferSize,
		msgPacker:    opt.MsgPacker,
		msgCodec:     opt.MsgCodec,
	}
}

func (t *TcpServer) Serve(addr string) error {
	address, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return err
	}
	lis, err := net.ListenTCP("tcp", address)
	if err != nil {
		return err
	}
	t.listener = lis

	return t.acceptLoop()
}

func (t *TcpServer) acceptLoop() error {
	for {
		conn, err := t.listener.AcceptTCP()
		if err != nil {
			return fmt.Errorf("accept err: %s", err)
		}
		if t.rwBufferSize > 0 {
			if err := conn.SetReadBuffer(t.rwBufferSize); err != nil {
				return fmt.Errorf("conn set read buffer err: %s", err)
			}
			if err := conn.SetWriteBuffer(t.rwBufferSize); err != nil {
				return fmt.Errorf("conn set write buffer err: %s", err)
			}
		}

		// handle conn in a new goroutine
		go t.handleConn(conn)
	}
}

// handleConn
// create a new session and save it to memory
// read/write loop
// route incoming message to handler
// wait for session to close
// remove session from memory
func (t *TcpServer) handleConn(conn *net.TCPConn) {
	// create a new session
	sess := session.New(conn, t.msgPacker, t.msgCodec)
	session.Sessions().Add(sess)
	go sess.ReadLoop()
	go sess.WriteLoop()
	go func() {
		// route incoming message to handler
		if err := router.Inst().Loop(sess); err != nil {
			t.log.Errorf("router loop stopped: %s", err)
		}
	}()
	if err := sess.WaitToClose(); err != nil {
		t.log.Errorf("session close err: %s", err)
	}
	t.log.Tracef("session (%s) closed", sess.Id)
	session.Sessions().Remove(sess.Id) // session has been closed, remove it
}

// Stop 让 server 停止，关闭 router, session 和 listener
func (t *TcpServer) Stop() error {
	closedNum := 0
	session.Sessions().Range(func(id string, sess *session.Session) (next bool) {
		sess.Close()
		closedNum++
		return true
	})
	t.log.Warnf("%d session(s) closed", closedNum)

	defer func() { t.log.Warnf("listener is stopped") }()
	return t.listener.Close()
}

// GracefulStop 优雅停止，监听 syscall.Signal, 触发 Stop()
func (t *TcpServer) GracefulStop() error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	sig := <-sigCh
	t.log.Warnf("receive signal: %s | graceful shutdown now", sig)
	return t.Stop()
}
