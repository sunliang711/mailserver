# mailserver

一个基于 Go 和 Gin 的简单邮件发送服务，提供 HTTP 接口用于发送邮件。

## 功能特性

- 提供 HTTP API 发送邮件
- 基于 SMTP 服务发信
- 支持通过配置文件管理邮件账号和服务端口
- 支持 TLS 启动
- 记录请求和响应日志，便于排查问题

## 技术栈

- Go
- Gin
- Viper
- Logrus

## 项目结构

```text
.
├── main.go              # 服务入口与接口实现
├── config.yaml          # 服务配置
├── Dockerfile           # Docker 构建文件
├── docker-compose.yaml  # Docker Compose 配置
├── go.mod
└── go.sum
```

## 配置说明

服务启动时会读取当前目录下的 `config.yaml`。

示例配置：

```yaml
email:
  host: smtp.host.com
  port: 465
  user: user@host.com
  password: your_password

server:
  port: 3000

tls:
  enable: false
  key: xx.key
  cert: xx.pem

auth:
  key: secretKeyxxx
```

字段说明：

- `email.host`: SMTP 服务器地址
- `email.port`: SMTP 端口
- `email.user`: SMTP 登录账号
- `email.password`: SMTP 登录密码
- `server.port`: HTTP 服务监听端口
- `tls.enable`: 是否启用 HTTPS
- `tls.key`: TLS 私钥文件路径
- `tls.cert`: TLS 证书文件路径
- `auth.key`: 调用接口时需要提供的认证密钥

## 本地运行

### 1. 安装依赖

```bash
go mod download
```

### 2. 修改配置

根据你的 SMTP 服务商信息修改 `config.yaml`：

- 邮箱服务器地址
- 端口
- 用户名
- 密码
- 自定义接口认证密钥

### 3. 启动服务

```bash
go run main.go
```

启动后默认监听：

```text
http://localhost:3000
```

如果启用了 TLS，则使用：

```text
https://localhost:3000
```

## Docker 运行

### 构建镜像

```bash
docker build -t mailserver .
```

### 启动容器

```bash
docker run --rm -p 3000:3000 -v $(pwd)/config.yaml:/usr/local/bin/config.yaml mailserver
```

### 使用 Docker Compose

```bash
docker compose up --build
```

## 接口说明

### 发送邮件

- 方法：`POST`
- 路径：`/send`
- Content-Type：`application/json`

#### 请求体

```json
{
  "to": "receiver@example.com",
  "subject": "Test Subject",
  "body": "Hello, this is a test email.",
  "auth_key": "secretKeyxxx"
}
```

字段说明：

- `to`: 收件人邮箱
- `subject`: 邮件主题
- `body`: 邮件正文
- `auth_key`: 接口认证密钥，需要与 `config.yaml` 中的 `auth.key` 一致

#### 成功响应

```json
{
  "code": 0,
  "msg": "email sent"
}
```

#### 失败响应

参数错误或缺少字段时：

```json
{
  "code": 1,
  "msg": "{\"to\":\"receiver\",\"subject\":\"your subject\",\"body\":\"your content\",\"auth_key\":\"someKey\"} as request body"
}
```

认证失败时：

```json
{
  "code": 1,
  "msg": "Invalid auth key"
}
```

发送失败或服务内部错误时：

```json
{
  "code": 1,
  "msg": "send email error"
}
```

或：

```json
{
  "code": 1,
  "msg": "New email agent error: xxx"
}
```

## 调用示例

### curl 示例

```bash
curl -X POST http://localhost:3000/send \
  -H "Content-Type: application/json" \
  -d '{
    "to": "receiver@example.com",
    "subject": "Test Mail",
    "body": "Hello from mailserver",
    "auth_key": "secretKeyxxx"
  }'
```

### HTTPS 示例

如果启用了 TLS：

```bash
curl -k -X POST https://localhost:3000/send \
  -H "Content-Type: application/json" \
  -d '{
    "to": "receiver@example.com",
    "subject": "Secure Mail",
    "body": "Hello over TLS",
    "auth_key": "secretKeyxxx"
  }'
```

## 日志说明

服务会记录以下内容：

- 请求方法
- 请求路径
- Query 参数
- 请求体
- 客户端 IP
- User-Agent
- 响应状态码
- 响应内容
- 请求耗时

便于排查接口调用和发信问题。

## 注意事项

- `auth_key` 放在请求体中，不是放在请求头中
- 当前仅提供一个发信接口：`POST /send`
- 邮件内容 `body` 为普通字符串内容
- 生产环境建议妥善保管 `config.yaml` 中的邮箱密码和认证密钥
- 建议为 `auth.key` 配置一个足够复杂的随机字符串

## License

MIT
