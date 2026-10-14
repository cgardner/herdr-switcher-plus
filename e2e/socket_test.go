//go:build e2e

package e2e

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// socketCall is one request the program sent over the Herdr socket.
type socketCall struct {
	Method string
	Params map[string]any
}

// socketServer answers the Herdr socket protocol: newline-delimited JSON, a
// request of {id, method, params} and a reply of {id, result} or
// {id, error: {code, message}}.
type socketServer struct {
	path string

	mu    sync.Mutex
	calls []socketCall
	fail  map[string]string // method to the error code it returns
}

// startSocket listens in a directory of its own. A unix socket path holds about
// 100 bytes on macOS, and t.TempDir is too long to leave room for one.
func startSocket(t *testing.T) *socketServer {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "e2e")
	if err != nil {
		t.Fatal(err)
	}
	s := &socketServer{path: filepath.Join(dir, "h.sock"), fail: map[string]string{}}
	ln, err := net.Listen("unix", s.path)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ln.Close()
		os.RemoveAll(dir)
	})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s
}

func (s *socketServer) serve(conn net.Conn) {
	defer conn.Close()
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return
	}
	var req struct {
		ID     any            `json:"id"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if json.Unmarshal(line, &req) != nil {
		return
	}
	s.mu.Lock()
	s.calls = append(s.calls, socketCall{req.Method, req.Params})
	code, failing := s.fail[req.Method]
	s.mu.Unlock()

	reply := map[string]any{"id": req.ID, "result": map[string]any{}}
	if failing {
		reply = map[string]any{"id": req.ID, "error": map[string]any{"code": code, "message": "injected failure"}}
	}
	b, _ := json.Marshal(reply)
	_, _ = conn.Write(append(b, '\n'))
}

// failing makes a method return an error with this code.
func (s *socketServer) failing(method, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail[method] = code
}

// seen is every request received so far.
func (s *socketServer) seen() []socketCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]socketCall(nil), s.calls...)
}

// requests is the requests for one method.
func (s *socketServer) requests(method string) []socketCall {
	var out []socketCall
	for _, c := range s.seen() {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}
