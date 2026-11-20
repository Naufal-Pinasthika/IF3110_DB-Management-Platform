package jobs

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/access"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/contracts"
)

type Handler struct{ Service *Service }

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	options := ListOptions{Status: contracts.JobStatus(r.URL.Query().Get("status"))}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			writeJobError(w, apperror.ErrInvalidInput)
			return
		}
		options.Limit = n
	}
	if raw := r.URL.Query().Get("before"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 1 {
			writeJobError(w, apperror.ErrInvalidInput)
			return
		}
		options.Before = n
	}
	actor, _ := auth.PrincipalFromContext(r.Context())
	page, err := h.Service.List(r.Context(), actor, r.PathValue("dbId"), options)
	if err != nil {
		writeJobError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	api.WriteDataResponse(w, http.StatusOK, page)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.PrincipalFromContext(r.Context())
	job, err := h.Service.Get(r.Context(), actor, r.PathValue("dbId"), r.PathValue("jobId"))
	if err != nil {
		writeJobError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	api.WriteDataResponse(w, http.StatusOK, job)
}

func writeJobError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrUnauthenticated), errors.Is(err, access.ErrUnauthenticated):
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
	case errors.Is(err, access.ErrForbidden), errors.Is(err, apperror.ErrForbidden):
		api.WriteAPIError(w, http.StatusForbidden, "FORBIDDEN", "Insufficient database access")
	case errors.Is(err, apperror.ErrNotFound), errors.Is(err, access.ErrNotFound):
		api.WriteAPIError(w, http.StatusNotFound, "NOT_FOUND", "Job not found")
	case errors.Is(err, apperror.ErrInvalidInput):
		api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid job reference, status, or pagination")
	case errors.Is(err, apperror.ErrConflict), errors.Is(err, ErrClaimLost):
		api.WriteAPIError(w, http.StatusConflict, "CONFLICT", "Job state conflict")
	case errors.Is(err, ErrUnavailable):
		api.WriteAPIError(w, http.StatusServiceUnavailable, "JOBS_NOT_CONFIGURED", "Migration execution integration is unavailable")
	default:
		api.WriteAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Job operation failed")
	}
}
