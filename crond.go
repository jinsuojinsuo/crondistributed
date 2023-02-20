package crond

import (
	"context"
	"errors"
	"fmt"
	"gitee.com/liujinsuo/tool"
	"github.com/go-redis/redis/v8"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cast"
	"os"
	"time"
)

var distributed = time.Second * 4

type cronT struct {
}

// 1.获取最后一次执行的服务，不存在则添加当前服务为最后一次执行的服务
// 2.返回最后一次执行的服务
const cronTLastRunServerScript = `
-- 获取最后一次执行的服务
local str = redis.call("GET",KEYS[2])
if str == KEYS[1] then
	return str
elseif str == false then
	--无数据添加最后一次执行的服务为自己
	local ok = redis.call("SET", KEYS[2],KEYS[1],"EX",ARGV[1],"NX")
	if not ok then
		return "锁定服务失败"
	else
		return KEYS[1]
	end
else
	return str
end
`

// Distributed 支持分布式定时任务
// jobName 实际是redis中的key,一个服务部署到多台机器共用一个jobName。不同服务直接不能重复
func (s cronT) Distributed(rdb *redis.Client, logger cron.Logger, jobName string) cron.JobWrapper {
	redisKeyServer := s.redisKeyServer(jobName)
	redisKeyLastRunServer := s.redisKeyLastRunServer(jobName)
	s.serverRenewal(rdb, logger, jobName, redisKeyServer, redisKeyLastRunServer)     //首次添加服务
	s.distributedServer(rdb, logger, jobName, redisKeyServer, redisKeyLastRunServer) //自动对服务数据续期
	return func(NextJob cron.Job) cron.Job {
		return cron.FuncJob(func() {
			if result, err := rdb.Eval(context.Background(), cronTLastRunServerScript, []string{
				redisKeyServer,        //当前服务
				redisKeyLastRunServer, //最后一次执行的服务
			}, distributed/time.Second, //时长
			).Result(); err != nil {
				logger.Error(err, "redis命令执行失败")
				return
			} else if cast.ToString(result) == redisKeyServer {
				logger.Info("锁定服务成功", "result", result) //锁定服务是当前服务 继续执行
			} else {
				logger.Info("锁定服务失败", "result", result)
				return
			}
			NextJob.Run()
		})
	}
}

// 将当前服务写入reids,主要用于查看当前有哪些服务在运行
func (s cronT) distributedServer(rdb *redis.Client, logger cron.Logger, jobName string, redisKeyServer, redisKeyLastRunServer string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error(errors.New(fmt.Sprintf("%+v", r)), "redis操作失败")
			}
		}()
		for {
			time.Sleep(distributed / 2)
			s.serverRenewal(rdb, logger, jobName, redisKeyServer, redisKeyLastRunServer)
		}
	}()
}

// 当前服务续期
const cronTServerRenewalScript = `
-- 当前服务续期
redis.call("SETEX",KEYS[1],ARGV[1],1)

-- 获取最后一次执行的服务
local str = redis.call("GET",KEYS[2])
if str == ARGV[2] then
	--最后执行的服务是当前服务 续期
	redis.call("EXPIRE", KEYS[2],ARGV[1])
end
`

// 当前服务续期
func (s cronT) serverRenewal(rdb *redis.Client, logger cron.Logger, jobName string, redisKeyServer, redisKeyLastRunServer string) {
	if err := rdb.Eval(context.Background(), cronTServerRenewalScript, []string{
		redisKeyServer,        //当前服务
		redisKeyLastRunServer, //最后一次执行的服务
	}, distributed/time.Second, redisKeyServer).Err(); err == redis.Nil {
		logger.Info("服务器信息续期成功") //无需处理
	} else if err != nil {
		logger.Error(err, "服务器信息续期失败")
		return
	}
}

// 返回服务id
// host、pid 主要是为了查看方便实际没什么作用
func (s cronT) redisKeyServer(jobName string) string {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown-host"
	}
	pid := os.Getpid()
	TaskID := tool.NewMongoID().ToBase62() //如果有多个AddJob需要考TaskID来隔离
	return fmt.Sprintf("%v:%s_%d_%s", jobName, host, pid, TaskID)
}

// 最后一次执行的服务器
func (s cronT) redisKeyLastRunServer(jobName string) string {
	return fmt.Sprintf("%v:last_run_server", jobName)
}

// Distributed 分布式定时任务中间件
// jobName 实际是redis中的key,一个服务部署到多台机器共用一个jobName。不同服务直接不能重复
func Distributed(rdb *redis.Client, logger cron.Logger, jobName string) cron.JobWrapper {
	return (cronT{}).Distributed(rdb, logger, jobName)
}
