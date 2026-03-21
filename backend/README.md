# Backend

该目录现在提供了一个基于 Golang + PostgreSQL 的浏览器历史记录后端服务，支持：

- 浏览记录的新增、查询、更新、删除（CRUD）
- Edge 扩展批量上报浏览记录
- 基于数据库 `VIEW` 查询最近历史
- 返回适合客户端直接展示的字段，如 `displayTitle`、`displayVisitedDate`、`displayVisitedTime`
- 按最近几天历史记录的根 URL（去掉路径、查询参数、片段）进行聚合分析

## 技术栈

- Go 1.22
- PostgreSQL
- 标准库 `net/http`
- 通过纯 Go 实现的 PostgreSQL 协议驱动连接数据库

## 启动方式

### 1. 初始化数据库

```bash
cd backend
go run ./cmd/initdb -file init.sql
```

### 2. 启动服务

```bash
cd backend
go run ./cmd/server
```

> 所有数据库查询均通过纯 Go 实现的 PostgreSQL 协议驱动执行，无需依赖 `libpq` 或 `psql` 命令行。

默认配置：

- `LISTEN_ADDR=:8000`
- `DATABASE_URL=postgres://postgres:postgres@127.0.0.1:5432/myanalyzer?sslmode=disable`


## GitHub Release 打包

仓库包含 GitHub Actions 工作流 `.github/workflows/release-backend.yml`，用于自动打包后端服务：

- 触发方式：发布 GitHub Release（`release.published`）
- 构建目标：`linux/amd64` Ubuntu 可执行文件
- 产物内容：
  - `myanalyzer-backend` 可执行文件
  - `README.md`
  - `init.sql`
- 上传内容：
  - `myanalyzer-backend-linux-amd64.tar.gz`
  - `myanalyzer-backend-linux-amd64.tar.gz.sha256`
- Release 上传方式：在 `release.published` 事件里直接执行 `gh release upload <tag> --clobber`，将产物上传到当前 Release，避免第三方 Action 在已有 Release 上重复创建/更新时的兼容性问题。

也支持手动触发 `workflow_dispatch`，手动触发时会将相同产物上传为 GitHub Actions Artifact，便于预先验证打包流程。

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

### 查询最近几天的根 URL 聚合

- `GET /api/history/root-urls?days=3&limit=20`
- `days` 默认 3，范围 1~30；`limit` 默认 20，范围 1~100
- 返回字段包括：`rootURL`、`recordCount`、`visitCountTotal`、`lastVisitedAt`、`latestTitle`、`latestURL`

## 数据库设计

`init.sql` 包含：

- `browser_history` 主表（包含 `root_url` 聚合字段）
- 按访问时间、域名、根 URL 的索引
- 基于 `pg_trgm` 的搜索索引
- `v_browser_history_client` 视图：提供客户端直接可用的展示字段
- `v_browser_history_recent` 视图：提供最近 100 条历史记录的快速读取
