package easyws

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
)

// 错误返回
var (
	ErrHubClosed         = errors.New("hub is closed")
	ErrHubBufferFull     = errors.New("hub is closed")
	ErrSessionClosed     = errors.New("session is closed")
	ErrSessionBufferFull = errors.New("session buffer is full")
)

// message 消息包
type message struct {
	t    int
	data []byte
}

// 登记处
type registry struct {
	isRegister bool
	sess       *Session
}

// Hub 管理中心
type Hub struct {
	sessions  map[*Session]struct{}
	registry  chan registry
	broadcast chan *message
	started   int32
	mu        sync.Mutex
	cancel    context.CancelFunc
	option    *Options
}

// New 创建管理中心
func New(op ...*Options) *Hub {
	var opt *Options
	if len(op) > 0 {
		opt = op[0]
	} else {
		opt = NewOptions()
	}
	return &Hub{
		sessions:  make(map[*Session]struct{}),
		registry:  make(chan registry),
		broadcast: make(chan *message, opt.config.MessageBufferSize),
		option:    opt,
	}
}

// NewWithRun 创建管理中心并运行
func NewWithRun(op ...*Options) *Hub {
	h := New(op...)
	go h.Run(context.Background())
	return h
}

// manageSession 管理会话
func (this *Hub) manageSession(isRegister bool, ses *Session) {
	this.registry <- registry{isRegister, ses}
}

// Run 运行管理中心
func (this *Hub) Run(ctx context.Context) {
	var lctx context.Context

	lctx, this.cancel = context.WithCancel(ctx)
	atomic.StoreInt32(&this.started, 1)
	for {
		select {
		case reg := <-this.registry:
			this.mu.Lock()
			if reg.isRegister {
				this.sessions[reg.sess] = struct{}{}
			} else {
				delete(this.sessions, reg.sess)
			}
			this.mu.Unlock()
		case m := <-this.broadcast:
			this.mu.Lock()
			for sess := range this.sessions {
				sess.WriteMessage(m.t, m.data)
			}
			this.mu.Unlock()

		case <-lctx.Done():
			atomic.StoreInt32(&this.started, 0) // 如果外面cancel,要选置位关闭
			this.mu.Lock()
			for s := range this.sessions { // 删除所有客户端
				s.Close()
			}
			this.sessions = make(map[*Session]struct{})
			this.mu.Unlock()
			return
		}
	}
}

// BroadCast 广播消息
func (this *Hub) BroadCast(t int, data []byte) error {
	if this.IsClosed() {
		return ErrHubClosed
	}

	select {
	case this.broadcast <- &message{t, data}:
	default:
		return ErrHubBufferFull
	}
	return nil
}

// Close 关闭
func (this *Hub) Close() {
	if atomic.CompareAndSwapInt32(&this.started, 1, 0) {
		this.cancel()
	}
}

// IsClosed 判断是否关闭
func (this *Hub) IsClosed() bool {
	return atomic.LoadInt32(&this.started) == 0
}

// RunWithUpgrade 升级成websocket并运行起来
func (this *Hub) RunWithUpgrade(w http.ResponseWriter, r *http.Request) error {
	if this.IsClosed() {
		return ErrHubClosed
	}
	conn, err := this.option.upgrader.Upgrade(w, r, w.Header())
	if err != nil {
		return err
	}

	sess := NewSession(this, conn, this.option.config)
	this.option.connectHandler(sess)
	sess.run()
	this.option.disconnectHandler(sess)
	return nil
}
