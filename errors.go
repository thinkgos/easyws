package easyws

import "errors"

// 错误返回
var (
	ErrHubClosed         = errors.New("hub is closed")
	ErrHubBufferFull     = errors.New("hub is closed")
	ErrSessionClosed     = errors.New("session is closed")
	ErrSessionBufferFull = errors.New("session buffer is full")
)
