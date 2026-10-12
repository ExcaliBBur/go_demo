package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type result struct {
	ok     []time.Duration
	status map[int]int
	errors int
	first  error
}

func main() {
	url := flag.String("url", "http://localhost:8080/report", "адрес")
	c := flag.Int("c", 64, "число одновременных клиентов")
	d := flag.Duration("d", 10*time.Second, "длительность")
	ids := flag.Int("ids", 500, "диапазон для {id}")
	backoff := flag.Duration("backoff", 20*time.Millisecond, "пауза клиента после ответа 503")
	flag.Parse()

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp4", addr)
			},
			MaxIdleConns:        *c,
			MaxIdleConnsPerHost: *c,
		},
	}

	results := make([]result, *c)
	deadline := time.Now().Add(*d)
	start := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < *c; i++ {
		wg.Add(1)
		time.Sleep(time.Millisecond)
		go func(res *result) {
			defer wg.Done()
			res.status = make(map[int]int)
			for time.Now().Before(deadline) {
				target := strings.ReplaceAll(*url, "{id}", strconv.Itoa(1+rand.IntN(*ids)))
				t := time.Now()
				resp, err := client.Get(target)
				if err != nil {
					res.errors++
					if res.first == nil {
						res.first = err
					}
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				res.status[resp.StatusCode]++
				if resp.StatusCode < 300 {
					res.ok = append(res.ok, time.Since(t))
				}
				if resp.StatusCode == http.StatusServiceUnavailable {
					time.Sleep(*backoff)
				}
			}
		}(&results[i])
	}
	wg.Wait()
	elapsed := time.Since(start)

	var ok []time.Duration
	status := make(map[int]int)
	total, errs := 0, 0
	var firstErr error
	for _, r := range results {
		ok = append(ok, r.ok...)
		errs += r.errors
		if firstErr == nil {
			firstErr = r.first
		}
		for code, n := range r.status {
			status[code] += n
			total += n
		}
	}
	sort.Slice(ok, func(i, j int) bool { return ok[i] < ok[j] })

	fmt.Printf("url:       %s\n", *url)
	fmt.Printf("clients:   %d, duration: %s\n", *c, elapsed.Round(time.Millisecond))
	fmt.Printf("requests:  %d (errors: %d)\n", total, errs)
	if firstErr != nil {
		fmt.Printf("  first error: %v\n", firstErr)
	}
	codes := make([]int, 0, len(status))
	for code := range status {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	for _, code := range codes {
		fmt.Printf("  HTTP %d: %d\n", code, status[code])
	}
	fmt.Printf("RPS (2xx): %.0f\n", float64(len(ok))/elapsed.Seconds())
	if len(ok) > 0 {
		fmt.Printf("latency:   p50=%s  p95=%s  p99=%s  max=%s\n",
			pct(ok, 50), pct(ok, 95), pct(ok, 99), round(ok[len(ok)-1]))
	}
}

func pct(sorted []time.Duration, p int) time.Duration {
	i := len(sorted) * p / 100
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return round(sorted[i])
}

func round(d time.Duration) time.Duration {
	if d < time.Millisecond {
		return d.Round(time.Microsecond)
	}
	return d.Round(100 * time.Microsecond)
}
