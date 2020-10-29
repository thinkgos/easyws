package easyws

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
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
	broadcast chan *message
	ctx       context.Context
	cancel    context.CancelFunc
	config

	// 以下需要持锁
	mu       sync.Mutex
	sessions map[string]map[string]*Session
	groupCnt int
	sessCnt  int
}

// New 创建管理中心
func New(opts ...Option) *Hub {
	ctx, cancel := context.WithCancel(context.Background())
	hub := &Hub{
		sessions: make(map[string]map[string]*Session),
		config:   defaultConfig(),
		ctx:      ctx,
		cancel:   cancel,
	}

	for _, opt := range opts {
		opt(hub)
	}

	hub.broadcast = make(chan *message, hub.MessageBufferSize)
	return hub
}

func (sf *Hub) register(sess *Session) {
	sf.mu.Lock()
	if sf.sessions[sess.GroupID] == nil {
		sf.sessions[sess.GroupID] = make(map[string]*Session)
		sf.groupCnt++
	}
	if oldSess, ok := sf.sessions[sess.GroupID][sess.ID]; ok {
		oldSess.cancel()
	} else {
		sf.sessCnt++
	}
	sf.sessions[sess.GroupID][sess.ID] = sess
	sf.mu.Unlock()
}

func (sf *Hub) UnRegister(groupID, id string) {
	sf.mu.Lock()
	if group, ok := sf.sessions[groupID]; ok {
		if client, ok := group[id]; ok {
			delete(group, id)
			sf.sessCnt--
			if len(group) == 0 {
				delete(sf.sessions, groupID)
				sf.groupCnt--
			}
			client.cancel()
		}
	}
	sf.mu.Unlock()
}

// Run 运行管理中心
func (sf *Hub) Run(ctx context.Context) {
	defer func() {
		sf.mu.Lock()
		for _, group := range sf.sessions {
			for _, sess := range group {
				sess.cancel()
			}
		}
		sf.sessions = make(map[string]map[string]*Session)
		sf.mu.Unlock()
	}()

	for {
		select {
		case <-sf.broadcast:

		case <-ctx.Done():
			sf.cancel() // local cancel mark it closed
			return
		case <-sf.ctx.Done():
			return
		}
	}
}

// BroadCast 广播消息
func (sf *Hub) BroadCast(t int, data []byte) error {
	select {
	case sf.broadcast <- &message{t, data}:
	case <-sf.ctx.Done():
		return ErrHubClosed
	default:
		return ErrHubBufferFull
	}
	return nil
}

// SessionLen 返回客户端会话的数量
func (sf *Hub) SessionLen() (count int) {
	sf.mu.Lock()
	count = sf.sessCnt
	sf.mu.Unlock()
	return
}

func (sf *Hub) GroupLen() (count int) {
	sf.mu.Lock()
	count = sf.groupCnt
	sf.mu.Unlock()
	return
}

// Close 关闭
func (sf *Hub) Close() {
	sf.cancel()
}

// IsClosed 判断是否关闭
func (sf *Hub) IsClosed() bool {
	return sf.ctx.Err() != nil
}

// UpgradeWithRun 升级成websocket并运行起来
func (sf *Hub) UpgradeWithRun(w http.ResponseWriter, r *http.Request) error {
	upGrader := &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
	conn, err := upGrader.Upgrade(w, r, w.Header())
	if err != nil {
		return err
	}

	sess := &Session{
		GroupID: "",
		ID:      "",
		Request: r,
		Conn:    conn,
		Hub:     sf,
	}

	sess.Run()

	return nil
}
