// proxy：隧道内 JSON HTTP 代理。客户端发 {t:"j",m,path,q,h,b}→回填鉴权头→
// 经 gin 路由 ServeHTTP→回 {t:"J",status,b}。业务成败由 body.code 判断（后端恒 200+code）。
package p2p

import (
	"bytes"
	"net/http/httptest"
	"net/url"
	"strings"
)

func (t *Tunnel) handleJSON(env Envelope) {
	if env.P == "" || env.M == "" {
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