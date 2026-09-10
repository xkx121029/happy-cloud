# Happy-Cloud API 契约 v2.0

> 供 Web / Windows（Flutter）/ Android 客户端统一对接。

## 通用约定

- **Base URL**：`http://<host>/api`（默认 Web 网关 80 端口，后端 8080）
- **鉴权**：除「注册 / 登录 / 分享访问 / 公共共享下载」外，其余接口需携带请求头 `Authorization: Bearer <token>`
- **JWT**：登录后返回 `token`，有效期 7 天；Payload 含 `user_id`、`username`、`role`
- **内容类型**：JSON 接口用 `Content-Type: application/json`；上传用 `multipart/form-data`

### 统一响应格式

```json
{ "code": 0, "message": "ok", "data": {} }
```

- `code = 0` 成功；非 0 失败，`message` 为中文错误说明
- 分页统一为 `data = { total, page, page_size, items }`

### 常见错误码

| code | 含义 |
|---|---|
| 400 | 参数错误 |
| 401 | 未登录 / 密码错误 |
| 403 | 无权限 / 账号禁用 |
| 404 | 资源不存在 |
| 409 | 重名冲突 |
| 410 | 分享链接已过期 |
| 500 | 服务内部错误 |
| 507 | 存储空间不足 |

### 数据对象

**user**：`{id, username, email, role, quota_max, quota_used, status, created_at, updated_at}`
- `role`: 0 普通 / 1 管理员；`status`: 0 正常 / 1 禁用；`quota_max/quota_used` 单位为字节

**file**：`{id, parent_id, name, type, size, hash, is_shared, created_at, updated_at}`
- `type`: 0 文件夹 / 1 文件；`parent_id`: 0 表示根目录；`is_shared`: 0 私有 / 1 已共享到公共目录

**share**：`{id, file_id, token, expire_at, views, created_at}`（`password` 不回传）

**transfer**：`{id, sender_id, sender_name, receiver_id, file_id, file_name, file_size, status, created_at, updated_at}`
- `status`: 0 待处理 / 1 已接受 / 2 已拒绝

**notification**：`{id, type, title, content, transfer_id, is_read, created_at}`
- `type`: `transfer`（文件转送）；通知列表项额外带 `transfer_status`（-1 无关联转送）

---

## 认证

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| POST | /api/auth/register | 否 | 注册 `{username, password, email}` → `{token, user}` |
| POST | /api/auth/login | 否 | 登录 `{username, password}` → `{token, user}` |
| GET | /api/auth/me | 是 | 当前登录用户信息 → user |
| POST | /api/auth/change-password | 是 | 修改密码 `{old_password, new_password, target_user_id?}` → ok |
| GET | /api/user/settings | 是 | 读取个性化设置 → `{settings: "<json字符串>"}` |
| PUT | /api/user/settings | 是 | 保存个性化设置 `{settings: "<json字符串>"}` → ok |

> `change-password`：不传 `target_user_id` 时校验 `old_password` 改自己；管理员传 `target_user_id` 可重置他人密码（无需旧密码）。

---

## 文件（需登录）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/files/list?parent_id=0&page=1&page_size=100 | 目录文件列表（分页） |
| POST | /api/files/mkdir | 新建文件夹 `{parent_id, name}` → file |
| POST | /api/files/upload/hash | 秒传校验 `{name, size, hash, parent_id}` → `{exists, file_id}` |
| POST | /api/files/upload | multipart 直传（≤100MB）：字段 `file`、`parent_id` → file |
| POST | /api/files/upload/chunk | multipart 分片上传：字段 `file`、`hash`、`chunk_index`、`chunk_total` → `{chunk_index}` |
| POST | /api/files/upload/merge | 合并分片 `{hash, name, size, parent_id, chunk_total}` → file |
| GET | /api/files/download?file_id= | 下载文件流（Content-Disposition 文件名） |
| POST | /api/files/rename | 重命名 `{file_id, new_name}` → file |
| POST | /api/files/move | 移动 `{file_id, target_parent_id}` → ok |
| POST | /api/files/delete | 删除 `{file_ids: [1,2]}` → ok |
| GET | /api/files/quota | 空间用量 → `{quota_max, quota_used}` |

### 分片上传流程（客户端实现）
1. 计算整个文件 SHA256 → `upload/hash` 秒传校验；`exists=true` 直接完成
2. 否则按每片 10MB 切分，依次 `upload/chunk`（`chunk_index` 从 0 起）
3. 全部传完调 `upload/merge` 合并入库
- 阈值：单文件 >100MB 走分片；每分片固定 10MB

### 公共共享目录
| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /api/files/shared/toggle | 共享/取消 `{file_id, shared}` → `{is_shared}`（仅文件、仅所有者） |
| GET | /api/files/shared/list | 全站公共共享文件列表（含 `username` 上传者） |
| GET | /api/files/shared/download?file_id= | 下载公共共享文件流 |

---

## 转送（私聊发送，需登录）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/transfer/users?keyword= | 搜索用户（排除自己，limit 20）→ `[{id, username, email}]` |
| POST | /api/transfer/send | 发送 `{receiver_id, file_id}` → transfer（接收方生成通知） |
| GET | /api/transfer/incoming | 我收到的转送列表（limit 50） |
| POST | /api/transfer/:id/accept | 接受转送：转存到我的网盘根目录 → file |
| POST | /api/transfer/:id/reject | 拒绝转送 → transfer |

> 仅文件可发送；接受方需配额充足（不足返回 507）；接受后原文件实体被多用户复用。

---

## 通知（需登录）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/notifications/list?page=1&page_size=20 | 通知列表（分页，附 `transfer_status`） |
| POST | /api/notifications/:id/read | 标记单条已读 → ok |
| GET | /api/notifications/unread | 未读数量 → `{count}` |

---

## 分享（公开访问）

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| POST | /api/share/create | 是 | 创建 `{file_id, password?, expire_at?}` → `{token, url}` |
| GET | /api/share/:token | 否 | 分享信息 → `{share, file, password_required, files?}` |
| POST | /api/share/:token/verify | 否 | 校验密码 `{password}` → `{verified}` |
| GET | /api/share/:token/download?file_id=&password= | 否 | 分享下载文件流（密码可走 query 或 `X-Share-Password` 头） |

> `expire_at` 用 ISO8601 时间字符串（RFC3339）。文件夹分享：`GetShare` 返回子项 `files`，下载需指定 `file_id`（必须是分享目录后代）。

---

## 管理（需登录 + role=1）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/admin/users?page=&page_size=&keyword= | 用户列表（分页） |
| PATCH | /api/admin/users/:id | `{status?, quota_max?}` 禁用/启用/改配额 |
| DELETE | /api/admin/users/:id | 删除用户 |
| GET | /api/admin/files?page=&page_size=&keyword= | 全站文件检索（分页） |
| DELETE | /api/admin/files/:id | 强制删除文件 |
| GET | /api/admin/stats | 统计 → `{user_count, file_count, storage_used, today_uploads, online_users}` |
| GET | /api/admin/logs?page=&page_size= | 操作日志（分页）`{id, user_id, username, action, detail, created_at}` |

---

## 客户端对接要点

- **Token 存储**：登录后本地持久化（localStorage / SharedPreferences / secure storage），失效（401）后跳登录页
- **下载**：文件下载为流式响应，文件名从 `Content-Disposition` 的 `filename*`（UTF-8）解析
- **上传**：大文件分片 + 秒传 + 断点续传，客户端自行管理 chunk 与进度
- **通知**：客户端轮询 `/api/notifications/unread`（推荐 30–60s）驱动红点与下拉刷新
- **公共共享**：`/api/files/shared/list` 公开只读（需登录），可直接展示他人共享文件
