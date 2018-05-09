package main

import (
	"bytes"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	var httpHostPort string
	var natsHostPort string
	var count int
	var workers int
	var requestPath string
	var payload int

	flag.StringVar(&requestPath, "p", "/foo", "requestPath")
	flag.IntVar(&payload, "d", 0, "request payload")
	flag.StringVar(&httpHostPort, "hp", "localhost:8080", "HTTP host port")
	flag.StringVar(&natsHostPort, "np", "localhost:4222", "NATS host port")
	flag.IntVar(&count, "c", 1000000, "number of requests")
	flag.IntVar(&workers, "w", 100, "number of workers")
	flag.Parse()

	wg := sync.WaitGroup{}
	wg.Add(workers)

	var errorCount int64

	now := time.Now()
	url := fmt.Sprintf("http://%s%s", httpHostPort, requestPath)

	work := count / workers
	count = work * workers

	data := make([]byte, payload)
	rand.Read(data)

	for i := 0; i < workers; i++ {
		go func(work int) {
			defer wg.Done()
			c := &http.Client{
				Transport: &http.Transport{
					IdleConnTimeout: 500 * time.Millisecond,
				},
			}
			for i := 0; i < work; i++ {
				if i%1000 == 0 {
					fmt.Print("+")
				}
				_, err := c.Post(url, "text/plain", bytes.NewReader(data))
				if err != nil {
					if atomic.AddInt64(&errorCount, 1)%100 == 0 {
						fmt.Print("O")
					}
				}
			}
		}(work)
	}
	wg.Wait()
	fmt.Println()

	stop := time.Since(now)
	fmt.Printf("sent %d messages in %s\n", count, stop.String())
	fmt.Printf("sent %.1f / msgs sec\n", float64(count)/stop.Seconds())
	fmt.Printf("errors sending %d \n", errorCount)
}
