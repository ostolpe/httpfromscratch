package main

import (
	"fmt"
	"net"
)
func main() {
	server, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		panic(err)
	}
	defer server.Close()	
	
	server.Write([]byte("GET /hello HTTP/1.1\r\nHost:localhost:8080\r\n\r\n"))
	buffer := make([]byte, 1024)
	server.Read(buffer)
	fmt.Printf("response: %v", string(buffer))
}

