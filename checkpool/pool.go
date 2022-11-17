package checkpool

import (
	"context"
	"github.com/Jeffail/tunny"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
	"runtime"
	"sync"
	"time"
)

var Logger *zap.Logger

func init() {
	Logger = zap.New(nil)
}

func EnableLogger() {
	// set logger
	hook := lumberjack.Logger{
		Filename: "monitor.log", // path of log
		MaxSize:  128,
		Compress: true,
	}
	encoderConfig := zapcore.EncoderConfig{
		LevelKey:       "level",
		TimeKey:        "time",
		NameKey:        "logger",
		CallerKey:      "line",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000"),
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeName:     zapcore.FullNameEncoder,
	}

	// set log level
	consoleLevel := zap.NewAtomicLevel()
	consoleLevel.SetLevel(zap.DebugLevel)
	fileLevel := zap.NewAtomicLevel()
	fileLevel.SetLevel(zap.WarnLevel)

	// console printer
	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		consoleLevel,
	)
	// file printer
	fileCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(&hook),
		fileLevel,
	)

	caller := zap.AddCaller()
	development := zap.Development()
	Logger = zap.New(zapcore.NewTee(consoleCore, fileCore), caller, development)
}

type Weight interface {

	// Add a naming element with weight
	Add(name string, weight int)

	// Next return existed element by balancing rule
	Next() string

	// Remove an element by its name
	Remove(name string)

	// Get the weight of element by its name, if element not exist, return error
	Get(name string) (int, error)

	// Update weight for element by name, if element not exist, return error
	Update(name string, weight int) error

	// Reduce weight for element by name
	// return weight after reduce, and error if element not exist
	Reduce(name string) (int, error)

	// All get hosts list
	All() []string
}

type WeightHostPool struct {
	weightPool       Weight
	origin           map[string]int
	deadPool         lockPool
	wakeUpAndRecover chan bool
	minWeight        int
}

type lockPool struct {
	pool []string
	sync.RWMutex
}

// NewWeightHostPool Build a new WeightHostPool.
// A rule object that implements the Weight interface, a host collection, and minWeight need to be given.
// Hosts with a weight lower than minWeight will be considered temporarily unavailable
func NewWeightHostPool(weightPool Weight, hosts map[string]int, minWeight int) *WeightHostPool {
	p := WeightHostPool{}
	p.InitPool(weightPool, hosts, minWeight)
	return &p
}

func (p *WeightHostPool) InitPool(weightPool Weight, hosts map[string]int, minWeight int) {
	p.weightPool = weightPool
	p.origin = make(map[string]int)
	for host, weight := range hosts {
		p.weightPool.Add(host, weight)
		p.origin[host] = weight
	}
	p.deadPool.pool = make([]string, 0, len(p.origin))
	p.wakeUpAndRecover = make(chan bool)
	p.minWeight = minWeight
}

// GetHost get host by rule
func (p *WeightHostPool) GetHost() string {
	host := p.weightPool.Next()
	Logger.Debug("monitor:Get host:" + host)
	return host
}

// GetWeight by given name
func (p *WeightHostPool) GetWeight(name string) (int, error) {
	return p.weightPool.Get(name)
}

// SetWeight by given name and weight
func (p *WeightHostPool) SetWeight(name string, weight int) error {
	return p.weightPool.Update(name, weight)
}

// MarkFailed is used to report host failure, will reduce its weight
// If its weight is lower than minWeight, it will be removed
func (p *WeightHostPool) MarkFailed(host string) {
	if ew, err := p.weightPool.Reduce(host); err == nil {
		if ew <= p.minWeight {
			p.deadPool.Lock()
			defer p.deadPool.Unlock()
			p.weightPool.Remove(host)
			p.deadPool.pool = append(p.deadPool.pool, host)
			Logger.Warn("monitor:Host removed", zap.String("host", host))
		}
	}
}

// MarkEmpty use to report get "" from the pool,to wake up the recover goroutine
func (p *WeightHostPool) MarkEmpty() {
	select {
	case p.wakeUpAndRecover <- true:
	default:
	}
}

// StartChecking starts two goroutines to check the normal host and the invalid host respectively
// Input interval time of each round of inspection, timeout, and the specific implementation of the check
func (p *WeightHostPool) StartChecking(ctx context.Context, interval time.Duration, timeout time.Duration, doCheck func(host string) bool) (stopChecking func()) {
	goPool := tunny.NewFunc(runtime.NumCPU(), func(host interface{}) interface{} {
		return doCheck(host.(string))
	})
	checkCtx, checkCancel := context.WithCancel(ctx)
	// health check goroutine
	go func() {
		timer := time.NewTimer(timeout)
		for {
			hosts := p.weightPool.All()
			if hosts != nil {
				for _, h := range hosts {
					select {
					case <-ctx.Done():
						return
					default:
					}
					newCtx, cancel := context.WithTimeout(checkCtx, timeout)
					passCheck, err := goPool.ProcessCtx(newCtx, h)
					if err != nil {
						Logger.Debug("monitor:Host timeout", zap.String("host", h))
						p.MarkFailed(h)
					} else if !passCheck.(bool) {
						Logger.Debug("monitor:Host check failed", zap.String("host", h))
						p.MarkFailed(h)
					} else {
						Logger.Debug("monitor:Host pass", zap.String("host", h))
					}
					cancel()
				}
			} else {
				Logger.Warn("monitor:Host list is empty,skip check...")
				select {
				case p.wakeUpAndRecover <- true:
				default:
				}
			}

			// sleep and wait ctx down
			if !timer.Stop() {
				select {
				case <-timer.C: //try to drain from the channel
				default:
				}
			}
			timer.Reset(interval)
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
	}()
	// recover goroutine, regularly check whether the host in the deadpool is recovered
	go func() {
		timer := time.NewTimer(timeout)
		for {
			if p.deadPool.pool != nil && len(p.deadPool.pool) != 0 {
				for _, h := range p.deadPool.pool {
					select {
					case <-ctx.Done():
						return
					default:
					}
					newCtx, cancel := context.WithTimeout(checkCtx, timeout)
					passCheck, err := goPool.ProcessCtx(newCtx, h)
					if err != nil {
						Logger.Debug("host still not available", zap.String("host", h))
					} else if !passCheck.(bool) {
						Logger.Debug("host still check failed", zap.String("host", h))
					} else {
						p.weightPool.Add(h, p.origin[h])
						//fmt.Println(p.All())
						p.recover(h)
						Logger.Warn("monitor:Host recovered", zap.String("host", h))
					}
					cancel()
				}
			}
			// sleep and wait ctx down
			if !timer.Stop() {
				select {
				case <-timer.C: //try to drain from the channel
				default:
				}
			}
			timer.Reset(interval)
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			case <-p.wakeUpAndRecover:
			}
		}
	}()
	return func() {
		checkCancel()
		goPool.Close()
	}
}

// recover host from deadpool
func (p *WeightHostPool) recover(host string) {
	p.deadPool.Lock()
	defer p.deadPool.Unlock()
	for i, h := range p.deadPool.pool {
		if h == host {
			p.deadPool.pool = append(p.deadPool.pool[0:i], p.deadPool.pool[i+1:]...)
			//fmt.Println(p.deadPool)
			return
		}
	}

}
