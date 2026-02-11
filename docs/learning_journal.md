### Why Would I build an HTTP server from scratch?

**"No Libraries No Frameworks Hand Rolling HTTP From TCP, Only Using The TCP Library"**

- To understand TCP connections, socket programming, HTTP message parsing, and the request-response cycle without abstractions hiding the details.
- It demystifies what frameworks do behind the scenes, makes me a better debugger when things go wrong, and gives me deep knowledge of networking fundamentals that applies everywhere.

---

### Step 1: Reading Bytes (The Beginning)

- I started with trying to read 8 bytes at a time from a file in Golang. **WHY?**
  - Because that will lead up to understanding how to read from an HTTP stream.

```go
f, err := os.Open("message.txt")
if err != nil {
    log.Fatal("error: ", err)
}

for {
    data := make([]byte, 8)
    n, err := f.Read(data)
    if err != nil {
        log.Fatal("Finished: ", err)
        break
    }

    fmt.Printf("read: %s\n", string(data[:n]))
}
```

*Reading 8 bytes at a time is a good start, but 8 byte chunks aren't how people tend to communicate...*

---

### Step 2: Buffering & Parsing (Moving to Lines)

**The Problem with Simple Chunks**
- Messages rarely fit perfectly into 8-byte blocks. A single line might be split across two reads (e.g., "Hel" in chunk 1, "lo\n" in chunk 2).
- If I just print chunks as they come in, I break the message structure.

**The Solution: A Line Buffer**
- I need a variable (like `s`) to act as a "holding area."
- I keep appending bytes to `s` until I find a newline character `\n`.
- Once `\n` is found, I process the full line and clear `s` for the next one.

---

### Step 3: Handling Fragmentation (The "Dirty Buffer" & "Multiple Newlines")

**Critical Issue: The "Multiple Newlines" Bug**
- A single read (e.g., 8 bytes) might contain *two* newlines (e.g., `A\nB\n`).
- A simple `if` check only catches the first one. I must loop *inside* the read loop to catch them all.

**Critical Issue: The "Dirty Buffer"**
- `f.Read(data)` does not clear the buffer; it just overwrites.
- If I read 8 bytes, then 2 bytes, the last 6 bytes in the buffer are old "ghost" data.
- **Fix:** Always slice using `n` (the number of bytes actually read): `validData := data[:n]`.

**Code Evolution: The Robust Line Reader**

```go
f, err := os.Open("message.txt")
if err != nil {
    log.Fatal(err)
}

// 1. Allocating once outside loop saves memory
data := make([]byte, 8) 
var s string

for {
    n, err := f.Read(data)
    
    // 2. Handle EOF gracefully (it's not a fatal error)
    if err != nil {
        if err == io.EOF {
            break 
        }
        log.Fatal(err)
    }

    // 3. Only look at the valid bytes read
    chunk := data[:n]

    // 4. Inner loop to consume ALL newlines in this chunk
    for {
        i := bytes.IndexByte(chunk, '\n')
        
        // If no newline, break inner loop and append remainder to s
        if i == -1 {
            break 
        }

        // Found newline: complete line, print, reset s
        s += string(chunk[:i])
        fmt.Printf("read: %s\n", s)
        s = ""

        // Advance the chunk past the processed line
        chunk = chunk[i+1:]
    }

    // Append whatever is left to wait for the next read
    s += string(chunk)
}

// 5. Don't forget any leftovers after EOF
if len(s) > 0 {
    fmt.Printf("read: %s\n", s)
}
```

---

### Step 4: Deep Dive into Slices

**What does `chunk = chunk[i+1:]` actually do?**
- It creates a **new view** (window) into the same underlying array.
- It "slides" the start of the slice forward.
- **Crucially:** The indices reset. The byte at `i+1` becomes index `0` in the new slice.
- This allows me to "walk" through the buffer, processing one line at a time without copying the data to a new array.

---

### Step 5: Refactoring with Concurrency (Channels)

**Why use Channels?**
- In the previous version, the `main` function was stuck waiting for the disk to spin and read bytes.
- By moving the reading logic into a **Goroutine**, I decouple "getting the data" from "using the data."
- This is the **Generator Pattern**: A function that spins up a background worker and immediately returns a channel to listen to.

```Go
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
				if err == io.EOF {
					break
				}
				log.Fatal(err)
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
    f, err := os.Open("message.txt")

    if err != nil {
		log.Fatal("error: ", err)
	}

    lines := getLinesChannel(conn)
    for line := range lines {
        fmt.Printf("read: %s\n", line)
    }
}
```
**The Pipeline**
1.  **Producer (Goroutine):** Reads bytes, assembles lines, and pushes them into the pipe (channel).
2.  **Consumer (Main):** Stands at the other end of the pipe and processes lines as soon as they arrive.

**Critical Details**
- **`defer close(s)`**: This is mandatory. It tells the `range` loop in the main function "I am done, stop waiting." Without this, the main function would hang forever (deadlock).
- **Buffered Channels `make(chan string, 1)`**: Giving the channel a buffer allows the reader to prepare the next line while the main function is busy printing the previous one.

**Mistake to Avoid**
- **`len(channel)` vs `len(string)`**: Never check `len(myChannel)` to see if I have work left to do. The channel length only tells me what is *currently* buffered. Always check the data variable itself (`str`).

---

### Step 6: The Mechanics of `range` over Channels

**Blocking Synchronization**
- The receiver (`main`) cannot proceed until the sender (`goroutine`) is ready.
- This creates a natural "backpressure" mechanism—neither side can run too far ahead of the other.

**Termination**
- `range` loops over channels are infinite by default.
- They only stop when the sender explicitly calls `close(channel)`.
- **Always defer `close(channel)`** in the producer function to ensure the receiver knows when to stop waiting.

---

### The Pivot: From Disk to Network

**So far: I have built a concurrent system that reads a stream of bytes from a file.**

*Why did I do this?* Because in Go, a TCP connection (`net.Conn`) implements the exact same `io.Reader` interface as a file. 

*While reading from a file you pull the bytes from a file, but when reading from a network you get pushed with the data*

### Files vs. Network: The Stream Abstraction

**1. The "Everything is a Stream" Philosophy**
- **Insight:** To my code, a file on the disk and a TCP connection to a server are identical. They are just streams of bytes.
- **The Interface Magic:** This is why Go's `io.Reader` and `io.Writer` are so powerful.
  - Because my `getLinesChannel` function accepts an `io.ReadCloser`, it didn't care if I passed it `os.File` or `net.Conn`.
  - It treats them both as "something I can read bytes from."

**2. The Critical Difference: Pull vs. Push**

While the *interface* is the same, the *control flow* is radically different.

| Feature | Files (Pull) | Network (Push) |
| :--- | :--- | :--- |
| **Control** | **I am in control.** | **The Sender is in control.** |
| **Timing** | I decide when to read. | Data arrives whenever it wants. |
| **Amount** | I decide how much to read. | I get whatever the network delivers. |
| **Stopping** | I stop when I hit EOF. | I stop only when the connection dies. |

**The Engineering Implication:**
- With files, I "pull" data at my own pace.
- With networks, data is "pushed" to me. My code must be **reactive**. It must sit and wait (block) until the data arrives. This is why Concurrency (Goroutines) became necessary immediately after we switched to TCP—we have to wait for data without freezing the whole program.
  
**Next Step:** I will swap the `os.File` for a `net.Conn`. The parsing logic I just wrote shouldn't know the difference.

*When data is sent over a network, it is sent in packets. Each message is split into packets, the packets are sent, they arrive (potentially) out of order, and they are reassembled on the other side. And without a protocol like TCP, you can't guarantee that the order is correct...*

*You might end up with "i am evil" instead of "i am live"! TCP solves this problem.*

### Step 6: The Pivot to TCP (The Interface Power Move)

**The Hypothesis**
- In Go, a TCP connection (`net.Conn`) and a File (`os.File`) both implement the exact same interface: `io.ReadCloser`.
- **Theory:** I should be able to swap the file for a network connection without changing a single line of my parsing logic (`getLinesChannel`).

**The Implementation**
- I replaced `os.Open` with `net.Listen` and `ln.Accept`.
- I passed the TCP connection directly into my existing `getLinesChannel` function.

```go
func main() {
    // 1. Listen on a TCP port
    ln, err := net.Listen("tcp", ":42069")
    if err != nil {
        log.Fatal("error: ", err)
    }
    
    fmt.Println("Server listening on :42069")

    for {
        // 2. Wait for a new client to connect
        conn, err := ln.Accept()
        if err != nil {
            fmt.Println("Error: ", err)
            continue
        }

        // 3. Pass the network connection to the SAME parser we used for files
        // This works because 'conn' satisfies 'io.ReadCloser'
        for line := range getLinesChannel(conn) {
            fmt.Printf("read: %s\n", line)
        }
        
        // 4. Close properly
        conn.Close()
        fmt.Printf("Connection Closed.\n")
    }
}
```

**How to Test**
- Instead of a text file, I now use `netcat` (or `telnet`) to act as the client.
- Command: `nc localhost 42069`
- Typing into the terminal sends bytes to the server, which parses them line-by-line.

**Current Observation**
- The server successfully accepts a connection.
- It reads data, buffers it, and prints it line-by-line.
- **However:** The code is currently running in a single loop. I need to verify what happens if a second client tries to connect while the first one is still talking...

### TCP vs UDP: The Transport Layer Rivals

**Why use TCP? (The "Reliable" Choice)**
- **Connection-Oriented:** Requires a handshake (SYN, SYN-ACK, ACK) before any data moves.
- **Ordered:** Guarantees packets arrive in the exact order they were sent (1, 2, 3...).
- **Reliable:** If a packet is lost, TCP resends it.
- **Analogy:** A phone call. *"Hello? Can you hear me? Good. Here is the message..."*

**Why use UDP? (The "Yeet" Choice)**
- **Connectionless:** No handshake. Just start sending.
- **Unordered:** Packets might arrive out of order (1, 3, 2...).
- **Unreliable:** If a packet drops, it's gone forever.
- **Fast:** Lower latency because there is no overhead for error checking or ordering.
- **Analogy:** Screaming mail. You throw letters at the house; you don't care if they actually catch them.

[Image of TCP vs UDP data flow diagram]

| Feature | TCP | UDP |
| :--- | :--- | :--- |
| **Connection** | Yes | No |
| **Handshake** | Yes | No |
| **Order** | Guaranteed | Chaos |
| **Speed** | Slower | Blazingly Fast |

---

### Project Restructuring & CLI Testing

**1. Organizing the Workspace**
- **Run Command:** `go run ./cmd/tcplistener`

**2. The "Connection Refused" Lesson**
- **Experiment:** Try running `nc -v localhost 42069` *without* the server running.
- **Result:** `Connection refused`.
- **Why?** Because TCP *requires* a handshake. The operating system rejects the incoming SYN packet because no process is listening on that port.
- **Contrast:** If this were UDP, there is no handshake, so the client might not know the server is down until it tries to send data and gets no reply (or an ICMP error).

**3. Useful Tool: `tee`**
- We want to see the logs *and* save them to a file for debugging.
- **Command:** `go run ./cmd/tcplistener | tee /tmp/tcplistener.txt`
- `tee` splits the output stream: one copy goes to stdout (screen), one copy goes to the file.

### Step 7: The UDP Sender ("Yeet" Protocol)

**The Difference in Setup**
- **TCP:** Required `ln.Accept()` on the server and `net.Dial` on the client. The handshake had to happen before I could send a single byte.
- **UDP:** I used `net.ResolveUDPAddr` and `net.DialUDP`.
  - **Wait, "Dial"?** The name `DialUDP` is a bit of a lie. In UDP, there is no "connection."
  - When I "dial" UDP, Go just stores the destination IP locally so it knows where to aim the packets. It doesn't send anything to the network yet.



**The "Connection Refused" Trap**
- I ran the sender *without* the listener running.
- **Result:** No error! The program happily sent bytes into the void.
- **Why?** UDP is "fire and forget." It doesn't care if anyone is listening.
- **Exception:** Sometimes, if the OS is helpful, it might send back an **ICMP Port Unreachable** message, which causes the *next* write to fail. This is the OS "yeeting" me back.

**Critical Bug: The "Hungry Buffer"**
- I initially put `bufio.NewReader(os.Stdin)` *inside* my `for` loop.
- **The specific failure:** `bufio` reads big chunks (4KB) at a time. If I pasted 3 lines of text:
  1. Reader 1 consumes all 3 lines from Stdin into its buffer.
  2. It returns Line 1.
  3. The loop restarts. Reader 1 (holding Lines 2 & 3) is overwritten by Reader 2.
  4. Reader 2 looks at Stdin, sees it's empty, and waits.
  5. **Result:** Lines 2 & 3 are lost forever.
- **Fix:** Always initialize buffered readers *outside* the loop.

```go
// BAD
for {
    reader := bufio.NewReader(os.Stdin) // Creates a new buffer every time
    text, _ := reader.ReadString('\n')
}

// GOOD
reader := bufio.NewReader(os.Stdin) // Buffer created once
for {
    text, _ := reader.ReadString('\n') // reuses the same buffer
}
```
**My Code:**
```Go
func udpConnection(conn *net.UDPConn) <-chan string {
	ch := make(chan string)
	go func(connection *net.UDPConn) {
		reader := bufio.NewReader(os.Stdin)
		for {
			
			fmt.Printf("> ")
			str, err := reader.ReadString('\n')
			
			if err != nil {
				fmt.Printf("Error reading input: %s\n", err)
				break
			}

			
			_, err = connection.Write([] byte(str))
			
			if err != nil {
				fmt.Printf("Error writing to UDP: %s\n", err)
				continue
			}
			ch <- str
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
```

### Deep Dive: The `net` Library (UDP Edition)

In the UDP sender, I used two specific functions to set up the connection. Here is what they actually do under the hood.

#### 1. `net.ResolveUDPAddr(network, address string)`

* **What it does:**
    It takes a human-readable string like `"localhost:42069"` and converts it into a `*net.UDPAddr` struct. This involves:
    1.  **Parsing:** Splitting the IP and Port.
    2.  **DNS Lookup:** If you give it a domain name (e.g., `google.com`), it asks the DNS server for the IP address.
    
* **Why we used it:**
    The older `net.DialUDP` function *requires* this specific struct type (`*net.UDPAddr`) as an argument. It won't accept a simple string.

#### 2. `net.DialUDP(network string, laddr, raddr *net.UDPAddr)`

* **What it does:**
    It creates a `net.UDPConn`.
    * **"Dialing" in UDP:** Unlike TCP, this does **not** send any packets to the network. It simply sets the "default destination" for this socket.
    * **Effect:** When you later call `conn.Write()`, the OS knows exactly where to send the packet without you having to specify the address every single time.
    
* **Arguments:**
    * `laddr` (Local Address): We passed `nil`. This tells the OS, "I don't care which port I send *from*, just pick an open one."
    * `raddr` (Remote Address): The target we resolved earlier (`localhost:42069`).

---

### Is there a better way? (The Modern Standard)

Yes. While `ResolveUDPAddr` and `DialUDP` are precise, they are a bit verbose.

In modern Go code, we often prefer the generic **`net.Dial`** function.

```go
// The "Old" Way (Specific)
addr, _ := net.ResolveUDPAddr("udp", "localhost:42069")
conn, _ := net.DialUDP("udp", nil, addr)

// The "New" Way (Standard)
conn, _ := net.Dial("udp", "localhost:42069")
```

**Why use `net.Dial` instead?**
1.  **Polymorphism:** It returns a `net.Conn` interface (same as TCP!). This means I can write code that doesn't care if it's using TCP or UDP.
2.  **Convenience:** It handles the address resolution (`ResolveUDPAddr`) automatically inside.
3.  **Refactoring:** If I want to switch to TCP later, I just change the string `"udp"` to `"tcp"`.

**Why did we use the verbose way?**
To understand the mechanics. `DialUDP` returns a `*net.UDPConn`, which exposes UDP-specific methods (like `ReadFromUDP` and `WriteToUDP`) that give us more control over individual packets, which is critical when building low-level servers.

### Phase 2: Building HTTP/1.1

**The Target**
We are building **HTTP/1.1**.
* *Why not HTTP/2 or HTTP/3?*
    * HTTP/1.1 is text-based and readable by humans.
    * HTTP/2 and HTTP/3 are binary protocols optimized for performance, but they obscure the fundamental "Request/Response" semantics. To understand the web, you must master 1.1 first.

**The Map: Request For Comments (RFCs)**
The internet is built on documents called RFCs. They are the technical laws that browsers and servers must follow to talk to each other.



**The RFC Landscape for HTTP/1.1**
It is messy because the spec has been rewritten multiple times to be clearer.

| RFC | Status | Description |
| :--- | :--- | :--- |
| **RFC 2616** | **Deprecated** | The old bible of HTTP. Do not read this. It is obsolete. |
| **RFC 7231** | **Active** | Widely referenced, but very verbose. |
| **RFC 9110** | **Semantics** | The dictionary. Defines *what* things mean (e.g., what a "404" is, what "GET" implies). |
| **RFC 9112** | **Messaging** | The blueprint. Defines *how* to format the bytes on the wire. |

**Our Strategy**
We will focus on **RFC 9112** and **RFC 9110**.
* **Why?** They separate the "Wire Format" (9112) from the "Meaning" (9110).
* **RFC 9112** is concise and tells us exactly where to put the spaces and newlines.
* **RFC 9110** tells us what to do once we parse those bytes.

### HTTP Anatomy: GET vs. POST

**1. The Shape of a Request**
By capturing the output of `curl`, I can finally see what an HTTP request looks like on the wire.

**A GET Request (No Body)**
```http
GET /goodies HTTP/1.1
Host: localhost:42069
User-Agent: curl/8.6.0
Accept: */*
```
* **Request Line:** `Method Path Version`
* **Headers:** Metadata about the request.
* **Body:** Empty (for GET).

**A POST Request (With Body)**
```http
POST /coffee HTTP/1.1
Host: localhost:42069
User-Agent: curl/8.6.0
Accept: */*
Content-Type: application/json
Content-Length: 23

{"flavor":"dark mode"}
```
* **Note:** There is always an empty line (CRLF) separating the headers from the body.

---

### The "Hanging Body" Problem

**The Experiment**
I sent a POST request with a JSON body:
`curl -X POST -d '{"flavor":"dark mode"}' ...`

**The Observation**
1.  The headers printed immediately.
2.  The program **hung**. The body `{"flavor":"dark mode"}` did not appear.
3.  When I killed `curl`, the body suddenly appeared.



**The Reason: The Flaw in "Line Reading"**
My current TCP listener is designed to **read lines** (waiting for `\n`).
* HTTP Headers *do* end in newlines.
* HTTP Bodies **do not** necessarily end in newlines.

The `curl` command sent the JSON data (no newline at the end) and then waited for a response. My server sat there waiting for a `\n` that never came. It only printed when the connection closed (EOF), forcing the buffer to flush.

**Conclusion:** I cannot parse HTTP bodies using a simple `Scanner` or `ReadString('\n')`. I must read the headers, find the `Content-Length`, and then read exactly that many bytes.

---

### HTTP Parser Setup & Testing Philosophy

**1. Project Structure: The `internal` Directory**
- I created `internal/request`.
- **Rule:** In Go, packages inside `internal/` are private. They can be imported by my code, but if someone else imports my project as a library, the compiler forbids them from accessing `internal`.
- **Why:** The HTTP parser is the engine room. I don't want users importing it directly; they should use the public server API I build later.

**2. Testing Strategy: Table-Driven vs. Assertions**

| Feature | Table-Driven (Standard) | Assertions (Testify) |
| :--- | :--- | :--- |
| **Logic** | Loop over struct slice | Linear, step-by-step |
| **Readability** | Compact, high abstraction | Verbose, explicit |
| **Debugging** | Harder (generic error msgs) | Easier (line number = exact failure) |
| **Use Case** | Simple inputs/outputs | Complex state & parsers |

**Decision:** We are using **Procedural/Assert tests** because the parser logic will be complex. We want the tests to be "dumb" and explicit so we don't have to debug the tests themselves.

---

### HTTP Request Parser: Phase 1 (Request Line)

**Goal:** Parse the "Start-Line" of an HTTP request (e.g., `GET /index.html HTTP/1.1`) while validating format and version.

#### 1. The Strategy: "Slurp and Cut"
Instead of reading byte-by-byte (complex), I opted for a simpler initial approach to get the logic working:
1.  **Slurp:** Read the entire input into memory using `io.ReadAll`.
2.  **Isolate:** Extract *only* the first line (`\r\n`).
3.  **Parse:** Split that line into its three required components.

#### 2. Key Technical Decisions

**A. Using `bytes.Cut` over `strings.Split`**
I used `bytes.Cut` for safety and precision.
* **Safety:** `strings.Split` creates a slice of all parts. If the input is malformed, accessing `index[2]` causes a panic (crash). `bytes.Cut` returns a boolean `found` flag, allowing me to handle errors gracefully.
* **Precision:** It splits on the *first* occurrence only, preventing me from accidentally chopping up data later in the string.

**B. Isolating the Request Line**
* **The Problem:** If I pass the whole request to the parser, it might find spaces inside the **Headers** and think they belong to the Request Line.
* **The Fix:**
    ```go
    line, _, found := bytes.Cut(data, []byte("\r\n"))
    ```
    This ensures `parseRequestLine` only ever sees the first line (`GET / HTTP/1.1`), guaranteeing that any spaces found are actual delimiters.

**C. Validation Rules**
I implemented strict validation per RFC 9112/9110:
1.  **Method:** Must be uppercase alphabetic characters (A-Z).
    * *Implementation:* Iterated over bytes checking `char < 'A' || char > 'Z'`.
2.  **Version:** Must be exactly `HTTP/1.1`.
    * *Implementation:* I cut the prefix `HTTP/` (using `/` as separator) and strictly compared the remainder to `"1.1"`.

#### 3. Code Walkthrough

```go
func RequestFromReader(reader io.Reader) (*Request, error) {
    // ... read all data ...

    // 1. ISOLATE the first line (Critical Step)
    line, _, found := bytes.Cut(data, []byte("\r\n"))
    if !found {
        return nil, fmt.Errorf("invalid request: no newlines found")
    }

    // 2. Parse ONLY that line
    requestLine, err := parseRequestLine(line)
    // ...
}

func parseRequestLine(data []byte) (*RequestLine, error){
    // 1. Extract Method (Separated by Space)
    method, rest, found := bytes.Cut(data, []byte(" "))
    if !found { return nil, Error... }

    // 2. Extract Target (Separated by Space)
    target, rest, found := bytes.Cut(rest, []byte(" "))
    if !found { return nil, Error... }

    // 3. Extract Version Prefix ("HTTP/")
    _, rest, found = bytes.Cut(rest, []byte("/"))
    if !found { return nil, Error... }

    version := rest // Remainder is the version number (e.g. "1.1")

    // 4. Validate Method (Uppercase only)
    for _, char := range method {
        if char < 'A' || char > 'Z' { return nil, Error... }
    }

    // 5. Validate Version (Must be 1.1)
    if string(version) != "1.1" { return nil, Error... }

    return &RequestLine{...}, nil
}
```

#### 4. Future Optimization
Currently, `io.ReadAll` reads the *entire* request (including potentially large bodies) just to parse the first line. In the future, I should switch to `bufio.Reader` to read line-by-line to 
prevent memory exhaustion attacks.

---

### Phase 2: Streaming Architecture & State Machine

**Goal:** Move away from "Slurp All" (which blocks and wastes memory) to a "Streaming" approach that handles data as it arrives in chunks.

#### 1. The "Chunked Reading" Strategy
Instead of waiting for the entire request to arrive (which might never happen in "Keep-Alive" connections), I implemented a buffer loop:
1.  **Read:** Grab a small chunk (e.g., 1024 bytes) from the connection.
2.  **Append:** Add it to a temporary buffer (`buf`).
3.  **Try Parse:** Attempt to parse the buffer.
4.  **Slice:** If parsing succeeded, remove the used bytes from the buffer. If not, keep them and wait for the next chunk.

**Why this matters:**
This allows the server to handle slow clients (who send `"G"`, wait 1 second, send `"ET"`) without hanging or crashing.

#### 2. The State Machine Pattern
I introduced a `parserState` enum to track progress. This turns the parser into a "Brain" that knows what to look for next.

```go
type parserState int
const (
    stateInitialized parserState = iota // 0: Waiting for Request Line
    stateDone                           // 1: Finished (for now)
)
```

**The logic flow:**
* **Input:** `RequestFromReader` feeds raw bytes to `req.parse()`.
* **Decision:** `req.parse()` checks `req.state`.
    * If `Initialized` -> Call `parseRequestLine`.
    * If `Done` -> Stop.

#### 3. Handling Partial Data (The "0 Bytes" Rule)
The most critical change is how `parseRequestLine` handles missing data.
* **Old Way:** If `\r\n` is missing -> Error "Invalid Request".
* **New Way:** If `\r\n` is missing -> Return `0` bytes consumed, `nil` error.

This signals the main loop: *"I see data, but not enough to form a complete line. Please read more from the network and ask me again."*

#### 4. Code Walkthrough: The Read Loop

```go
func RequestFromReader(reader io.Reader) (*Request, error) {
    req := newRequest()
    buf := make([]byte, 0)      // Accumulates data
    chunk := make([]byte, 1024) // Temp read buffer

    for req.state != stateDone {
        // 1. Read from wire
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

        // 2. Append to accumulator
        buf = append(buf, chunk[:numBytesRead]...)

        // 3. Try to parse
        numBytesParsed, err := req.parse(buf)
        if err != nil { return nil, err }

        // 4. Slide the Window (Discard parsed bytes)
        if numBytesParsed > 0 {
            buf = buf[numBytesParsed:]
        }
    }
    return &req, nil
}
```

#### 5. The State Enum (Iota)
To manage the parser's lifecycle, I created a custom type using Go's `iota` identifier. This allows me to define a sequence of related constants that automatically increment.

```go
type parserState int

const (
    stateInitialized parserState = iota // Value: 0
    stateDone                           // Value: 1
)
```
**Why:** This prevents "magic numbers" in the code. Instead of checking `if state == 0`, I check `if state == stateInitialized`, making the logic readable and type-safe.

#### 6. Dynamic Error Constructors
Instead of static error variables, I implemented **Error Constructor Functions**.

```go
func ErrorInvalidMethod(method string) error {
    return fmt.Errorf("invalid method: %s", method)
}

func ErrorInvalidVersion(version string) error {
    return fmt.Errorf("Unsupported HTTP Version: %s", version)
}
```
**Why:** Static variables (like `var ErrInvalidMethod = ...`) cannot contain dynamic context. By using functions, I can include the *specific* invalid value (e.g., "GET123") in the error message, which is crucial for debugging.

#### 7. The Parse "Router"
The `parse` method is the heart of the state machine. It doesn't do the parsing itself; it decides *which* parser function to call based on the current state.

```go
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
```
**Key Logic:**
* **Input:** A slice of bytes (`p`).
* **Output:** How many bytes were successfully used (`n`).
* **Flow:** It checks `r.state`. If initialized, it calls `parseRequestLine`. If that succeeds, it transitions to `stateDone`.

---

### Phase 3: TCP Listener Integration & Concurrency

**Goal:** Connect the generic `RequestFromReader` parser to a real TCP socket and handle multiple clients simultaneously.

#### 1. The Interface Magic
Connecting the components was seamless because `net.Conn` implements the `io.Reader` interface.
* **Parser Expects:** `func RequestFromReader(reader io.Reader)`
* **TCP Provides:** `conn, _ := ln.Accept()`
* **Result:** I can pass the active network connection directly to the parser without any adapter code. This validates the decision to code against interfaces rather than concrete types.

#### 2. Resource Management (The Defer Pattern)
A critical issue in long-running servers is **Resource Leaks**.
* **Problem:** If a connection is not closed, the File Descriptor remains open indefinitely. Eventually, the OS limits are reached ("Too many open files"), and the server crashes.
* **Fix:** I applied the `defer` keyword immediately after accepting the connection.
  ```go
  defer conn.Close()
  ```
  This guarantees cleanup happens even if the parser panics or returns an error early.

#### 3. Concurrency: Moving from Blocking to Non-Blocking
The initial implementation handled requests in the main loop:
1. Accept A -> 2. Process A -> 3. Accept B
**Flaw:** If Client A is slow (or malicious), Client B is blocked forever.

**The Fix (Goroutines):**
I moved the processing logic into a generic background worker using `go func`.
1. Accept A -> Spawn Worker for A (Background)
2. Immediately Loop back to Accept B.

#### 4. The "Loop Variable Closure" Trap (Critical Bug)
I encountered a classic Go concurrency bug when using anonymous functions inside a `for` loop.

**The Bug:**
```go
for {
    conn, _ := ln.Accept()
    go func() {
        // DANGER: Using 'conn' from the outer loop!
        handle(conn) 
    }()
}
```
Because the goroutine starts *after* a small delay, the main loop might have already accepted a *new* connection and updated the `conn` variable. The worker for Client A might accidentally read from Client B's connection.

**The Solution (Shadowing):**
I must pass the variable *into* the function to create a local copy.
```go
go func(c net.Conn) { // 'c' is a local copy strictly for this worker
    handle(c)
}(conn) // Pass current value here
```

**Code Walkthrough:**

```Go
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

			fmt.Printf("Request line:\n- Method: %s\n- Target: %s\n- Version: %s", req.RequestLine.Method, req.RequestLine.RequestTarget, req.RequestLine.HttpVersion)

			fmt.Printf("\nConeection Has Been Closed.\n")
		}(conn)
	}
	
}
```

---

### Phase 4: Parsing Headers

**Goal:** Parse the headers section of the HTTP request into a `map[string]string`.

#### 1. The Headers Package
I created a dedicated package `headers` because headers are used in both Requests (parsing) and Responses (sending).
* **Type:** `type Headers map[string]string`
* **Constructor:** `func NewHeaders() Headers`

#### 2. Field-Line Syntax (RFC 9110)
Structure: `Key: Value\r\n`
* **Colon Rule:** No whitespace allowed between the Key and the Colon (`Key : Val` is invalid).
* **Value Rule:** Whitespace is allowed/ignored around the value (`Key:   Val   ` becomes `"Val"`).
* **Indentation:** Historically, headers could be indented (folded), though modern HTTP discourages it. The parser supports it by trimming leading space from keys.

#### 3. The Parse Loop Logic
The `Parse` method parses **one header line at a time**.
* **Input:** The raw byte slice starting from the current position.
* **Logic:**
    1.  Find the next `\r\n`.
    2.  **Empty Line Check:** If `\r\n` is at index 0, we have hit the blank line separating Headers from Body. Return `done=true`.
    3.  **Cut & Clean:** Split on the first colon (`:`). Check for spaces before the colon (Strict validation). Trim spaces around the value.
    4.  **Store:** Add to the map.
    5.  **Return:** Number of bytes consumed so the caller can advance the buffer.

#### 4. Validating the "No Space Before Colon" Rule
To satisfy the RFC requirement that `Host : localhost` is invalid, I implemented a specific check:
```go
// After cutting on ":", check the last byte of the key
if key[len(key)-1] == ' ' {
    return error
}
```

#### 5. Code WalkThrough:

```Go
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
		n += 2
		return n, done, nil
	}

	
	header := data[:idx]
	
	key, remaining, found := bytes.Cut(header, []byte(":"))
	
	if !found {
		return 0, done, ERROR_CANNOT_PARSE_KEY_FROM_VALUE
	}
	
	if len(key) == 0 {
         return n, false, ERROR_MALFORMED_KEY
    }

	if key[len(key) - 1] == ' ' {
		return 0, done, ERROR_INVALID_HEADER_SPACE_BETWEEN_KEY_AND_COLOUMN
	}
	
	n += idx + 2
	value := bytes.TrimSpace(remaining)
	key = bytes.TrimLeft(key, " ")

	h[string(key)] = string(value)
	return n, done, nil
}
```
---

### Phase 4b: Header Validation & Normalization (RFC 9110)

**Goal:** Enforce strict RFC compliance for Header Keys (Field Names) regarding allowed characters and case insensitivity.

#### 1. The "Case Insensitivity" Problem
In Lesson 1, `Host` and `host` were treated as two different keys in the map.
* **The RFC:** "Field names are case-insensitive."
* **The Implication:** `Content-Length` and `content-length` must act as the same key.
* **The Fix:** I implemented **Normalization**. Before adding a key to the map, I convert it to lowercase. This ensures that `h["content-length"]` always retrieves the value, regardless of how the client sent it (e.g., `CoNtEnT-LeNgTh`).

#### 2. Strict Character Validation (The Token Rule)
Lesson 1 allowed any string as a key. Lesson 2 restricts this to valid "Tokens" defined in RFC 9110 Section 5.6.2.
* **Allowed:** Alphanumeric (`a-z`, `0-9`) and specific symbols (`! # $ % & ' * + - . ^ _ ` | ~`).
* **Forbidden:** Spaces, control characters, and other symbols (like `©`, `@`, `[`, `]`).

#### 3. Implementation: The Validation Loop
I refactored the parsing logic to include a pass over the key bytes.

**What differs from Lesson 1:**
Instead of just slicing bytes (`key := data[:idx]`), I now allocate a new slice (`normalizedKey`) and process it byte-by-byte.

```go
// 1. Create a safe lower case copy of the key
normalizedKey := bytes.ToLower(key)
for _, b := range normalizedKey {
    if !isTokenChar(b) {
        return n, done, ErrInvalidCharInKey
    }
}
```

#### 4. Helper Function: `isValidHeaderChar`
To keep the main logic clean, I extracted the RFC character list into a helper function.
* **Input:** `byte`
* **Output:** `bool`
* **Logic:** Checks ranges (`a-z`, `0-9`) and a `switch` statement for the specific allowed symbols.

#### Summary of Changes

| Feature | Lesson 1 (Basic) | Lesson 2 (Compliant) |
| :--- | :--- | :--- |
| **Map Keys** | Case-sensitive (`Host` != `host`) | **Normalized** (`host` == `Host`) |
| **Invalid Chars** | Allowed (`H@st` was valid) | **Rejected** (`H@st` returns Error) |
| **Safety** | Sliced original buffer | **Allocates** new key copy |


---

### Phase 4c: Handling Duplicate Headers (Multi-Value Fields)

**Goal:** Handle cases where the client sends the same header key multiple times (e.g., `Set-Cookie`, `Via`, or custom lists).

#### 1. The RFC Rule (RFC 9110 Section 5.2)
HTTP allows a header field name to appear multiple times in a message. The receiver must treat this as a single header where the values are joined by a **comma**.

**Example:**
```text
My-List: Item 1
My-List: Item 2
```
Must become: `My-List: Item 1, Item 2`

#### 2. Implementation
I updated the map insertion logic to check for existence first.

```go
// 8. Update Map (Handle Duplicates)
keyStr := string(normalizedKey)
valStr := string(value)

// Check if key already exists
if existingValue, ok := h[keyStr]; ok {
    // Found! Append with comma
    h[keyStr] = existingValue + ", " + valStr
} else {
    // New key, just insert
    h[keyStr] = valStr
}
```

**Why string conversion?**
I cast `normalizedKey` (byte slice) to `string` because Go maps require comparable types as keys, and `[]byte` (a slice) is not comparable. This allocation is necessary.

---

### Phase 5: Integrating Headers & The Parsing Loop

**Goal:** Hook the standalone `Headers.Parse` logic into the main Request State Machine so we can parse a full HTTP request (Request Line + Headers).

#### 1. The Challenge: The "Cursor Problem"
In previous lessons, we only parsed one thing (the Request Line) at the start of the buffer. Now, we need to parse multiple things (Request Line -> Header 1 -> Header 2 -> ...).

**The Bug:**
If we simply loop and call `parse(data)`, the function always starts looking at index 0.
1.  **Round 1:** Parses Request Line (`GET /...`). Success.
2.  **Round 2:** Still looks at index 0! It sees `GET /...` again, but now it expects a Header. It fails because `GET` is not a valid header key.

**The Fix (Sliding Window):**
We need to "slide the window" forward after every successful parse. We do this by tracking `totalBytesParsed`.

#### 2. The Solution: Driver vs. Logic Split
To solve the cursor problem cleanly, I refactored the parsing into two distinct methods:

**A. The Driver (`parse`)**
* **Role:** Manages the loop and the cursor (`totalBytesParsed`).
* **Logic:** It calls `parseSingle` with a slice of the data that *hasn't been parsed yet* (`data[totalBytesParsed:]`).
* **Why:** This ensures the state machine always sees fresh data at index 0.

**B. The Logic (`parseSingle`)**
* **Role:** The State Machine. It looks at `r.state` and decides *what* to parse next.
* **Logic:** It parses exactly **one item** (one request line OR one header) and returns how many bytes it used.

#### 3. The New State: `requestStateParsingHeaders`
I added a new state to the enum to bridge the gap between "Initialized" and "Done".

* **Transition 1 (Init -> Headers):** After `parseRequestLine` succeeds, we switch to `requestStateParsingHeaders`.
* **Transition 2 (Headers -> Done):** Inside the header loop, if `headers.Parse` returns `done=true` (meaning it found the empty line `\r\n`), we switch to `requestStateDone`.

#### 4. Handling Partial Data (The "Safety Check")
A critical edge case is when the stream cuts off in the middle of a header (e.g., `User-Ag`).

* **The Behavior:** The parser waits for `\r\n`. If it doesn't find it, it returns `0 bytes consumed`.
* **The Result:** The Driver loop sees `0` and breaks. The unparsed bytes remain in the buffer.
* **Why this is safe:** It prevents the parser from crashing or inventing data. It effectively says, "I'm pausing here until the network sends the rest."

#### 5. Final Code Structure

```go
// The Driver (Manages the Loop)
func (r *Request) parse(data []byte) (int, error) {
    totalBytesParsed := 0
    for r.state != requestStateDone {
        // Safety: Stop if we run out of data
        if totalBytesParsed >= len(data) { break }

        // Pass only the Remaining Data
        n, err := r.parseSingle(data[totalBytesParsed:])
        if err != nil { return totalBytesParsed, err }
        
        // Stop if we need more data
        if n == 0 { break }

        totalBytesParsed += n
    }
    return , nil
}

// The Logic (Manages the State)
func (r *Request) parseSingle(data []byte) (int, error) {
    switch r.state {
    case requestStateInitialized:
        // ... parse request line ...
        r.state = requestStateParsingHeaders // Switch!
        return n, nil

    case requestStateParsingHeaders:
        // Initialize map if needed
        if r.Headers == nil { r.Headers = NewHeaders() }
        
        n, done, err := r.Headers.Parse(data)
        // ... handle err ...
        
        if done {
            r.state = requestStateDone // Finish!
        }
        return n, nil
    }
    return 0, nil
}
```

#### 6. Verification (Tests)
I added comprehensive tests to verify the integration:
* **Case Insensitivity:** `CoNtEnT-TyPe` -> `content-type`
* **Duplicate Headers:** `My-List: A` + `My-List: B` -> `My-List: A, B`
* **Partial Data:** `User-Ag` (cutoff) -> safely ignored until more data arrives.

---

### Phase 6: Parsing the Request Body

**Goal:** Parse the optional message body (e.g., JSON payload in a POST request) based on the `Content-Length` header.

#### 1. Struct Update
I added a byte slice to hold the raw body data.

```go
type Request struct {
    RequestLine RequestLine
    Headers     headers.Headers
    Body        []byte      // <--- New Field
    state       parserState
}
```

#### 2. The Logic: Handling "Content-Length"
Parsing the body is different from headers because it is not terminated by a specific character (like `\r\n`). Instead, we must read exactly `Content-Length` bytes.

**The State Transition:**
I modified `requestStateParsingHeaders` to decide the next step dynamically:
1.  **Headers Done:** When `headers.Parse` returns `done=true`.
2.  **Check Content-Length:**
    * **Missing:** Assume no body (standard for Requests). Transition to `stateDone`.
    * **Present:** Transition to `stateParsingBody`.

#### 3. The "Body" State Logic
Inside `requestStateParsingBody`, the logic is:
1.  **Read Target:** Parse `Content-Length` string to `int`.
2.  **Consume:** Append **all** available bytes in the buffer to `r.Body`.
3.  **Validate:**
    * If `len(Body) > Content-Length`: **Error** (Payload too large).
    * If `len(Body) == Content-Length`: **Success** (Transition to `stateDone`).
    * If `len(Body) < Content-Length`: **Wait** (Return 0, wait for more TCP chunks).



#### 4. Critical Fix: "Unexpected EOF"
I encountered a bug where tests failed because the loop would exit successfully even if the body was incomplete (e.g., expected 100 bytes, got 50).

**The Fix:**
I added a validation check *after* the read loop in `RequestFromReader`.

```go
// Inside RequestFromReader...
for req.state != requestStateDone {
    // ... read loop ...
    if n == 0 { break } // EOF
}

// VALIDATION: Did we actually finish?
if req.state != requestStateDone {
    return nil, fmt.Errorf("unexpected EOF: request incomplete")
}
```

#### 5. Final Code Snippet (Body Parsing)

```go
case requestStateParsingBody:
    // 1. Get Target Length
    clVal := r.Headers.Get("Content-Length")
    expectedLen, _ := strconv.Atoi(clVal)

    // 2. Accumulate Data
    r.Body = append(r.Body, p...)

    // 3. Validation
    if len(r.Body) > expectedLen {
        return len(p), fmt.Errorf("body length %d exceeds Content-Length %d", len(r.Body), expectedLen)
    }

    // 4. Completion
    if len(r.Body) == expectedLen {
        r.state = requestStateDone
    }

    return len(p), nil
```