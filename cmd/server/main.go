package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	poetry "github.com/karenepitaya/curated-poetry-api"
	"github.com/karenepitaya/curated-poetry-api/internal/api"
)

var version = "dev"

func main() {
	catalog, err := poetry.Load()
	if err != nil {
		log.Fatalf("load catalog: %v", err)
	}

	address := os.Getenv("POETRY_API_ADDR")
	if address == "" {
		address = "127.0.0.1:8787"
	}
	server := &http.Server{
		Addr:              address,
		Handler:           api.New(catalog, version),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown: %v", err)
		}
	}()

	log.Printf("curated-poetry-api version=%s works=%d listening=%s", version, catalog.Count(), address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
}
