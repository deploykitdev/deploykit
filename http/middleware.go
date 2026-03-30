package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/heyjorgedev/deploykit"
)

type contextKey string

const userContextKey contextKey = "user"

// NewContextWithUser returns a new context with the given user attached.
func NewContextWithUser(ctx context.Context, user *deploykit.User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// UserFromContext returns the authenticated user from the context, or nil.
func UserFromContext(ctx context.Context) *deploykit.User {
	user, _ := ctx.Value(userContextKey).(*deploykit.User)
	return user
}

// authenticate is middleware that validates the Bearer token from the
// Authorization header and injects the authenticated user into the context.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			s.errorResponse(w, r, deploykit.Errorf(deploykit.EUNAUTHORIZED, "Authentication required."))
			return
		}

		// Try access token first, then API key.
		user, err := s.AuthService.ValidateAccessToken(r.Context(), token)
		if err != nil {
			user, err = s.AuthService.ValidateAPIKey(r.Context(), token)
		}
		if err != nil {
			s.errorResponse(w, r, deploykit.Errorf(deploykit.EUNAUTHORIZED, "Invalid or expired token."))
			return
		}

		ctx := NewContextWithUser(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// cors is middleware that sets CORS headers for SPA cross-origin requests.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", s.CORSOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// extractBearerToken extracts the token from an "Authorization: Bearer <token>" header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}
