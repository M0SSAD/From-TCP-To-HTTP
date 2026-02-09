package headers

import (
	"bytes"
	"fmt"
)

type Headers map[string]string


func NewHeaders() (Headers) {
	return make(Headers)
}

var (
	ErrNoColon           = fmt.Errorf("malformed header: no colon found")
	ErrSpaceBeforeColon  = fmt.Errorf("malformed header: Found Space Between Key And Colon.")
	ErrEmptyKey          = fmt.Errorf("malformed header: empty key")
	ErrInvalidCharInKey  = fmt.Errorf("malformed header: invalid characters in key")
)

// Function to parse the request data into Headers Map.
// returns: number of the bytes in the headers, 
func (h Headers) Parse(data []byte) (n int, done bool, err error) {
	//"       Host: localhost:42069       \r\n\r\n"
	// Find First \r\n
	// n += index(\r\n) + 2
	// now I should parse the isolated header alone, then move the slice

	idx := bytes.Index(data, []byte("\r\n"))
	
	if idx == -1 {
		return 0, false, nil
	}

	if idx == 0 {
		done = true
		return idx + 2, done, nil
	}

	
	header := data[:idx]
	
	key, remaining, found := bytes.Cut(header, []byte(":"))
	
	if !found {
		return 0, done, ErrNoColon
	}
	
	if len(key) == 0 {
         return 0, false, ErrEmptyKey
    }

	if key[len(key) - 1] == ' ' {
		return 0, done, ErrSpaceBeforeColon
	}
	
	value := bytes.TrimSpace(remaining)
	key = bytes.TrimLeft(key, " ")
	normalizedKey := bytes.ToLower(key)

	
	for _, b := range normalizedKey {
		if !isTokenChar(b) {
			return 0, done, ErrInvalidCharInKey
		}
	}
	
	keyStr := string(normalizedKey)
	valStr := string(value)

	if existingValue, ok := h[keyStr]; ok {
		// RFC 9110: Append with comma
		h[keyStr] = existingValue + ", " + valStr
	} else {
		h[keyStr] = valStr
	}

	return idx + 2, done, nil
}

func isTokenChar(b byte) bool {
    // 1. Check Contiguous Ranges
    if (b >= 'a' && b <= 'z') || 
       (b >= '0' && b <= '9') {
        return true
    }

    // 2. Check Scattered Symbols
    switch b {
    case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
        return true
    }

    return false
}