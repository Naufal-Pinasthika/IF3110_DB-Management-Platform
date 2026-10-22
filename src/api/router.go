package api

import (
	"net/http"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/health"
)

type Middleware func(http.Handler) http.Handler

func NewRouter() http.Handler {
	handlers := RouteHandlers{
		Health: http.HandlerFunc(health.Health),
	}

	middleware := RouteMiddleware{}

	mux := http.NewServeMux()
	RegisterRoutes(mux, handlers, middleware)
	return  mux
}
