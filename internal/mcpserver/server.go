package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r Request) UnmarshalParams(target any) error {
	if len(r.Params) == 0 {
		return nil
	}
	return json.Unmarshal(r.Params, target)
}

type handler func(context.Context, Request) (any, error)
type notificationHandler func(context.Context, Request) error

type Server struct {
	logger        *log.Logger
	methods       map[string]handler
	notifications map[string]notificationHandler
}

func New(logger *log.Logger) *Server {
	return &Server{
		logger:        logger,
		methods:       make(map[string]handler),
		notifications: make(map[string]notificationHandler),
	}
}

func (s *Server) HandleInitialize(h handler) {
	s.methods["initialize"] = h
}

func (s *Server) HandleMethod(method string, h handler) {
	s.methods[method] = h
}

func (s *Server) HandleNotification(method string, h notificationHandler) {
	s.notifications[method] = h
}

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	encoder := json.NewEncoder(out)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			if err := s.writeError(encoder, nil, -32700, "parse error", err); err != nil {
				return err
			}
			continue
		}

		if req.ID == nil || string(req.ID) == "null" {
			if h, ok := s.notifications[req.Method]; ok {
				if err := h(ctx, req); err != nil {
					s.logger.Printf("notification %s failed: %v", req.Method, err)
				}
			}
			continue
		}

		h, ok := s.methods[req.Method]
		if !ok {
			if err := s.writeError(encoder, req.ID, -32601, "method not found", fmt.Errorf("%s", req.Method)); err != nil {
				return err
			}
			continue
		}

		result, err := h(ctx, req)
		if err != nil {
			if req.Method == "tools/call" {
				if err := encoder.Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      json.RawMessage(req.ID),
					"result":  result,
				}); err != nil {
					return err
				}
				continue
			}
			if err := s.writeError(encoder, req.ID, -32000, err.Error(), err); err != nil {
				return err
			}
			continue
		}

		if err := encoder.Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      json.RawMessage(req.ID),
			"result":  result,
		}); err != nil {
			return err
		}
	}

	return scanner.Err()
}

func (s *Server) writeError(encoder *json.Encoder, id json.RawMessage, code int, message string, err error) error {
	body := map[string]any{
		"jsonrpc": "2.0",
		"error": map[string]any{
			"code":    code,
			"message": message,
			"data": map[string]any{
				"details": err.Error(),
			},
		},
	}
	if id != nil {
		body["id"] = json.RawMessage(id)
	}
	return encoder.Encode(body)
}
