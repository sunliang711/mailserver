package main

import (
	"fmt"

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

// 2019/10/12 10:40:30
func main() {
	gin.SetMode(gin.ReleaseMode)
	srv := gin.New()
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
