package yetanothersmtpd

import (
	"crypto/tls"
	"errors"
	"net"
	"time"
)

type Server struct {
	Hostname        string
	OnNewConnection func(peer net.Addr, isTLS bool) (SessionHandler, error)
	RequireAuth     bool
	RequireTLS      bool
	Timeout         time.Duration
	TLSConfig       *tls.Config
}

func (s *Server) Serve(listener net.Listener) error {
	if s.OnNewConnection == nil {
		return errors.New("new connection callback not be nil")
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				time.Sleep(time.Second)
				continue
			}
			return err
		}
		handler, err := s.OnNewConnection(conn.RemoteAddr(), false)
		if err != nil {
			return err
		}
		if handler == nil {
			// This must have been a conscious decision on the
			// part of the OnNewConnection function so not treating
			// that as an error. In fact, not even logging it since
			// the OnNewConnection callback is perfectly capable of
			// doing that.
			continue
		}
		sn := newSession(s, handler, conn, false)
		go sn.serve()
	}
}
