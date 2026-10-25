package api

import "net/http"

type Middleware func(http.Handler) http.Handler

func NewRouter(handlers RouteHandlers, middleware RouteMiddleware) http.Handler {
	mux := http.NewServeMux()
	RegisterRoutes(mux, handlers, middleware)
	return mux
}
