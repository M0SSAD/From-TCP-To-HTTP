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

