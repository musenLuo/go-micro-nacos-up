# go-micro 使用 nacos 作为注册中心

## 使用说明

```go
package main

import (
	"github.com/asim/go-micro/v3/registry"
	"github.com/asim/go-micro/v3/web"
	"github.com/gin-gonic/gin"
	"gitee.com/jawide/go-micro-nacos"
)

func main() {
	rou := initGin()
	reg := initReg()
	ser := initService(rou, reg)

	err := ser.Run()
	if err != nil {
		panic(err)
	}
}

func initService(rou *gin.Engine, reg registry.Registry) web.Service {
	// service init
	ser := web.NewService(
		web.Name("go-micro-nacos"),
		web.Registry(reg),
		web.Handler(rou),
		web.Address("0.0.0.0:8080"),
	)
	return ser
}

func initReg() registry.Registry {
	// reg init
	reg := nacos.NewRegistry(func(options *registry.Options) {
		options.Addrs = []string{"127.0.0.1:8848"}
	})
	return reg
}

func initGin() *gin.Engine {
	// gin init
	r := gin.Default()
	r.GET("/hello", func(context *gin.Context) {
		context.String(200, "hello")
	})
	return r
}
```