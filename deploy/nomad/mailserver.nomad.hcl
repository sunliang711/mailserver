variable "image" {
  type    = string
  default = "sunliang711/mailserver:v1.1.0"
}

job "mailserver" {
  datacenters = ["dc1"]
  type        = "service"

  group "server" {
    count = 1

    # 多节点 Nomad 集群建议解除下方注释并锁定运行节点，避免重新调度漂移到其他节点找不到本地 SQLite 数据：
    # constraint {
    #   attribute = "${attr.unique.hostname}"
    #   value     = "your-node-hostname"
    # }

    update {
      max_parallel = 1
    }

    network {
      port "http" {
        static = 8100
        to     = 8100
      }
    }

    service {
      name     = "mailserver"
      provider = "nomad"
      port     = "http"

      # 若开启 MAILSERVER_TLS_ENABLE="true"，请为下列 check 补充 protocol = "https"（自签证书可加 tls_skip_verify = true）
      check {
        name     = "liveness"
        type     = "http"
        path     = "/healthz"
        interval = "10s"
        timeout  = "2s"
      }

      check {
        name     = "readiness"
        type     = "http"
        path     = "/readyz"
        interval = "10s"
        timeout  = "3s"
      }
    }

    volume "mailserver" {
      type      = "host"
      source    = "mailserver"
      read_only = false
    }

    task "mailserver" {
      driver = "docker"

      # 环境变量优先级高于容器内置 config.yaml 与数据库默认配置（仅命令行参数优先级更高）。
      env {
        # --- 基础服务与持久化路径 ---
        MAILSERVER_CONFIG   = "/app/config.yaml"
        MAILSERVER_PORT     = "8100"
        MAILSERVER_DATA_DIR = "/data"
        # 对外访问基准地址（用于同源校验；局域网/反代部署请改为实际访问地址）
        MAILSERVER_PUBLIC_BASE_URL = "http://localhost:8100"
        # 可选：固定 32 字节加密主密钥（64 位十六进制或 Base64）；不设置时自动生成并持久化到 /data/master.key
        # MAILSERVER_MASTER_KEY    = ""
        # 可选：子路径反代前缀（如挂载到 https://www.example.com/api/mail 时填写 "/api/mail"）
        # MAILSERVER_BASE_PATH     = "/api/mail"
        # 可选：额外允许跨域的 Origin 白名单（多个用英文逗号分隔）
        # MAILSERVER_CORS_ORIGINS  = "http://192.168.1.10:8100"

        # --- 可选：HTTPS / TLS 配置（与 config.yaml 共同作为启动引导配置）---
        # MAILSERVER_TLS_ENABLE    = "true"
        # MAILSERVER_TLS_CERT      = "/certs/server.pem"
        # MAILSERVER_TLS_KEY       = "/certs/server.key"

        # 注：SMTP 发信账号、多客户端 auth_key、发信记录留存策略、定时任务（含 SafeKeep 定时备份）
        # 与管理员账号均统一持久化在 SQLite 数据库，请在 Web 管理端中配置（修改后即时生效）。
      }

      # host volume: mailserver 需要自己创建，可以使用 nomad-manager host-volume add mailserver --create来创建
      volume_mount {
        volume      = "mailserver"
        destination = "/data"
        read_only   = false
      }

      config {
        image = var.image
        ports = ["http"]

        # 若启用 TLS，可将宿主机证书目录挂载到容器内：
        # volumes = [
        #   "/path/to/certs:/certs:ro",
        # ]
      }

      resources {
        cpu    = 200
        memory = 256
      }
    }
  }
}
