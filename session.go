package easyws

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/astaxie/beego/logs"
	"github.com/gorilla/websocket"
)

type Session struct {
	conn     *websocket.Conn
	outBound chan *Message
	started  bool
	alive    int32
	mu       sync.Mutex
	cancel   context.CancelFunc
	hub      *hub
}

// 创建一个会话实例
func newSession(h *hub, conn *websocket.Conn, cfg *SessionConfig) *Session {
	return &Session{
		conn:     conn,
		outBound: make(chan *Message, cfg.MaxMessageSize),
		hub:      h,
	}
}

// 写消息
func (this *Session) WriteMessage(messageType int, data []byte) error {
	if this.IsClosed() {
		return errors.New("session is closed")
	}

	select {
	case this.outBound <- &Message{messageType, data}:
	default:
		return errors.New("session buffer is full")
	}

	return nil
}

//写控制消息 (CloseMessage, PingMessage and PongMessag.)
func (this *Session) WriteControl(messageType int, data []byte) error {
	if this.IsClosed() {
		return errors.New("session is closed")
	}

	return this.conn.WriteControl(messageType, data,
		time.Now().Add(this.hub.option.config.WriteWait))
}

//
func (this *Session) writePump(ctx context.Context) {
	var retries int

	cfg := this.hub.option.config
	monTick := time.NewTicker(cfg.KeepAlive * time.Duration(cfg.Radtio) / 100)
	defer func() {
		logs.Error("Run write: closed")
		monTick.Stop()
		this.conn.Close()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-this.outBound:
			this.conn.SetWriteDeadline(time.Now().Add(cfg.WriteWait))
			if !ok {
				this.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			err := this.conn.WriteMessage(msg.t, msg.data)
			if err != nil {
				logs.Error("Run write: ", err)
				return
			}
			this.hub.option.sendHandler(this, msg.t, msg.data)
		case <-monTick.C:
			if atomic.AddInt32(&this.alive, 1) > 1 {
				if retries++; retries > 3 {
					return
				}
				err := this.conn.WriteControl(websocket.PingMessage, []byte{},
					time.Now().Add(cfg.WriteWait))
				if err != nil {
					logs.Error("run Write: ", err)
					return
				}
			} else {
				retries = 0
			}
		}
	}
}

//
func (this *Session) run(ctx context.Context) {
	var lctx context.Context

	this.hub.manageSession(true, this)
	lctx, this.cancel = context.WithCancel(ctx)
	go this.writePump(lctx)

	cfg := this.hub.option.config
	readWait := cfg.KeepAlive * time.Duration(cfg.Radtio) / 100 * (tuple + 1)

	this.conn.SetPongHandler(func(string) error {
		atomic.StoreInt32(&this.alive, 0)
		this.conn.SetReadDeadline(time.Now().Add(readWait))
		logs.Debug("%s pong", this.conn.RemoteAddr().String())
		return nil
	})

	this.conn.SetPingHandler(func(message string) error {
		atomic.StoreInt32(&this.alive, 0)
		this.conn.SetReadDeadline(time.Now().Add(readWait))
		err := this.conn.WriteControl(websocket.PongMessage,
			[]byte(message), time.Now().Add(cfg.WriteWait))
		if err != nil {
			if err == websocket.ErrCloseSent {
				// see default handler
			} else if e, ok := err.(net.Error); ok && e.Temporary() {
				// see default handler
			} else {
				return err
			}
		}
		logs.Debug("%s ping", this.conn.RemoteAddr().String())
		return nil
	})

	if cfg.MaxMessageSize > 0 {
		this.conn.SetReadLimit(cfg.MaxMessageSize)
	}
	this.conn.SetReadDeadline(time.Now().Add(readWait))
	for {
		t, data, err := this.conn.ReadMessage()
		if err != nil {
			logs.Error("Run Read: ", err)
			break
		}
		this.hub.option.receiveHandler(this, t, data)
	}
	if !this.hub.IsClosed() {
		this.hub.manageSession(false, this)
	}
	this.Close()
}

// 关闭
func (this *Session) Close() {
	this.mu.Lock()
	if this.started && this.cancel != nil {
		this.started = false
		this.mu.Unlock()
		this.cancel()
		return
	}
	this.mu.Unlock()
}

// 判断是否关闭
func (this *Session) IsClosed() bool {
	this.mu.Lock()
	b := this.started
	this.mu.Unlock()
	return !b
}
