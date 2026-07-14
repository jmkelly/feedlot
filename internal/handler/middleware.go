package handler

import (
	"context"
	"net/http"

	"github.com/james/feedlot/internal/auth"
	"github.com/james/feedlot/internal/db"
)

type contextKey string

const userIDKey contextKey = "user_id"

type Handler struct {
	DB   *db.DB
	Auth *auth.Auth
}

func New(database *db.DB, a *auth.Auth) *Handler {
	return &Handler{
		DB:   database,
		Auth: a,
	}
}

// RequireAuth is HTTP middleware that checks for a valid JWT in the feedlot_token cookie.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("feedlot_token")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		userID, err := h.Auth.ValidateToken(cookie.Value)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserID extracts the user ID from the request context.
// Must only be called after RequireAuth middleware.
func GetUserID(r *http.Request) int64 {
	uid, _ := r.Context().Value(userIDKey).(int64)
	return uid
}

// OptionalAuth is middleware that checks for a valid JWT but doesn't require it.
func (h *Handler) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("feedlot_token")
		if err == nil {
			userID, err := h.Auth.ValidateToken(cookie.Value)
			if err == nil {
				ctx := context.WithValue(r.Context(), userIDKey, userID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// GetUserIDOrDefault returns the user ID from context, or 0 if not set.
func GetUserIDOrDefault(r *http.Request) int64 {
	uid, _ := r.Context().Value(userIDKey).(int64)
	return uid
}
