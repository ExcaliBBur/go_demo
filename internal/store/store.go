package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrNotFound = errors.New("product not found")

type Product struct {
	ID    int      `json:"id"`
	Name  string   `json:"name"`
	Price float64  `json:"price"`
	Stock int      `json:"stock"`
	Tags  []string `json:"tags"`
}

type Store struct {
	products []Product
	latency  time.Duration
	conns    chan struct{}
}

func New(n int, latency time.Duration, maxConns int) *Store {
	tags := []string{"go", "web", "server", "fast", "cloud", "api"}
	products := make([]Product, n)
	for i := range products {
		products[i] = Product{
			ID:    i + 1,
			Name:  fmt.Sprintf("Product #%d", i+1),
			Price: float64(100+i*37%900) + 0.99,
			Stock: i * 13 % 250,
			Tags:  []string{tags[i%len(tags)], tags[(i+2)%len(tags)]},
		}
	}
	return &Store{
		products: products,
		latency:  latency,
		conns:    make(chan struct{}, maxConns),
	}
}

func (s *Store) Len() int { return len(s.products) }

func (s *Store) Get(ctx context.Context, id int) (Product, error) {
	if err := s.roundTrip(ctx); err != nil {
		return Product{}, err
	}
	if id < 1 || id > len(s.products) {
		return Product{}, ErrNotFound
	}
	return s.products[id-1], nil
}

func (s *Store) All(ctx context.Context) ([]Product, error) {
	if err := s.roundTrip(ctx); err != nil {
		return nil, err
	}
	return s.products, nil
}

func (s *Store) roundTrip(ctx context.Context) error {
	select {
	case s.conns <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.conns }()

	if s.latency <= 0 {
		return nil
	}
	t := time.NewTimer(s.latency)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
