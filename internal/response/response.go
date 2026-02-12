package response

import (
	"fmt"
	"io"

	"boot.mossad.http/internal/headers"
)


type StatusCode int 

const (
	StatusOK StatusCode = 200
	StatusBadRequest StatusCode = 400
	StatusInternalServerError StatusCode = 500
)

func WriteStatusLine(w io.Writer, statusCode StatusCode) error {
	var statusLine string

	switch statusCode {
	case StatusOK:
		statusLine = "HTTP/1.1 200 OK\r\n"
	case StatusBadRequest:
		statusLine = "HTTP/1.1 400 Bad Request\r\n"
	case StatusInternalServerError:
		statusLine = "HTTP/1.1 500 Internal Server Error\r\n"
	default:
		// for unknown codes, Leave reason phrase blank
		statusLine = fmt.Sprintf("HTTP/1.1 %d \r\n", statusCode)
	}

	_, err := w.Write([]byte(statusLine))
	return err
}

func GetDefaultHeaders(contentLen int) headers.Headers {
	h := headers.NewHeaders()
	h["Content-Length"] = fmt.Sprintf("%d", contentLen)
	h["Connection"] = "close"
	h["Content-Type"] = "text/plain"

	/* 
	a few more noteworthy mentions that we won't care about for now are:
    - Content-Encoding: Is the response content encoded/compressed? If so, then this should be included to tell the client how to decode it. (Remember, encoded != encrypted)
    - Date: The date and time that the message was sent. This is useful for caching and other things.
    - Cache-Control: Directives for caching mechanisms in both requests and responses. This is useful for telling the client or any intermediaries how to cache the response.
	**/

	return h
}

func WriteHeaders(w io.Writer, headers headers.Headers) error {
	for k, v := range headers {
		// "Key: Value\r\n"
		line := fmt.Sprintf("%s: %s\r\n", k, v)
		_, err := w.Write([]byte(line))
		if err != nil {
			return err
		}
	}

	return nil
}

