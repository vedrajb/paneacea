package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

const MaxMessage = 8 * 1024 * 1024

type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}
type Response struct {
	ID     string          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}
type Output struct {
	Snapshot       bool   `json:"snapshot"`
	Restored       bool   `json:"restored"`
	ViewportOffset int    `json:"viewportOffset"`
	Columns        int    `json:"columns"`
	Rows           int    `json:"rows"`
	Data           []byte `json:"data"`
	Exited         bool   `json:"exited"`
	Truncated      bool   `json:"truncated"`
	Sequence       uint64 `json:"sequence"`
}
type Client struct {
	mu      sync.Mutex
	conn    net.Conn
	scanner *bufio.Scanner
}

func Scanner(conn net.Conn) *bufio.Scanner {
	s := bufio.NewScanner(conn)
	s.Buffer(make([]byte, 65536), MaxMessage)
	return s
}
func Connect(ctx context.Context) (*Client, error) {
	conn, err := Dial(ctx)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, scanner: Scanner(conn)}, nil
}
func ConnectLocal(ctx context.Context) (*Client, error) {
	conn, err := DialLocal(ctx)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, scanner: Scanner(conn)}, nil
}
func (c *Client) Close() error { return c.conn.Close() }
func (c *Client) Call(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn.SetDeadline(time.Now().Add(15 * time.Second))
	defer c.conn.SetDeadline(time.Time{})
	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if err = json.NewEncoder(c.conn).Encode(Request{ID: "rpc", Method: method, Params: data}); err != nil {
		return nil, err
	}
	if !c.scanner.Scan() {
		if err = c.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("runtime disconnected")
	}
	var response Response
	if err = json.Unmarshal(c.scanner.Bytes(), &response); err != nil {
		return nil, err
	}
	if !response.OK {
		return nil, fmt.Errorf("%s", response.Error)
	}
	return response.Result, nil
}
