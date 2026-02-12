package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"boot.mossad.http/internal/request"
	"boot.mossad.http/internal/response"
	"boot.mossad.http/internal/server"
)

const port = 42069

func main() {
	server, err := server.Serve(port, handler)
	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
	defer server.Close()
	log.Println("Server started on port", port)

	// Common pattern in golang for gracefully shutting down a server
	// The program won't exit the main until a signal from the os is send like ctrl+c
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	log.Println("Server gracefully stopped")
}


const html200 = `<html>
  <head>
    <title>200 OK</title>
  </head>
  <body>
    <h1>Success!</h1>
    <p>Your request was an absolute banger.</p>
  </body>
</html>`

const html400 = `<html>
  <head>
    <title>400 Bad Request</title>
  </head>
  <body>
    <h1>Bad Request</h1>
    <p>Your request honestly kinda sucked.</p>
  </body>
</html>`

const html500 = `<html>
  <head>
    <title>500 Internal Server Error</title>
  </head>
  <body>
    <h1>Internal Server Error</h1>
    <p>Okay, you know what? This one is on me.</p>
  </body>
</html>`


func handler (w *response.Writer, req *request.Request)  {
	path := req.RequestLine.RequestTarget
	var body []byte
	var status response.StatusCode

	switch path {
	case "/400":
		status = response.StatusBadRequest
		body = []byte(html400)
	case "/500":
		status = response.StatusInternalServerError
		body = []byte(html500)
	default:
		// Default to 200 OK for "/" or any other path
		status = response.StatusOK
		body = []byte(html200)
	}

	err := w.WriteStatusLine(status)
    if err != nil {
		log.Printf("Failed to write status: %v", err)
        return 
    }

	h := response.GetDefaultHeaders(len(body))
    h.Set("Content-Type", "text/html")

	err = w.WriteHeaders(h)
    if err != nil {
		log.Printf("Failed to write headers: %v", err)
        return
    }

	_, err = w.WriteBody(body)
	if err != nil {
		log.Printf("Failed to write body: %v", err)
	}
}