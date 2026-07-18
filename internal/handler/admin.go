package handler

import (
	"log"
	"net/http"
	"strconv"
)

const logsPerPage = 100

type adminData struct {
	Logs       []logEntryRow
	Total      int
	Page       int
	TotalPages int
}

type logEntryRow struct {
	ID        int64
	Level     string
	Message   string
	CreatedAt string
}

func (h *Handler) AdminLogs(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}

	total, err := h.DB.CountLogs()
	if err != nil {
		log.Printf("count logs: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	totalPages := (total + logsPerPage - 1) / logsPerPage
	offset := (page - 1) * logsPerPage

	entries, err := h.DB.GetLogs(logsPerPage, offset)
	if err != nil {
		log.Printf("get logs: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	rows := make([]logEntryRow, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, logEntryRow{
			ID:        e.ID,
			Level:     e.Level,
			Message:   e.Message,
			CreatedAt: e.CreatedAt.Format("Jan 02 15:04:05"),
		})
	}

	data := adminData{
		Logs:       rows,
		Total:      total,
		Page:       page,
		TotalPages: totalPages,
	}

	if err := adminTmpl.Execute(w, data); err != nil {
		log.Printf("render admin: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
