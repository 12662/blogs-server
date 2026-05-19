package main

import (
	"server/core"
	"server/flag"
	"server/global"
	"server/initialize"
)

func main() {
	global.Config = core.InitConf()
	global.Log = core.InitLogger()
	initialize.OtherInit()
	global.DB = initialize.InitGorm()
	global.Redis = initialize.ConnectRedis()
	global.ESClient = initialize.ConnectEs()
	global.AsynqClient = initialize.ConnectAsynqClient()

	defer global.Redis.Close()
	defer global.AsynqClient.Close()
	flag.InitFlag()

	global.AsynqServer = initialize.RunAsynqServer()
	defer global.AsynqServer.Shutdown()

	cronRunner := initialize.InitCron()
	defer func() {
		if cronRunner != nil {
			cronCtx := cronRunner.Stop()
			<-cronCtx.Done()
		}
	}()

	core.RunServer()
}
