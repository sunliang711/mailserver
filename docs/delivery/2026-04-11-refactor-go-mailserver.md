# 重构交付说明

## 重构背景

项目原本全部逻辑集中在根目录 `main.go` 中，包含配置读取、日志中间件、路由注册和邮件发送流程。此次重构目标是引入 `Fx`、`Zerolog` 和清晰分层结构，同时保持配置文件、部署方式和接口输入输出不变。

## 重构方案

### 重构目标

- 用 `Fx` 管理依赖注入和 HTTP Server 生命周期
- 用 `Zerolog` 替换 `Logrus`
- 拆分配置、日志、中间件、Handler、Service、邮件工厂和 Server 组装逻辑
- 在日志中脱敏 `auth_key`

### 重构范围

- 根入口：`main.go`
- 新增目录：`internal/app`、`internal/config`、`internal/logging`、`internal/middleware`、`internal/handler`、`internal/service`、`internal/email`、`internal/server`
- 模块依赖：`go.mod`

### 行为保持策略

- 保持 `config.yaml` 文件名与字段结构不变
- 保持 `POST /send` 路径不变
- 保持请求字段 `to`、`subject`、`body`、`auth_key` 不变
- 保持响应体 `{code,msg}`、HTTP 状态码和消息文本不变
- 保持根目录 `main.go` 作为启动入口，兼容 `go run main.go` 和现有 Docker 构建方式

## 文件变更

- `main.go`：改为仅负责启动 Fx 应用
- `go.mod`：新增 `Fx` 和 `Zerolog` 依赖，移除 `Logrus`
- `internal/config/config.go`：集中读取并反序列化 `config.yaml`
- `internal/logging/logging.go`：初始化 `Zerolog`
- `internal/middleware/logger.go`：实现请求/响应日志并脱敏 `auth_key`
- `internal/email/factory.go`：封装邮件客户端创建
- `internal/service/mail.go`：承载邮件发送逻辑和错误类型
- `internal/handler/mail.go`：保留现有接口契约并转换响应
- `internal/server/server.go`：注册路由并用 `Fx Lifecycle` 管理 HTTP Server
- `internal/handler/mail_test.go`：补充关键响应路径回归测试

## 测试与验证结果

- 计划验证：
  - `go test ./...`
  - `go build ./...`
- 人工验证重点：
  - `config.yaml` 不需要改动
  - `POST /send` 请求与响应不需要调用方修改
  - 缺参、鉴权失败、内部错误返回值保持兼容
  - 日志不再输出明文 `auth_key`

## 风险与后续建议

- 当前仅补充了关键 Handler 回归测试，未覆盖真实 SMTP 集成链路
- 若后续需要继续演进，可在不改接口契约的前提下补充 Service 单测和启动级集成测试
