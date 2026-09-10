package daemon

import (
	"context"
	"encoding/json"
	"github.com/paneacea/paneacea/internal/ipc"
	"net"
	"time"
)

func Serve(ctx context.Context, listener net.Listener, r *Runtime) error {
	go func() { <-ctx.Done(); listener.Close() }()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go serveConnection(ctx, conn, r)
	}
}
func serveConnection(ctx context.Context, conn net.Conn, r *Runtime) {
	defer conn.Close()
	scanner := ipc.Scanner(conn)
	encoder := json.NewEncoder(conn)
	for scanner.Scan() {
		var request ipc.Request
		response := ipc.Response{}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			response.Error = "invalid JSON request"
		} else {
			response.ID = request.ID
			result, err := r.Call(ctx, request.Method, request.Params)
			if err != nil {
				response.Error = err.Error()
			} else {
				data, err := json.Marshal(result)
				if err != nil {
					response.Error = err.Error()
				} else {
					response.OK = true
					response.Result = data
				}
			}
		}
		conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
		if encoder.Encode(response) != nil {
			return
		}
	}
}
