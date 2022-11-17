# go-checkhostpool
Weight host pool with polling check function (golang) 轮询检查权重host池

为满足host池的可用性需求，在权重算法之上实现代理层，提供启动goroutine来轮询检查host的方法。

用法：

自定义检查方法的实现，用来检查host的有效性

```go
func testCheck(string) bool {
   rand.Seed(time.Now().UnixNano())
   num := rand.Intn(5000)
   time.Sleep(time.Duration(num) * time.Millisecond)
   if num%10 == 0 {
      return false
   }
   return true
}
```

构建WeightHostPool，目前实现了round robin和random

```go
ctx, _ := context.WithCancel(context.Background())
arr := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3", "192.168.1.4", "192.168.1.5"}
checkpool.EnableLogger()
hosts := make(map[string]int)
for _, h := range arr {
   hosts[h] = 5
}
pool := checkpool.NewWeightHostPool(checkpool.NewRandom(), hosts, 0)
```

开启轮询检查

```go
pool.StartChecking(ctx, 10*time.Second, 3*time.Second, testCheck)
```

尝试获取host

```go
for {
   fmt.Println(pool.GetHost())
   time.Sleep(1 * time.Second)
}
```

## 特性

开启轮询检查后，会开启两个goroutine。一个goroutine轮询检查活动池中host的状况。若检查失败，会降低其权重。若权重低于minWeight，host会被移至死亡池中。另一goroutine用来检查死亡池中的host是否复活。

当池中host全部位于死亡池中时（即全不可用），请求获取host会持续获取空字符串，直至有host可用。

## API使用

### 日志

使用go.uber.org/zap来管理日志。使用EnableLogger()来开启日志输出。

要修改日志配置，可以操作Logger变量

### 构建

使用NewWeightHostPool()来构建host池。需给定实现Weight接口的活动池。目前实现了round robin规则（使用NewRoundRobin()来构建）和random规则（使用NewRandom()来构建）的活动池
