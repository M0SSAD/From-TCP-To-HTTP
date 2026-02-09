package main

import (
	"fmt"
	"log"
	"net"

	"boot.mossad.http/internal/request"
)

// func getLinesChannel(f io.ReadCloser) <-chan string {
// 	s := make(chan string, 1)

// 	go func() {
// 		defer f.Close()
// 		defer close(s)
// 		str := ""
// 		data := make([]byte, 8)
// 		for {
// 			n, err := f.Read(data)
// 			var chunk []byte
// 			if n > 0 {
// 				chunk = data[:n]
// 				for {
// 					i := bytes.IndexByte(chunk, '\n')
// 					if i == -1 {
// 						break
// 					}
// 					str += string(chunk[:i])
// 					s <- str
// 					str = ""
// 					chunk = chunk[i+1:]
// 					time.Sleep(500 * time.Millisecond) //Just to visualise the pipelining
// 				}
// 				str += string(chunk)
// 			}
// 			if err != nil {
// 				if err != io.EOF {
// 					fmt.Println("Read error:", err) // Log it, don't crash
// 				}
// 				break
// 			}
// 		}
// 		if len(str) != 0 {
// 			s <- str
// 		}
// 	}()

// 	return s
// }

func main() {
	ln, err := net.Listen("tcp", ":42069")
	// f, err := os.Open("../../message.txt")

	if err != nil {
		log.Fatal("error: ", err)
	}
	
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error: ", err)
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			fmt.Printf("Connection Has Been Accepted.\n")

			// for line := range getLinesChannel(conn) {
			// 	fmt.Printf("read: %s\n", line)
			// }
			req, err := request.RequestFromReader(c)

			if err != nil {
				fmt.Println("Error Getting Request: ", err)
				return
			}

			fmt.Printf("Request Line:\n- Method: %s\n- Target: %s\n- Version: %s\n", 
				req.RequestLine.Method, 
				req.RequestLine.RequestTarget, 
				req.RequestLine.HttpVersion)

			fmt.Println("Headers:")
			for key, value := range req.Headers {
				fmt.Printf("- %s: %s\n", key, value)
			}

			fmt.Printf("\nConeection Has Been Closed.\n")
		}(conn)
	}
	
}
