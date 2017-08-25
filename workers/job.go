package workers

import (
	"sync"
	"time"

	"github.com/anchenbaba/randomstrings"
)

// JobFunc 工作方法
type JobFunc func(string, interface{}) error

// Job 一个作业
type Job struct {
	name string // 作业名称
	args interface{}
	fn   JobFunc
	Options
	// id       string // 唯一标识符
	// start    int64
	// callback string
}

// Options Job选项
type Options struct {
	ID       string // 唯一标识符
	Start    int64
	Callback string
}

// JobServer 预定义各种作业方法
type JobServer struct {
	mt sync.Mutex
	// jobs 作业方法的一个集合
	// jobs sync.Map
	jobs     map[string]JobFunc
	jobQueue []*Job
}

// NewJobServer 初始化
func NewJobServer() *JobServer {
	js := new(JobServer)
	js.jobs = make(map[string]JobFunc)
	return js
}

// Register 注册一个作业的定义
// todo 定义每个注册的作业最大的任务队列长度
func (js *JobServer) Register(name string, v JobFunc) {
	js.mt.Lock()
	defer js.mt.Unlock()
	if _, ok := js.jobs[name]; !ok {
		js.jobs[name] = v
	}
}

// Enqueue 添加一个作业到队列 入队
func (js *JobServer) Enqueue(name string, args interface{}, option ...Options) string {
	var options Options
	if len(option) > 0 {
		options = option[0]
	}
	if options.ID == "" {
		options.ID = name + "_" + randomstrings.RandomStringNum(16)
	}

	js.mt.Lock()
	defer js.mt.Unlock()

	fn, ok := js.jobs[name]
	if !ok {
		return ""
	}
	// 定时不能超过一百年
	if (options.Start - time.Now().Unix()) > 3153600000 {
		options.Start = 0
	}
	theJob := &Job{name: name, args: args, fn: fn, Options: options}
	js.jobQueue = append(js.jobQueue, theJob)
	return options.ID
}

// Dequeue 获取一个作业到队列 出队运行
func (js *JobServer) Dequeue(name string) *Job {
	js.mt.Lock()
	defer js.mt.Unlock()

	L := len(js.jobQueue)
	if L == 0 {
		return nil
	}

	t := -1
	for k, j := range js.jobQueue {
		if j.Start > time.Now().Unix() {
			continue
		}
		t = k
	}
	if t == -1 {
		return nil
	}

	theJob := js.jobQueue[t]
	if L == 1 {
		js.jobQueue = nil
	} else {
		js.jobQueue = append(js.jobQueue[t+1:], js.jobQueue[:t]...)
	}

	return theJob
}

// JobQueueLen 当前队列长度
func (js *JobServer) JobQueueLen() int {
	js.mt.Lock()
	defer js.mt.Unlock()
	return len(js.jobQueue)
}
