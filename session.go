package easyws

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Session 会话
type Session struct {
	GroupID  string
	ID       string
	Conn     *websocket.Conn
	Request  *http.Request
	alive    int32
	lctx     context.Context
	cancel   context.CancelFunc
	outBound chan *message

	Hub *Hub
}

// LocalAddr 获取本地址
func (sf *Session) LocalAddr() net.Addr {
	return sf.Conn.LocalAddr()
}

// RemoteAddr 获取远程地址
func (sf *Session) RemoteAddr() net.Addr {
	return sf.Conn.RemoteAddr()
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
		return ErrSessionClosed
	default:
	}
	return sf.Conn.WriteControl(messageType, data,
		time.Now().Add(sf.Hub.SessionConfig.WriteWait))
}

// Run
func (sf *Session) Run() {
	sf.outBound = make(chan *message, sf.Hub.SessionConfig.MessageBufferSize)
	sf.lctx, sf.cancel = context.WithCancel(sf.Hub.ctx)
	sf.Hub.register(sf)
	sf.Hub.connectHandler(sf)
	defer func() {
		sf.Hub.UnRegister(sf.GroupID, sf.ID)
		sf.Hub.disconnectHandler(sf)
	}()
	go sf.writePump()

	cfg := sf.Hub.SessionConfig
	readWait := cfg.KeepAlive * time.Duration(cfg.Radtio) / 100 * 4

	// 设置 pong handler
	sf.Conn.SetPongHandler(func(message string) error {
		atomic.StoreInt32(&sf.alive, 0)
		// sf.Conn.SetReadDeadline(time.Now().Add(readWait))
		sf.Hub.pongHandler(sf, message)
		return nil
	})

	// 设置 ping handler
	sf.Conn.SetPingHandler(func(message string) error {
		atomic.StoreInt32(&sf.alive, 0)
		// sf.Conn.SetReadDeadline(time.Now().Add(readWait))
		err := sf.Conn.WriteControl(websocket.PongMessage, []byte(message), time.Now().Add(cfg.WriteWait))
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
		sf.Conn.SetCloseHandler(func(code int, text string) error {
			return sf.Hub.closeHandler(sf, code, text)
		})
	}

	if cfg.MaxMessageSize > 0 {
		sf.Conn.SetReadLimit(cfg.MaxMessageSize)
	}
	sf.Conn.SetReadDeadline(time.Now().Add(readWait))
	for {
		t, data, err := sf.Conn.ReadMessage()
		if err != nil {
			sf.Hub.errorHandler(sf, fmt.Errorf("Run read %w", err))
			break
		}
		atomic.StoreInt32(&sf.alive, 0)
		sf.Hub.receiveHandler(sf, t, data)
	}

	sf.Conn.Close()
	sf.cancel()
}

// writePump
func (sf *Session) writePump() {
	var retries int

	cfg := sf.Hub.SessionConfig
	monTick := time.NewTicker(cfg.KeepAlive * time.Duration(cfg.Radtio) / 100)
	defer func() {
		monTick.Stop()
		sf.Conn.Close()
	}()
	for {
		select {
		case <-sf.lctx.Done():
			return
		case msg, ok := <-sf.outBound:
			if !ok {
				sf.Conn.SetWriteDeadline(time.Now().Add(cfg.WriteWait)) // nolint: errcheck
				sf.Conn.WriteMessage(websocket.CloseMessage, []byte{})  // nolint: errcheck
				sf.Conn.SetWriteDeadline(time.Time{})                   // nolint: errcheck
				return
			}

			if msg.t == websocket.CloseMessage {
				return
			}

			err := sf.Conn.WriteMessage(msg.t, msg.data)
			if err != nil {
				sf.Hub.errorHandler(sf, fmt.Errorf("Run write %w", err))
				return
			}
			sf.Hub.sendHandler(sf, msg.t, msg.data)
		case <-monTick.C:
			if atomic.AddInt32(&sf.alive, 1) > 1 {
				if retries++; retries > 3 {
					return
				}
				err := sf.Conn.WriteControl(websocket.PingMessage, []byte{},
					time.Now().Add(cfg.WriteWait))
				if err != nil {
					sf.Hub.errorHandler(sf, fmt.Errorf("Run write %w", err))
					return
				}
			} else {
				retries = 0
			}
		}
	}
}
