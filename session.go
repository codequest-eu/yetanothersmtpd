package yetanothersmtpd

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strings"
	"time"
)

type operationHandler func(*session, command) error

var (
	operationHandlers = map[string]operationHandler{
		"AUTH":     (*session).handleAUTH,
		"DATA":     (*session).handleDATA,
		"EHLO":     (*session).handleEHLO,
		"HELO":     (*session).handleHELO,
		"MAIL":     (*session).handleMAIL,
		"NOOP":     (*session).handleNOOP,
		"QUIT":     (*session).handleQUIT,
		"RCPT":     (*session).handleRCPT,
		"RSET":     (*session).handleRSET,
		"STARTTLS": (*session).handleSTARTTLS,
	}

	authMap = map[string]operationHandler{
		"LOGIN": (*session).authLOGIN,
		"PLAIN": (*session).authPLAIN,
	}
)

type session struct {
	server    *Server
	sHandler  SessionHandler
	mHandler  MessageHandler
	conn      net.Conn
	gotHelo   bool
	isTLS     bool
	keepGoing bool

	reader  *textproto.Reader
	writer  *textproto.Writer
	scanner *bufio.Scanner
}

func newSession(server *Server, handler SessionHandler, conn net.Conn, isTLS bool) *session {
	return &session{
		server:    server,
		sHandler:  handler,
		conn:      conn,
		gotHelo:   false,
		isTLS:     isTLS,
		keepGoing: true,
		reader:    textproto.NewReader(bufio.NewReader(conn)),
		writer:    textproto.NewWriter(bufio.NewWriter(conn)),
	}
}

func (s *session) serve() {
	defer s.closeOrReport(s.conn)
	err := s.writer.PrintfLine("%d %s ESMTP ready", StatusServiceReady, s.server.Hostname)
	if err != nil {
		s.sHandler.HandleSessionError(err)
		return
	}
	for {
		if s.keepGoing {
			s.serveOne()
		}
	}
}

func (s *session) serveOne() {
	readBy := time.Now().Add(s.server.Timeout)
	if err := s.conn.SetReadDeadline(readBy); err != nil {
		s.handleError(err)
		return
	}
	line, err := s.reader.ReadLine()
	if err != nil {
		s.handleError(err)
		return
	}
	cmd := parseLine(line)
	action, exists := operationHandlers[cmd.action]
	if !exists {
		s.handleError(
			NewReportableStatus(
				StatusCommandNotImplemented,
				"Unsupported command",
			),
		)
		return
	}
	s.handleError(action(s, cmd))
}

func (s *session) handleError(err error) {
	if err == nil {
		return
	}
	rErr, isStatus := err.(*ReportableStatus)
	if isStatus {
		s.handleError(s.writer.PrintfLine("%d %s", rErr.Code, rErr.Message))
		return
	}
	s.keepGoing = false
	s.sHandler.HandleSessionError(err)
}

func (s *session) handleAUTH(cmd command) error {
	if !s.gotHelo {
		return ErrNoHelo
	}
	if len(cmd.fields) < 2 {
		return ErrInvalidSyntax
	}
	mechanism := strings.ToUpper(cmd.fields[1])
	action, exists := authMap[mechanism]
	if !exists {
		return NewReportableStatus(
			StatusCommandNotImplemented,
			"Unknown authentication mechanism",
		)
	}
	return action(s, cmd)
}

func (s *session) handleDATA(cmd command) error {
	if s.mHandler == nil {
		return ErrBadSequence
	}
	writeCloser, err := s.mHandler.GetDataWriter()
	if err != nil {
		return err
	}
	err = s.writer.PrintfLine(
		"%d Go ahead. End your data with <CR><LF>.<CR><LF>",
		StatusStartMailInput,
	)
	if err != nil {
		return err
	}
	dotReader := s.reader.DotReader()
	if _, err := io.Copy(writeCloser, dotReader); err != nil {
		return err
	}
	if err := writeCloser.Close(); err != nil {
		return err
	}
	s.mHandler = nil // this is the end of the current message
	return ThankYou
}

func (s *session) handleEHLO(cmd command) error {
	if len(cmd.fields) < 2 {
		return ErrInvalidSyntax
	}
	s.mHandler = nil // reset message in case of duplicate HELO
	if err := s.sHandler.HandleHELO(cmd.fields[1], true); err != nil {
		return err
	}
	extensions := s.extensions()
	if len(extensions) > 1 {
		for _, ext := range extensions[:len(extensions)-1] {
			err := s.writer.PrintfLine("%d-%s", StatusOK, ext)
			if err != nil {
				return err
			}
		}
	}
	s.gotHelo = true
	return NewReportableStatus(StatusOK, extensions[len(extensions)-1])
}

func (s *session) handleHELO(cmd command) error {
	if len(cmd.fields) < 2 {
		return ErrInvalidSyntax
	}
	s.mHandler = nil // reset message in case of duplicate HELO
	if err := s.sHandler.HandleHELO(cmd.fields[1], false); err != nil {
		return err
	}
	s.gotHelo = true
	return GoAhead
}

func (s *session) handleMAIL(cmd command) error {
	if !s.gotHelo {
		return ErrNoHelo
	}
	if s.server.RequireTLS && !s.isTLS {
		return NewReportableStatus(
			StatusBadSequence, "please start TLS 1st")
	}
	from, err := parseAddress(cmd.params[1])
	if err != nil {
		return err
	}
	mHandler, err := s.sHandler.GetMessageHandler(from)
	if err != nil {
		return err
	}
	s.mHandler = mHandler
	return GoAhead
}

func (s *session) handleNOOP(cmd command) error {
	return GoAhead
}

func (s *session) handleQUIT(cmd command) error {
	s.keepGoing = false
	return NewReportableStatus(StatusServiceClosing, "OK, bye")
}

func (s *session) handleRCPT(cmd command) error {
	if s.mHandler == nil {
		return ErrBadSequence
	}
	recipient, err := parseAddress(cmd.params[1])
	if err != nil {
		return err
	}
	if err := s.mHandler.AddRecipient(recipient); err != nil {
		return err
	}
	return GoAhead
}

func (s *session) handleRSET(cmd command) error {
	s.mHandler = nil
	return GoAhead
}

func (s *session) handleSTARTTLS(cmd command) error {
	if s.isTLS {
		return NewReportableStatus(StatusSyntaxError, "already running TLS")
	}
	if s.server.TLSConfig == nil {
		return NewReportableStatus(StatusCommandNotImplemented, "TLS not supported")
	}
	tlsConn := tls.Server(s.conn, s.server.TLSConfig)
	if err := s.writer.PrintfLine("%d Go ahead", StatusOK); err != nil {
		return err
	}
	s.conn.SetDeadline(time.Time{})
	newHandler, err := s.server.OnNewConnection(s.conn.RemoteAddr(), true)
	if err != nil {
		return err
	}
	*s = *newSession(s.server, newHandler, tlsConn, true)
	return nil
}

func (s *session) authLOGIN(cmd command) error {
	err := s.writer.PrintfLine("%d VXNlcm5hbWU6", StatusProvideCredentials)
	if err != nil {
		return err
	}
	line, err := s.reader.ReadLine()
	if err != nil {
		return err
	}
	byteUsername, err := base64.StdEncoding.DecodeString(line)
	if err != nil {
		return ErrDecodingCredentials
	}
	err = s.writer.PrintfLine("%d UGFzc3dvcmQ6", StatusProvideCredentials)
	if err != nil {
		return err
	}
	line, err = s.reader.ReadLine()
	if err != nil {
		return err
	}
	bytePassword, err := base64.StdEncoding.DecodeString(line)
	if err != nil {
		return ErrDecodingCredentials
	}
	return s.sHandler.Authenticate(string(byteUsername), string(bytePassword))
}

func (s *session) authPLAIN(cmd command) error {
	auth := ""
	if len(cmd.fields) < 3 {
		err := s.writer.PrintfLine("%d Give me your credentials", StatusProvideCredentials)
		if err != nil {
			return err
		}
		auth, err = s.reader.ReadLine()
		if err != nil {
			return err
		}
	} else {
		auth = cmd.fields[2]
	}
	data, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return ErrDecodingCredentials
	}
	parts := bytes.Split(data, []byte{0})
	if len(parts) != 3 {
		return ErrDecodingCredentials
	}
	return s.sHandler.Authenticate(string(parts[1]), string(parts[2]))
}

func (s *session) extensions() []string {
	extensions := []string{
		fmt.Sprintf("%d SIZE", s.sHandler.MaxMessageSize()),
		"8BITMIME",
		"PIPELINING",
	}
	if s.server.TLSConfig != nil && !s.isTLS {
		extensions = append(extensions, "STARTTLS")
	}
	if s.server.RequireAuth {
		extensions = append(extensions, "AUTH PLAIN LOGIN")
	}
	return extensions
}

// closeOrReport provides a wrapper that allows deferred 'Close' operations to
// have their errors reported to the session error handler.
func (s *session) closeOrReport(closer io.Closer) {
	if err := closer.Close(); err != nil {
		s.sHandler.HandleSessionError(err)
	}
}
