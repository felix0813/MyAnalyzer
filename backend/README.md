# Backend

当前仓库中仍未实现具体后端服务，但 Edge 扩展现在已经支持把本地浏览记录直接 POST 到你指定的本地 Agent 或后端接口。

## 推荐接口约定
- 方法：`POST`
- 路径示例：`/api/history`
- `Content-Type`：`application/json`
- 成功条件：返回任意 `2xx` HTTP 状态码

只要接口返回 `2xx`，扩展就会视为发送成功，并删除扩展本地缓存的数据；否则会保留本地数据，方便再次发送。
