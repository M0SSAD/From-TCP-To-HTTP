package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

func getLinesChannel(f io.ReadCloser) <-chan string {
	s := make(chan string, 1)
	
	go func() {
		defer f.Close()
		defer close(s)
		str := ""
		data := make([]byte, 8)
		for {
			n, err := f.Read(data)
			if err != nil {
				if err != io.EOF {
					fmt.Println("Read error:", err) // Log it, don't crash
				}
				break
			}
			chunk := data[:n]
			for {
				i := bytes.IndexByte(chunk, '\n')
				if i == -1 {
					break 
				}
				str += string(chunk[:i])
				s <- str
				str = ""
				chunk = chunk[i+1:]
				time.Sleep(500 * time.Millisecond) //Just to visualise the pipelining
			}
			str += string(chunk)
		}
		if len(str) != 0 {
			s <- str
		}
	}()

	return s
}

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
		fmt.Printf("Connection Has Been Accepted.\n")

		for line := range getLinesChannel(conn) {
			fmt.Printf("read: %s\n", line)
		}

		fmt.Printf("Coneection Has Been Closed.\n")
	}
	
}
