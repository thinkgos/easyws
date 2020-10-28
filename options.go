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

// config 配置
type config struct {
	SessionConfig
	upgrader          *websocket.Upgrader
	sendHandler       func(s *Session, t int, data []byte)
	receiveHandler    func(s *Session, t int, data []byte)
	connectHandler    func(s *Session)
	disconnectHandler func(s *Session)
	pingHandler       func(s *Session, str string)
	pongHandler       func(s *Session, str string)
	closeHandler      func(s *Session, code int, text string) error
	errorHandler      func(s *Session, err error)
}

// defaultConfig 创建默认选项
func defaultConfig() config {
	return config{
		SessionConfig{
			WriteWait:         1 * time.Second,
			KeepAlive:         60 * time.Second,
			Radtio:            110,
			MaxMessageSize:    0,
			MessageBufferSize: 32,
		},
		&websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
		func(s *Session, t int, data []byte) {},
		func(s *Session, t int, data []byte) {},
		func(s *Session) {},
		func(s *Session) {},
		func(s *Session, str string) {},
		func(s *Session, str string) {},
		nil,
		func(s *Session, err error) {},
	}
}

type Option func(hub *Hub)

// SetSessionConfig 设置会话配置
func WithSessionConfig(cfg *SessionConfig) Option {
	return func(hub *Hub) {
		hub.SessionConfig = *cfg
	}
}

// SetUpgrade 设置升级配置
func WithUpgrade(u *websocket.Upgrader) Option {
	return func(hub *Hub) {
		hub.upgrader = u
	}
}

// SetReceiveHandler 设置接收回调
func WithReceiveHandler(f func(s *Session, t int, data []byte)) Option {
	return func(hub *Hub) {
		hub.receiveHandler = f
	}
}

// SetSendHandler 设置发送回调
func WithSendHandler(f func(s *Session, t int, data []byte)) Option {
	return func(hub *Hub) {
		hub.sendHandler = f
	}
}

// SetConnectHandler 设置连接回调
func WithConnectHandler(f func(s *Session)) Option {
	return func(hub *Hub) {
		hub.connectHandler = f
	}
}

// SetDisonnectHandler 设置断开连接回调
func WithDisonnectHandler(f func(s *Session)) Option {
	return func(hub *Hub) {
		hub.disconnectHandler = f
	}
}

// SetPongHandler 设置收到Pong回调
func WithPongHandler(f func(s *Session, str string)) Option {
	return func(hub *Hub) {
		hub.pongHandler = f
	}

}

// SetPingHandler 设置收到Ping回调
func WithPingHandler(f func(s *Session, str string)) Option {
	return func(hub *Hub) {
		hub.pingHandler = f
	}
}
