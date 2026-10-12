package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"godemo/internal/app"
	"godemo/internal/store"
)

func main() {
	mode := flag.String("mode", "naive", "naive или fast")
	addr := flag.String("addr", "localhost:8080", "адрес сервиса")
	pprofAddr := flag.String("pprof", "localhost:6060", "адрес профилировщика")
	flag.Parse()

	db := store.New(500, 5*time.Millisecond, 10)

	go func() { log.Println(http.ListenAndServe(*pprofAddr, nil)) }()

	log.Printf("mode=%s addr=%s pprof=%s cpus=%d", *mode, *addr, *pprofAddr, runtime.NumCPU())

	switch *mode {
	case "naive":
		log.Fatal(http.ListenAndServe(*addr, app.NewNaive(db).Routes()))
	case "fast":
		runFast(*addr, db)
	default:
		log.Fatalf("unknown mode %q", *mode)
	}
}

func runFast(addr string, db *store.Store) {
	a := app.NewFast(db)

	srv := &http.Server{
		Addr:              addr,
		Handler:           a.Routes(),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	log.Println("shutdown: stop accepting, finishing active requests...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	a.Close()
	log.Println("shutdown: done")
}
