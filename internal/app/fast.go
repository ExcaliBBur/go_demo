package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"godemo/internal/store"
)

type Fast struct {
	db        *store.Store
	dbTimeout time.Duration

	requests atomic.Int64

	cacheMu  sync.RWMutex
	cache    map[int]cacheEntry
	cacheTTL time.Duration

	listMu      sync.RWMutex
	list        []store.Product
	listExpires time.Time
	listTTL     time.Duration

	bufPool sync.Pool
	pool    *WorkerPool
}

type cacheEntry struct {
	product store.Product
	expires time.Time
}

func NewFast(db *store.Store) *Fast {
	workers := max(1, runtime.NumCPU()/2)
	return &Fast{
		db:        db,
		dbTimeout: 200 * time.Millisecond,
		cache:     make(map[int]cacheEntry),
		cacheTTL:  30 * time.Second,
		listTTL:   time.Second,
		bufPool:   sync.Pool{New: func() any { return new(bytes.Buffer) }},
		pool:      NewWorkerPool(workers, workers*4),
	}
}

func (a *Fast) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /products/{id}", a.product)
	mux.HandleFunc("GET /report", a.report)
	mux.HandleFunc("GET /hash", a.hash)
	mux.HandleFunc("GET /stats", a.stats)
	return mux
}

func (a *Fast) Close() { a.pool.Close() }

func (a *Fast) product(w http.ResponseWriter, r *http.Request) {
	a.requests.Add(1)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	p, err := a.getProduct(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case errors.Is(err, context.DeadlineExceeded):
		http.Error(w, "database timeout", http.StatusGatewayTimeout)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.writeJSON(w, p)
}

func (a *Fast) getProduct(ctx context.Context, id int) (store.Product, error) {
	a.cacheMu.RLock()
	e, ok := a.cache[id]
	a.cacheMu.RUnlock()
	if ok && time.Now().Before(e.expires) {
		return e.product, nil
	}

	ctx, cancel := context.WithTimeout(ctx, a.dbTimeout)
	defer cancel()
	p, err := a.db.Get(ctx, id)
	if err != nil {
		return store.Product{}, err
	}

	a.cacheMu.Lock()
	a.cache[id] = cacheEntry{product: p, expires: time.Now().Add(a.cacheTTL)}
	a.cacheMu.Unlock()
	return p, nil
}

func (a *Fast) report(w http.ResponseWriter, r *http.Request) {
	a.requests.Add(1)
	products, err := a.allProducts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusGatewayTimeout)
		return
	}
	a.writeJSON(w, products)
}

func (a *Fast) allProducts(ctx context.Context) ([]store.Product, error) {
	a.listMu.RLock()
	list, expires := a.list, a.listExpires
	a.listMu.RUnlock()
	if list != nil && time.Now().Before(expires) {
		return list, nil
	}

	ctx, cancel := context.WithTimeout(ctx, a.dbTimeout)
	defer cancel()
	list, err := a.db.All(ctx)
	if err != nil {
		return nil, err
	}

	a.listMu.Lock()
	a.list, a.listExpires = list, time.Now().Add(a.listTTL)
	a.listMu.Unlock()
	return list, nil
}

func (a *Fast) writeJSON(w http.ResponseWriter, v any) {
	buf := a.bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer a.bufPool.Put(buf)

	if err := json.NewEncoder(buf).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.Write(buf.Bytes())
}

func (a *Fast) hash(w http.ResponseWriter, r *http.Request) {
	a.requests.Add(1)
	ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
	defer cancel()

	sum, err := a.pool.Do(ctx, []byte(r.URL.Query().Get("data")))
	switch {
	case errors.Is(err, ErrOverloaded):
		w.Header().Set("Retry-After", "1")
		http.Error(w, "server is busy", http.StatusServiceUnavailable)
		return
	case err != nil:
		http.Error(w, "timeout", http.StatusGatewayTimeout)
		return
	}
	fmt.Fprintf(w, "%x\n", sum)
}

func (a *Fast) stats(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, map[string]int64{"requests": a.requests.Load()})
}
