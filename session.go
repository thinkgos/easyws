package easyws

import (
	"context"
	"net"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

// Session 会话
type Session struct {
	conn     *websocket.Conn
	outBound chan *message
	started  int32
	alive    int32
	cancel   context.CancelFunc
	Hub      *Hub
}

// NewSession 创建一个会话实例
func NewSession(h *Hub, conn *websocket.Conn, cfg *SessionConfig) *Session {
	return &Session{
		conn:     conn,
		outBound: make(chan *message, cfg.MessageBufferSize),
		Hub:      h,
	}
}

// LocalAddr 获取本地址
func (this *Session) LocalAddr() net.Addr {
	return this.conn.LocalAddr()
}

// RemoteAddr 获取远程地址
func (this *Session) RemoteAddr() net.Addr {
	return this.conn.RemoteAddr()
}

// WriteMessage 写消息
func (this *Session) WriteMessage(messageType int, data []byte) error {
	if this.IsClosed() {
		return ErrSessionClosed
	}

	select {
	case this.outBound <- &message{messageType, data}:
	default:
		return ErrSessionBufferFull
	}

	return nil
}

// WriteControl 写控制消息 (CloseMessage, PingMessage and PongMessag.)
func (this *Session) WriteControl(messageType int, data []byte) error {
	if this.IsClosed() {
		return ErrSessionClosed
	}

	return this.conn.WriteControl(messageType, data,
		time.Now().Add(this.Hub.option.config.WriteWait))
}

// writePump
func (this *Session) writePump(ctx context.Context) {
	var retries int

	cfg := this.Hub.option.config
	monTick := time.NewTicker(cfg.KeepAlive * time.Duration(cfg.Radtio) / 100)
	defer func() {
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

			if msg.t == websocket.CloseMessage {
				return
			}

			err := this.conn.WriteMessage(msg.t, msg.data)
			if err != nil {
				this.Hub.option.errorHandler(this, errors.Wrap(err, "Run write"))
				return
			}
			this.Hub.option.sendHandler(this, msg.t, msg.data)
		case <-monTick.C:
			if atomic.AddInt32(&this.alive, 1) > 1 {
				if retries++; retries > 3 {
					return
				}
				err := this.conn.WriteControl(websocket.PingMessage, []byte{},
					time.Now().Add(cfg.WriteWait))
				if err != nil {
					this.Hub.option.errorHandler(this, errors.Wrap(err, "Run write"))
					return
				}
			} else {
				retries = 0
			}
		}
	}
}

// run
func (this *Session) run() {
	var lctx context.Context

	lctx, this.cancel = context.WithCancel(context.Background())
	atomic.StoreInt32(&this.started, 1)
	this.Hub.manageSession(true, this)
	go this.writePump(lctx)

	cfg := this.Hub.option.config
	readWait := cfg.KeepAlive * time.Duration(cfg.Radtio) / 100 * 4

	this.conn.SetPongHandler(func(message string) error {
		atomic.StoreInt32(&this.alive, 0)
		this.conn.SetReadDeadline(time.Now().Add(readWait))
		this.Hub.option.pongHandler(this, message)
		return nil
	})

	this.conn.SetPingHandler(func(message string) error {
		atomic.StoreInt32(&this.alive, 0)
		this.conn.SetReadDeadline(time.Now().Add(readWait))
		err := this.conn.WriteControl(websocket.PongMessage,
			[]byte(message), time.Now().Add(cfg.WriteWait))
		if err != nil {
			if e, ok := err.(net.Error); !(ok && e.Temporary() ||
				err == websocket.ErrCloseSent) {
				return err
			}
		}
		this.Hub.option.pingHandler(this, message)
		return nil
	})
	if this.Hub.option.closeHandler != nil {
		this.conn.SetCloseHandler(func(code int, text string) error {
			return this.Hub.option.closeHandler(this, code, text)
		})
	}

	if cfg.MaxMessageSize > 0 {
		this.conn.SetReadLimit(cfg.MaxMessageSize)
	}
	this.conn.SetReadDeadline(time.Now().Add(readWait))
	for {
		t, data, err := this.conn.ReadMessage()
		if err != nil {
			this.Hub.option.errorHandler(this, errors.Wrap(err, "Run read"))
			break
		}
		atomic.StoreInt32(&this.alive, 0)
		this.Hub.option.receiveHandler(this, t, data)
	}

	if !this.Hub.IsClosed() {
		this.Hub.manageSession(false, this)
	}
	this.Close()
}

// Close 关闭会话
func (this *Session) Close() {
	this.conn.Close()
	if atomic.CompareAndSwapInt32(&this.started, 1, 0) {
		this.cancel()
	}
}

// IsClosed 判断会话是否关闭
func (this *Session) IsClosed() bool {
	return atomic.LoadInt32(&this.started) == 0
}
