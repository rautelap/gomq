package gomq

import (
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/zeromq/gomq/zmtp"
)

var (
	// ErrInvalidSockAction is returned when an action is performed
	// on a socket type that does not support the action
	ErrInvalidSockAction = errors.New("action not valid on this socket")

	defaultRetry = 250 * time.Millisecond
)

// Connection holds a connection to a ZeroMQ socket.
type Connection struct {
	netconn  net.Conn
	zmtpconn *zmtp.Connection
}

// Socket represents a ZeroMQ socket. Sockets may have multiple connections.
type Socket interface {
	Recv() ([]byte, error)
	Send([]byte) error
	Connect(endpoint string) error
	Bind(endpoint string) (net.Addr, error)
	Close()
}

type socket struct {
	sockType      zmtp.SocketType
	asServer      bool
	conns         []*Connection
	retryInterval time.Duration
	lock          sync.Mutex
	mechanism     zmtp.SecurityMechanism
	messageChan   chan *zmtp.Message
}

func newSocket(sockType zmtp.SocketType, asServer bool, mechanism zmtp.SecurityMechanism) Socket {
	return &socket{
		asServer:      asServer,
		sockType:      sockType,
		retryInterval: defaultRetry,
		mechanism:     mechanism,
		conns:         make([]*Connection, 0),
		messageChan:   make(chan *zmtp.Message),
	}
}

// Connect connects to an endpoint.
func (s *socket) Connect(endpoint string) error {
	if s.asServer {
		return ErrInvalidSockAction
	}

	parts := strings.Split(endpoint, "://")

Connect:
	netconn, err := net.Dial(parts[0], parts[1])
	if err != nil {
		time.Sleep(s.retryInterval)
		goto Connect
	}

	zmtpconn := zmtp.NewConnection(netconn)
	_, err = zmtpconn.Prepare(s.mechanism, s.sockType, s.asServer, nil)
	if err != nil {
		return err
	}

	conn := &Connection{
		netconn:  netconn,
		zmtpconn: zmtpconn,
	}

	s.conns = append(s.conns, conn)

	zmtpconn.Recv(s.messageChan)
	return nil
}

// Bind binds to an endpoint.
func (s *socket) Bind(endpoint string) (net.Addr, error) {
	var addr net.Addr

	if !s.asServer {
		return addr, ErrInvalidSockAction
	}

	parts := strings.Split(endpoint, "://")

	ln, err := net.Listen(parts[0], parts[1])
	if err != nil {
		return addr, err
	}

	netconn, err := ln.Accept()
	if err != nil {
		return addr, err
	}

	zmtpconn := zmtp.NewConnection(netconn)
	_, err = zmtpconn.Prepare(s.mechanism, s.sockType, s.asServer, nil)
	if err != nil {
		return netconn.LocalAddr(), err
	}

	conn := &Connection{
		netconn:  netconn,
		zmtpconn: zmtpconn,
	}

	s.conns = append(s.conns, conn)

	zmtpconn.Recv(s.messageChan)

	return netconn.LocalAddr(), nil
}

// Close closes all underlying connections in a socket.
func (s *socket) Close() {
	for _, v := range s.conns {
		v.netconn.Close()
	}
}

// NewClient creates a new ZMQ_CLIENT socket.
func NewClient(mechanism zmtp.SecurityMechanism) Socket {
	return newSocket(zmtp.ClientSocketType, false, mechanism)
}

// NewServer creates a new ZMQ_SERVER socket.
func NewServer(mechanism zmtp.SecurityMechanism) Socket {
	return newSocket(zmtp.ServerSocketType, true, mechanism)
}

// Recv receives the next message from the socket.
func (s *socket) Recv() ([]byte, error) {
	msg := <-s.messageChan
	if msg.MessageType == zmtp.CommandMessage {
	}
	return msg.Body, msg.Err
}

// Send sends a message.
func (s *socket) Send(b []byte) error {
	return s.conns[0].zmtpconn.SendFrame(b)
}
