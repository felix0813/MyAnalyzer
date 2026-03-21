# Backend

该目录现在提供了一个基于 Golang + PostgreSQL 的浏览器历史记录后端服务，支持：

- 浏览记录的新增、查询、更新、删除（CRUD）
- Edge 扩展批量上报浏览记录
- 基于数据库 `VIEW` 查询最近历史
- 自动提取每条记录的根 URL（去掉查询参数与路径）
- 返回适合客户端直接展示的字段，如 `displayTitle`、`displayVisitedDate`、`displayVisitedTime`

## 技术栈
- Go 1.22
- PostgreSQL
- 标准库 `net/http`
- 通过 `psql` 命令行连接 PostgreSQL（无第三方 Go 依赖）

## 启动方式

### 1. 初始化数据库
```bash
psql "$DATABASE_URL" -f backend/init.sql
```

### 2. 启动服务
```bash
cd backend
go run ./cmd/server
```

> 运行服务前请确认环境中已安装 `psql`。

默认配置：
- `LISTEN_ADDR=:8000`
- `DATABASE_URL=postgres://postgres:postgres@127.0.0.1:5432/myanalyzer?sslmode=disable`

## API 说明

### 健康检查
- `GET /healthz`

### 批量导入扩展记录
- `POST /api/history`
- 兼容当前扩展发送的 JSON 格式：

```json
{
  "source": "edge-history-recorder",
  "sentAt": "2026-03-20T12:00:00.000Z",
  "recordCount": 2,
  "records": [
    {
      "url": "https://example.com",
      "title": "Example Domain",
      "visitedAt": "2026-03-20T11:58:00.000Z"
    }
  ]
}
```

### 新增单条记录
- `POST /api/history/records`

```json
{
  "url": "https://example.com/article",
  "title": "示例文章",
  "visitedAt": "2026-03-21T09:30:00Z",
  "notes": "从首页点击进入",
  "visitCount": 1
}
```

### 查询记录列表
- `GET /api/history/records?limit=20&offset=0&search=example`

### 查询单条记录
- `GET /api/history/records/{id}`

### 更新记录
- `PUT /api/history/records/{id}`

### 删除记录
- `DELETE /api/history/records/{id}`

### 查询最近历史（来自 VIEW）
- `GET /api/history/recent?limit=20`

### 查询最近历史的根 URL 聚合
- `GET /api/history/recent/root-urls?limit=20`
- 返回最近历史中按根 URL 聚合后的访问次数、最近访问时间、最新页面标题与最新 URL

示例返回：

```json
{
  "items": [
    {
      "rootUrl": "https://example.com",
      "domain": "example.com",
      "recentVisitCount": 5,
      "lastVisitedAt": "2026-03-21T10:00:00Z",
      "displayLastVisitedAt": "2026-03-21T10:00:00Z",
      "latestTitle": "Example Article",
      "latestUrl": "https://example.com/article?id=1"
    }
  ],
  "total": 1,
  "limit": 20,
  "recentOnly": true
}
```

## 数据库设计

`init.sql` 包含：
- `browser_history` 主表
- 按访问时间、域名的索引
- 基于 `pg_trgm` 的搜索索引
- `v_browser_history_client` 视图：提供客户端直接可用的展示字段，并暴露 `root_url`
- `v_browser_history_recent` 视图：提供最近 100 条历史记录的快速读取
- `v_browser_history_recent_roots` 视图：按根 URL 聚合最近历史，便于客户端展示站点级访问趋势
