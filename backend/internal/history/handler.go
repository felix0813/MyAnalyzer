package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"myanalyzer/backend/internal/database"
)

type Handler struct {
	repo *Repository
	db   *database.Client
}

func NewHandler(db *database.Client) *Handler {
	return &Handler{repo: NewRepository(db), db: db}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/myAnalyzer/healthz", h.handleHealth)
	mux.HandleFunc("/myAnalyzer/api/history/recent", h.handleRecent)
	mux.HandleFunc("/myAnalyzer/api/history/root-urls", h.handleRootURLs)
	mux.HandleFunc("/myAnalyzer/api/history/records/", h.handleRecordByID)
	mux.HandleFunc("/myAnalyzer/api/history/records", h.handleRecords)
	mux.HandleFunc("/myAnalyzer/api/history", h.handleBatchImport)
	return withJSON(mux)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := h.db.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleBatchImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var payload batchPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid json: %v", err))
		return
	}

	inserted, err := h.repo.CreateBatch(r.Context(), payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"message":        "history batch imported",
		"source":         payload.Source,
		"recordCount":    inserted,
		"requestedCount": len(payload.Records),
	})
}

func (h *Handler) handleRecords(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := parseInt(r.URL.Query().Get("limit"), 20, 1, 200)
		offset := parseInt(r.URL.Query().Get("offset"), 0, 0, 100000)
		search := strings.TrimSpace(r.URL.Query().Get("search"))

		items, total, err := h.repo.List(r.Context(), limit, offset, search)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, listResponse{Items: items, Total: total, Limit: limit, Offset: offset, Search: search, RecentOnly: false})
	case http.MethodPost:
		var payload recordPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid json: %v", err))
			return
		}
		if strings.TrimSpace(payload.URL) == "" {
			writeError(w, http.StatusBadRequest, "url is required")
			return
		}
		record, err := h.repo.Create(r.Context(), payload)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, record)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) handleRecordByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/history/records/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch r.Method {
	case http.MethodGet:
		record, err := h.repo.GetByID(r.Context(), id)
		if err != nil {
			handleRepositoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, record)
	case http.MethodPut:
		var payload recordPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid json: %v", err))
			return
		}
		if strings.TrimSpace(payload.URL) == "" {
			writeError(w, http.StatusBadRequest, "url is required")
			return
		}
		record, err := h.repo.Update(r.Context(), id, payload)
		if err != nil {
			handleRepositoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, record)
	case http.MethodDelete:
		if err := h.repo.Delete(r.Context(), id); err != nil {
			handleRepositoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) handleRecent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limit := parseInt(r.URL.Query().Get("limit"), 20, 1, 100)
	items, err := h.repo.ListRecent(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, listResponse{Items: items, Total: len(items), Limit: limit, Offset: 0, Search: "", RecentOnly: true})
}

func (h *Handler) handleRootURLs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	days := parseInt(r.URL.Query().Get("days"), 3, 1, 30)
	limit := parseInt(r.URL.Query().Get("limit"), 20, 1, 100)
	items, err := h.repo.ListRootURLStats(r.Context(), days, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, rootURLStatsResponse{Items: items, Days: days, Limit: limit})
}

func parseIDFromPath(path, prefix string) (int64, error) {
	rawID := strings.TrimPrefix(path, prefix)
	if rawID == "" || rawID == path || strings.Contains(rawID, "/") {
		return 0, errors.New("invalid record id")
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid record id")
	}
	return id, nil
}

func parseInt(raw string, fallback, min, max int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func handleRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

func withJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message, "status": status, "success": false})
}
