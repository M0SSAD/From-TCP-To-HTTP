package request

import (
	"bytes"
	"fmt"
	"io"
)

// creating my enum
type parserState int

const (
	stateInitialized parserState = iota
	stateDone
)

type RequestLine struct {
	HttpVersion   string
	RequestTarget string
	Method        string
}

type Request struct {
	RequestLine RequestLine
	state parserState // 0 for initialized, 1 for done
}

var ERROR_PARSING_METHOD_IN_REQUEST_LINE = fmt.Errorf("invalid request line: parsing method")
var ERROR_PARSING_TARGET_IN_REQUEST_LINE = fmt.Errorf("invalid request line: parsing target")
var ERROR_PARSING_HTTP_VERSION_IN_REQUEST_LINE = fmt.Errorf("invalid request line: parsing HTTP version")
func ErrorInvalidMethod(method string) error {
    return fmt.Errorf("invalid method: %s", method)
}

func ErrorInvalidVersion(version string) error {
    return fmt.Errorf("Unsupported HTTP Version: %s", version)
}

func newRequest() Request {
	return Request{state: stateInitialized}
}

// Read The request, agnostic approach, doesn't care if it is a stream of bytes or a full message.
func RequestFromReader(reader io.Reader) (*Request, error) {
	/* // OLD LOGIC (ReadAll) - Kept for learning reference
    data, err := io.ReadAll(reader)
    if err != nil && err != io.EOF { return nil, err }
    
    line, _, found := bytes.Cut(data, []byte("\r\n"))
    if !found { return nil, fmt.Errorf("invalid request: no newlines found") }
    
    requestLine, err := parseRequestLine(line)
    if err != nil { return nil, err }
    return &Request{RequestLine: *requestLine}, nil 
    */

	req := newRequest()

	// store the data that didn't get parsed yet.
	buf := make([]byte, 0)
	// store the chunks of bytes that will be added to the buf.
	chunk := make([]byte, 1024)

	for req.state != stateDone {
		numBytesRead, err := reader.Read(chunk)
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
		}

		if numBytesRead == 0 && err == io.EOF {
			req.state = stateDone
			break
		}

		// n is the length of the data in the chunk
		buf = append(buf, chunk[:numBytesRead]...)

		// Parse from the buffer
		numBytesParsed, err := req.parse(buf)

		if err != nil {
			return nil, err
		}

		// if I parsed some data, move the slice forward
		// to skip the parsed data.
		if numBytesParsed > 0 {
			buf = buf[numBytesParsed:]
		}
	}

	return &req, nil
	
}

// A Helper function to change the state of the request based on if it got parsed
// or not.
// if n =0, with no error 
// That means I need more chunks of data to parse.
func (r *Request) parse(p []byte) (int, error) {
	numBytesParsed := 0
outer:	
	for {
		switch r.state {
		case stateInitialized:
			// Try to parse the Request Line
			rlp, numBytesParsed, _, err := parseRequestLine(p)
			if err != nil {
				return 0, err
			}

			// If numBytesParsed is 0, we need more data. Break and wait.
			if numBytesParsed == 0 {
				break outer
			}

			// Success: Update struct and State
			r.RequestLine = *rlp
			r.state = stateDone
		case stateDone:
			break outer // DO NOTHING!
		}
	}
	return numBytesParsed, nil
}

// The Parser I will use to parse the request line
// It returns, pointer to a struct of the RL, number of bytes parsed, 
// the rest of the request (Headers, body), error if exists.
func parseRequestLine(data []byte) (*RequestLine, int, []byte, error){
	line, restOfMsg, found := bytes.Cut(data, []byte("\r\n"))

	if !found {
		return nil, 0, data, nil
	}

	numBytesParsed := len(line) + 2

	method, rest, found := bytes.Cut(line, []byte(" "))
    if !found {
        return nil, 0, data, ERROR_PARSING_METHOD_IN_REQUEST_LINE
    }

	target, rest, found := bytes.Cut(rest, []byte(" "))
    if !found {
        return nil, 0, data, ERROR_PARSING_TARGET_IN_REQUEST_LINE
    }

	if string(rest) != "HTTP/1.1" {
		 return nil, 0, data, ErrorInvalidVersion(string(rest))
	}

	_, rest, found = bytes.Cut(rest, []byte("/"))
	if !found {
		return nil, 0, data, ERROR_PARSING_HTTP_VERSION_IN_REQUEST_LINE
	}

	version := rest
	for _, char := range method {
        if char < 'A' || char > 'Z' {
             return nil, 0, data, ErrorInvalidMethod(string(method))
        }
    }

	
    return &RequestLine{
        Method:        string(method),
        RequestTarget: string(target),
        HttpVersion:   string(version),
    }, numBytesParsed, restOfMsg, nil

	// strs := strings.Split(string(data), " ")
	// requestLine.Method = strs[0]
	// requestLine.RequestTarget = strs[1]
	// requestLine.HttpVersion = strs[2]
}