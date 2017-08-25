package workers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/anchenbaba/randomstrings"
)

// Worker 工作者
type Worker struct {
	ID          string
	PJob        *Job
	lastUseTime time.Time
	// jobCh       chan *Job
}

// WorkerPool 工作者池
type WorkerPool struct {
	Log               []string
	MaxWorkersCount   int
	MaxWorkerDuration time.Duration // 工作者最大空闲时间
	JobServ           *JobServer

	lock         *sync.Mutex
	workersCount int
	workers      []*Worker
	// mustStop     bool
}

// NewWorkerPool ...
func NewWorkerPool() *WorkerPool {
	maxWorkersCount := runtime.NumCPU()
	jobServer := NewJobServer()

	wp := &WorkerPool{
		MaxWorkersCount:   maxWorkersCount,
		MaxWorkerDuration: time.Second * 60,
		JobServ:           jobServer,
		lock:              new(sync.Mutex),
	}

	wp.Register("jobCallback", jobCallback)
	return wp
}

// AddLog 添加内存日志
func (wp *WorkerPool) AddLog(log string) {
	if len(wp.Log) > 99 {
		wp.Log = wp.Log[1:]
	}
	wp.Log = append(wp.Log, log)
}

// GetLog 读取日志
func (wp *WorkerPool) GetLog() []string {
	return wp.Log
}

// AddJob 添加一个作业
func (wp *WorkerPool) AddJob(name string, args interface{}, option ...Options) string {
	return wp.JobServ.Enqueue(name, args, option...)
}

// Register 添加一个作业执行方法
func (wp *WorkerPool) Register(name string, v JobFunc) {
	wp.JobServ.Register(name, v)
}

// Start 启动服务
func (wp *WorkerPool) Start() {
	go func() {
		for {
			wp.clean()
			time.Sleep(wp.MaxWorkerDuration)
		}
	}()

	jobCh := make(chan *Job)
	workerCh := make(chan *Worker)
	go func() {
		for {
			job := wp.getJob()
			if job == nil {
				time.Sleep(100 * time.Millisecond)
			} else {
				jobCh <- job
			}
		}
	}()
	go func() {
		for {
			worker := wp.getWorker()
			if worker == nil {
				time.Sleep(100 * time.Millisecond)
			} else {
				workerCh <- worker
			}
		}
	}()
	for {
		worker := <-workerCh
		worker.PJob = <-jobCh
		go wp.workerFunc(worker)
	}
}

// 查看最近使用的Woker, 如果他的最近使用间隔大于某个值，那么把这个Worker清理了
func (wp *WorkerPool) clean() {
	now := time.Now()
	wp.lock.Lock()
	ready := wp.workers
	var tmp []*Worker
	for _, w := range ready {
		if now.Sub(w.lastUseTime) < wp.MaxWorkerDuration {
			tmp = append(tmp, w)
			// wp.workers = append(ready[:k], ready[k+1:]...)
		}
	}
	wp.workersCount = wp.workersCount - (len(ready) - len(tmp))
	wp.workers = tmp
	wp.lock.Unlock()
}

func (wp *WorkerPool) getWorker() *Worker {
	var worker *Worker
	wp.lock.Lock()
	workers := wp.workers
	n := len(workers) - 1 // 空闲工作者数组下标最大值
	if n < 0 {
		// 如果空闲工作者为空
		// 总工作者数小于最大工作者数量
		if wp.workersCount < wp.MaxWorkersCount {
			wp.workersCount = wp.workersCount + 1
		} else {
			wp.lock.Unlock()
			return nil
		}
	} else {
		worker = workers[n]
		workers[n] = nil
		wp.workers = workers[:n] // 把这个worker从空闲队列删除
	}
	wp.lock.Unlock()

	if worker == nil {
		// 创建一个新的worker
		worker = &Worker{
			ID:          randomstrings.RandomStringAll(12),
			PJob:        nil,
			lastUseTime: time.Now(),
		}
	}
	return worker
}

// 执行worker的Func方法
func (wp *WorkerPool) workerFunc(w *Worker) {
	defer func() {
		// 一个作业失败也不会导致整个程序崩溃
		err := recover()
		status := true
		if err != nil {
			// log.Printf("任务失败：工作者=%v,任务=%v,err=%v\r\n", w.ID, w.PJob.name, err)
			wp.AddLog(fmt.Sprintf("%v 任务失败：工作者=%v,任务=%v,任务ID=%v,err=%v\r\n", time.Now().Format("2006-01-02 15:04:05"), w.ID, w.PJob.name, w.PJob.ID, err))
			status = false
		}
		// else {
		// 	log.Printf("任务完成：工作者=%v,任务=%v\r\n", w.ID, w.PJob.name)
		// }
		// callback 执行函数
		if w.PJob.Callback != "" {
			wp.AddJob("jobCallback", map[string]interface{}{
				"jobCallback": w.PJob.Callback,
				"jobID":       w.PJob.ID,
				"status":      status,
				"err":         err,
			})
		}
		w.PJob = nil
		_ = wp.release(w)
	}()
	j := w.PJob
	err := j.fn(j.name, j.args)
	if err != nil {
		panic(err)
	}
}

func (wp *WorkerPool) getJob() *Job {
	return wp.JobServ.Dequeue("")
}

// 释放工作者 到 空闲队列中
func (wp *WorkerPool) release(w *Worker) bool {
	w.lastUseTime = time.Now()
	wp.lock.Lock()
	wp.workers = append(wp.workers, w)
	wp.lock.Unlock()
	return true
}

func jobCallback(name string, args interface{}) error {
	m := args.(map[string]interface{})
	switch m["jobCallback"].(type) {
	case string:
		return httpDo(m["jobCallback"].(string), m["jobID"].(string), m["status"].(bool), m["err"])
	}
	return nil
}

func httpDo(httpURL, jobID string, status bool, err interface{}) error {
	errStr := ""
	if err != nil {
		errStr = err.(error).Error()
	}
	jsondata, _ := json.Marshal(map[string]interface{}{
		"jobID":  jobID,
		"status": status,
		"err":    errStr,
	})
	data := base64.URLEncoding.EncodeToString(jsondata)
	client := &http.Client{
		Timeout: time.Duration(time.Second * 5), // 超时
	}
	var v = url.Values{}
	v.Add("jsonStr", data)
	vdata := v.Encode()
	req, _ := http.NewRequest("GET", httpURL, strings.NewReader(vdata))
	res, errs := client.Do(req)
	if errs != nil {
		return errors.New("回调失败: jobID=" + jobID + ",url=" + httpURL)
	}
	if res.StatusCode != 200 {
		errs = errors.New("回调失败: jobID=" + jobID + ",url=" + httpURL)
	}
	_ = res.Body.Close()
	return errs
}

// StrToLocalTime 如果解析来源是GMT的时间
func StrToLocalTime(value string) (time.Time, error) {
	if value == "" || value == "now" {
		return time.Now(), nil
	}
	layouts := []string{
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05 -0700 MST",
		"2006/01/02 15:04:05 -0700",
		"2006/01/02 15:04:05",
		"2006-01-02 -0700 MST",
		"2006-01-02 -0700",
		"2006-01-02",
		"2006/01/02 -0700 MST",
		"2006/01/02 -0700",
		"2006/01/02",
		"2006-01-02 15:04:05 -0700 -0700",
		"2006/01/02 15:04:05 -0700 -0700",
		"2006-01-02 -0700 -0700",
		"2006/01/02 -0700 -0700",
		time.ANSIC,
		time.UnixDate,
		time.RubyDate,
		time.RFC822,
		time.RFC822Z,
		time.RFC850,
		time.RFC1123,
		time.RFC1123Z,
		time.RFC3339,
		time.RFC3339Nano,
		time.Kitchen,
		time.Stamp,
		time.StampMilli,
		time.StampMicro,
		time.StampNano,
	}

	var t time.Time
	var err error
	for _, layout := range layouts {
		// loc, _ := time.LoadLocation("Local")
		t, err = time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, err
}
