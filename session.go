package easyws

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

// Session 会话
type Session struct {
	Request  *http.Request
	alive    int32
	lctx     context.Context
	cancel   context.CancelFunc
	conn     *websocket.Conn
	outBound chan *message

	Hub *Hub
}

// LocalAddr 获取本地址
func (sf *Session) LocalAddr() net.Addr {
	return sf.conn.LocalAddr()
}

// RemoteAddr 获取远程地址
func (sf *Session) RemoteAddr() net.Addr {
	return sf.conn.RemoteAddr()
}

// WriteMessage 写消息
func (sf *Session) WriteMessage(messageType int, data []byte) error {
	select {
	case <-sf.lctx.Done():
		return ErrSessionClosed
	case sf.outBound <- &message{messageType, data}:
	}

	return nil
}

// WriteControl 写控制消息 (CloseMessage, PingMessage and PongMessag.)
func (sf *Session) WriteControl(messageType int, data []byte) error {
	select {
	case <-sf.lctx.Done():
		return ErrHubClosed
	default:
	}
	return sf.conn.WriteControl(messageType, data,
		time.Now().Add(sf.Hub.SessionConfig.WriteWait))
}

// writePump
func (sf *Session) writePump() {
	var retries int

	cfg := sf.Hub.SessionConfig
	monTick := time.NewTicker(cfg.KeepAlive * time.Duration(cfg.Radtio) / 100)
	defer func() {
		monTick.Stop()
		sf.conn.Close()
	}()
	for {
		select {
		case <-sf.lctx.Done():
			return
		case msg, ok := <-sf.outBound:
			if !ok {
				sf.conn.SetWriteDeadline(time.Now().Add(cfg.WriteWait)) // nolint: errcheck
				sf.conn.WriteMessage(websocket.CloseMessage, []byte{})  // nolint: errcheck
				sf.conn.SetWriteDeadline(time.Time{})                   // nolint: errcheck
				return
			}

			if msg.t == websocket.CloseMessage {
				return
			}

			err := sf.conn.WriteMessage(msg.t, msg.data)
			if err != nil {
				sf.Hub.errorHandler(sf, fmt.Errorf("run write %w", err))
				return
			}
			sf.Hub.sendHandler(sf, msg.t, msg.data)
		case <-monTick.C:
			if atomic.AddInt32(&sf.alive, 1) > 1 {
				if retries++; retries > 3 {
					return
				}
				err := sf.conn.WriteControl(websocket.PingMessage, []byte{},
					time.Now().Add(cfg.WriteWait))
				if err != nil {
					sf.Hub.errorHandler(sf, errors.Wrap(err, "Run write"))
					return
				}
			} else {
				retries = 0
			}
		}
	}
}

// run
func (sf *Session) run() {
	go sf.writePump()

	cfg := sf.Hub.SessionConfig
	readWait := cfg.KeepAlive * time.Duration(cfg.Radtio) / 100 * 4

	sf.conn.SetPongHandler(func(message string) error {
		atomic.StoreInt32(&sf.alive, 0)
		sf.conn.SetReadDeadline(time.Now().Add(readWait))
		sf.Hub.pongHandler(sf, message)
		return nil
	})

	sf.conn.SetPingHandler(func(message string) error {
		atomic.StoreInt32(&sf.alive, 0)
		sf.conn.SetReadDeadline(time.Now().Add(readWait))
		err := sf.conn.WriteControl(websocket.PongMessage,
			[]byte(message), time.Now().Add(cfg.WriteWait))
		if err != nil {
			if e, ok := err.(net.Error); !(ok && e.Temporary() ||
				err == websocket.ErrCloseSent) {
				return err
			}
		}
		sf.Hub.pingHandler(sf, message)
		return nil
	})
	if sf.Hub.closeHandler != nil {
		sf.conn.SetCloseHandler(func(code int, text string) error {
			return sf.Hub.closeHandler(sf, code, text)
		})
	}

	if cfg.MaxMessageSize > 0 {
		sf.conn.SetReadLimit(cfg.MaxMessageSize)
	}
	sf.conn.SetReadDeadline(time.Now().Add(readWait))
	for {
		t, data, err := sf.conn.ReadMessage()
		if err != nil {
			sf.Hub.errorHandler(sf, errors.Wrap(err, "Run read"))
			break
		}
		atomic.StoreInt32(&sf.alive, 0)
		sf.Hub.receiveHandler(sf, t, data)
	}

	sf.Close()
}

// Close 关闭会话
func (sf *Session) Close() {
	sf.conn.Close()
	sf.cancel()
}

// IsClosed 判断会话是否关闭
func (sf *Session) IsClosed() bool {
	return sf.lctx.Err() != nil
}
