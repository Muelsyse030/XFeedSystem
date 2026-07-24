# 前端接口说明

本文档基于当前后端路由与 Handler 实现整理，方便前端对接调用。

## 通用约定

- 返回体大多采用以下格式：
  - `code`: 状态码
  - `message`: 提示信息
  - `data`: 业务数据
- 鉴权方式：
  - 需要登录的接口使用 JWT
  - 请求时需携带登录后的 token
- 分页参数：
  - `cursor`: 游标，默认 `0`
  - `limit`: 每页数量，默认 `10`

---

## 1. 健康检查

### `GET /ping`

用于检查服务是否可用。

**请求参数**

- 无

**响应示例**

```json
{
  "message": "pong"
}
```

---

## 2. 用户相关接口

### `POST /register`

注册用户。

**请求体**

```json
{
  "username": "string",
  "password": "string",
  "confirm_password": "string"
}
```

**响应**

```json
{
  "message": "注册成功"
}
```

---

### `POST /login`

用户登录，返回 token。

**请求体**

```json
{
  "username": "string",
  "password": "string"
}
```

**响应**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "token": "jwt-token"
  }
}
```

---

### `GET /users/:id`

获取指定用户资料。

**路径参数**

- `id`: 用户 ID

**响应字段**

- `id`
- `username`
- `avatar_url`
- `bio`
- `created_at`
- `updated_at`

---

### `GET /me`

获取当前登录用户资料。

**鉴权**

- 需要 JWT

**响应字段**

- `id`
- `username`
- `avatar_url`
- `bio`
- `created_at`
- `updated_at`

---

### `PATCH /me/updata`

更新当前登录用户资料。

**鉴权**

- 需要 JWT

**请求体**

```json
{
  "avatar_url": "string",
  "bio": "string"
}
```

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

---

### `POST /users/:id/follow`

关注用户。

**鉴权**

- 需要 JWT

**请求体**

```json
{
  "user_id": 1,
  "follow_id": 2
}
```

**响应**

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

---

### `DELETE /users/:id/unfollow`

取消关注用户。

**鉴权**

- 需要 JWT

**请求体**

```json
{
  "user_id": 1,
  "follow_id": 2
}
```

**响应**

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

---

### `POST /users/:id/isfollow`

判断是否已关注。

**鉴权**

- 需要 JWT

**请求体**

```json
{
  "user_id": 1,
  "follow_id": 2
}
```

**响应**

```json
{
  "code": 200,
  "message": "ok",
  "follow": true
}
```

---

### `POST /users/:id/block`

拉黑用户。拉黑后双方均不可见对方动态、不可互动互相关注。拉黑时自动双向取消关注。

**鉴权**

- 需要 JWT

**路径参数**

- `id`: 被拉黑用户 ID

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

**错误码**

| code | 含义 |
|------|------|
| 4003 | 用户 ID 无效 |
| 4004 | 已拉黑、用户不存在、不能拉黑自己 |

---

### `DELETE /users/:id/unblock`

取消拉黑用户。

**鉴权**

- 需要 JWT

**路径参数**

- `id`: 被取消拉黑的用户 ID

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

**错误码**

| code | 含义 |
|------|------|
| 4003 | 用户 ID 无效 |
| 4004 | 取消失败 |

---

## 3. 文件上传

### `POST /upload/image`

上传图片。

**请求方式**

- `multipart/form-data`
- 字段名：`file`
- 仅允许 `image/*` 类型

**响应**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "url": "https://...图片地址"
  }
}
```

---

## 4. 笔记相关接口

### `POST /notes`

发布笔记。

**鉴权**

- 需要 JWT

**请求体**

```json
{
  "title": "string",
  "content": "string",
  "type": 1
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| title | string | 标题 |
| content | string | 正文 |
| type | int | 笔记类型，默认 1 |

**响应字段**

- `id`
- `author_id`
- `title`
- `content`
- `type`
- `published_at`
- `created_at`

---

### `GET /notes/:id`

获取笔记详情。

**路径参数**

- `id`: 笔记 ID

**响应字段**

- `id`
- `author_id`
- `title`
- `content`
- `published_at`
- `created_at`
- `like_count`
- `favorite_count`
- `comment_count`

---

### `GET /users/:id/notes`

获取某个用户发布的笔记列表。

**路径参数**

- `id`: 用户 ID

**查询参数**

- `cursor`: 游标，默认 `0`
- `limit`: 数量，默认 `10`

**响应**

```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "list": [],
    "next_cursor": 0
  }
}
```

---

### `PATCH /notes/updata/:id`

更新笔记。

**鉴权**

- 需要 JWT

**路径参数**

- `id`: 笔记 ID

**请求体**

```json
{
  "title": "string",
  "content": "string"
}
```

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

---

### `DELETE /notes/:id`

删除笔记。

**鉴权**

- 需要 JWT

**路径参数**

- `id`: 笔记 ID

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

---

### `POST /notes/:id/like`

点赞笔记。

**鉴权**

- 需要 JWT

**路径参数**

- `id`: 笔记 ID

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

---

### `DELETE /notes/:id/unlike`

取消点赞。

**鉴权**

- 需要 JWT

**路径参数**

- `id`: 笔记 ID

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

---

### `POST /notes/:id/favorite`

收藏笔记。

**鉴权**

- 需要 JWT

**路径参数**

- `id`: 笔记 ID

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

---

### `DELETE /notes/:id/unfavorite`

取消收藏。

**鉴权**

- 需要 JWT

**路径参数**

- `id`: 笔记 ID

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

---

### `GET /me/favorites`

获取当前用户收藏列表。

**鉴权**

- 需要 JWT

**查询参数**

- `cursor`: 游标，默认 `0`
- `limit`: 数量，默认 `10`

**响应**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "list": [],
    "next_cursor": 0
  }
}
```

---

### `POST /notes/:id/comments`

给笔记发表评论或回复评论。

**鉴权**

- 需要 JWT

**路径参数**

- `id`: 笔记 ID

**请求体**

```json
{
  "content": "string",
  "parent_id": 0,
  "reply_to_user_id": 0
}
```

**说明**

- `parent_id = 0` 表示一级评论
- `parent_id != 0` 表示回复某条评论

**响应字段**

- `id`
- `note_id`
- `user_id`
- `parent_id`
- `reply_to_user_id`
- `content`
- `created_at`
- `replies`

---

### `GET /notes/:id/comments`

获取笔记评论列表。

**路径参数**

- `id`: 笔记 ID

**查询参数**

- `cursor`: 游标，默认 `0`
- `limit`: 数量，默认 `10`

**响应**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "list": [],
    "next_cursor": 0
  }
}
```

每条评论包含：

- `id`
- `note_id`
- `user_id`
- `content`
- `created_at`
- `replies`

---

### `DELETE /notes/:id/comments/:comment_id`

删除评论。

**鉴权**

- 需要 JWT

**路径参数**

- `id`: 笔记 ID
- `comment_id`: 评论 ID

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

---

## 5. 动态流接口

### `GET /feed`

获取信息流。

**查询参数**

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| type | string | foryou | `foryou`（推荐）或 `following`（关注流） |
| cursor | string | "" | base64 编码游标 |
| limit | int | 10 | 每页数量，最大 50 |

**鉴权**

- `type=foryou`：可选登录（匿名可访问）
- `type=following`：必须登录

**响应**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "id": 1,
        "author_id": 1,
        "author": {
          "id": 1,
          "username": "user1",
          "avatar_url": "https://..."
        },
        "title": "笔记标题",
        "content": "正文摘要（截断约100字）",
        "type": 1,
        "published_at": "2026-05-26T00:00:00Z"
      }
    ],
    "next_cursor": "base64-encoded-cursor"
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| items[].id | int64 | 笔记 ID |
| items[].author_id | int64 | 作者 ID |
| items[].author.id | int64 | 作者 ID |
| items[].author.username | string | 作者用户名 |
| items[].author.avatar_url | string | 作者头像 |
| items[].title | string | 标题 |
| items[].content | string | 正文摘要 |
| items[].type | int | 笔记类型 |
| items[].published_at | string | 发布时间 |
| next_cursor | string | 下一页游标（空表示无更多） |

> **拉黑过滤**：已登录用户在 `foryou` / `following` 流和搜索结果中都会自动过滤被拉黑用户的笔记。

---

## 6. 通知接口

### 通知类型说明

| type 值 | 含义 |
|---------|------|
| 1 | 点赞 |
| 2 | 评论 |
| 3 | 回复 |
| 4 | 关注 |
| 5 | 收藏 |

### `GET /me/notifications`

获取当前用户的**通知列表**。

**鉴权**

- 需要 JWT

**查询参数**

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| cursor | int | 0 | 游标（通知 ID），翻页时传上一页最后一条的 id |
| limit | int | 10 | 每页数量，最大 50 |

**响应**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "list": [
      {
        "id": 1,
        "type": 1,
        "actor": {
          "id": 7,
          "username": "test_user_b",
          "avatar_url": ""
        },
        "target_id": 100,
        "target_note_id": 100,
        "message": "赞了你的笔记",
        "is_read": false,
        "created_at": "2026-06-17T00:00:00+08:00"
      }
    ],
    "next_cursor": 0
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| list[].id | int64 | 通知 ID |
| list[].type | int | 通知类型（1-5） |
| list[].actor | object | 触发者信息（id + username + avatar_url） |
| list[].target_id | int64 | 关联实体 ID（点赞/评论时为对应 ID，关注时为被关注者 ID） |
| list[].target_note_id | int64 | 关联笔记 ID（用于前端跳转到笔记详情，关注类型为 0） |
| list[].message | string | 预渲染通知文案，可直接展示 |
| list[].is_read | bool | 是否已读 |
| list[].created_at | string | 通知时间 |
| next_cursor | int64 | 下一页游标，0 表示无更多数据 |

> **前端跳转建议**：根据 `type` + `target_note_id` 跳转。如 type=1/2/3/5 时跳转到笔记详情页 `/notes/{target_note_id}`，type=4 时跳转到用户主页 `/users/{target_id}`。

---

### `GET /me/notifications/unread-count`

获取当前用户**未读通知数量**（用于红点/角标展示）。

**鉴权**

- 需要 JWT

**响应**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "count": 5
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| count | int64 | 未读通知数量 |

---

### `PATCH /me/notifications/:id/read`

**标记单条通知为已读**（点击通知后调用）。

**鉴权**

- 需要 JWT

**路径参数**

- `id`: 通知 ID

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

---

### `PATCH /me/notifications/read-all`

**标记全部通知为已读**（点击「全部已读」按钮时调用）。

**鉴权**

- 需要 JWT

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

> 调用后 unread-count 将归零。

---

## 7. 管理员接口

管理员接口需要 JWT + 管理员权限（role >= 1），超级管理员接口需要 role >= 2。

### 角色说明

| role | 称号 | 权限 |
|:----:|------|------|
| 0 | 普通用户 | 无管理权限 |
| 1 | 管理员 | 用户列表、封禁/解封、内容删除、系统统计 |
| 2 | 超级管理员 | 以上全部 + 硬删除用户 |

### `GET /admin/users`

获取用户列表，支持搜索和游标分页。

**鉴权**

- 需要 JWT + AdminAuth（role >= 1）

**查询参数**

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| cursor | int | 0 | 游标（上一页最后一条的 id） |
| limit | int | 20 | 每页数量，最大 50 |
| q | string | "" | 按用户名搜索 |

**响应**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "list": [
      {
        "id": 9,
        "username": "bob",
        "avatar_url": "",
        "role": 0,
        "status": 1,
        "created_at": "2026-06-17 09:54:10"
      }
    ],
    "next_cursor": 5,
    "total": 9
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| list[].role | int | 0=普通, 1=管理员, 2=超级管理员 |
| list[].status | int | 0=封禁, 1=正常 |
| next_cursor | int64 | 下一页游标，0 表示无更多 |
| total | int64 | 符合条件的用户总数 |

---

### `PATCH /admin/users/:id/ban`

封禁/解封用户（toggle 操作，封禁变正常，正常变封禁）。

**鉴权**

- 需要 JWT + AdminAuth（role >= 1）
- 不能操作同级别或更高级管理员

**路径参数**

- `id`: 用户 ID

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

**错误码**

| code | 含义 |
|------|------|
| 4003 | 用户 ID 无效 |
| 4004 | 不能操作更高级管理员 |
| 4030 | 无管理员权限 |

---

### `DELETE /admin/notes/:id`

管理员删除任意笔记（软删除）。

**鉴权**

- 需要 JWT + AdminAuth（role >= 1）

**路径参数**

- `id`: 笔记 ID

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

---

### `DELETE /admin/comments/:id`

管理员删除任意评论（软删除）。

**鉴权**

- 需要 JWT + AdminAuth（role >= 1）

**路径参数**

- `id`: 评论 ID

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

---

### `GET /admin/stats`

系统数据统计。

**鉴权**

- 需要 JWT + AdminAuth（role >= 1）

**响应**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "user_count": 8,
    "note_count": 169,
    "comment_count": 22
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| user_count | int64 | 注册用户总数 |
| note_count | int64 | 已发布笔记数 |
| comment_count | int64 | 评论总数 |

---

### `DELETE /admin/users/:id`

硬删除用户及其所有关联数据（笔记、评论、点赞、收藏、关注、拉黑）。

**鉴权**

- 需要 JWT + SuperAdminAuth（role >= 2）
- 不能删除同级别或更高级管理员

**路径参数**

- `id`: 用户 ID

**响应**

```json
{
  "code": 0,
  "message": "ok"
}
```

**错误码**

| code | 含义 |
|------|------|
| 4003 | 用户 ID 无效 |
| 4004 | 不能操作更高级管理员 |
| 4031 | 无超级管理员权限 |

---

## 8. 前端接入建议

## 7. 前端接入建议

1. 登录成功后保存返回的 `token`。
2. 访问需要登录的接口时，在请求头中携带 JWT。
3. 列表接口统一使用 `cursor + limit` 做分页。
4. 用户资料、笔记详情、评论列表都可直接用于详情页渲染。
5. 评论接口同时支持一级评论和回复评论。

---

## 9. 接口总览

| 方法 | 路径 | 说明 | 是否需要登录 |
|---|---|---|---|
| GET | `/ping` | 健康检查 | 否 |
| POST | `/register` | 用户注册 | 否 |
| POST | `/login` | 用户登录 | 否 |
| POST | `/upload/image` | 上传图片 | 否 |
| GET | `/users/:id` | 获取用户资料 | 否 |
| GET | `/me` | 获取当前用户资料 | 是 |
| PATCH | `/me/updata` | 更新当前用户资料 | 是 |
| POST | `/users/:id/follow` | 关注用户 | 是 |
| DELETE | `/users/:id/unfollow` | 取消关注 | 是 |
| POST | `/users/:id/isfollow` | 是否关注 | 是 |
| POST | `/users/:id/block` | 拉黑用户 | 是 |
| DELETE | `/users/:id/unblock` | 取消拉黑 | 是 |
| POST | `/notes` | 发布笔记 | 是 |
| GET | `/notes/:id` | 笔记详情 | 否 |
| GET | `/users/:id/notes` | 用户笔记列表 | 否 |
| PATCH | `/notes/updata/:id` | 更新笔记 | 是 |
| DELETE | `/notes/:id` | 删除笔记 | 是 |
| POST | `/notes/:id/like` | 点赞 | 是 |
| DELETE | `/notes/:id/unlike` | 取消点赞 | 是 |
| POST | `/notes/:id/favorite` | 收藏 | 是 |
| DELETE | `/notes/:id/unfavorite` | 取消收藏 | 是 |
| GET | `/me/favorites` | 我的收藏列表 | 是 |
| POST | `/notes/:id/comments` | 发表评论/回复 | 是 |
| GET | `/notes/:id/comments` | 评论列表 | 否 |
| DELETE | `/notes/:id/comments/:comment_id` | 删除评论 | 是 |
| GET | `/feed` | 信息流 | 部分 |
| GET | `/me/notifications` | 通知列表 | 是 |
| GET | `/me/notifications/unread-count` | 未读通知数 | 是 |
| PATCH | `/me/notifications/:id/read` | 标记单条已读 | 是 |
| PATCH | `/me/notifications/read-all` | 全部已读 | 是 |
| GET | `/admin/users` | 用户列表（管理员） | 是 |
| PATCH | `/admin/users/:id/ban` | 封禁/解封用户（管理员） | 是 |
| DELETE | `/admin/notes/:id` | 删除任意笔记（管理员） | 是 |
| DELETE | `/admin/comments/:id` | 删除任意评论（管理员） | 是 |
| GET | `/admin/stats` | 系统统计（管理员） | 是 |
| DELETE | `/admin/users/:id` | 硬删除用户（超级管理员） | 是 |

---

## 10. 错误码参考

| code | 含义 |
|------|------|
| 0 | 成功 |
| 4001 | 请求参数解析失败 |
| 4002 | ID 无效 |
| 4003 | 请求参数无效 |
| 4004 | limit 参数无效 |
| 4005 | 无权操作（非作者） |
| 4010 | 未登录 / Token 无效 |
| 4012 | Token 中用户 ID 类型错误 |
| 4030 | 无管理员权限 |
| 4031 | 无超级管理员权限 |
| 4040 | 资源不存在 |
| 5001-5009 | 服务端内部错误 |

---

## 11. 备注

当前代码里部分接口路径包含 `updata`、`isfollow` 这类命名，前端对接时请以实际路由为准；如后续后端统一修正命名，文档也应同步更新。
