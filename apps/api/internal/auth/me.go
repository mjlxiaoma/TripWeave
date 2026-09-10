package auth

import (
	"errors"
	"net/http"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/user"
	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
)

// Me returns the authenticated user's profile. Mounted under RequireAuth.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	u, err := h.users.ByID(r.Context(), UserIDFromContext(r.Context()))
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			phttp.Fail(w, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to load user")
		return
	}
	phttp.OK(w, http.StatusOK, toUserDTO(u))
}
