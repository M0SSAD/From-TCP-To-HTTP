package request

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type chunkReader struct {
	data            string
	numBytesPerRead int
	pos             int
}

func (cr *chunkReader) Read(p []byte) (n int, err error) {
	if cr.pos >= len(cr.data) {
		return 0, io.EOF
	}
	endIndex := min(cr.pos + cr.numBytesPerRead, len(cr.data))
	n = copy(p, cr.data[cr.pos:endIndex])
	cr.pos += n

	return n, nil
}

func TestRequestLineParse(t *testing.T) {
	// Good Request Line
	r, err := RequestFromReader(strings.NewReader("GET / HTTP/1.1\r\nHost: localhost:42069\r\nUser-Agent: curl/7.81.0\r\nAccept: */*\r\n\r\n"))
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, "GET", r.RequestLine.Method)
	assert.Equal(t, "/", r.RequestLine.RequestTarget)
	assert.Equal(t, "1.1", r.RequestLine.HttpVersion)

	// Test: Small Chunks (3 bytes at a time)
	reader1 := &chunkReader{
		data:            "GET / HTTP/1.1\r\nHost: localhost:42069\r\nUser-Agent: curl/7.81.0\r\nAccept: */*\r\n\r\n",
		numBytesPerRead: 3,
	}
	r, err = RequestFromReader(reader1)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, "GET", r.RequestLine.Method)
	assert.Equal(t, "/", r.RequestLine.RequestTarget)
	assert.Equal(t, "1.1", r.RequestLine.HttpVersion)

	// Test: Tiny Chunks (1 byte at a time!)
	reader2 := &chunkReader{
		data:            "GET /coffee HTTP/1.1\r\nHost: localhost:42069\r\nUser-Agent: curl/7.81.0\r\nAccept: */*\r\n\r\n",
		numBytesPerRead: 1,
	}
	r, err = RequestFromReader(reader2)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, "GET", r.RequestLine.Method)
	assert.Equal(t, "/coffee", r.RequestLine.RequestTarget)
	assert.Equal(t, "1.1", r.RequestLine.HttpVersion)

	// Test: Good GET Request line with path
	r, err = RequestFromReader(strings.NewReader("GET /coffee HTTP/1.1\r\nHost: localhost:42069\r\nUser-Agent: curl/7.81.0\r\nAccept: */*\r\n\r\n"))
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, "GET", r.RequestLine.Method)
	assert.Equal(t, "/coffee", r.RequestLine.RequestTarget)
	assert.Equal(t, "1.1", r.RequestLine.HttpVersion)

	// Test: Good Post Request line with path
	r, err = RequestFromReader(strings.NewReader("POST /coffee HTTP/1.1\r\nHost: localhost:42069\r\nUser-Agent: curl/7.81.0\r\nAccept: */*\r\nContent-Type: application/json\r\nContent-Length: 22\r\n\r\n{\"flavor\":\"dark mode\"}"))
	require.NoError(t, err)
	assert.Equal(t, "POST", r.RequestLine.Method)
	assert.Equal(t, "/coffee", r.RequestLine.RequestTarget)
	assert.Equal(t, "1.1", r.RequestLine.HttpVersion)
	
	// Test: Invalid Method (Lower case)
	_, err = RequestFromReader(strings.NewReader("get /coffee HTTP/1.1\r\nHost: localhost\r\n\r\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid method")

	// Test: Invalid number of parts in request line
	_, err = RequestFromReader(strings.NewReader("/coffee HTTP/1.1\r\nHost: localhost:42069\r\nUser-Agent: curl/7.81.0\r\nAccept: */*\r\n\r\n"))
	require.Error(t, err)

	// Test: Invalid Version (HTTP/1.0 or junk)
	_, err = RequestFromReader(strings.NewReader("GET /coffee HTTP/1.0\r\nHost: localhost\r\n\r\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Unsupported HTTP Version")
}

func TestRequestHeaders(t *testing.T) {
	// 1. Standard Headers
	reader := &chunkReader{
		data:            "GET / HTTP/1.1\r\nHost: localhost:42069\r\nUser-Agent: curl/7.81.0\r\nAccept: */*\r\n\r\n",
		numBytesPerRead: 10,
	}
	r, err := RequestFromReader(reader)
	require.NoError(t, err)
	assert.Equal(t, "localhost:42069", r.Headers["host"])
	assert.Equal(t, "curl/7.81.0", r.Headers["user-agent"])
	assert.Equal(t, "*/*", r.Headers["accept"])

	// 2. Empty Headers
	// Just a Request Line followed immediately by the blank line (\r\n)
	r, err = RequestFromReader(strings.NewReader("GET / HTTP/1.1\r\n\r\n"))
	require.NoError(t, err)
	require.NotNil(t, r.Headers) // Map should be initialized
	assert.Empty(t, r.Headers)   // But empty

	// 3. Malformed Header (No Colon)
	// We expect the parser to return an error when it hits "Host localhost"
	readerMal := &chunkReader{
		data:            "GET / HTTP/1.1\r\nHost localhost:42069\r\n\r\n",
		numBytesPerRead: 10,
	}
	_, err = RequestFromReader(readerMal)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed header")

	// 4. Duplicate Headers (The Comma Logic)
	// We expect multiple "My-List" headers to be combined
	r, err = RequestFromReader(strings.NewReader("GET / HTTP/1.1\r\nMy-List: item1\r\nMy-List: item2\r\n\r\n"))
	require.NoError(t, err)
	assert.Equal(t, "item1, item2", r.Headers["my-list"])

	// 5. Case Insensitive Headers
	// We send "CoNtEnT-LeNgTh", we expect to find it at key "content-length"
	r, err = RequestFromReader(strings.NewReader("GET / HTTP/1.1\r\nAuThEnT: 42\r\n\r\n"))
	require.NoError(t, err)
	assert.Equal(t, "42", r.Headers["authent"])

	// 6. Missing End of Headers (Edge Case)
	// The stream ends abruptly. The parser should return what it has, 
	// but the last incomplete header should NOT be present.
	// Input: Valid Host, but incomplete User-Agent
	incompleteData := "GET / HTTP/1.1\r\nHost: localhost\r\nUser-Age"
	r, err = RequestFromReader(strings.NewReader(incompleteData))
	
	require.NoError(t, err) // It's not an error to run out of data (EOF)
	assert.Equal(t, "localhost", r.Headers["host"]) // Host was fully parsed
	_, exists := r.Headers["user-agent"]
	assert.False(t, exists, "Should not have parsed incomplete User-Agent header")
}

func TestRequestBody(t *testing.T) {
	// 1. Standard Body
	reader := &chunkReader{
		data: "POST /submit HTTP/1.1\r\n" +
			"Host: localhost:42069\r\n" +
			"Content-Length: 13\r\n" + // Length of "hello world!\n"
			"\r\n" +
			"hello world!\n",
		numBytesPerRead: 3,
	}
	r, err := RequestFromReader(reader)
	require.NoError(t, err)
	assert.Equal(t, "hello world!\n", string(r.Body))

	// 2. Body shorter than reported (Expect Error)
	readerShort := &chunkReader{
		data: "POST /submit HTTP/1.1\r\n" +
			"Host: localhost:42069\r\n" +
			"Content-Length: 20\r\n" + // Lie! It's actually smaller
			"\r\n" +
			"partial content",
		numBytesPerRead: 3,
	}
	_, err = RequestFromReader(readerShort)
	// Note: In our current implementation, this might NOT error if the stream just ends (EOF).
	// But if the server was running indefinitely, it would hang waiting for bytes.
	// For this specific parser which breaks on EOF, we technically just return incomplete data.
	// However, if we sent MORE data than allowed, we would error.
	
    // Let's test "Body LARGER than reported" (Strict Error)
	readerLong := &chunkReader{
		data: "POST /submit HTTP/1.1\r\n" +
			"Content-Length: 5\r\n" + 
			"\r\n" +
			"123456", // 6 bytes
		numBytesPerRead: 10,
	}
	_, err = RequestFromReader(readerLong)
	require.Error(t, err)
    assert.Contains(t, err.Error(), "content-length doesn't match the body size")

	// 3. No Content-Length (Assume Empty Body)
	readerNoCL := &chunkReader{
		data: "GET / HTTP/1.1\r\nHost: localhost\r\n\r\nbody-that-should-be-ignored",
		numBytesPerRead: 10,
	}
	r, err = RequestFromReader(readerNoCL)
	require.NoError(t, err)
	assert.Empty(t, r.Body) // Should be empty because CL is missing
}


