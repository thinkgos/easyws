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

// WriteMessage 写消息
func (sf *Session) WriteMessage(messageType int, data []byte) error {
	if !(messageType == websocket.TextMessage || messageType == websocket.BinaryMessage) {
		return ErrBadMessageType
	}
	sf.outBound <- &message{messageType, data}
	return nil
}

// WriteControl 写控制消息 (websocket.CloseMessage, websocket.PingMessage and websocket.PongMessage.)
func (sf *Session) WriteControl(messageType int, data []byte) error {
	return sf.Conn.WriteControl(messageType, data,
		time.Now().Add(sf.Hub.SessionConfig.WriteTimeout))
}

// Run
func (sf *Session) Run() {
	cfg := sf.Hub.SessionConfig

	sf.outBound = make(chan *message, cfg.MessageBufferSize)
	sf.lctx, sf.cancel = context.WithCancel(sf.Hub.ctx)
	sf.Hub.register(sf)
	sf.Hub.connectHandler(sf)
	defer func() {
		sf.Conn.Close()
		sf.cancel()
		sf.Hub.UnRegister(sf.GroupID, sf.ID)
		sf.Hub.disconnectHandler(sf)
	}()
	go sf.writePump()

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
		err := sf.Conn.WriteControl(websocket.PongMessage, []byte(message), time.Now().Add(cfg.WriteTimeout))
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
			sf.Hub.errorHandler(sf, fmt.Errorf("run read %w", err))
			return
		}
		atomic.StoreInt32(&sf.alive, 0)
		sf.Hub.receiveHandler(sf, t, data)
	}
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
		case msg := <-sf.outBound:
			err := sf.Conn.WriteMessage(msg.messageType, msg.data)
			if err != nil {
				sf.Hub.errorHandler(sf, fmt.Errorf("run write %w", err))
				return
			}
			sf.Hub.sendHandler(sf, msg.messageType, msg.data)
		case <-monTick.C:
			if atomic.AddInt32(&sf.alive, 1) > 1 {
				retries++
				if retries > 3 {
					return
				}
				err := sf.Conn.WriteControl(websocket.PingMessage, []byte{},
					time.Now().Add(cfg.WriteTimeout))
				if err != nil {
					sf.Hub.errorHandler(sf, fmt.Errorf("run write %w", err))
					return
				}
			} else {
				retries = 0
			}
		}
	}
}
