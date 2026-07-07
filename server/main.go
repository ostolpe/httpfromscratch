package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

type Router struct {
	routes map[string]func(*Request) *Response
}

func NewRouter() *Router {
	return &Router {
		routes: make(map[string]func(*Request) *Response),
	}
}

func (s *Router) CreateRoute(method, path string, handler func(*Request) *Response) {
	s.routes[method+" "+path] = handler
}

type Response struct {
	Statuscode int
	Body string
}

type Request struct {
	RequestLine *RequestLine
	Header      *Header
	Body        *Body
}

type RequestLine struct {
	Method  string
	Route   string
	Version string
}

type Header struct {
	Fields map[string]string
}

type Body struct {
	Data []byte
}

var ErrInvalidHttpReq = errors.New("invalid http request")

const (
	Separator = "\r\n"
)

func ParseRequest(b []byte) (*Request, error) {
	rl, remainingReq, err := ParseRequestLine(b)
	if err != nil {
		return nil, ErrInvalidHttpReq
	}

	header, remainingReq, err := ParseHeader(remainingReq)
	if err != nil {
		return nil, ErrInvalidHttpReq
	}

	contentLength, err := GetContentLength(header.Fields)
	if err != nil {
		return nil, ErrInvalidHttpReq
	} else if contentLength == 0 {
		return &Request{
			RequestLine: rl,
			Header:      header,
			Body:        nil,
		}, nil
	}

	body, remainingReq, err := ParseBody(remainingReq, contentLength)
	if err != nil {
		return nil, ErrInvalidHttpReq
	}

	return &Request{
		RequestLine: rl,
		Header:      header,
		Body:        body,
	}, nil
}

func ParseRequestLine(b []byte) (*RequestLine, []byte, error) {
	rl, remainingReq, ok := bytes.Cut(b, []byte(Separator))
	if !ok {
		return nil, nil, ErrInvalidHttpReq
	}

	parts := bytes.Split(rl, []byte(" "))
	if len(parts) != 3 {
		return nil, nil, ErrInvalidHttpReq
	}

	return &RequestLine{
		Method:  string(parts[0]),
		Route:   string(parts[1]),
		Version: string(parts[2]),
	}, remainingReq, nil
}

func ParseHeader(b []byte) (*Header, []byte, error) {
	// A request with no header fields leaves just the blank line here.
	if bytes.HasPrefix(b, []byte(Separator)) {
		return &Header{Fields: make(map[string]string)}, b[len(Separator):], nil
	}

	header, remainingReq, ok := bytes.Cut(b, []byte("\r\n\r\n"))
	if !ok {
		return nil, nil, ErrInvalidHttpReq
	}

	lines := bytes.Split(header, []byte("\r\n"))
	fields := make(map[string]string)
	for _, line := range lines {
		key, value, ok := bytes.Cut(line, []byte(":"))
		if !ok {
			return nil, nil, ErrInvalidHttpReq
		}
		name := strings.ToLower(string(bytes.TrimSpace(key)))
		fields[name] = string(bytes.TrimSpace(value))
	}

	return &Header{
		Fields: fields,
	}, remainingReq, nil
}

func GetContentLength(fields map[string]string) (int, error) {
	val, ok := fields["content-length"]
	if !ok {
		return 0, nil
	}

	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("could not convert value: %v to int", val)
	}
	return n, nil
}

func ParseBody(b []byte, length int) (*Body, []byte, error) {
	if length > len(b) {
		return nil, nil, ErrInvalidHttpReq 
	}

	return &Body{Data: b[:length]}, nil, nil
}

var reasonPhrases = map[int]string{
	200: "OK",
	201: "Created",
	204: "No Content",
	400: "Bad Request",
	404: "Not Found",
	405: "Method Not Allowed",
	500: "Internal Server Error",
}

func reasonPhrase(code int) string {
	if p, ok := reasonPhrases[code]; ok {
		return p
	}
	return "Unknown"
}

// ReadRequest keeps reading from the connection until it has the full headers
// plus however many body bytes Content-Length asks for. A single Read isn't
// guaranteed to return the whole request.
func ReadRequest(conn net.Conn) ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 1024)
	for {
		n, err := conn.Read(tmp)
		buf = append(buf, tmp[:n]...)

		if idx := bytes.Index(buf, []byte("\r\n\r\n")); idx >= 0 {
			contentLength := headerContentLength(buf[:idx])
			if len(buf)-(idx+4) >= contentLength {
				return buf, nil
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return buf, nil
			}
			return nil, err
		}
	}
}

// headerContentLength pulls Content-Length out of the raw header block so we
// know how much body to wait for. Returns 0 if it's missing or unparseable.
func headerContentLength(headerBlock []byte) int {
	for _, line := range bytes.Split(headerBlock, []byte(Separator)) {
		key, value, ok := bytes.Cut(line, []byte(":"))
		if !ok {
			continue
		}
		if strings.EqualFold(string(bytes.TrimSpace(key)), "Content-Length") {
			if n, err := strconv.Atoi(string(bytes.TrimSpace(value))); err == nil {
				return n
			}
		}
	}
	return 0
}

func main() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		panic(err)
	}
	fmt.Printf("listening on localhost:8080..\n")

	router := NewRouter()
	router.CreateRoute("GET", "/hello", func(req *Request) *Response {
		return &Response{Statuscode: 200, Body: "<h1>Hello</h1><h2>World!</h2>"}
	})

	for {
		client, err := listener.Accept()
		if err != nil {
			fmt.Printf("accept error: %v\n", err)
			continue
		}

		go func() {
			raw, err := ReadRequest(client)
			if err != nil {
				client.Write([]byte("HTTP/1.1 400 Bad Request\r\nContent-Length: 11\r\n\r\nBad Request"))
				client.Close()
				return
			}

			request, err := ParseRequest(raw)
			if err != nil {
				client.Write([]byte("HTTP/1.1 400 Bad Request\r\nContent-Length: 11\r\n\r\nBad Request"))
				client.Close()
				return
			}

			key := request.RequestLine.Method + " " + request.RequestLine.Route
			endpoint, ok := router.routes[key]
			if !ok {
				client.Write([]byte("HTTP/1.1 404 Not Found\r\nContent-Length: 9\r\n\r\nNot Found"))
				client.Close()
				return
			}
			response := endpoint(request)	
			client.Write(fmt.Appendf(nil, "HTTP/1.1 %d %s\r\nContent-Length: %d\r\n\r\n%s", response.Statuscode, reasonPhrase(response.Statuscode), len(response.Body), response.Body))
			client.Close()
		}()
	}
}
