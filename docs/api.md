# Happy-Cloud API 契约 v1.0

统一前缀：`/api`。除登录注册外，所有接口需在请求头携带 `Authorization: Bearer <token>`。

## 统一响应格式

```json
{ "code": 0, "message": "ok", "data": {} }
```

`code = 0` 表示成功；非 0 表示失败，`message` 为错误说明（中文）。`data` 为具体业务数据。

## 认证

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /api/auth/register | 注册 `{username, password, email}` → `{token, user}` |
| POST | /api/auth/login | 登录 `{username, password}` → `{token, user}` |
| GET | /api/auth/me | 当前用户信息 |

user 对象：`{id, username, email, role, quota_max, quota_used, status, created_at}`，`role`: 0 普通 / 1 管理员。

## 文件（需登录）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/files/list?parent_id=0&page=1&page_size=100 | 文件/文件夹列表 |
| POST | /api/files/mkdir | 新建文件夹 `{parent_id, name}` |
| POST | /api/files/upload/hash | 秒传校验 `{name, size, hash, parent_id}` → `{exists: true, file_id}` |
| POST | /api/files/upload | multipart 小文件直传（表单字段: file, parent_id） |
| POST | /api/files/upload/chunk | multipart 上传分片（字段: file, hash, chunk_index, chunk_total） |
| POST | /api/files/upload/merge | 合并分片 `{hash, name, size, parent_id, chunk_total}` → file |
| GET | /api/files/download?file_id= | 下载文件流（含 Content-Disposition） |
| POST | /api/files/rename | 重命名 `{file_id, new_name}` |
| POST | /api/files/move | 移动 `{file_id, target_parent_id}` |
| POST | /api/files/delete | 删除 `{file_ids: [1,2]}` |
| GET | /api/files/quota | 空间用量 `{quota_max, quota_used}` |

file 对象：`{id, parent_id, name, type, size, hash, created_at}`，`type`: 0 文件夹 / 1 文件。

## 分享

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /api/share/create | `{file_id, password?, expire_at?}` → `{token, url}` |
| GET | /api/share/:token | 分享信息（需密码时先 verify）`{file, files?, password_required}` |
| POST | /api/share/:token/verify | `{password}` → ok |
| GET | /api/share/:token/download?file_id= | 分享下载文件流 |

## 管理（需 role=1）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/admin/users?page=&page_size=&keyword= | 用户列表 |
| PATCH | /api/admin/users/:id | `{status?, quota_max?}` 禁用/启用/改配额 |
| DELETE | /api/admin/users/:id | 删除用户 |
| GET | /api/admin/files?page=&page_size=&keyword= | 全站文件检索 |
| DELETE | /api/admin/files/:id | 强制删除文件 |
| GET | /api/admin/stats | 统计 `{user_count, file_count, storage_used, today_uploads, online_users}` |
| GET | /api/admin/logs?page=&page_size= | 操作日志 `{id, user_id, username, action, detail, created_at}` |

## 鉴权说明

- JWT 有效期 7 天，Payload 含 `user_id`、`username`、`role`。
- 大文件阈值 100MB，每分片 10MB。
- 秒传 Hash 算法：SHA256（客户端计算整个文件）。
