package easyws

import (
	"context"
	"errors"
	"net/http"
	"sync"
)

// 错误返回
var (
	ErrHubClosed         = errors.New("hub is closed")
	ErrHubBufferFull     = errors.New("hub buffer is full")
	ErrSessionClosed     = errors.New("session is closed")
	ErrSessionBufferFull = errors.New("session buffer is full")
)

// message 消息包
type message struct {
	t    int
	data []byte
}

// Hub 管理中心
type Hub struct {
	sessions   map[*Session]struct{}
	registry   chan *Session
	unRegistry chan *Session
	broadcast  chan *message
	mu         sync.Mutex
	lctx       context.Context
	cancel     context.CancelFunc
	config
}

// New 创建管理中心
func New(opts ...Option) *Hub {
	ctx, cancel := context.WithCancel(context.Background())
	hub := &Hub{
		sessions:   make(map[*Session]struct{}),
		registry:   make(chan *Session),
		unRegistry: make(chan *Session),
		config:     defaultConfig(),
		lctx:       ctx,
		cancel:     cancel,
	}

	for _, opt := range opts {
		opt(hub)
	}

	hub.broadcast = make(chan *message, hub.MessageBufferSize)
	return hub
}

// NewWithRun 创建管理中心并运行
func NewWithRun(opt ...Option) *Hub {
	h := New(opt...)
	go h.Run(context.TODO())
	return h
}

func (sf *Hub) Register(sess *Session) {
	sf.registry <- sess
}

func (sf *Hub) UnRegister(sess *Session) {
	sf.unRegistry <- sess
}

// Run 运行管理中心
func (sf *Hub) Run(ctx context.Context) {
	defer func() {
		sf.mu.Lock()
		sf.sessions = make(map[*Session]struct{})
		sf.mu.Unlock()
	}()

	for {
		select {
		case sess := <-sf.registry:
			sf.mu.Lock()
			sf.sessions[sess] = struct{}{}
			sf.mu.Unlock()
		case sess := <-sf.unRegistry:
			sf.mu.Lock()
			delete(sf.sessions, sess)
			sf.mu.Unlock()
		case m := <-sf.broadcast:
			sf.mu.Lock()
			for sess := range sf.sessions {
				sess.WriteMessage(m.t, m.data)
			}
			sf.mu.Unlock()
		case <-ctx.Done():
			sf.cancel() // local cancel mark it closed
			return
		case <-sf.lctx.Done():
			return
		}
	}
}

// BroadCast 广播消息
func (sf *Hub) BroadCast(t int, data []byte) error {
	select {
	case sf.broadcast <- &message{t, data}:
	case <-sf.lctx.Done():
		return ErrHubClosed
	default:
		return ErrHubBufferFull
	}
	return nil
}

// SessionLen 返回客户端会话的数量
func (sf *Hub) SessionLen() int {
	sf.mu.Lock()
	l := len(sf.sessions)
	sf.mu.Unlock()
	return l
}

// Close 关闭
func (sf *Hub) Close() {
	sf.cancel()
}

// IsClosed 判断是否关闭
func (sf *Hub) IsClosed() bool {
	return sf.lctx.Err() != nil
}

// UpgradeWithRun 升级成websocket并运行起来
func (sf *Hub) UpgradeWithRun(w http.ResponseWriter, r *http.Request) error {
	if sf.IsClosed() {
		return ErrHubClosed
	}
	conn, err := sf.upgrader.Upgrade(w, r, w.Header())
	if err != nil {
		return err
	}

	sess := &Session{
		Request:  r,
		conn:     conn,
		outBound: make(chan *message, sf.SessionConfig.MessageBufferSize),
		Hub:      sf,
	}
	sess.lctx, sess.cancel = context.WithCancel(sf.lctx)

	sf.Register(sess)
	sf.connectHandler(sess)
	sess.run()
	if !sf.IsClosed() {
		sf.UnRegister(sess)
	}
	sf.disconnectHandler(sess)
	return nil
}
