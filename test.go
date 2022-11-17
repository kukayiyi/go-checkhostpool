package main

import (
	"context"
	"fmt"
	"math/rand"
	"nigs-clean/checkpool"
	"strconv"
	"time"
)

func testCheck(string) bool {
	rand.Seed(time.Now().UnixNano())
	num := rand.Intn(5000)
	fmt.Println("rand:" + strconv.Itoa(num))
	time.Sleep(time.Duration(num) * time.Millisecond)
	if num%10 == 0 {
		return false
	}
	return true
}

func main() {
	ctx, _ := context.WithCancel(context.Background())
	arr := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3", "192.168.1.4", "192.168.1.5"}
	checkpool.EnableLogger()
	hosts := make(map[string]int)
	for _, h := range arr {
		hosts[h] = 5
	}
	pool := checkpool.NewWeightHostPool(checkpool.NewRandom(), hosts, 0)

	pool.StartChecking(ctx, 10*time.Second, 3*time.Second, testCheck)

	for {
		fmt.Println(pool.GetHost())
		time.Sleep(1 * time.Second)
	}

	//time.Sleep(30 * time.Second)
	//cancel()

}
