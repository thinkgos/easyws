package easyws

import (
	"context"
	"errors"
	"net/http"
	"sync"
)

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
type hub struct {
	sessions  map[*Session]struct{}
	registry  chan registry
	broadcast chan *Message
	started   bool
	mu        sync.Mutex
	cancel    context.CancelFunc
	option    *Options
}

// 管理中心
func New(op *Options) *hub {
	h := &hub{
		sessions:  make(map[*Session]struct{}),
		registry:  make(chan registry),
		broadcast: make(chan *Message, op.config.MessageBufferSize),
		option:    op,
	}
	h.run(context.Background())
	return h
}

// 管理会话
func (this *hub) manageSession(isRegister bool, ses *Session) {
	this.registry <- registry{isRegister, ses}
}

// 运行管理中心
func (this *hub) run(ctx context.Context) {
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
				_ = sess.WriteMessage(m.t, m.data)
			}
			this.mu.Unlock()

		case <-lctx.Done():
			this.mu.Lock()
			for s := range this.sessions {
				s.Close()
			}
			this.sessions = make(map[*Session]struct{})
			this.started = false
			this.mu.Unlock()
			return
		}
	}
}

// 广播消息
func (this *hub) BroadCast(t int, data []byte) error {
	if this.IsClosed() {
		return errors.New("hub is closed")
	}

	this.broadcast <- &Message{t, data}
	return nil
}

// 关闭
func (this *hub) Close() {
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
func (this *hub) IsClosed() bool {
	this.mu.Lock()
	b := this.started
	this.mu.Unlock()
	return !b
}

// 升级成websocket并运行起来
func (this *hub) RunWithUpgrade(w http.ResponseWriter, r *http.Request) error {
	if this.IsClosed() {
		return errors.New("hub is closed")
	}

	conn, err := this.option.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	newSession(this, conn, this.option.config).run(context.TODO())
	return nil
}
