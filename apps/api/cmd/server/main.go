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

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/ai"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/auth"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/config"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/day"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/inspire"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/location"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/mail"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/middleware"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/planner"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/share"
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
	if err := cfg.Validate(); err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
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

	rdb, err := redis.NewClient(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisTLS)
	if err != nil {
		log.Error("redis unavailable", "error", err)
		os.Exit(1)
	}
	defer rdb.Close()

	users := user.NewRepository(pool)
	refreshStore := auth.NewRefreshStore(rdb, cfg.RefreshTTL)
	verifyStore := auth.NewVerificationStore(rdb)
	mailer := mail.NewSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom)
	if mailer == nil {
		log.Warn("SMTP not configured: verification codes will be logged instead of emailed")
	}
	authHandler := auth.NewHandler(users, refreshStore, verifyStore, mailer, cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL)

	trips := trip.NewRepository(pool)
	tripHandler := trip.NewHandler(trips)

	days := day.NewRepository(pool)
	dayHandler := day.NewHandler(days, trips)
	requireAuth := auth.RequireAuth(cfg.JWTSecret)

	// AI planner: provider is optional at boot (nil when no key configured) so
	// the API still runs without LLM access; chat then returns a clear error.
	var provider ai.Provider
	if cfg.AIAPIKey != "" {
		p, err := ai.NewDeepSeek(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel)
		if err != nil {
			log.Error("failed to init AI provider", "error", err)
			os.Exit(1)
		}
		provider = p
	} else {
		log.Warn("DEEPSEEK_API_KEY not set: AI chat endpoints will be unavailable")
	}
	plannerRepo := planner.NewRepository(pool)

	// Map service: like the AI provider, optional at boot — without
	// AMAP_SERVICE_KEY the API still runs, map endpoints then return 503 and
	// activities are created without auto-geocoding.
	if cfg.MapAPIKey == "" {
		log.Warn("AMAP_SERVICE_KEY not set: map search/route endpoints will be unavailable")
	}
	locSvc := location.NewService(location.NewAmapClient(cfg.MapAPIKey), location.NewRepository(pool))
	locHandler := location.NewHandler(locSvc, days, trips)

	engine := planner.NewEngine(trips, days, plannerRepo, provider, planner.NewRedisLocker(rdb), locSvc, cfg)
	plannerHandler := planner.NewHandler(engine, plannerRepo, trips)

	shareHandler := share.NewHandler(trips, days, locSvc)

	// 首页灵感标签：AI 生成 + Redis 缓存（provider 为 nil 时返回空，前端走静态兜底）
	inspireHandler := inspire.NewHandler(inspire.NewService(provider, rdb))

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
		// 认证端点是撞库/爆破的主要目标：每 IP 5 次/分钟、突发 10 次
		authLimited := middleware.RateLimit(5.0/60.0, 10)
		api.With(authLimited).Mount("/", authHandler.Routes())
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

		// 地图端点按 IP 限流：每次搜索/路线都烧上游高德 Web 服务配额
		mapLimited := middleware.RateLimit(20.0/60.0, 10)
		authed.With(mapLimited).Get("/locations/search", locHandler.Search)
		authed.With(mapLimited).Get("/days/{dayId}/route", locHandler.DayRoute)

		// 分享：token 管理需登录，公开只读视图无需登录但限流（防爬取）
		authed.Post("/trips/{id}/share", shareHandler.Create)
		authed.Delete("/trips/{id}/share", shareHandler.Clear)
		shareLimited := middleware.RateLimit(30.0/60.0, 20)
		api.With(shareLimited).Get("/share/{token}", shareHandler.Trip)
		api.With(shareLimited).Get("/share/{token}/days/{dayId}/route", shareHandler.Route)

		// 首页灵感标签：公开 + 限流（缓存命中时零成本，未命中才打 LLM）
		inspireLimited := middleware.RateLimit(30.0/60.0, 20)
		api.With(inspireLimited).Get("/inspiration", inspireHandler.Chips)
		api.With(inspireLimited).Get("/inspiration/theme", inspireHandler.Destinations)

		// AI 端点按 IP 限流：LLM 调用成本高，约每 10s 一条消息
		aiLimited := middleware.RateLimit(6.0/60.0, 3)
		authed.With(aiLimited).Post("/trips/{id}/ai/chat", plannerHandler.Chat)
		authed.Get("/trips/{id}/ai/messages", plannerHandler.Messages)
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
