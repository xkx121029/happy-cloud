// 独立信令服务器：仅做 SDP/ICE 的房间配对转发，零业务逻辑。
// 它不校验 token，也不关心数据内容；真正的鉴权由后端在 WebRTC DataChannel
// 握手（R 帧）时完成。这样信令服务器无需与业务服务共享任何密钥，可独立部署。
//
// 路由约定（支持多客户端并发）：
//   - 每个连接收到 welcome 时被分配唯一 id。
//   - client → host：转发给本 room 的唯一 host（即使 host 刚连上）。
//   - host  → client：消息 JSON 需带 "to": "<clientId>"（点对点）或 "*"（广播本 room 所有 client）。
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/websocket"
)

type conn struct {
	id   string
	ws   *websocket.Conn
	role string // "host" 或 "client"
	room string
}

type room struct {
	host    *conn
	clients map[string]*conn // key: conn.id
}

var (
	mu         sync.Mutex
	rooms      = map[string]*room{}
	connSeq    int
	upgrader   = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		// 信令仅中转，允许跨域（最终业务鉴权在后端隧道内完成）
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

func getRoom(id string) *room {
	r := rooms[id]
	if r == nil {
		r = &room{clients: map[string]*conn{}}
		rooms[id] = r
	}
	return r
}

func nextID() string {
	mu.Lock()
	connSeq++
	id := fmt.Sprintf("%d", connSeq)
	mu.Unlock()
	return id
}

func sendRaw(c *conn, msg string) {
	if c == nil || c.ws == nil {
		return
	}
	_ = c.ws.WriteMessage(websocket.TextMessage, []byte(msg))
}

func routeFromHost(r *room, from *conn, msg []byte) {
	var env struct {
		To string `json:"to"`
	}
	_ = json.Unmarshal(msg, &env)
	if env.To == "" || env.To == "*" {
		for _, c := range r.clients {
			if c != from {
				sendRaw(c, string(msg))
			}
		}
		return
	}
	if c, ok := r.clients[env.To]; ok && c != from {
		sendRaw(c, string(msg))
	}
}

func leaveRoom(c *conn) {
	mu.Lock()
	defer mu.Unlock()
	r, ok := rooms[c.room]
	if !ok {
		return
	}
	if c.role == "host" {
		if r.host == c {
			r.host = nil
		}
		for _, cl := range r.clients {
			if cl != c {
				sendRaw(cl, `{"t":"host-offline"}`)
			}
		}
	} else {
		delete(r.clients, c.id)
	}
	if r.host == nil && len(r.clients) == 0 {
		delete(rooms, c.room)
	}
	c.ws.Close()
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	role := r.URL.Query().Get("role")
	rm := r.URL.Query().Get("room")
	if role != "host" && role != "client" {
		http.Error(w, "bad role", 400)
		return
	}
	if rm == "" {
		http.Error(w, "bad room", 400)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade err: %v", err)
		return
	}
	c := &conn{id: nextID(), ws: ws, role: role, room: rm}

	mu.Lock()
	r0 := getRoom(rm)
	if role == "host" {
		if r0.host != nil && r0.host != c {
			sendRaw(r0.host, `{"t":"replace"}`)
			r0.host.ws.Close()
		}
		r0.host = c
	} else {
		r0.clients[c.id] = c
	}
	mu.Unlock()

	c.ws.WriteMessage(websocket.TextMessage, []byte(`{"t":"welcome","id":"`+c.id+`","room":"`+rm+`"}`))

	for {
		mt, data, err := c.ws.ReadMessage()
		if err != nil {
			break
		}
		if mt == websocket.CloseMessage {
			break
		}
		msg := string(data)
		mu.Lock()
		r0 = rooms[rm]
		if role == "host" {
			// host → 指定 client
			if r0 != nil {
				routeFromHost(r0, c, data)
			}
		} else {
			// client → host
			if r0 != nil && r0.host != nil {
				sendRaw(r0.host, msg)
			}
		}
		mu.Unlock()
	}
	leaveRoom(c)
}

func main() {
	port := os.Getenv("SIGNALING_PORT")
	if port == "" {
		port = "8787"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleWS)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("happy-cloud signaling server"))
	})
	log.Printf("Happy-Cloud 信令服务器监听于 %s", ":"+port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}