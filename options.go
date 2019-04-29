package easyws

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	tuple = 3
)

// websocket 配置
type SessionConfig struct {
	WriteWait         time.Duration // 写超时时间
	KeepAlive         time.Duration // 保活时间
	Radtio            int           // 监控比例, 需大于100,默认系统是110 即比例1.1
	MaxMessageSize    int64         // 消息最大字节数, 如果为0,使用系统默认设置
	MessageBufferSize int           // 消息缓存数
}

type Options struct {
	config         *SessionConfig
	upgrader       *websocket.Upgrader
	receiveHandler func(s *Session, t int, data []byte)
	sendHandler    func(s *Session, t int, data []byte)
}

func NewOptions() *Options {
	return &Options{
		config: &SessionConfig{
			WriteWait:         1 * time.Second,
			KeepAlive:         60 * time.Second,
			Radtio:            110,
			MaxMessageSize:    0,
			MessageBufferSize: 32,
		},
		upgrader: &websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
		receiveHandler: func(s *Session, t int, data []byte) {},
		sendHandler:    func(s *Session, t int, data []byte) {},
	}
}

func (this *Options) SetConfig(cfg *SessionConfig) {
	this.config = cfg
}

func (this *Options) SetUpgrade(u *websocket.Upgrader) {
	this.upgrader = u
}

func (this *Options) SetReceiveHandler(f func(s *Session, t int, data []byte)) {
	if f != nil {
		this.receiveHandler = f
	}
}
