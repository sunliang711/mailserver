package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
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

	log.Infof("host: %v", host)
	log.Infof("port: %v", port)
	log.Infof("user: %v", user)
	log.Infof("authKey: %v", authKey)

}

// 2019/10/12 10:40:30
func main() {
	gin.SetMode(gin.ReleaseMode)
	srv := gin.New()
	srv.POST("/send", sendEmail)

	addr := fmt.Sprintf(":%d", viper.GetInt("server.port"))
	log.Infof("listen on %v", addr)

	if viper.GetBool("tls.enable") {
		err := srv.RunTLS(addr, viper.GetString("tls.cert"), viper.GetString("tls.key"))
		if err != nil {
			logrus.Error(err.Error())
		}
	} else {
		err := srv.Run(addr)
		if err != nil {
			logrus.Error(err.Error())
		}
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
func sendEmail(c *gin.Context) {
	var ec emailContent
	err := c.ShouldBindJSON(&ec)
	if err != nil || ec.To == "" || ec.Subject == "" || ec.Body == "" || ec.AuthKey == "" {
		log.Error("Bad request")
		c.JSON(400, gin.H{
			"code": 1,
			"msg":  `{"to":"receiver","subject":"your subject","body":"your content","auth_key":"someKey"} as request body`,
		})
		return
	}
	if ec.AuthKey != authKey {
		msg := fmt.Sprintf("Invalid auth key")
		log.Error(msg)
		c.JSON(400, gin.H{
			"code": 1,
			"msg":  msg,
		})
		return
	}
	agent, err := emailagent.NewEmailAgent(host, port, user, password)
	if err != nil {
		msg := fmt.Sprintf("New email agent error: %v", err)
		c.JSON(500, gin.H{
			"code": 1,
			"msg":  msg,
		})
		log.Error(msg)
		return
	}
	err = agent.SendEmail(ec.To, ec.Subject, ec.Body)
	defer agent.Close()
	if err != nil {
		c.JSON(500, gin.H{
			"code": 1,
			"msg":  "send email error",
		})
		return
	}
	c.JSON(200, gin.H{
		"code": 0,
		"msg":  "email sent",
	})
	log.Info("email sent.")
}
