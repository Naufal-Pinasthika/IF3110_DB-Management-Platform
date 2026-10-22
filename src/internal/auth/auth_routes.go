package auth

// Routing already lives in src/api/routes.go and should stay there.
// TODO Build Repository then Service then Handler in src/cmd/server/main.go.
// TODO Assign Login and Me and Logout to the existing RouteHandlers fields.
// TODO Assign session and CSRF middleware to the existing RouteMiddleware fields.
// TODO Add router dependency injection when wiring the finished implementation.
// TODO Keep composition in cmd because importing auth from api creates a cycle.
