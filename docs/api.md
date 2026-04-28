# XFeedSystem API 文档

本文档基于当前代码实现整理，覆盖接口说明、调用方式、请求参数、响应格式、鉴权、分页游标和完整 cURL 示例。

机器可读版本（可导入 Apifox / Swagger UI）：[`docs/openapi.yaml`](openapi.yaml)

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

#### Feed `/feed`（`type=foryou` 与 `type=following` 共用）
- 使用字符串游标 `cursor`
- 编码格式：`<published_at_unix>_<note_id>`，例如 `1710000000_123`
- 查询逻辑：
  - `published_at < cursor.published_at`
  - 或 `published_at = cursor.published_at 且 id < cursor.id`
- 排序：`published_at DESC, id DESC`
- `limit` 允许范围 `1~50`，否则后端强制为 `10`
- `type=following` 时必须携带有效 JWT；`type=foryou` 可不鉴权（可选 Token 仅用于中间件写入用户信息）

#### 我的收藏 `/me/favorites`
- 使用整型游标 `cursor`（基于 `note_favorites.id`）
- 查询逻辑：`id < cursor`
- 排序：`id DESC`
- `limit` 允许范围 `1~50`，否则后端强制为 `10`
- 首次请求建议 `cursor=0`；`next_cursor` 为本页最后一条收藏关系记录的 `id`，无更多数据时为 `0`

#### 评论列表 `/notes/:id/comments`
- 使用整型游标 `cursor`（本质是 `note_comments.id`）
- 查询逻辑：`id < cursor`
- 排序：`id DESC`
- `limit` 允许范围 `1~50`，否则后端强制为 `10`
- 首次请求建议 `cursor=0`；`next_cursor` 为本页最后一条评论记录的 `id`，无更多数据时为 `0`

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
    "type": 1,
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

## 3.9 点赞笔记

### `POST /notes/:id/like`

说明：为指定笔记点赞；需登录。重复点赞不会报错，也不会重复增加 `like_count`（数据库唯一约束 + `ON DUPLICATE` 忽略写入）。

鉴权：需要 `Bearer Token`

路径参数：`id` 为笔记 ID。

成功响应：`200`
```json
{
  "code": 0,
  "message": "ok"
}
```

失败示例：
- `400`（非法笔记 id）
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
- `400`（用户 id 非法，一般不应出现）
```json
{
  "code": 4003,
  "message": "invalid user id"
}
```
- `404`
```json
{
  "code": 4040,
  "message": "note not found"
}
```
- `500`
```json
{
  "code": 5005,
  "message": "like note failed"
}
```

cURL：
```bash
curl -X POST 'http://127.0.0.1:8000/notes/100/like' \
  -H 'Authorization: Bearer <JWT_TOKEN>'
```

---

## 3.10 取消点赞

### `DELETE /notes/:id/unlike`

说明：取消对指定笔记的点赞。未点赞过再次取消仍为成功（不扣减已为 0 的计数）。

鉴权：需要 `Bearer Token`

成功响应：`200`
```json
{
  "code": 0,
  "message": "ok"
}
```

失败示例：路径非法、未登录、用户 id 非法、笔记不存在、服务错误分别对应 `4002` / `4010` / `4003` / `4040`；服务异常为 `5006`：`unlike note failed`。

cURL：
```bash
curl -X DELETE 'http://127.0.0.1:8000/notes/100/unlike' \
  -H 'Authorization: Bearer <JWT_TOKEN>'
```

---

## 3.11 收藏笔记

### `POST /notes/:id/favorite`

说明：收藏笔记；需登录。重复收藏不重复增加 `favorite_count`。

鉴权：需要 `Bearer Token`

成功响应：`200`：`{"code":0,"message":"ok"}`

失败示例：与点赞类似，`500` 时为 `5007`：`favorite note failed`。

cURL：
```bash
curl -X POST 'http://127.0.0.1:8000/notes/100/favorite' \
  -H 'Authorization: Bearer <JWT_TOKEN>'
```

---

## 3.12 取消收藏

### `DELETE /notes/:id/unfavorite`

说明：取消收藏；未收藏过再次取消仍为成功。

鉴权：需要 `Bearer Token`

成功响应：`200`：`{"code":0,"message":"ok"}`

失败示例：服务异常为 `5008`：`unfavorite note failed`。

cURL：
```bash
curl -X DELETE 'http://127.0.0.1:8000/notes/100/unfavorite' \
  -H 'Authorization: Bearer <JWT_TOKEN>'
```

---

## 3.13 我的收藏列表

### `GET /me/favorites`

说明：分页返回当前用户已收藏的笔记（仅已发布笔记会出现在列表中；若笔记已删除则该条收藏在结果中可能被跳过）。

鉴权：需要 `Bearer Token`

查询参数：

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|---|---|---|---|---|
| cursor | int64 | 否 | 0 | 见上文 2.3「我的收藏」 |
| limit | int | 否 | 10 | 每页条数，建议 1~50 |

成功响应：`200`
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "list": [
      {
        "id": 100,
        "author_id": 1,
        "title": "标题",
        "content": "正文",
        "published_at": "2026-03-20T10:00:00Z",
        "created_at": "2026-03-20T10:00:00Z"
      }
    ],
    "next_cursor": 55
  }
}
```

失败示例：`401`（`4010` / `unauthorized`）、`400`（`4003` / `invalid user id`）、`500`（`5009` / `list favorites failed`）。

cURL：
```bash
curl -X GET 'http://127.0.0.1:8000/me/favorites?cursor=0&limit=10' \
  -H 'Authorization: Bearer <JWT_TOKEN>'
```

---

## 3.14 评论

### `POST /notes/:id/comments`

说明：对指定笔记发表评论或回复评论；需登录。请求体仅支持 JSON（`Content-Type: application/json`）。

鉴权：需要 `Bearer Token`

路径参数：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| id | int64 | 是 | 笔记 ID |

请求体：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| content | string | 是 | 评论内容（去除首尾空白后不能为空） |
| parent_id | int64 | 否 | 父评论 ID；为空或 0 表示一级评论 |
| reply_to_user_id | int64 | 否 | 回复对象用户 ID；通常由后端自动补齐 |

请求示例：
```json
{
  "content": "这是一条评论"
}
```

成功响应：`200`
```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "id": 2,
    "note_id": 3,
    "user_id": 3,
    "parent_id": 0,
    "reply_to_user_id": 0,
    "content": "这是一条评论",
    "created_at": "2026-04-27T08:39:38+08:00",
    "replies": []
  }
}
```

失败示例：
- `400`（非法笔记 id）
```json
{
  "code": 4002,
  "message": "invalid note id"
}
```
- `400`（请求体非法或 content 为空）
```json
{
  "code": 4003,
  "message": "content required"
}
```
- `401`
```json
{
  "code": 4010,
  "message": "unauthorized"
}
```
- `500`
```json
{
  "code": 5001,
  "message": "create comment failed"
}
```

cURL：
```bash
curl -X POST 'http://127.0.0.1:8000/notes/3/comments' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <JWT_TOKEN>' \
  -d '{"content":"这是一条评论"}'
```

---

### `GET /notes/:id/comments`

说明：获取指定笔记的评论列表（倒序分页）。一级评论会内嵌 `replies` 子回复数组，便于前端直接渲染楼中楼。

鉴权：需要 `Bearer Token`

路径参数：`id` 为笔记 ID。

查询参数：

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|---|---|---|---|---|
| cursor | int64 | 否 | 0 | 游标，返回 `id < cursor` 的记录 |
| limit | int | 否 | 10 | 每页条数，建议 1~50 |

成功响应：`200`
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "list": [
      {
        "id": 2,
        "note_id": 3,
        "user_id": 3,
        "parent_id": 0,
        "reply_to_user_id": 0,
        "content": "这是一条评论",
        "created_at": "2026-04-27T08:39:38+08:00",
        "replies": [
          {
            "id": 3,
            "note_id": 3,
            "user_id": 4,
            "parent_id": 2,
            "reply_to_user_id": 3,
            "content": "这是回复",
            "created_at": "2026-04-27T08:40:10+08:00"
          }
        ]
      }
    ],
    "next_cursor": 2
  }
}
```

失败示例：
- `400`（非法参数）
```json
{
  "code": 4004,
  "message": "invalid limit"
}
```
- `401`
```json
{
  "code": 4010,
  "message": "unauthorized"
}
```
- `500`
```json
{
  "code": 5002,
  "message": "list comments failed"
}
```

cURL：
```bash
curl -X GET 'http://127.0.0.1:8000/notes/3/comments?cursor=0&limit=10' \
  -H 'Authorization: Bearer <JWT_TOKEN>'
```

---

### `DELETE /notes/:id/comments/:comment_id`

说明：删除自己发布的评论（逻辑删除）。需登录。

鉴权：需要 `Bearer Token`

路径参数：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| id | int64 | 是 | 笔记 ID（当前实现不使用该参数，仅用于路由匹配） |
| comment_id | int64 | 是 | 评论 ID |

成功响应：`200`
```json
{
  "code": 0,
  "message": "ok"
}
```

失败示例：
- `400`（非法评论 id）
```json
{
  "code": 4002,
  "message": "invalid comment id"
}
```
- `400`（业务错误，例如评论不存在/非本人）
```json
{
  "code": 4005,
  "message": "<error>"
}
```
- `401`
```json
{
  "code": 4010,
  "message": "unauthorized"
}
```

cURL：
```bash
curl -X DELETE 'http://127.0.0.1:8000/notes/3/comments/2' \
  -H 'Authorization: Bearer <JWT_TOKEN>'
```

---

## 3.15 Feed 流

### `GET /feed`

说明：获取 Feed；`type=foryou` 为推荐流，`type=following` 为「我关注的人」的动态。两种类型使用相同的字符串游标规则（见上文 2.3）。

鉴权：
- `type=foryou`：可不传 Token；若传合法 `Authorization`，中间件会解析并写入 `userID`（便于扩展）。
- `type=following`：必须 `Bearer Token`。

查询参数：

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|---|---|---|---|---|
| type | string | 否 | foryou | `foryou` \| `following` |
| cursor | string | 否 | 空 | 游标格式 `<unix_ts>_<id>` |
| limit | int | 否 | 10 | 每页条数；非法时回退为 `10`；`<=0` 或 `>50` 时强制为 `10` |

请求示例：
- 推荐首次：`/feed?type=foryou&limit=10`
- 推荐下一页：`/feed?type=foryou&cursor=1710000000_123&limit=10`
- 关注流：`/feed?type=following&limit=10`（需 Token）

成功响应：`200`（`message` 为小写 `ok`）
```json
{
  "code": 0,
  "message": "ok",
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

说明：`foryou` 正文中摘取约 100 字，`following` 约 120 字（服务层 `BuildSummary`）。

失败示例：
- `400`（不支持的 type）
```json
{
  "code": 4002,
  "message": "unsupported feed type"
}
```
- `400`（limit 非数字）
```json
{
  "code": 4001,
  "message": "invalid limit"
}
```
- `401`（`following` 且未登录或无效用户）
```json
{
  "code": 4010,
  "message": "unauthorized"
}
```
- `500`（游标解析或服务错误，message 为具体错误文案）
```json
{
  "code": 5001,
  "message": "<错误详情>"
}
```

cURL：
```bash
curl -X GET 'http://127.0.0.1:8000/feed?type=foryou&limit=10'
```

```bash
curl -X GET 'http://127.0.0.1:8000/feed?type=following&limit=10' \
  -H 'Authorization: Bearer <JWT_TOKEN>'
```

---

## 3.16 关注用户

### `POST /users/:id/follow`

说明：`user_id` 关注 `follow_id`。当前处理器**未读取路径参数 `:id`**，以 JSON 体为准（路径中可写占位，例如与 `follow_id` 相同以便路由匹配）。

鉴权：需要 `Bearer Token`

请求体：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| user_id | int64 | 是 | 发起关注的用户 ID |
| follow_id | int64 | 是 | 被关注用户 ID |

请求示例：
```json
{
  "user_id": 1,
  "follow_id": 2
}
```

成功响应：`200`
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "user_id": 1,
    "follow_id": 2
  }
}
```

失败示例：`400`
```json
{
  "error": "<业务或绑定错误>"
}
```

cURL：
```bash
curl -X POST 'http://127.0.0.1:8000/users/2/follow' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <JWT_TOKEN>' \
  -d '{"user_id":1,"follow_id":2}'
```

---

## 3.17 取消关注

### `DELETE /users/:id/unfollow`

说明：取消关注；路径参数 `:id` 同样**未参与业务逻辑**，以 JSON 体为准。

鉴权：需要 `Bearer Token`

请求体：与关注相同（`user_id`、`follow_id`）

成功响应：`200`
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "user_id": 1,
    "unfollow_id": 2
  }
}
```

失败示例：`400`
```json
{
  "error": "<业务或绑定错误>"
}
```

cURL：
```bash
curl -X DELETE 'http://127.0.0.1:8000/users/2/unfollow' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <JWT_TOKEN>' \
  -d '{"user_id":1,"follow_id":2}'
```

---

## 3.18 是否已关注

### `POST /users/:id/isfollow`

说明：查询 `user_id` 是否已关注 `follow_id`；路径 `:id` **未参与业务逻辑**。

鉴权：需要 `Bearer Token`

请求体：与关注相同

成功响应：`200`
```json
{
  "code": 200,
  "message": "ok",
  "follow": true
}
```

失败示例：`500`
```json
{
  "code": 0,
  "message": "error"
}
```

cURL：
```bash
curl -X POST 'http://127.0.0.1:8000/users/2/isfollow' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <JWT_TOKEN>' \
  -d '{"user_id":1,"follow_id":2}'
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

6) 点赞 / 收藏 / 我的收藏（需替换笔记 ID）
```bash
curl -X POST 'http://127.0.0.1:8000/notes/100/like' \
  -H 'Authorization: Bearer <JWT_TOKEN>'
```

7) 评论 / 拉评论 / 删评论（需替换笔记 ID / 评论 ID）
```bash
curl -X POST 'http://127.0.0.1:8000/notes/100/comments' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <JWT_TOKEN>' \
  -d '{"content":"第一条评论"}'
```

```bash
curl -X GET 'http://127.0.0.1:8000/notes/100/comments?cursor=0&limit=10' \
  -H 'Authorization: Bearer <JWT_TOKEN>'
```

```bash
curl -X DELETE 'http://127.0.0.1:8000/notes/100/comments/2' \
  -H 'Authorization: Bearer <JWT_TOKEN>'
```

8) 关注 / 取关 / 是否关注（需替换 ID）
```bash
curl -X POST 'http://127.0.0.1:8000/users/2/follow' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <JWT_TOKEN>' \
  -d '{"user_id":1,"follow_id":2}'
```

## 5. 已知实现差异（调用方需注意）

- 不同接口的 `code` 与 `message/error` 字段风格不统一，前端需按接口兼容解析。
- `/notes` 创建接口成功 `code` 为 `200`，而部分接口成功 `code` 为 `0`；`/users/:id/isfollow` 成功时 `code` 为 `200`。
- `POST/DELETE /users/:id/follow|unfollow|isfollow` 路径中的 `:id` 当前未被 handler 使用，与 body 可不一致；联调时建议三者一致以免混淆。
- `GET /feed` 在 `type=foryou` 下可不鉴权；`type=following` 必须带 Token。
- 点赞、收藏相关表与 `notes.like_count` / `favorite_count` 等字段见 `migrations/001_interactions.sql`（部署后接口方可完整工作）。

OpenAPI 3.0 定义见同目录 [`openapi.yaml`](openapi.yaml)。