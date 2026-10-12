package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"godemo/internal/store"
)

type Naive struct {
	db *store.Store

	mu   sync.Mutex
	hits map[string]int
}

func NewNaive(db *store.Store) *Naive {
	return &Naive{db: db, hits: make(map[string]int)}
}

func (a *Naive) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /products/{id}", a.product)
	mux.HandleFunc("GET /report", a.report)
	mux.HandleFunc("GET /hash", a.hash)
	mux.HandleFunc("GET /stats", a.stats)
	return mux
}

func (a *Naive) count(name string) {
	a.mu.Lock()
	a.hits[name]++
	a.mu.Unlock()
}

func (a *Naive) product(w http.ResponseWriter, r *http.Request) {
	a.count("product")
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	p, err := a.db.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body, _ := json.Marshal(p)
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (a *Naive) report(w http.ResponseWriter, r *http.Request) {
	a.count("report")
	products, err := a.db.All(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := "["
	for i, p := range products {
		if i > 0 {
			out += ","
		}
		tags, _ := json.Marshal(p.Tags)
		out += fmt.Sprintf(`{"id":%d,"name":%q,"price":%.2f,"stock":%d,"tags":%s}`,
			p.ID, p.Name, p.Price, p.Stock, tags)
	}
	out += "]"
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(out))
}

func (a *Naive) hash(w http.ResponseWriter, r *http.Request) {
	a.count("hash")
	sum := heavyHash([]byte(r.URL.Query().Get("data")))
	fmt.Fprintf(w, "%x\n", sum)
}

func (a *Naive) stats(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	body, _ := json.Marshal(a.hits)
	a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}
