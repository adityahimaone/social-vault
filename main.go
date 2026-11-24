package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"social-vault/internal/api"
	"social-vault/internal/auth"
	"social-vault/internal/crawler"
	"social-vault/internal/db"
	"social-vault/internal/tagging"
)

//go:embed all:web/dist
var webDist embed.FS

func main() {
	addr := normalizeListenAddr(os.Getenv("PORT"))
	dataDir := envOrDefault("DATA_DIR", "./data")
	password := os.Getenv("VAULT_PASSWORD")
	apiKey := os.Getenv("VAULT_API_KEY")
	jevAPIKey := strings.TrimSpace(os.Getenv("JEV_API_KEY"))
	nsfwPassword := strings.TrimSpace(os.Getenv("NSFW_TAG_PASSWORD"))

	store, err := db.Open(dataDir)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()

	_ = store.PurgeExpiredSessions(context.Background())

	webFS, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Fatalf("mount embedded frontend: %v", err)
	}

	authenticator := auth.New(store, password, apiKey).WithNsfwPassword(nsfwPassword)
	fetcher := crawler.NewFetcher(nil)
	jevClient := tagging.NewJevClient(jevAPIKey)
	tagger := tagging.NewService(store, jevClient)
	srv := api.NewServer(store, authenticator, fetcher, webFS, tagger)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	if password == "" {
		log.Printf("warning: VAULT_PASSWORD is empty; web UI login will fail until configured")
	}
	if apiKey == "" {
		log.Printf("warning: VAULT_API_KEY is empty; API key authentication is disabled")
	}
	if jevAPIKey == "" {
		log.Printf("warning: JEV_API_KEY is empty; layer 2 Jev auto-tagging is disabled")
	}
	if nsfwPassword == "" {
		log.Printf("warning: NSFW_TAG_PASSWORD is empty; nsfw tag stays hidden and cannot be unlocked")
	}

	go func() {
		log.Printf("social-vault listening on http://%s (data=%s)", addr, dataDir)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown error: %v", err)
	}
	tagger.Wait()
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func normalizeListenAddr(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "127.0.0.1:8787"
	}
	isDigits := true
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			isDigits = false
			break
		}
	}
	if isDigits {
		return ":" + v
	}
	return v
}
