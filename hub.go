package easyws

import (
	"context"
	"net/http"
	"sync"
)

// 消息包
type Message struct {
	t    int
	data []byte
}

// 登记处
type registry struct {
	isRegister bool
	sess       *Session
}

// 管理中心
type Hub struct {
	sessions  map[*Session]struct{}
	registry  chan registry
	broadcast chan *Message
	started   bool
	mu        sync.Mutex
	cancel    context.CancelFunc
	option    *Options
}

// 创建管理中心
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
		broadcast: make(chan *Message, opt.config.MessageBufferSize),
		option:    opt,
	}
}

// 创建管理中心并运行
func NewWithRun(op ...*Options) *Hub {
	h := New(op...)
	go h.Run(context.Background())
	return h
}

// 管理会话
func (this *Hub) manageSession(isRegister bool, ses *Session) {
	this.registry <- registry{isRegister, ses}
}

// 运行管理中心
func (this *Hub) Run(ctx context.Context) {
	var lctx context.Context

	lctx, this.cancel = context.WithCancel(ctx)
	this.mu.Lock()
	this.started = true
	this.mu.Unlock()
	for {
		select {
		case reg := <-this.registry:
			this.mu.Lock()
			if reg.isRegister {
				this.sessions[reg.sess] = struct{}{}
			} else if _, ok := this.sessions[reg.sess]; ok {
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
			this.mu.Lock()
			this.started = false           // 如果外面cancel,要先置位
			for s := range this.sessions { // 删除所有客户端
				s.Close()
			}
			this.sessions = make(map[*Session]struct{})
			this.mu.Unlock()
			return
		}
	}
}

// 广播消息
func (this *Hub) BroadCast(t int, data []byte) error {
	if this.IsClosed() {
		return ErrHubClosed
	}

	select {
	case this.broadcast <- &Message{t, data}:
	default:
		return ErrHubBufferFull
	}
	return nil
}

// 关闭
func (this *Hub) Close() {
	this.mu.Lock()
	this.started = false
	if this.cancel != nil {
		this.mu.Unlock()
		this.cancel()
		return
	}
	this.mu.Unlock()
}

// 判断是否关闭
func (this *Hub) IsClosed() bool {
	this.mu.Lock()
	b := this.started
	this.mu.Unlock()
	return !b
}

// 升级成websocket并运行起来
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
