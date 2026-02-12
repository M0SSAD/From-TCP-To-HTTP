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
	ErrKeyDoesntExist    = fmt.Errorf("malformed call: Key Doesn't Exist")

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
		return 0, false, nil // No headers Yet
	}

	if idx == 0 {
		done = true // Finished The Headers, Empty Line.
		return idx + 2, done, nil // Returns 2, to skip these two bytes.
	}

	
	header := data[:idx] // header is the bytes till the CRLF
	
	key, remaining, found := bytes.Cut(header, []byte(":")) // Divide it by the :
	
	if !found {
		return 0, done, ErrNoColon 
	}
	
	if len(key) == 0 {
         return 0, false, ErrEmptyKey
    }

	if key[len(key) - 1] == ' ' {
		return 0, done, ErrSpaceBeforeColon // RFC Requires that there mustn't be a space between the key and :
	}
	
	value := bytes.TrimSpace(remaining) // RFC Allows white spaces before the key and after the :
	key = bytes.TrimLeft(key, " ")
	normalizedKey := bytes.ToLower(key) // Keys are stored with lower cases

	
	for _, b := range normalizedKey {
		if !isTokenChar(b) {
			return 0, done, ErrInvalidCharInKey // ensure valid characters in the key
		}
	}
	
	keyStr := string(normalizedKey)
	valStr := string(value)

	if existingValue, ok := h[keyStr]; ok {
		// RFC 9110: Append with comma
		h[keyStr] = existingValue + ", " + valStr // Believe it or not, but it is allowed to have more than one value for the same key ;D.
	} else {
		h[keyStr] = valStr
	}

	return idx + 2, done, nil // return how many bytes were processed to move the windows of the bytes.
}

// Function to get the value responding to a specific key
func (h Headers) Get(key []byte) (value string, err error) {
	key = bytes.ToLower(key)
	i, Ok := h[string(key)]
	if(!Ok) {
		return "", ErrKeyDoesntExist
	}
	return i, nil
}

func (h Headers) Set(key string, value string) {
	keyStr := bytes.ToLower([]byte(key))
	h[string(keyStr)] = string(value)
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