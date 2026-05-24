# HTTP from TCP

HTTP/1.1 server and client built from raw TCP in Go. No `net/http`.

## Structure

- `server/` — TCP server with router, request parsing, response building
- `client/` — TCP client that sends raw HTTP requests

## Run

```bash
# Start server
go run ./server

# In another terminal, run client
go run ./client
```
