package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"time"
)

func udpConnection(conn *net.UDPConn) <-chan string {
	ch := make(chan string)
	go func(connection *net.UDPConn) {
		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Printf("> ")
			str, err := reader.ReadString('\n')
			
			if err != nil {
				fmt.Printf("Error reading input: %s", err)
				break
			}

			ch <- str
			time.Sleep(500 * time.Millisecond)
			_, err = connection.Write([] byte(str))
			
			if err != nil {
				fmt.Printf("Error writing to UDP: %s\n", err)
				continue
			}
		}
	close(ch)
	}(conn)
	return ch
}

func main() {
	addr, err := net.ResolveUDPAddr("udp", "localhost:42069")

	if err != nil {
		fmt.Printf("Error resolving: %s\n", err)
		return
	}
	conn, err := net.DialUDP("udp", nil, addr)

	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}
	defer conn.Close()

	for input := range udpConnection(conn) {
		fmt.Printf("Sent: %s\n", input)
	}

}