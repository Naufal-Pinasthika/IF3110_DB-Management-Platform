package auth

import (
	"context"
	"net/http"
	"net/url"
	"time"

)

type contextKey string

const (
	// key used to store/retrieve Principal from request context
	principalContextKey contextKey = "principal"
)

type sessionContextKey struct{}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	session, ok := ctx.Value(sessionContextKey{}).(ValidatedSession)
	if !ok {
		return Principal{}, false
	}
	return session.Principal, true
}

func SetPrincipalContext(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, principal)
}

func SetValidatedPrincipalContext(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, ValidatedSession{Principal: principal})
}

func (h *Handler) SessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")

		cookie, err := r.Cookie(h.Service.Policy.CookieName)
		if err != nil {
			WriteAuthError(w, ErrUnauthenticated)
			return 
		}

		validated, err := h.Service.ValidateSession(r.Context(), cookie.Value, time.Now().UTC())
		if err != nil {
			WriteAuthError(w, err)
			return 
		}

		if validated.RotatedToken != "" {
			h.setSessionCookie(w, validated.RotatedToken, validated.Session.AbsoluteExpiresAt)
		}

		ctx := context.WithValue(r.Context(), sessionContextKey{}, validated)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// checks request origin against public origin
func (h *Handler) sameOrigin(r *http.Request) bool {
	if h.PublicOrigin == "" {
		return false
	}

	target := r.Header.Get("Origin")
	if target == "" {
		referer, err := url.Parse(r.Header.Get("Referer"))
		if err == nil && referer.Scheme != "" && referer.Host != "" {
			target = referer.Scheme + "://" + referer.Host
		}
	}

	if target == "" {
		return false
	}

	if target == h.PublicOrigin {
		return true
	}

	uTarget, err1 := url.Parse(target)
	uPublic, err2 := url.Parse(h.PublicOrigin)
	if err1 == nil && err2 == nil && uTarget.Scheme == uPublic.Scheme && uTarget.Port() == uPublic.Port() {
		h1 := uTarget.Hostname()
		h2 := uPublic.Hostname()
		if (h1 == "localhost" || h1 == "127.0.0.1") && (h2 == "localhost" || h2 == "127.0.0.1") {
			return true
		}
	}

	return false
}

func (h *Handler) LoginCSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.sameOrigin(r) {
			WriteAuthError(w, ErrCSRF)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (h *Handler) CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		validated, ok := r.Context().Value(sessionContextKey{}).(ValidatedSession)
		if !ok {
			WriteAuthError(w, ErrUnauthenticated)
			return
		}

		if !h.sameOrigin(r) {
			WriteAuthError(w, ErrCSRF)
			return
		}

		token := r.Header.Get("X-CSRF-Token")

		err := h.Service.ValidateCSRF(r.Context(), validated.Session, token)
		if err != nil {
			WriteAuthError(w, err)
			return
		}

		next.ServeHTTP(w, r)
	})
}
