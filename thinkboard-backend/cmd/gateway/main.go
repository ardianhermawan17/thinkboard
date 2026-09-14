// Command gateway is the single binary: Gin router + graceful shutdown (03-backend-folder-architecture.md §2, §7.10).
package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"

	"thinkboard-backend/internal/httpapi"
	"thinkboard-backend/internal/platform/config"
	"thinkboard-backend/internal/platform/postgres"
)

func main() {
	cfg := config.Load()
	gin.SetMode(cfg.GinMode)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		log.Fatalf("postgres pool: %v", err)
	}
	defer pool.Close()

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: httpapi.NewRouter(cfg, pool),
	}

	go func() {
		log.Printf("gateway listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	stop()
	log.Println("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
}
