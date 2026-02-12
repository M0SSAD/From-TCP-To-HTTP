package server

import (
	"fmt"
	"log"
	"net"
	"sync/atomic"

	"boot.mossad.http/internal/request"
	"boot.mossad.http/internal/response"
)

// type Handler func(w io.Writer, req *request.Request) *HandlerError
type Handler func(w *response.Writer, req *request.Request)

type Server struct {
	listener net.Listener
	handler Handler
	isClosed atomic.Bool
}

type HandlerError struct {
	StatusCode response.StatusCode
	Message    string
}

func Serve(port int, handler Handler) (*Server, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}

	s := &Server{
		listener: ln,
		handler: handler,
	}

	go s.listen()
	
	return s, nil
	
}

func (s *Server) Close() error {
	 s.isClosed.Store(true)

	 return s.listener.Close()
}

func (s *Server) listen() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.isClosed.Load() {
				return
			}
			log.Printf("Error accepting connection: %v\n", err)
			continue
		}
		go s.handle(conn)
	}
}

//  HTTP-message   = start-line CRLF
//                   *( field-line CRLF )
//                   CRLF
//                   [ message-body ]

// status-line = HTTP-version SP status-code SP [ reason-phrase ]
// A server MUST send the space that separates the status-code from the reason-phrase even when the reason-phrase is absent (i.e., the status-line would end with the space).
func (s *Server) handle(conn net.Conn) {
	defer conn.Close()

	req, err := request.RequestFromReader(conn)

	if err != nil {
		return
	}

	w := response.NewWriter(conn)

	s.handler(w, req)

	// REFACTORED THE STRUCTURE, SO NOW DECISION MAKING MOVED TO THE APPLICATION ITSELF.
	// if err != nil {
	// 	handlerError := &HandlerError{
	// 		StatusCode: response.StatusBadRequest,
	// 		Message:    err.Error(),
	// 	}
	// 	handlerError.Write(conn)
	// 	return
	// }

	// buf := new(bytes.Buffer)

	// if handlerError := s.handler(buf, req) ; handlerError != nil {
	// 	handlerError.Write(conn)
	// 	return
	// }
		
	// if err := response.WriteStatusLine(conn, response.StatusOK); err != nil {
	// 	return
	// }
	// h := response.GetDefaultHeaders(buf.Len())
	// if err := response.WriteHeaders(conn, h); err != nil {
	// 	return
	// }
	// conn.Write([]byte("\r\n"))
	// conn.Write(buf.Bytes())

}

// func (e HandlerError) Write(w io.Writer) error {
// 	// Status Line
// 	if err := response.WriteStatusLine(w, e.StatusCode); err != nil {
// 		return err
// 	}

// 	// Headers (Content-Length is length of the error message)
// 	h := response.GetDefaultHeaders(len(e.Message))
// 	if err := response.WriteHeaders(w, h); err != nil {
// 		return err
// 	}

// 	// Empty Line (End of Headers)
// 	if _, err := w.Write([]byte("\r\n")); err != nil {
// 		return err
// 	}

// 	// Body (The Error Message)
// 	if _, err := w.Write([]byte(e.Message)); err != nil {
// 		return err
// 	}

// 	return nil
// }