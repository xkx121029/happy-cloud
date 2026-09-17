// proxy：隧道内 JSON HTTP 代理。客户端发 {t:"j",m,path,q,h,b}→回填鉴权头→
// 经 gin 路由 ServeHTTP→回 {t:"J",status,b}。业务成败由 body.code 判断（后端恒 200+code）。
package p2p

import (
	"bytes"
	"log"
	"net/http/httptest"
	"net/url"
	"strings"
)

// validMethod 校验 HTTP 方法白名单：仅允许标准读写方法
func validMethod(m string) bool {
	switch m {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}

// validPath 校验请求路径：必须以 '/' 开头且不含空白/控制字符
// （httptest.NewRequest 对非法 RequestURI 内部会 panic，必须在调用前拦截）
func validPath(p string) bool {
	if len(p) == 0 || p[0] != '/' {
		return false
	}
	for i := 0; i < len(p); i++ {
		if p[i] <= 0x20 || p[i] == 0x7f {
			return false
		}
	}
	return true
}

func (t *Tunnel) handleJSON(env Envelope) {
	// 兜底：DataChannel 回调 goroutine 不在 gin Recovery 保护栈内，任何 panic
	// 都会终止整个进程，此处 recover 防止异步回调崩溃（Bug2 修复）
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[p2p] 隧道 JSON 处理 panic 已捕获: %v", r)
			t.send(Envelope{T: "E", ID: env.ID, C: 400, Msg: "参数错误"}, nil)
		}
	}()
	if env.P == "" || env.M == "" {
		t.send(Envelope{T: "E", ID: env.ID, C: 400, Msg: "参数错误"}, nil)
		return
	}
	// 校验 HTTP 方法白名单：仅允许 GET/POST/PUT/PATCH/DELETE
	if !validMethod(env.M) {
		t.send(Envelope{T: "E", ID: env.ID, C: 400, Msg: "参数错误"}, nil)
		return
	}
	// 校验路径：必须以 '/' 开头且不含空白/控制字符，防止 httptest.NewRequest panic
	if !validPath(env.P) {
		t.send(Envelope{T: "E", ID: env.ID, C: 400, Msg: "参数错误"}, nil)
		return
	}
	u := &url.URL{Path: env.P}
	q := u.Query()
	for k, v := range env.Q {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	var body []byte
	if len(env.B) > 0 {
		body = env.B
	}
	req := httptest.NewRequest(env.M, u.RequestURI(), bytes.NewReader(body))
	if l := len(body); l > 0 {
		req.ContentLength = int64(l)
	}
	for k, v := range env.H {
		req.Header.Set(k, v)
	}
	if !strings.HasPrefix(req.Header.Get("Content-Type"), "application/") && len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	// 回填鉴权：隧道内就代表已登录，与 HTTP 直连保持同样权限
	req.Header.Set("Authorization", "Bearer "+t.token)

	rec := httptest.NewRecorder()
	t.s.engine.ServeHTTP(rec, req)

	resp := append([]byte(nil), rec.Body.Bytes()...)
	t.send(Envelope{T: "J", ID: env.ID, Status: rec.Code}, resp)
}
