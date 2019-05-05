package easyws

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// SessionConfig 会话配置
type SessionConfig struct {
	WriteWait         time.Duration // 写超时时间
	KeepAlive         time.Duration // 保活时间
	Radtio            int           // 监控比例, 需大于100,默认系统是110 即比例1.1
	MaxMessageSize    int64         // 消息最大字节数, 如果为0,使用系统默认设置
	MessageBufferSize int           // 消息缓存数
}

// Options 选项配置
type Options struct {
	config            *SessionConfig
	upgrader          *websocket.Upgrader
	receiveHandler    func(s *Session, t int, data []byte)
	sendHandler       func(s *Session, t int, data []byte)
	closeHandler      func(s *Session, code int, text string) error
	connectHandler    func(s *Session)
	disconnectHandler func(s *Session)
	errorHandler      func(s *Session, err error)
	pongHandler       func(s *Session, str string)
	pingHandler       func(s *Session, str string)
}

// NewOptions 创建默认选项
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
		receiveHandler:    func(s *Session, t int, data []byte) {},
		sendHandler:       func(s *Session, t int, data []byte) {},
		connectHandler:    func(s *Session) {},
		disconnectHandler: func(s *Session) {},
		closeHandler:      nil,
		errorHandler:      func(s *Session, err error) {},
		pongHandler:       func(s *Session, str string) {},
		pingHandler:       func(s *Session, str string) {},
	}
}

// SetSessionConfig 设置会话配置
func (this *Options) SetSessionConfig(cfg *SessionConfig) {
	this.config = cfg
}

// SetUpgrade 设置升级配置
func (this *Options) SetUpgrade(u *websocket.Upgrader) {
	this.upgrader = u
}

// SetReceiveHandler 设置接收回调
func (this *Options) SetReceiveHandler(f func(s *Session, t int, data []byte)) {
	if f != nil {
		this.receiveHandler = f
	}
}

// SetSendHandler 设置发送回调
func (this *Options) SetSendHandler(f func(s *Session, t int, data []byte)) {
	if f != nil {
		this.sendHandler = f
	}
}

// SetConnectHandler 设置连接回调
func (this *Options) SetConnectHandler(f func(s *Session)) {
	if f != nil {
		this.connectHandler = f
	}
}

// SetDisonnectHandler 设置断开连接回调
func (this *Options) SetDisonnectHandler(f func(s *Session)) {
	if f != nil {
		this.disconnectHandler = f
	}
}

// SetPongHandler 设置收到Pong回调
func (this *Options) SetPongHandler(f func(s *Session, str string)) {
	if f != nil {
		this.pongHandler = f
	}
}

// SetPingHandler 设置收到Ping回调
func (this *Options) SetPingHandler(f func(s *Session, str string)) {
	if f != nil {
		this.pingHandler = f
	}
}
