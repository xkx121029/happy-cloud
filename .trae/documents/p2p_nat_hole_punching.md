# HappyCloud 全线 P2P NAT 打洞实现方案

## Context（背景）

HappyCloud 客户端（Android/Web）目前只能 HTTP 直连后端。在跨 NAT/受限网络下无法直连。目标：为全线产品增加 P2P 打洞，客户端穿透 NAT **直连后端服务器**读索引、传文件，局域网/穿透直连加速，TURN 兜底。

**已确认决策**：WebRTC + TURN 兜底；覆盖 Android + Web + 后端；本次交付「端到端可用」（信令连通 → JSON 隧道代理 → 文件直传），**不含公网 TURN 部署**（留待下阶段）；**额外制作一个独立信令服务器**（独立进程，做 SDP/ICE 协调）。

**核心思路**：WebRTC DataChannel 作隧道，最大化复用现有 handler。
- 轻量请求（列表/配额/新建/重命名/删除/秒传/合并）→ 隧道内 **JSON HTTP 代理**（`r.ServeHTTP`+回填 `Authorization: Bearer`），现有 handler 零逻辑改动。
- 大文件 → 隧道内**专用二进制流**（下载 `D` 帧、上传分片 `U` 帧，复用落盘/合并）。

**架构**
```
客户端(Android/Web)                       信令服务器(独立 Go, :8787)         后端(host role, :8080)
    │─ws(config.ws, room=hostId)─┐       房间配对转发 SDP/ICE/心跳            ─ws(room)→┘
    │                            │            offer/answer/ice 双向       host连入→answerer
    ▼                            ▼                                        │
   WebRTC.DataChannel 直接打洞到后端 (隧道帧 + 16KB分帧) ◄──────────────┘  │ host role 提供隧道
   TURN 兜底(下阶段, relay)                                                 │
   （局域网/弱NAT用 host/srflx/prflx succeed）
```

**必须践行的已知事实**：
1. 后端业务失败恒为 `HTTP 200 + {code!=0}`，代理响应判断成败看 `body.code`。
2. 浏览器原生 WebSocket 不能带 Header → 信令鉴权由**后端在 DataChannel 握手 `R` 时校验 token**（信令服务器只转发，零业务/零密钥）。
3. DataChannel 大消息跨端上限不一 → 隧道必须**应用层 ≤16KB 分帧 + 序号重组**。
4. 分片落盘复用 `util.ChunkDir`；秒传/合并复用 `UploadHash`/`UploadMerge`。
5. 打洞判定：`selectedCandidatePair.candidateType` host/srflx/prflx=直连，relay=TURN。

---

## 一、独立信令服务器（新增 [backend/cmd/signaling](d:\xkx\xkx_appproj\happy_teach\happy_cloud\backend\cmd\signaling)）

- `main.go`：`mu + rooms map[string]*Room{host net.Conn; clients []*Conn}`；WS 端点 `/ws?role=host|client&room=`；按 `room` 配对，host 只需 1 个、client 多对多。
- 消息类型（透传转发，不解析业务）：`{t:"welcome"}`、`offer`、`answer`、`ice`、`ping/pong`、`close`。host 侧的 `offer` 转发给对应 client，client 的 `answer/ice` 转发给 host；client 间互不转发。
- 生命周期：心跳 25s、断线清理 room、host 掉线通知全部 client（前端据 `host-offline` 重建）。
- 独立 `Dockerfile.cmd` 或复用 Dockerfile 的多阶段；端口由 `SIGNALING_PORT`(`:8787`)控制。

## 二、后端（[backend](d:\xkx\xkx_appproj\happy_teach\happy_cloud\backend)）

### 依赖与配置
- `go.mod` 新增：`pion/webrtc/v4`、`pion/stun/v3`、`gorilla/websocket`（信令共用）。TURN 本次不启用。
- [config.go](d:\xkx\xkx_appproj\happy_teach\happy_cloud\backend\config\config.go) 新增：`P2PEnabled`(true)、`StunEnabled`(true)、`StunAddr`(`:3478`)、`P2PMaxStreams`(4)、`SignalingURL`(后端 host 连信令的 ws 地址)、`PublicHost`(下发客户的 STUN 地址)、`HostRoom`(host 连接的 room id，默认 `happy-cloud`)。

### 新增 `internal/p2p/`
| 文件 | 职责 |
|---|---|
| `signaling_client.go` | 后端作为 host 主动连信令服务器 `room`：收 `offer`→(交给 session answerer)`answer`+`ice`；断线重连 |
| `session.go` | 作为 answerer：`webrtc.NewPeerConnection`、`OnDataChannel`→`setupTunnel`、trickle `OnICECandidate` 转发 `ice`、`OnConnectionStateChange` 清理、每用户单会话/并发上限 |
| `tunnel.go` | DataChannel 握手 `R`(校验 Bearer token→`middleware.ParseToken`)、帧编解码(16KB 分帧聚合)、心跳 `P/p`、分发 |
| `proxy.go` | JSON 代理：`{t:j,id,m,path,q,h,b64}`→回填 token→`r.ServeHTTP(recorder,req)`→`{t:J,...}`（成败看 body.code） |
| `transfer.go` | 下载流 `D`(256KB 切帧/`last`/ACK 窗口/取消)、上传分片 `U`(写 `ChunkDir(userID,hash)`)、merge 走代理复用 `UploadMerge`；抽公共 `ResolveFile` |
| `iceserver.go` | 内置 STUN 引导；`welcome` 拼装 `iceServers`；`GET /api/p2p/config`(Auth) 返回 `{ws, room, iceServers}`(三端用它取信令/ICE 配置) |

### [main.go](d:\xkx\xkx_appproj\happy_teach\happy_cloud\backend\main.go)
```go
p2p := api.Group("/p2p")
p2p.GET("/config", middleware.Auth(), p2pSvc.Config)   // {ws, room, iceServers}
```
启动时：`if P2PEnabled { go runSTUN(StunAddr); go hostSignaling(SignalingURL, HostRoom) }`（后端作为 host 参与打洞协商）。

## 三、Web（[web](d:\xkx\xkx_appproj\happy_teach\happy_cloud\web)）—— 无新 npm 依赖
新增 `src/p2p/`：`frames.ts`(帧编解码/`FrameDecoder`/bufferedAmount)、`config.ts`(`fetchP2PConfig()`)、`signaling.ts`(WS 连信令 `config.ws?role=client&room=`+心跳+退避重连)、`transport.ts`(`RTCPeerConnection`+iceServers+`createDataChannel('tunnel')`+candidateType 判定)、`tunnel.ts`(`tunnelRequest`/`downloadStream`/`uploadChunk`)；`composables/useP2P.ts`(mode/enabled, 存 `pinia/settings.ts`)。
接入：`api/request.ts` 抽 `call()`(HTTP 优先, p2p 开启且失败时降级 `tunnelRequest`)；`utils/uploader.ts` 大文件分片走隧道；`utils/download.ts` p2p `downloadStream`；`SettingsDrawer.vue`/顶栏连接模式标签。

## 四、Android（[android](d:\xkx\xkx_appproj\happy_teach\happy_cloud\android)）
依赖：`libs.versions.toml`+[app/build.gradle.kts](d:\xkx\xkx_appproj\happy_teach\happy_cloud\android\app\build.gradle.kts) 新增 `org.webrtc:google-webrtc:1.0.32006`。
新增 `data/p2p/`：`TunnelFrames.kt`、`SignalingClient.kt`(okhttp WS 连信令)、`PeerManager.kt`(`PeerConnection`+DataChannel+trickle+`getSelectedCandidatePair` 判 mode)、`TunnelRpc.kt`(JSON 代理)、`TunnelTransfer.kt`(下载流/上传分片/进度)。
接入：`data/Transport.kt`(`HttpTransport`+`PeerTransport`)、`Network.kt`(ws/config URL)、`HappyCloudApp.kt`(`transport`+`connectionMode: StateFlow`)、`UploadUtil/DownloadUtil`(P2P 分支)、`FilesViewModel`+顶部 `connMode` 标签。

## 五、编排（[docker-compose.yml](d:\xkx\xkx_appproj\happy_teach\happy_cloud\docker-compose.yml)）
新增 `signaling` 服务（独立容器，`SIGNALING_PORT:8787`）；`backend` 增环境变量 `P2P_ENABLED`、`SIGNALING_URL=ws://signaling:8787`、`STUN_ADDR`、`PUBLIC_HOST`。client 通过 `GET /api/p2p/config` 拿公网可达的 `ws` 与 `iceServers`。

---

## 六、关键协议要点（三端统一）
- 帧：`[4B BE jsLen][JSON信封][(4B BE binLen)][bin]`；`>16KB` 拆多 packet 带 `meta.f/fn` 聚合；`t`∈R/P/p/j/J/U/D/E/A/C。
- 下载：`j`+`meta.dl:{fileId}`→服务端循环 `D` 帧→窗口 ACK 背压→`last:true` 结束；取消 `A code:2`。
- 上传：`U`+bin 写 `ChunkDir/idx`→全传完走隧道 `j POST /files/upload/merge`；秒传走 `j /files/upload/hash`。
- 打洞：局域网/弱 NAT 用 host/srflx/prflx 成功；公网强 NAT+无 TURN 会失败——**本次明确依赖边界，不假装中继**。

## 七、验证（局域网内，无需公网 TURN）
1. `go build ./...`（含 cmd/signaling）；`docker compose up -d signaling backend` 后信令服务 8787 就绪。
2. 打洞连通：Web/Android 经 `GET /api/p2p/config` 拿 ws，连上后 `connectionState==='connected'`、`datachannel.onopen`；`getStats().candidateType` 须为 host/srflx/prflx，**不得 relay**。
3. JSON 代理：强制 P2P 后列表/配额/新建/删除正常；后端日志确认经隧道（无独立 HTTP 记录）。
4. 文件直传：两端各传 >100MB 触发分片 → `UploadMerge` 全量校验通过；下载后 `sha256sum` 一致；`bufferedAmount` 背压生效。
5. 真机局域网 IP 打通；UI 状态标签显示 `直连/打洞(srflx)`。

**外部依赖边界（本次不实现）**：公网强 NAT 穿透/(TURN relay)/临时凭据/TURN 生产部署 → 留待下一阶段。