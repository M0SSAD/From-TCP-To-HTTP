package headers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)


func TestRequestHeaders(t *testing.T) {
	// Test: Valid single header
	headers := NewHeaders()
	data := []byte("Host: localhost:42069\r\n\r\n")
	n, done, err := headers.Parse(data)
	require.NoError(t, err)
	require.NotNil(t, headers)
	assert.Equal(t, "localhost:42069", headers["host"])
	assert.Equal(t, 23, n)
	assert.False(t, done)

	// Test: Valid single header with extra whitespace
	h := NewHeaders()
	// Note the spaces around '123'
	data = []byte("Content-Length:   123   \r\n")
	
	n, done, err = h.Parse(data)
	
	require.NoError(t, err)
	assert.False(t, done)
	assert.Equal(t, "123", h["content-length"])
	// n should include all the spaces we consumed
	assert.Equal(t, len(data), n) 
    

	// Test: Valid 2 headers with existing headers
	h = NewHeaders()
	h["initial-key"] = "pre-existing" // Simulating data already there

	data = []byte("Header-A: 1\r\nHeader-B: 2\r\n")

	// Parse First Header
	n1, done1, err1 := h.Parse(data)
	require.NoError(t, err1)
	assert.False(t, done1)
	assert.Equal(t, "1", h["header-a"])

	// Parse Second Header (Simulate moving the cursor)
	n2, done2, err2 := h.Parse(data[n1:])
	require.NoError(t, err2)
	assert.False(t, done2)
	assert.Equal(t, "2", h["header-b"])

	// Ensure the old key is still there
	assert.Equal(t, "pre-existing", h["initial-key"])
	assert.Equal(t, len(data), n1+n2)

    // Test: Valid done
	h = NewHeaders()
	data = []byte("\r\n") // Empty line
	
	n, done, err = h.Parse(data)
	
	require.NoError(t, err)
	assert.True(t, done, "Should be done when hitting empty line")
	assert.Equal(t, 2, n, "Must consume exactly 2 bytes (\r\n)")


    // Test: Invalid spacing header
	h = NewHeaders()
	data = []byte("Host : localhost\r\n") // Space before colon
	
	_, _, err = h.Parse(data)
	
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Found Space Between Key And Colon.")

	// Test: Invalid character in header key
	h = NewHeaders()
	data = []byte("H©st: localhost:42069\r\n")

	_, _, err = h.Parse(data)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid characters in key")

	// Test: Multiple values for the same key (RFC 9110 5.2)
	h = NewHeaders()
	
	// 1. Simulate a header already existing (maybe from a previous packet)
	h["set-person"] = "lane-loves-go"

	// 2. Parse a new line with the SAME key
	data = []byte("Set-Person: prime-loves-zig\r\n")
	n, done, err = h.Parse(data)

	require.NoError(t, err)
	assert.False(t, done)
	assert.Equal(t, len(data), n)

	// 3. Verify they are combined with a comma
	expected := "lane-loves-go, prime-loves-zig"
	assert.Equal(t, expected, h["set-person"])
	
	// 4. Add a third one to be sure
	data2 := []byte("Set-Person: tj-loves-ocaml\r\n")
	h.Parse(data2)
	
	expected2 := "lane-loves-go, prime-loves-zig, tj-loves-ocaml"
	assert.Equal(t, expected2, h["set-person"])
}