package concurrency

import (
	"fmt"
	"quickpress/config"
	"sync"

	"github.com/panjf2000/ants/v2"
	"golang.org/x/net/context"

	"time"
)

// 枚举值含义
const (
	second = "s"
	minute = "m"
	hour   = "h"
)

func GroutinePool(conc config.Concurrency, f func()) {
	loop := conc.Loop
	unit := conc.Unit
	stages := conc.Stages
	// 处理时间
	var unt time.Duration
	switch unit {
	case second:
		unt = time.Second
	case minute:
		unt = time.Minute
	case hour:
		unt = time.Hour
	}
	// 前一个时间点
	var preDur = 0
	pool, _ := ants.NewPool(1)
	defer pool.Release()
	for i := 0; i < loop; i++ {
		for _, stage := range stages {
			concurrent := stage.Target
			now := time.Now()
			// 调整协程池大小
			pool.Tune(concurrent)
			fmt.Println("pool modify time: ", time.Since(now))
			ctx, cancel := context.WithCancel(context.Background())
			var wg sync.WaitGroup
			wg.Add(concurrent)
			// 提交任务
			for i := 0; i < concurrent; i++ {
				v := i
				err := pool.Submit(func() {
					for {
						select {
						case <-ctx.Done():
							fmt.Println("协程号：" + fmt.Sprintf("%d", v) + "退出")
							wg.Done()
							return
						default:
							// 执行方法
							f()
						}

					}
				})
				if err != nil {
					fmt.Println(err)
				}

			}
			// 运行时间
			time.Sleep(time.Duration(stage.Duration-preDur) * unt)
			preDur = stage.Duration
			cancel()
			wg.Wait()
			now = time.Now()
			fmt.Println("tasks all cancel finish time: ", time.Since(now))
		}
	}

}
