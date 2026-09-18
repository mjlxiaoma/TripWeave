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

// Chips handles GET /inspiration?locale=zh-CN — returns cached AI-generated
// chips, or an empty list when the model/cache is unavailable (frontend then
// falls back to its static chips).
func (h *Handler) Chips(w http.ResponseWriter, r *http.Request) {
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	if locale == "" {
		locale = "zh-CN"
	}
	// 限制 locale 白名单，避免缓存 key 被刷爆
	if locale != "zh-CN" && locale != "en-US" {
		locale = "zh-CN"
	}
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
