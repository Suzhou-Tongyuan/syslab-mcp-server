package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

type desktopJSONCodec struct {
	conn   desktopDeadlineConn
	buf    []byte
	debugf func(string, ...any)
}

func newDesktopJSONCodec(conn desktopDeadlineConn, debugf func(string, ...any)) *desktopJSONCodec {
	return &desktopJSONCodec{conn: conn, debugf: debugf}
}

func (c *desktopJSONCodec) Send(msg desktopMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = c.conn.Write(body)
	return err
}

func (c *desktopJSONCodec) Recv(timeout time.Duration) (desktopMessage, error) {
	deadline := time.Now().Add(timeout)
	for {
		if len(c.buf) > 0 {
			var msg desktopMessage
			dec := json.NewDecoder(bytes.NewReader(c.buf))
			if err := dec.Decode(&msg); err == nil {
				consumed := dec.InputOffset()
				if consumed > 0 {
					c.buf = bytes.TrimLeft(c.buf[consumed:], " \r\n\t")
					return msg, nil
				}
			}
		}

		if time.Now().After(deadline) {
			return desktopMessage{}, fmt.Errorf("timeout waiting for desktop response")
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return desktopMessage{}, fmt.Errorf("timeout waiting for desktop response")
		}
		stepTimeout := 200 * time.Millisecond
		if remaining < stepTimeout {
			stepTimeout = remaining
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(stepTimeout))
		chunk := make([]byte, 64*1024)
		n, err := c.conn.Read(chunk)
		if err != nil {
			if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
				continue
			}
			if c.debugf != nil {
				c.debugf("desktop codec read error: %v", err)
			}
			return desktopMessage{}, err
		}
		if n == 0 {
			continue
		}
		if c.debugf != nil {
			preview := string(chunk[:n])
			if len(preview) > 200 {
				preview = preview[:200]
			}
			c.debugf("desktop codec read chunk: bytes=%d preview=%q", n, preview)
		}
		c.buf = append(c.buf, chunk[:n]...)
	}
}
