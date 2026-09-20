package inspire

import (
	"net/http"
	"strings"

	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
)

// Handler exposes the public inspiration endpoint (mounted under /api/v1,
// no auth required — the homepage is public; rate limiting happens in main).
type Handler struct {
	svc *Service
}

// NewHandler creates the inspiration handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// normLocale 白名单归一，避免缓存 key 被刷爆。
func normLocale(v string) string {
	if v == "en-US" {
		return "en-US"
	}
	return "zh-CN"
}

// Chips handles GET /inspiration?locale=zh-CN — returns cached AI-generated
// chips, or an empty list when the model/cache is unavailable (frontend then
// falls back to its static chips).
func (h *Handler) Chips(w http.ResponseWriter, r *http.Request) {
	locale := normLocale(r.URL.Query().Get("locale"))
	chips, err := h.svc.Chips(r.Context(), locale)
	if err != nil {
		// 生成失败不暴露内部错误：返回空数组让前端走静态兜底
		chips = []Chip{}
	}
	if chips == nil {
		chips = []Chip{}
	}
	phttp.OK(w, http.StatusOK, map[string]any{"chips": chips, "source": "ai"})
}

// Destinations handles GET /inspiration/theme?theme=&locale= — cached
// destinations for the clicked theme chip.
func (h *Handler) Destinations(w http.ResponseWriter, r *http.Request) {
	theme := strings.TrimSpace(r.URL.Query().Get("theme"))
	if theme == "" || len([]rune(theme)) > 30 {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", "theme must be 1-30 characters")
		return
	}
	locale := normLocale(r.URL.Query().Get("locale"))
	dests, err := h.svc.Destinations(r.Context(), theme, locale)
	if err != nil {
		dests = []Destination{}
	}
	if dests == nil {
		dests = []Destination{}
	}
	phttp.OK(w, http.StatusOK, map[string]any{"theme": theme, "destinations": dests, "source": "ai"})
}
