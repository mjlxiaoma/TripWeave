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
	"github.com/joho/godotenv"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/auth"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/config"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/day"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/middleware"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/trip"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/user"
	"github.com/mjlxiaoma/TripWeave/apps/api/pkg/database"
	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
	"github.com/mjlxiaoma/TripWeave/apps/api/pkg/logger"
	"github.com/mjlxiaoma/TripWeave/apps/api/pkg/redis"
)

func main() {
	// 本地开发加载 apps/api/.env；生产环境使用真实环境变量，文件不存在时静默跳过
	_ = godotenv.Load()

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

	users := user.NewRepository(pool)
	refreshStore := auth.NewRefreshStore(rdb, cfg.RefreshTTL)
	authHandler := auth.NewHandler(users, refreshStore, cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL)

	trips := trip.NewRepository(pool)
	tripHandler := trip.NewHandler(trips)

	days := day.NewRepository(pool)
	dayHandler := day.NewHandler(days, trips)
	requireAuth := auth.RequireAuth(cfg.JWTSecret)

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
			phttp.Fail(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database check failed")
			return
		}
		if err := rdb.Ping(checkCtx).Err(); err != nil {
			phttp.Fail(w, http.StatusServiceUnavailable, "REDIS_UNAVAILABLE", "redis check failed")
			return
		}
		phttp.JSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	r.Route("/api/v1", func(api chi.Router) {
		api.Mount("/", authHandler.Routes())
		api.With(requireAuth).Get("/me", authHandler.Me)
		api.With(requireAuth).Post("/auth/logout", authHandler.Logout)
		authed := api.With(requireAuth)
		authed.Get("/trips", tripHandler.List)
		authed.Post("/trips", tripHandler.Create)
		authed.Get("/trips/{id}", tripHandler.Get)
		authed.Patch("/trips/{id}", tripHandler.Update)
		authed.Delete("/trips/{id}", tripHandler.Delete)
		authed.Get("/trips/{id}/days", dayHandler.ListDays)
		authed.Post("/trips/{id}/days", dayHandler.CreateDay)
		authed.Patch("/trips/{id}/days/{dayId}", dayHandler.UpdateDay)
		authed.Delete("/trips/{id}/days/{dayId}", dayHandler.DeleteDay)
		authed.Post("/days/{dayId}/activities", dayHandler.CreateActivity)
		authed.Post("/days/{dayId}/activities/reorder", dayHandler.ReorderActivities)
		authed.Patch("/activities/{id}", dayHandler.UpdateActivity)
		authed.Delete("/activities/{id}", dayHandler.DeleteActivity)
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
