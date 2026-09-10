// Command server is the TripWeave API entrypoint.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/config"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/middleware"
	"github.com/mjlxiaoma/TripWeave/apps/api/pkg/database"
	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
	"github.com/mjlxiaoma/TripWeave/apps/api/pkg/logger"
	"github.com/mjlxiaoma/TripWeave/apps/api/pkg/redis"
)

func main() {
	log := logger.New()
	slog.SetDefault(log)

	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.RunMigrate {
		if err := database.Migrate(cfg.DatabaseURL, cfg.MigrateDir); err != nil {
			log.Error("migration failed", "error", err)
			os.Exit(1)
		}
		log.Info("migrations applied")
	}

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	rdb, err := redis.NewClient(ctx, cfg.RedisAddr, cfg.RedisPassword)
	if err != nil {
		log.Error("redis unavailable", "error", err)
		os.Exit(1)
	}
	defer rdb.Close()

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recover(log))
	r.Use(middleware.Logging(log))
	r.Use(middleware.CORS(cfg.CORSOrigins))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		phttp.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/ready", func(w http.ResponseWriter, req *http.Request) {
		checkCtx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
		defer cancel()
		if err := pool.Ping(checkCtx); err != nil {
			phttp.Error(w, http.StatusServiceUnavailable, "db_unavailable", "database check failed")
			return
		}
		if err := rdb.Ping(checkCtx).Err(); err != nil {
			phttp.Error(w, http.StatusServiceUnavailable, "redis_unavailable", "redis check failed")
			return
		}
		phttp.JSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("server listening", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
		_ = srv.Close()
	}
	log.Info("server stopped")
}
