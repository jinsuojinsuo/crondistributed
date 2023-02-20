package main

import (
	"fmt"
	"gitee.com/liujinsuo/tool"
	"github.com/go-redis/redis/v8"
	"github.com/jinsuojinsuo/crondistributed"
	"github.com/robfig/cron/v3"
	"log"
	"os"
	"time"
)

func main() {

	Cron := cron.New(
		cron.WithSeconds(), //支持秒级时间
	)

	//设置日志
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	//连接redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       1,  // use default DB
	})

	//使用插件
	job := cron.NewChain(
		//cron.SkipIfStillRunning(cron.DefaultLogger), //上次任务未执行完成则跳过
		crondistributed.Distributed(rdb, cron.VerbosePrintfLogger(log.New(os.Stdout, "cron", log.LstdFlags|log.Lshortfile)), "test-job-name")).Then(cron.FuncJob(func() {
		fmt.Println("执行任务", time.Now())
		//time.Sleep(time.Second * 6)
		//fmt.Println("执行任务完成", time.Now())
	}))
	_, _ = Cron.AddJob(tool.Cron.Every(time.Second), job)

	Cron.Run()
}
