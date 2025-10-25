package main

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/sunliang711/emailagent"
)

var (
	host     string
	port     int
	user     string
	password string

	// agent *emailagent.EmailAgent

	authKey string
)

func init() {
	var err error
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	err = viper.ReadInConfig()
	if err != nil {
		panic(fmt.Sprintf("read config error: %v", err))
	}

	host = viper.GetString("email.host")
	port = viper.GetInt("email.port")
	user = viper.GetString("email.user")
	password = viper.GetString("email.password")

	authKey = viper.GetString("auth.key")

	logrus.Infof("host: %v", host)
	logrus.Infof("port: %v", port)
	logrus.Infof("user: %v", user)
	logrus.Infof("authKey: %v", authKey)

}

// 日志中间件
func loggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 读取请求体
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
		}
		// 恢复请求体，以便后续处理
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// 打印请求信息
		logrus.WithFields(logrus.Fields{
			"method":       c.Request.Method,
			"path":         c.Request.URL.Path,
			"query":        c.Request.URL.RawQuery,
			"request_body": string(bodyBytes),
			"client_ip":    c.ClientIP(),
			"user_agent":   c.Request.UserAgent(),
		}).Info("Request received")

		// 创建一个响应写入器来捕获响应
		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw

		// 处理请求
		c.Next()

		// 计算处理时间
		latency := time.Since(start)

		// 打印响应信息
		logrus.WithFields(logrus.Fields{
			"status":        c.Writer.Status(),
			"response_body": blw.body.String(),
			"latency":       latency,
		}).Info("Response sent")
	}
}

// 自定义响应写入器
type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// 2019/10/12 10:40:30
func main() {
	gin.SetMode(gin.ReleaseMode)
	srv := gin.New()

	// 添加日志中间件
	srv.Use(loggerMiddleware())

	srv.POST("/send", sendEmail)

	addr := fmt.Sprintf(":%d", viper.GetInt("server.port"))
	logrus.Infof("listen on %v", addr)

	var err error

	if viper.GetBool("tls.enable") {
		err = srv.RunTLS(addr, viper.GetString("tls.cert"), viper.GetString("tls.key"))
	} else {
		err = srv.Run(addr)
	}
	if err != nil {
		logrus.Error(err.Error())
	}
}

// emailContent TODO
// 2019/10/12 10:46:09
type emailContent struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	AuthKey string `json:"auth_key"`
}

// sendEmail TODO
// 2019/10/12 10:44:42
func sendEmail(ctx *gin.Context) {
	var req emailContent
	err := ctx.ShouldBindJSON(&req)
	if err != nil || req.To == "" || req.Subject == "" || req.Body == "" || req.AuthKey == "" {
		logrus.Error("Bad request")
		ctx.JSON(400, gin.H{
			"code": 1,
			"msg":  `{"to":"receiver","subject":"your subject","body":"your content","auth_key":"someKey"} as request body`,
		})
		return
	}
	if req.AuthKey != authKey {
		msg := "Invalid auth key"
		logrus.Error(msg)
		ctx.JSON(400, gin.H{
			"code": 1,
			"msg":  msg,
		})
		return
	}
	agent, err := emailagent.NewEmailAgent(host, port, user, password)
	if err != nil {
		msg := fmt.Sprintf("New email agent error: %v", err)
		ctx.JSON(500, gin.H{
			"code": 1,
			"msg":  msg,
		})
		logrus.Error(msg)
		return
	}
	err = agent.SendEmail(req.To, req.Subject, req.Body)
	defer agent.Close()
	if err != nil {
		ctx.JSON(500, gin.H{
			"code": 1,
			"msg":  "send email error",
		})
		return
	}
	ctx.JSON(200, gin.H{
		"code": 0,
		"msg":  "email sent",
	})
	logrus.Info("email sent.")
}
