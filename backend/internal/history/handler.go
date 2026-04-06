package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	mux.HandleFunc("/myAnalyzer/api/history/search", h.handleSearch)
	mux.HandleFunc("/myAnalyzer/api/history/root-urls", h.handleRootURLs)
	mux.HandleFunc("/myAnalyzer/api/history/records/", h.handleRecordByID)
	mux.HandleFunc("/myAnalyzer/api/history/records", h.handleRecords)
	mux.HandleFunc("/myAnalyzer/api/history", h.handleBatchImport)
	return withLogging(withJSON(mux))
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeRequestError(w, r, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	if err := h.db.Ping(r.Context()); err != nil {
		h.writeRequestError(w, r, http.StatusServiceUnavailable, err.Error(), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleBatchImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeRequestError(w, r, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}

	var payload batchPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.writeRequestError(w, r, http.StatusBadRequest, fmt.Sprintf("invalid json: %v", err), err)
		return
	}

	inserted, err := h.repo.CreateBatch(r.Context(), payload)
	if err != nil {
		h.writeRequestError(w, r, http.StatusBadRequest, err.Error(), err)
		return
	}

	log.Printf("history batch import completed source=%q requested=%d inserted=%d", payload.Source, len(payload.Records), inserted)
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
			h.writeRequestError(w, r, http.StatusInternalServerError, err.Error(), err)
			return
		}

		writeJSON(w, http.StatusOK, listResponse{Items: items, Total: total, Limit: limit, Offset: offset, Search: search, RecentOnly: false})
	case http.MethodPost:
		var payload recordPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			h.writeRequestError(w, r, http.StatusBadRequest, fmt.Sprintf("invalid json: %v", err), err)
			return
		}
		if strings.TrimSpace(payload.URL) == "" {
			h.writeRequestError(w, r, http.StatusBadRequest, "url is required", nil)
			return
		}
		record, err := h.repo.Create(r.Context(), payload)
		if err != nil {
			h.writeRequestError(w, r, http.StatusBadRequest, err.Error(), err)
			return
		}
		log.Printf("history record created id=%d url=%q", record.ID, record.URL)
		writeJSON(w, http.StatusCreated, record)
	default:
		h.writeRequestError(w, r, http.StatusMethodNotAllowed, "method not allowed", nil)
	}
}

func (h *Handler) handleRecordByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/history/records/")
	if err != nil {
		h.writeRequestError(w, r, http.StatusBadRequest, err.Error(), err)
		return
	}

	switch r.Method {
	case http.MethodGet:
		record, err := h.repo.GetByID(r.Context(), id)
		if err != nil {
			h.handleRepositoryError(w, r, err, id)
			return
		}
		writeJSON(w, http.StatusOK, record)
	case http.MethodPut:
		var payload recordPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			h.writeRequestError(w, r, http.StatusBadRequest, fmt.Sprintf("invalid json: %v", err), err)
			return
		}
		if strings.TrimSpace(payload.URL) == "" {
			h.writeRequestError(w, r, http.StatusBadRequest, "url is required", nil)
			return
		}
		record, err := h.repo.Update(r.Context(), id, payload)
		if err != nil {
			h.handleRepositoryError(w, r, err, id)
			return
		}
		log.Printf("history record updated id=%d url=%q", record.ID, record.URL)
		writeJSON(w, http.StatusOK, record)
	case http.MethodDelete:
		if err := h.repo.Delete(r.Context(), id); err != nil {
			h.handleRepositoryError(w, r, err, id)
			return
		}
		log.Printf("history record deleted id=%d", id)
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
	default:
		h.writeRequestError(w, r, http.StatusMethodNotAllowed, "method not allowed", nil)
	}
}

func (h *Handler) handleRecent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeRequestError(w, r, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}

	limit := parseInt(r.URL.Query().Get("limit"), 20, 1, 100)
	items, err := h.repo.ListRecent(r.Context(), limit)
	if err != nil {
		h.writeRequestError(w, r, http.StatusInternalServerError, err.Error(), err)
		return
	}

	writeJSON(w, http.StatusOK, listResponse{Items: items, Total: len(items), Limit: limit, Offset: 0, Search: "", RecentOnly: true})
}

func (h *Handler) handleRootURLs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeRequestError(w, r, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}

	days := parseInt(r.URL.Query().Get("days"), 3, 1, 30)
	limit := parseInt(r.URL.Query().Get("limit"), 20, 1, 100)
	items, err := h.repo.ListRootURLStats(r.Context(), days, limit)
	if err != nil {
		h.writeRequestError(w, r, http.StatusInternalServerError, err.Error(), err)
		return
	}

	writeJSON(w, http.StatusOK, rootURLStatsResponse{Items: items, Days: days, Limit: limit})
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeRequestError(w, r, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}

	limit := parseInt(r.URL.Query().Get("limit"), 100, 1, 500)
	offset := parseInt(r.URL.Query().Get("offset"), 0, 0, 100000)
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	startRaw := strings.TrimSpace(r.URL.Query().Get("startTime"))
	endRaw := strings.TrimSpace(r.URL.Query().Get("endTime"))

	startTime, err := parseRFC3339Optional(startRaw)
	if err != nil {
		h.writeRequestError(w, r, http.StatusBadRequest, "startTime must be RFC3339", err)
		return
	}

	endTime, err := parseRFC3339Optional(endRaw)
	if err != nil {
		h.writeRequestError(w, r, http.StatusBadRequest, "endTime must be RFC3339", err)
		return
	}

	if startTime != nil && endTime != nil && startTime.After(*endTime) {
		h.writeRequestError(w, r, http.StatusBadRequest, "startTime must be earlier than or equal to endTime", nil)
		return
	}

	items, total, err := h.repo.SearchRecords(r.Context(), limit, offset, keyword, startTime, endTime)
	if err != nil {
		h.writeRequestError(w, r, http.StatusInternalServerError, err.Error(), err)
		return
	}

	writeJSON(w, http.StatusOK, listResponse{
		Items:      items,
		Total:      total,
		Limit:      limit,
		Offset:     offset,
		Search:     keyword,
		StartTime:  startRaw,
		EndTime:    endRaw,
		RecentOnly: false,
	})
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

func parseRFC3339Optional(raw string) (*time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	ts, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, err
	}
	utc := ts.UTC()
	return &utc, nil
}

func (h *Handler) handleRepositoryError(w http.ResponseWriter, r *http.Request, err error, id int64) {
	switch {
	case errors.Is(err, errNotFound):
		h.writeRequestError(w, r, http.StatusNotFound, err.Error(), fmt.Errorf("record id=%d: %w", id, err))
	default:
		h.writeRequestError(w, r, http.StatusBadRequest, err.Error(), fmt.Errorf("record id=%d: %w", id, err))
	}
}

func (h *Handler) writeRequestError(w http.ResponseWriter, r *http.Request, status int, message string, err error) {
	if err != nil {
		log.Printf("request failed method=%s path=%s rawQuery=%q status=%d remote=%s error=%v", r.Method, r.URL.Path, r.URL.RawQuery, status, r.RemoteAddr, err)
	} else {
		log.Printf("request rejected method=%s path=%s rawQuery=%q status=%d remote=%s message=%q", r.Method, r.URL.Path, r.URL.RawQuery, status, r.RemoteAddr, message)
	}
	writeError(w, status, message)
}

func withJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		wrapped := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		log.Printf("request completed method=%s path=%s rawQuery=%q status=%d duration=%s remote=%s", r.Method, r.URL.Path, r.URL.RawQuery, wrapped.status, time.Since(startedAt).Round(time.Millisecond), r.RemoteAddr)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message, "status": status, "success": false})
}
