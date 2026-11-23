package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"social-vault/internal/mcp"
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	cfg, err := mcp.LoadConfigFromEnv(os.Getenv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "social-vault-mcp: configuration error: %v\n", err)
		os.Exit(1)
	}

	client, err := mcp.NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "social-vault-mcp: client init error: %v\n", err)
		os.Exit(1)
	}

	srv := mcp.NewServer(client)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx, &sdkmcp.StdioTransport{}); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
			return
		}
		fmt.Fprintf(os.Stderr, "social-vault-mcp: server error: %v\n", err)
		os.Exit(1)
	}
}
