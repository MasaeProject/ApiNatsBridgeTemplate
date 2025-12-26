# ApiNatsBridgeTemplate

[ApiNatsBridge](https://github.com/MasaeProject/ApiNatsBridge) 的微服务模板项目。通过 NATS 接收由 ApiNatsBridge 转发的 HTTP 请求，处理后返回响应。

本示例实现了一个 `/ping` 端点：接收可选的毫秒时间戳参数，返回时间差值与客户端 IP。

## 流程

```
客户端                    ApiNatsBridge                 本程序
  |                           |                          |
  |  GET /ping?timestamp=xxx  |                          |
  |-------------------------->|                          |
  |                           |  NATS (ping_req)         |
  |                           |------------------------->|
  |                           |                          | 计算时间差
  |                           |  NATS (响应)             |
  |                           |<-------------------------|
  |  JSON 响应                |                          |
  |<--------------------------|                          |
```

## 请求

```
GET /ping?timestamp=1716000000000
```

- `timestamp`（可选）：客户端发送时的毫秒级 Unix 时间戳。

## 响应

```json
{ "pong": 42, "ip": "127.0.0.1" }
```

| 字段   | 类型   | 说明                                                   |
| ------ | ------ | ------------------------------------------------------ |
| `pong` | int64  | 若提供 timestamp，为服务器当前时间与该值的差值（毫秒） |
| `ip`   | string | 客户端 IP 地址                                         |

## 构建

```bash
go build .
```

## 运行

```bash
./ApiNatsBridgeTemplate -c config.yaml
```

### 命令行参数

| 参数 | 说明                                                   |
| ---- | ------------------------------------------------------ |
| `-c` | 指定 YAML 配置文件路径（默认为可执行文件同名 `.yaml`） |
| `-o` | 指定日志输出文件路径（同时写入终端与文件）             |

## NATS 消息格式

### 请求（bridgeRequest）

ApiNatsBridge 转发 HTTP 请求时，NATS 消息的 JSON 结构如下：

```json
{
  "method": "GET",
  "path": "/ping",
  "headers": { "Accept": "application/json" },
  "cookies": {},
  "remote_addr": "127.0.0.1:12345",
  "ip": "127.0.0.1",
  "params": { "timestamp": "1716000000000" },
  "body": ""
}
```

| 字段          | 类型              | 说明                          |
| ------------- | ----------------- | ----------------------------- |
| `method`      | string            | HTTP 请求方法（GET、POST 等） |
| `path`        | string            | 请求路径                      |
| `headers`     | map[string]string | 请求标头                      |
| `cookies`     | map[string]string | 请求 Cookie                   |
| `remote_addr` | string            | 原始远端地址（含端口）        |
| `ip`          | string            | 客户端 IP 地址                |
| `params`      | map[string]string | 查询参数                      |
| `body`        | string            | 请求正文                      |

### 响应（bridgeResponse）

微服务处理完成后，需返回以下 JSON 结构：

```json
{
  "status_code": 200,
  "headers": { "Content-Type": "application/json; charset=utf-8" },
  "body": "{\"pong\":42,\"ip\":\"127.0.0.1\"}"
}
```

| 字段          | 类型              | 说明                    |
| ------------- | ----------------- | ----------------------- |
| `status_code` | int               | HTTP 响应状态码         |
| `headers`     | map[string]string | 响应标头                |
| `body`        | string            | 响应正文（JSON 字符串） |

## 配置文件

```yaml
nats_config:
  nats_server_host: 127.0.0.1
  nats_server_port: 4222
  nats_user: webapi
  nats_password: Ohr1biolei4aeD3eu7isaeyie9di4uuf
  nats_client_name: PingService
  nats_max_reconnects: 5
  nats_reconnect_wait: 2
  nats_connect_timeout: 10
  nats_encryption_key: "GLOBAL_BACKUP_KEY_32_CHARS_LONG!"

nats_subject: "ping_req"
```

## 测试

需要同时运行 NATS Server 和 ApiNatsBridge：

```powershell
# 1. 启动 NATS Server
Start-Process -FilePath ".\nats-server.exe" -WorkingDirectory "..\test\nats-server" -ArgumentList "-c nats-server.conf"

# 2. 启动本微服务
.\ApiNatsBridgeTemplate.exe -c config.yaml

# 3. 启动 ApiNatsBridge
..\ApiNatsBridge.exe -c ..\test\ApiNatsBridgeConfig.yaml

# 4. 发送请求（PowerShell）
Invoke-RestMethod "http://127.0.0.1:9080/ping?timestamp=$([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds())"
```

```bash
# 4. 发送请求（Bash）
curl "http://127.0.0.1:9080/ping?timestamp=$(date +%s%3N)"
```

## 项目结构

| 文件          | 用途                                      |
| ------------- | ----------------------------------------- |
| `main.go`     | 入口：配置加载、NATS 连接、订阅、优雅关闭 |
| `handler.go`  | ping 处理逻辑：时间戳差值计算与 IP 提取   |
| `config.yaml` | NATS 连接与订阅主题配置                   |

## 许可证

请修改为你自己的许可证。
