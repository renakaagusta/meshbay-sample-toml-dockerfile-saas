package app

import (
	"net/http"
)

type statsResponse struct {
	Queued    int64 `json:"queued"`
	Processed int64 `json:"processed"`
	Total     int64 `json:"total"`
}

// handleStats counts items per status, which is how the smoke script proves the
// worker actually drained the queue.
func (application *Application) handleStats(responseWriter http.ResponseWriter, httpRequest *http.Request) {
	rows, queryError := application.Database.Query(httpRequest.Context(),
		`SELECT status, count(*) FROM items GROUP BY status`)
	if queryError != nil {
		writeError(responseWriter, http.StatusInternalServerError, queryError.Error())
		return
	}
	defer rows.Close()
	payload := statsResponse{}
	for rows.Next() {
		var status string
		var count int64
		if scanError := rows.Scan(&status, &count); scanError != nil {
			writeError(responseWriter, http.StatusInternalServerError, scanError.Error())
			return
		}
		switch status {
		case "queued":
			payload.Queued = count
		case "processed":
			payload.Processed = count
		}
		payload.Total += count
	}
	if rowsError := rows.Err(); rowsError != nil {
		writeError(responseWriter, http.StatusInternalServerError, rowsError.Error())
		return
	}
	writeJSON(responseWriter, http.StatusOK, payload)
}
