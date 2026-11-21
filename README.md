# mailserver

一个基于 Go、Gin 和 SQLite 的轻量级 HTTP 邮件发送网关，内置 **React + Vite** Web 管理控制台（`//go:embed` 单二进制打包）。

## 功能特性

- **内置 Web 管理控制台**：首次启动自动进入初始化向导创建管理员账号，SMTP 配置、多客户端 `auth_key`、发信记录、定时任务、审计日志、环境迁移及全局运行参数均可在 Web UI 中配置并实时生效。
- **多客户端 `auth_key` 管理**：支持为不同业务系统签发独立的 `auth_key`（SHA-256 哈希鉴权 + 主密钥加密存储），支持独立 RPM 限流、过期时间、启停控制、密钥轮换与调用统计。
- **兼容既有发信接口**：`POST /send` 请求/响应契约保持不变，请求体中的 `auth_key` 在服务端校验并自动脱敏记录日志。
- **独立发信记录页面**：记录每次 `POST /send` 与控制台测试发信的收件人、主题、状态、耗时与错误信息，支持多维筛选、详情查看及 CSV/JSON 导出（默认不留存邮件正文，可在系统设置按需开启）。
- **定时任务与 SafeKeep 容灾备份**：内置 `robfig/cron/v3` 调度器（清理审计日志、清理历史发信记录、SMTP 连通性巡检、定时备份环境迁移包至 SafeKeep），支持配置可选迁移加密口令并调用 SafeKeep `POST /api/v1/backups` 上传。
- **审计日志与环境迁移（`.msbundle`）**：完整记录管理端与客户端关键行为；支持导出可移植加密迁移包（Argon2id + AES-256-GCM），在新实例预检冲突并自动用目标主密钥重加密恢复。

## 启动配置（配置文件与环境变量）

监听端口、TLS 证书与数据目录仍通过 `config.yaml` 与环境变量配置，优先级：**命令行参数 > 环境变量 > `config.yaml` > 默认值**。

`config.yaml` 示例：

```yaml
server:
  port: 3000
  data_dir: ./data

tls:
  enable: false
  key: xx.key
  cert: xx.pem
```

常用环境变量：

- `MAILSERVER_CONFIG`：配置文件路径（镜像内默认为 `/app/config.yaml`）
- `MAILSERVER_PORT`：HTTP 服务监听端口（默认 `3000`）
- `MAILSERVER_DATA_DIR`：数据持久化目录（默认 `./data`，镜像内为 `/data`，存放 `mailserver.db` 与 `master.key`）
- `MAILSERVER_MASTER_KEY`：可选 32 字节加密主密钥（64 位十六进制或 Base64；未设置时自动在数据目录生成 `master.key`）
- `MAILSERVER_TLS_ENABLE`：是否启用 HTTPS（`true` / `false`）
- `MAILSERVER_TLS_CERT`：TLS 证书文件路径
- `MAILSERVER_TLS_KEY`：TLS 私钥文件路径
- `MAILSERVER_CORS_ORIGINS`：额外允许的跨域 Origin（逗号分隔）
- `MAILSERVER_PUBLIC_BASE_URL`：对外访问地址默认值

> 注意：`auth_key` 不在 `config.yaml` 中配置，请在启动后通过 Web UI 的「客户端与密钥」页面创建。

## 构建与运行

### 方式一：从源码构建单二进制

```bash
# 构建前端资源与单二进制 ./mailserver
make build

# 启动服务
./mailserver
```

启动后访问 `http://localhost:3000` 完成首次管理员初始化，然后在「系统设置」配置 SMTP 服务器，并在「客户端与密钥」创建客户端生成 `auth_key`。

### 方式二：Docker Compose

```bash
docker compose up --build
```

## 发信接口说明

- 方法：`POST`
- 路径：`/send`
- Content-Type：`application/json`

请求体：

```json
{
  "to": "receiver@example.com",
  "subject": "Test Subject",
  "body": "Hello, this is a test email.",
  "auth_key": "ms_your_client_auth_key"
}
```

成功响应（`200`）：

```json
{
  "code": 0,
  "msg": "email sent"
}
```

## 测试

```bash
make test
```

## License

MIT
