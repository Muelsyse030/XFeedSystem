# XFeedSystem API 文档

本文档基于当前代码实现整理，覆盖接口说明、调用方式、请求参数、响应格式、鉴权、分页游标和完整 cURL 示例。

## 1. 基础信息

- Base URL: `http://127.0.0.1:8000`
- Content-Type: `application/json`
- 字符集: `UTF-8`
- 鉴权方式: `Authorization: Bearer <token>`（仅部分接口需要）

健康检查：
- `GET /ping`

## 2. 统一说明

### 2.1 响应结构

项目当前接口响应结构并不完全统一，存在以下几类：

1) 仅返回 message
```json
{
  "message": "pong"
}
```

2) code/message/data 结构
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "token": "xxx"
  }
}
```

3) 错误结构可能为 error 或 message
```json
{
  "error": "用户名已存在"
}
```
或
```json
{
  "message": "invalid or expired token"
}
```

### 2.2 鉴权说明

受保护接口要求请求头：

```http
Authorization: Bearer <JWT_TOKEN>
```

JWT 特征：
- 算法: `HS256`
- 默认有效期: `72h`
- Claims 包含:
  - `uid` (int64)
  - `username` (string)
  - 标准字段 `exp` / `iat` / `iss`

未携带或格式错误会返回 `401`。

### 2.3 分页规则

#### 用户笔记列表 `/users/:id/notes`
- 使用整型游标 `cursor`（本质是 note.id）
- 查询逻辑：`id < cursor`
- 排序：`id DESC`
- `limit` 允许范围 `1~50`，否则后端强制为 `10`
- 首次请求建议 `cursor=0`

#### Feed `/feed`
- 使用字符串游标 `cursor`
- 编码格式：`<published_at_unix>_<note_id>`，例如 `1710000000_123`
- 查询逻辑：
  - `published_at < cursor.published_at`
  - 或 `published_at = cursor.published_at 且 id < cursor.id`
- 排序：`published_at DESC, id DESC`
- `limit` 允许范围 `1~50`，否则后端强制为 `10`

## 3. 接口明细

---

## 3.1 健康检查

### `GET /ping`

说明：服务连通性检查。

请求参数：无

成功响应：`200`
```json
{
  "message": "pong"
}
```

cURL：
```bash
curl -X GET 'http://127.0.0.1:8000/ping'
```

---

## 3.2 用户注册

### `POST /register`

说明：创建账号。

请求体：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| username | string | 是 | 用户名 |
| password | string | 是 | 密码 |
| confirm_password | string | 是 | 确认密码，需与 password 一致 |

请求示例：
```json
{
  "username": "alice",
  "password": "123456",
  "confirm_password": "123456"
}
```

成功响应：`200`
```json
{
  "message": "注册成功"
}
```

失败示例：`400`
```json
{
  "error": "用户名已存在"
}
```

可能错误（业务文案来自服务层）：
- `用户名或者密码不能为空`
- `确认密码不一致`
- `用户名已存在`
- `密码加密失败`
- `注册失败`

cURL：
```bash
curl -X POST 'http://127.0.0.1:8000/register' \
  -H 'Content-Type: application/json' \
  -d '{
    "username":"alice",
    "password":"123456",
    "confirm_password":"123456"
  }'
```

---

## 3.3 用户登录

### `POST /login`

说明：登录并获取 JWT。

请求体：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| username | string | 是 | 用户名 |
| password | string | 是 | 密码 |

请求示例：
```json
{
  "username": "alice",
  "password": "123456"
}
```

成功响应：`200`
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "token": "<JWT_TOKEN>"
  }
}
```

失败示例：`400`
```json
{
  "error": "密码错误"
}
```

可能错误：
- `用户名或者密码不能为空`
- `用户不存在`
- `密码错误`
- `generate token failed`（500）

cURL：
```bash
curl -X POST 'http://127.0.0.1:8000/login' \
  -H 'Content-Type: application/json' \
  -d '{
    "username":"alice",
    "password":"123456"
  }'
```

---

## 3.4 获取当前登录用户信息

### `GET /me`

说明：返回当前 JWT 对应用户信息。

鉴权：需要 `Bearer Token`

请求参数：无

成功响应：`200`
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 1,
    "username": "alice"
  }
}
```

失败示例：
- `401`
```json
{
  "message": "missing authorization header"
}
```
- `401`
```json
{
  "message": "invalid authorization header"
}
```
- `401`
```json
{
  "message": "invalid or expired token"
}
```

cURL：
```bash
curl -X GET 'http://127.0.0.1:8000/me' \
  -H 'Authorization: Bearer <JWT_TOKEN>'
```

---

## 3.5 创建笔记

### `POST /notes`

说明：创建一条笔记，作者为当前登录用户。

鉴权：需要 `Bearer Token`

请求体：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| title | string | 是 | 标题 |
| content | string | 是 | 正文 |

请求示例：
```json
{
  "title": "Go 并发学习",
  "content": "今天复习了 goroutine 和 channel"
}
```

成功响应：`200`
```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "id": 100,
    "author_id": 1,
    "title": "Go 并发学习",
    "content": "今天复习了 goroutine 和 channel",
    "published_at": "2026-03-20T10:00:00Z",
    "created_at": "2026-03-20T10:00:00Z"
  }
}
```

失败示例：
- `400`（JSON 绑定失败）
```json
{
  "code": 4001,
  "message": "<bind error>"
}
```
- `401`（未登录）
```json
{
  "code": 4002,
  "message": "用户未登录"
}
```
- `500`
```json
{
  "code": 5002,
  "message": "<create note error>"
}
```

cURL：
```bash
curl -X POST 'http://127.0.0.1:8000/notes' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <JWT_TOKEN>' \
  -d '{
    "title":"Go 并发学习",
    "content":"今天复习了 goroutine 和 channel"
  }'
```

---

## 3.6 获取笔记详情

### `GET /notes/:id`

说明：按笔记 ID 查询详情。

路径参数：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| id | int64 | 是 | 笔记 ID |

成功响应：`200`
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 100,
    "author_id": 1,
    "title": "Go 并发学习",
    "content": "今天复习了 goroutine 和 channel",
    "published_at": "2026-03-20T10:00:00Z",
    "created_at": "2026-03-20T10:00:00Z"
  }
}
```

失败示例：
- `400`
```json
{
  "code": 4002,
  "message": "invalid note id"
}
```
- `500`
```json
{
  "code": 5004,
  "message": "get note failed"
}
```

cURL：
```bash
curl -X GET 'http://127.0.0.1:8000/notes/100'
```

---

## 3.7 获取某用户笔记列表

### `GET /users/:id/notes`

说明：按用户 ID 获取其已发布笔记列表（倒序分页）。

路径参数：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| id | int64 | 是 | 用户 ID |

查询参数：

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|---|---|---|---|---|
| cursor | int64 | 否 | 0 | 游标，返回 `id < cursor` 的记录 |
| limit | int | 否 | 10 | 每页条数，建议 1~50 |

请求示例：
- 首次：`/users/1/notes?cursor=0&limit=10`
- 下一页：`/users/1/notes?cursor=上一页next_cursor&limit=10`

成功响应：`200`
```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "list": [
      {
        "id": 120,
        "author_id": 1,
        "title": "标题A",
        "content": "内容A",
        "published_at": "2026-03-20T10:00:00Z",
        "created_at": "2026-03-20T10:00:00Z"
      }
    ],
    "next_cursor": 120
  }
}
```

失败示例：
- `400`
```json
{
  "code": 4003,
  "message": "invalid user id"
}
```
- `500`
```json
{
  "code": 5003,
  "message": "list notes failed"
}
```

cURL：
```bash
curl -X GET 'http://127.0.0.1:8000/users/1/notes?cursor=0&limit=10'
```

---

## 3.8 删除笔记

### `DELETE /notes/:id`

说明：删除当前登录用户自己的笔记（逻辑删除，status 从 published 改为 deleted）。

鉴权：需要 `Bearer Token`

路径参数：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| id | int64 | 是 | 笔记 ID |

成功响应：`200`
```json
{
  "code": 0,
  "message": "ok"
}
```

失败示例：
- `400`
```json
{
  "code": 4002,
  "message": "invalid note id"
}
```
- `401`
```json
{
  "code": 4010,
  "message": "unauthorized"
}
```
- `401`
```json
{
  "code": 4011,
  "message": "invalid user id"
}
```
- `500`
```json
{
  "code": 5002,
  "message": "delete note failed"
}
```

cURL：
```bash
curl -X DELETE 'http://127.0.0.1:8000/notes/100' \
  -H 'Authorization: Bearer <JWT_TOKEN>'
```

---

## 3.9 Feed 流

### `GET /feed`

说明：获取推荐流（当前仅支持 `type=foryou`）。

查询参数：

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|---|---|---|---|---|
| type | string | 否 | foryou | 仅支持 foryou |
| cursor | string | 否 | 空 | 游标格式 `<unix_ts>_<id>` |
| limit | int | 否 | 10 | 每页条数，建议 1~50 |

请求示例：
- 首次：`/feed?type=foryou&limit=10`
- 下一页：`/feed?type=foryou&cursor=1710000000_123&limit=10`

成功响应：`200`
```json
{
  "code": 0,
  "message": "OK",
  "data": {
    "items": [
      {
        "id": 123,
        "author_id": 1,
        "author": {
          "id": 1,
          "username": "alice",
          "avatar_url": ""
        },
        "title": "标题",
        "content": "自动摘要后的内容...",
        "type": 1,
        "published_at": "2026-03-20T10:00:00Z"
      }
    ],
    "next_cursor": "1710000000_123"
  }
}
```

失败示例：
- `400`
```json
{
  "code": "4002",
  "message": "invalid feed type"
}
```
- `400`
```json
{
  "code": 4001,
  "message": "invalid limit"
}
```
- `500`
```json
{
  "code": 5001,
  "message": "invalid cursor"
}
```

cURL：
```bash
curl -X GET 'http://127.0.0.1:8000/feed?type=foryou&limit=10'
```

---

## 4. 快速联调流程

1) 注册
```bash
curl -X POST 'http://127.0.0.1:8000/register' \
  -H 'Content-Type: application/json' \
  -d '{"username":"alice","password":"123456","confirm_password":"123456"}'
```

2) 登录拿 token
```bash
curl -X POST 'http://127.0.0.1:8000/login' \
  -H 'Content-Type: application/json' \
  -d '{"username":"alice","password":"123456"}'
```

3) 带 token 创建笔记
```bash
curl -X POST 'http://127.0.0.1:8000/notes' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <JWT_TOKEN>' \
  -d '{"title":"第一篇","content":"内容"}'
```

4) 获取我的信息
```bash
curl -X GET 'http://127.0.0.1:8000/me' \
  -H 'Authorization: Bearer <JWT_TOKEN>'
```

5) 拉取 feed
```bash
curl -X GET 'http://127.0.0.1:8000/feed?type=foryou&limit=10'
```

## 5. 已知实现差异（调用方需注意）

- 不同接口的 `code` 与 `message/error` 字段风格不统一，前端需按接口兼容解析。
- `/feed` 的 `invalid feed type` 错误里 `code` 是字符串 `"4002"`，其余多数是数值。
- `/notes` 创建接口成功 `code` 为 `200`，而部分接口成功 `code` 为 `0`。

后续如需，我可以再给你补一份 OpenAPI 3.0 (`yaml`) 版本，便于导入 Apifox / Swagger UI。