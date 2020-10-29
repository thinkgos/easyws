package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/thinkgos/easyws"
)

func main() {
	options := []easyws.Option{
		easyws.WithConnectHandler(func(sess *easyws.Session) {
			log.Println("connect ", sess.GroupID, sess.ID)
		}),
		easyws.WithDisconnectHandler(func(sess *easyws.Session) {
			log.Println("disconnect ", sess.GroupID, sess.ID)
		}),
		easyws.WithPingHandler(func(sess *easyws.Session, str string) {
			log.Println("ping ", sess.GroupID, sess.ID)
		}),
		easyws.WithPongHandler(func(sess *easyws.Session, str string) {
			log.Println("pong ", sess.GroupID, sess.ID)
		}),
		easyws.WithReceiveHandler(func(sess *easyws.Session, messageType int, data []byte) {
			log.Println("message ", sess.GroupID, sess.ID, messageType, string(data))
		}),
	}

	hub := easyws.New(options...)

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		upGrader := &websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		}
		conn, err := upGrader.Upgrade(w, r, w.Header())
		if err != nil {
			return
		}

		sess := &easyws.Session{
			GroupID: "testGroup",
			ID:      "testId",
			Request: r,
			Conn:    conn,
			Hub:     hub,
		}
		go func() {
			time.Sleep(time.Second)
			for {
				err := hub.WriteMessage("testGroup", "testId", websocket.TextMessage, []byte("hello world"))
				if err != nil {
					log.Println("write exit ", err)
					return
				}
				time.Sleep(time.Second * 2)
			}
		}()
		sess.Run()
	})
	http.ListenAndServe(":8080", nil)
}
