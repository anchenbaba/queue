# randomstrings
一个任务队列 in Golang.

#Installation
`go get -u github.com/anchenbaba/queue`

# Sample code
```go
package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/anchenbaba/queue/workers"
)

var wp = workers.NewWorkerPool()

func main() {

	// example 例子
	wp.Register("phpcmd", phpcmd)
	wp.Register("email", email)
	wp.Register("sendMsg", sendMsg)

	go func() {
		// 开启一个http服务 通过get或者post 接收新的作业
		http.HandleFunc("/api/v1", apiV1)
		http.HandleFunc("/admin/log", adminLog)
		err := http.ListenAndServe(":8888", nil) //设置监听的端口
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
		// now := time.Now().Unix()
		// for index := 0; index < 10; index++ {
		// 	wp.AddJob("sendMsg", "2017-08-19 21:50:01","", index)
		// }
		// time.Sleep(time.Second * 10)
		// wp.AddJob("email", 0,"", "other job")
	}()
	wp.Start()
}

func adminLog(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "当前任务个数: %v \r\n", wp.JobServ.JobQueueLen())
	logs := wp.GetLog()
	for _, l := range logs {
		fmt.Fprintf(w, l)
	}
}

func apiV1(w http.ResponseWriter, r *http.Request) {
	// 解析参数, 默认是不会解析的
	_ = r.ParseForm()
	if makeCros(w, r) {
		return
	}
	if len(r.Form["token"]) == 0 || r.Form["token"][0] != "123456" {
		return
	}

	// log.Println(r.PostForm)
	// log.Println(r.PostFormValue("token"))

	delete(r.Form, "token")
	if len(r.Form["name"]) > 0 {
		name := r.Form["name"][0]
		delete(r.Form, "name")
		var start int64
		var err error
		if len(r.Form["start"]) > 0 {
			if start, err = strconv.ParseInt(r.Form["start"][0], 10, 64); err != nil {
				if stime, err := workers.StrToLocalTime(r.Form["start"][0]); err == nil {
					start = stime.Unix()
				}
			}
			delete(r.Form, "start")
		}
		callback := ""
		if len(r.Form["callback"]) > 0 {
			callback = r.Form["callback"][0]
			delete(r.Form, "callback")
		}

		if jobID := wp.AddJob(name, r.Form, workers.Options{Start: start, Callback: callback}); jobID != "" {
			// log.Println("添加任务：", name)
			fmt.Fprintf(w, jobID) //输出到客户端的信息
			// result, _ := json.Marshal(map[string]interface{}{"jobID": jobID})
			// w.Header().Set("Content-Type", "application/json; charset=UTF-8")
			// fmt.Fprintf(w, string(result))
			return
		}
	}
	// log.Println("添加任务失败：", name)
}

func email(name string, args interface{}) error {
	form := args.(url.Values)
	user := "xxx@163.com"
	password := "123456"
	host := "smtp.163.com:25"
	mailtype := "html"

	if len(form["to"]) == 0 || form["to"][0] == "" {
		return errors.New("邮件接收者不能为空")
	}
	if len(form["subject"]) == 0 || form["subject"][0] == "" {
		return errors.New("标题不能为空")
	}
	if len(form["body"]) == 0 || form["body"][0] == "" {
		return errors.New("内容不能为空")
	}

	to := form["to"][0]
	subject := form["subject"][0]
	body := form["body"][0]

	hp := strings.Split(host, ":")
	auth := smtp.PlainAuth("", user, password, hp[0])
	var contentType string
	if mailtype == "html" {
		contentType = "Content-Type: text/" + mailtype + "; charset=UTF-8"
	} else {
		contentType = "Content-Type: text/plain" + "; charset=UTF-8"
	}

	msg := []byte("To: " + to + "\r\nFrom: " + user + "\r\nSubject: " + subject + "\r\n" + contentType + "\r\n\r\n" + body)
	sendTo := strings.Split(to, ";")
	err := smtp.SendMail(host, auth, user, sendTo, msg)
	return err
}

// 执行系统命令
func phpcmd(name string, args interface{}) error {
	form := args.(url.Values)
	if len(form["f"]) == 0 || form["f"][0] == "" {
		return errors.New("脚本名不能为空")
	}

	var buffer bytes.Buffer
	// buffer.WriteString("d:/work/")
	buffer.WriteString("/home/php/")
	file := strings.Replace(form["f"][0], "/", "", -1)
	file = strings.Replace(file, "\\", "", -1)
	buffer.WriteString(file)
	file = buffer.String()

	if _, err := os.Stat(file); err != nil {
		if os.IsNotExist(err) {
			return errors.New("php文件不存在")
		}
	}

	arg := []string{file}
	if len(form["arg"]) > 0 {
		arg = append(arg, form["arg"]...)
	}

	cmds := exec.Command("php", arg...)
	// out, err := cmds.CombinedOutput()
	_, err := cmds.CombinedOutput()
	return err
	// fmt.Println(string(out))
}

func sendMsg(name string, args interface{}) error {
	log.Println("开始发送短信", name, args)
	time.Sleep(time.Second * 5)
	log.Println("发送短信完成", name, args)
	// panic("this is a panic error")
	return nil
}

func makeCros(w http.ResponseWriter, r *http.Request) bool {
	Origin := r.Header.Get("Origin")
	if Origin == "" {
		Origin = "*"
	} else {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	w.Header().Set("Access-Control-Allow-Origin", Origin)
	w.Header().Set("Access-Control-Allow-Methods", "PUT,POST,GET,DELETE,OPTIONS")
	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Max-Age", "0")
		w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
		w.Header().Set("Content-Length", "0")
		w.Header().Set("Status", "204")
		w.Header().Set("Access-Control-Allow-Headers", "Origin,X-Requested-With,X_Requested_With,No-Cache,If-Modified-Since, Pragma, Last-Modified, Cache-Control, Expires, Content-Type, Content-Language, Cache-Control, X-E4M-With")
		return true
	}
	return false
}

```