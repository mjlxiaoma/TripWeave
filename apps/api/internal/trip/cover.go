// Package trip: 旅行自定义封面(上传/获取/移除)。
// 图片存 trip_covers 表(bytea),与列表查询分离:列表只在 DTO 带 has_cover 布尔,
// 前端按需拉取二进制并长缓存,避免 N 张卡片同时拖几十 KB~几百 KB 流量。
package trip

import (
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/auth"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/shared"
	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
)

// maxCoverBytes: 前端已压缩到 ≤400KB,这里留 4MB 冗余挡异常上传。
const maxCoverBytes = 4 << 20

// 魔数嗅探白名单(不信任客户端 Content-Type 头)。
var coverContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// requireEditor 校验 tripID 合法 + 成员身份为 owner/editor,失败时已写响应。
func (h *Handler) requireEditor(w http.ResponseWriter, r *http.Request, tripID string) (string, bool) {
	if !shared.IsUUID(tripID) {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", "malformed id")
		return "", false
	}
	userID := auth.UserIDFromContext(r.Context())
	role, err := h.repo.Role(r.Context(), tripID, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			phttp.Fail(w, http.StatusNotFound, "TRIP_NOT_FOUND", "trip not found")
			return "", false
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to check access")
		return "", false
	}
	if role != "owner" && role != "editor" {
		phttp.Fail(w, http.StatusForbidden, "FORBIDDEN", "editor access required")
		return "", false
	}
	return userID, true
}

// UploadCover handles PUT /trips/{id}/cover — raw image body (前端压缩后的 JPEG/PNG/WebP)。
func (h *Handler) UploadCover(w http.ResponseWriter, r *http.Request) {
	tripID := chi.URLParam(r, "id")
	if _, ok := h.requireEditor(w, r, tripID); !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxCoverBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil || len(data) == 0 {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", "cover image required (≤4MB)")
		return
	}
	ct := http.DetectContentType(data)
	if !coverContentTypes[ct] {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", "only JPG/PNG/WebP images are supported")
		return
	}
	if err := h.repo.SaveCover(r.Context(), tripID, data, ct); err != nil {
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to save cover")
		return
	}
	phttp.OK(w, http.StatusOK, map[string]any{"has_cover": true, "bytes": len(data)})
}

// GetCover handles GET /trips/{id}/cover — 成员可见,长缓存(图片不可变,以 updated_at 区分)。
func (h *Handler) GetCover(w http.ResponseWriter, r *http.Request) {
	tripID := chi.URLParam(r, "id")
	if !shared.IsUUID(tripID) {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", "malformed id")
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if _, err := h.repo.Role(r.Context(), tripID, userID); err != nil {
		if errors.Is(err, ErrNotFound) {
			phttp.Fail(w, http.StatusNotFound, "TRIP_NOT_FOUND", "trip not found")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to check access")
		return
	}
	img, ct, updatedAt, err := h.repo.GetCover(r.Context(), tripID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			phttp.Fail(w, http.StatusNotFound, "COVER_NOT_FOUND", "no cover uploaded")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to load cover")
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("ETag", `"`+updatedAt.UTC().Format("20060102150405")+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(img)
}

// DeleteCover handles DELETE /trips/{id}/cover — 移除后卡片回退默认语义封面。
func (h *Handler) DeleteCover(w http.ResponseWriter, r *http.Request) {
	tripID := chi.URLParam(r, "id")
	if _, ok := h.requireEditor(w, r, tripID); !ok {
		return
	}
	if err := h.repo.DeleteCover(r.Context(), tripID); err != nil {
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to delete cover")
		return
	}
	phttp.OK(w, http.StatusOK, map[string]any{"has_cover": false})
}
