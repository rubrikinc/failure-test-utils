// Copyright 2022 Rubrik, Inc.

//go:build !mysql
// +build !mysql

package testutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"rubrik/cqlproxy/failuregen"
	"rubrik/util/log"

	"github.com/pkg/errors"
)

// TCPProxy is the interface for test L4 proxy
type TCPProxy interface {
	Stop()
}

type testTCPProxy struct {
	ctx          context.Context
	listener     net.Listener
	frontendPort int
	backendPort  int
	quit         chan interface{}
	wg           sync.WaitGroup
	recvFg       failuregen.FailureGenerator
	acceptFg     failuregen.FailureGenerator
}

// NewTCPProxy creates a new instance of an L4 test proxy
func NewTCPProxy(
	ctx context.Context,
	frontendPort int,
	backendPort int,
	recvFg failuregen.FailureGenerator,
	acceptFg failuregen.FailureGenerator,
) (TCPProxy, error) {
	uuidStr := uuid.New().String()
	t := &testTCPProxy{
		ctx:          log.WithLogTag(ctx, uuidStr, nil),
		frontendPort: frontendPort,
		backendPort:  backendPort,
		quit:         make(chan interface{}),
		recvFg:       recvFg,
		acceptFg:     acceptFg,
		stats:        ProxyStats{}}
	l, err := net.Listen("tcp", LocalhostAddress(frontendPort))
	if err != nil {
		return nil, errors.Wrap(err, "listen")
	}
	t.listener = l
	t.wg.Add(1)
	go t.serve()
	log.Infof(t.ctx, "Started TCP-proxy on %d", frontendPort)
	return t, nil
}

// Stop stops the proxy from listening and also forcibly closes any connections.
func (t *testTCPProxy) Stop() {
	log.Warningf(
		t.ctx,
		"Stopping %d -> %d TCP-proxy",
		t.frontendPort,
		t.backendPort)
	close(t.quit)
	if err := t.listener.Close(); err != nil {
		log.Error(t.ctx, err)
	}
	t.wg.Wait()
}

// Stats provides the tcp proxy stats
func (t *testTCPProxy) Stats() ProxyStats {
	return t.stats
}

func (st ProxyStats) String() string {
	return fmt.Sprintf(
		"stats{activeConn: %d, frontendDrop: %d, backendDrop: %d}\n",
		st.activeConnCtr.Load(),
		st.FrontendDropCtr.Load(),
		st.backendDropCtr.Load())
}

// BlockIncomingConns blocks all incoming connection to the TCP proxy by
// dropping the connection.
func (t *testTCPProxy) BlockIncomingConns() {
	t.acceptFg.SetFailureProbability(1.0)
}

// close connections gracefully
func (t *testTCPProxy) closeFrontendConn(
	conn net.Conn,
	reason string,
) {
	if log.V(3) {
		log.Infof(t.ctx, "closing connection to %v", conn.RemoteAddr())
	}
	_ = conn.Close()
	if reason == "drop" {
		t.stats.FrontendDropCtr.Inc()
	}
	t.stats.activeConnCtr.Dec()
}

func (t *testTCPProxy) serve() {
	defer t.wg.Done()

	for {
		conn, err := t.listener.Accept()
		if err != nil {
			select {
			case <-t.quit:
				// error was because the proxy was stopped, safe to ignore
				return
			default:
				log.Errorf(t.ctx, "accept error: %v", err)
			}
		} else {
			if err := t.acceptFg.FailMaybe(); err != nil {
				log.Warningf(
					t.ctx,
					"injected accept failure %v,  %v",
					conn.RemoteAddr(),
					err)
				t.closeFrontendConn(conn, "drop")
			}
			t.wg.Add(1)
			t.stats.activeConnCtr.Inc()
			go func() {
				log.Infof(t.ctx, "Accepted connection from %v",
					conn.RemoteAddr())
				if err := t.handle(conn); err != nil {
					log.Errorf(t.ctx, "handle err: %v", err)
				}
				t.wg.Done()
			}()
		}
	}
}

func (t *testTCPProxy) copy(
	dest, src net.Conn,
	selfTermCh chan struct{},
	peerTermCh chan struct{},
) error {
	defer close(selfTermCh)
	buf := make([]byte, 1024)
	// Robustly close connections when proxy closes
	// https://eli.thegreenplace.net/2020/graceful-shutdown-of-a-tcp-server-in-go/#id1
	for {
		var nr int
		select {
		case <-peerTermCh:
			return nil
		case <-t.quit:
			return nil
		default:
			if err := src.SetReadDeadline(time.Now().Add(10 * time.Millisecond)); err != nil {
				return errors.Wrap(err, "set source deadline")
			}
			var err error
			nr, err = src.Read(buf)
			if err != nil {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					continue
				} else if err != io.EOF {
					return errors.Wrap(err, "read")
				}
			}
			if nr == 0 {
				return nil
			}
			if log.V(4) {
				log.Infof(t.ctx, "received from %v: %s", src.RemoteAddr(),
					string(buf[:nr]))
			}

			// TODO(CDM-362117)(Ambar) Change to a KMP filter to make this robust
			condFailGen, ok := (t.recvFg).(failuregen.ConditionalFailureGenerator)
			if ok {
				if err := condFailGen.FailOnCondition(buf); err != nil {
					t.stats.backendDropCtr.Inc()
					return errors.Wrap(err, "injected recv failure on satisfying condition")
				}
			} else {
				if err := t.recvFg.FailMaybe(); err != nil {
					t.stats.backendDropCtr.Inc()
					return errors.Wrap(err, "injected recv failure")
				}
			}
		}
		_, err := dest.Write(buf[:nr])

		if err != nil {
			return errors.Wrap(err, "write")
		}
		if log.V(4) {
			log.Infof(t.ctx, "written to %v: %s", dest.RemoteAddr(),
				string(buf[:nr]))
		}
	}
}

func (t *testTCPProxy) handle(frontendConn net.Conn) error {
	defer t.closeFrontendConn(frontendConn, "")
	backendConn, err := net.Dial("tcp", LocalhostAddress(t.backendPort))
	if err != nil {
		return errors.Wrap(err, "dial")
	}
	defer backendConn.Close()
	log.Infof(
		t.ctx,
		"Created proxy connection %v -> %v",
		backendConn.LocalAddr(),
		backendConn.RemoteAddr())

	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()

	onwardTermCh := make(chan struct{})
	returnTermCh := make(chan struct{})

	go func() {
		err := t.copy(backendConn, frontendConn, onwardTermCh, returnTermCh)
		if err != nil {
			log.Errorf(t.ctx, "copy from frontend to backend err: %v", err)
		}
		wg.Done()
	}()
	return t.copy(frontendConn, backendConn, returnTermCh, onwardTermCh)
}

func localhostAddress(port int) string {
	return fmt.Sprintf("localhost:%v", port)
}
