package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sleepcast/internal/cleanup"
	"sleepcast/internal/config"
	"sleepcast/internal/server"
	"sleepcast/internal/storage"
	ytint "sleepcast/internal/youtube"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("mkdir data: %v", err)
	}

	db, err := storage.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	media, err := storage.NewMedia(cfg.DataDir)
	if err != nil {
		log.Fatalf("media: %v", err)
	}

	cleaner := &cleanup.Cleaner{DB: db, Media: media, TTLHours: cfg.TTLHours}
	if err := cleaner.Reconcile(); err != nil {
		log.Printf("reconcile: %v", err)
	}

	srv := &server.Server{
		DB:      db,
		Media:   media,
		Jobs:    ytint.NewJobTracker(),
		Cleaner: cleaner,
		WebDir:  "web",
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go cleaner.Run(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		log.Println("shutting down")
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	log.Printf("sleepcast listening on %s", cfg.Addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http: %v", err)
	}
}
