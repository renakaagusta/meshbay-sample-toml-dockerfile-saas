package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const maximumNameLength = 200

type itemRecord struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ProcessedAt *time.Time `json:"processed_at"`
}

type itemPayload struct {
	Name string `json:"name"`
}

const itemColumns = `id, name, status, created_at, processed_at`

func scanItem(row pgx.Row) (itemRecord, error) {
	var record itemRecord
	failure := row.Scan(&record.ID, &record.Name, &record.Status, &record.CreatedAt, &record.ProcessedAt)
	return record, failure
}

func (application *Application) handleListItems(responseWriter http.ResponseWriter, httpRequest *http.Request) {
	rows, queryError := application.Database.Query(httpRequest.Context(),
		`SELECT `+itemColumns+` FROM items ORDER BY id`)
	if queryError != nil {
		writeError(responseWriter, http.StatusInternalServerError, queryError.Error())
		return
	}
	defer rows.Close()
	items := []itemRecord{}
	for rows.Next() {
		record, failure := scanItem(rows)
		if failure != nil {
			writeError(responseWriter, http.StatusInternalServerError, failure.Error())
			return
		}
		items = append(items, record)
	}
	if failure := rows.Err(); failure != nil {
		writeError(responseWriter, http.StatusInternalServerError, failure.Error())
		return
	}
	writeJSON(responseWriter, http.StatusOK, items)
}

func (application *Application) handleCreateItem(responseWriter http.ResponseWriter, httpRequest *http.Request) {
	name, valid := readItemName(responseWriter, httpRequest)
	if !valid {
		return
	}
	record, scanError := scanItem(application.Database.QueryRow(httpRequest.Context(),
		`INSERT INTO items (name) VALUES ($1) RETURNING `+itemColumns, name))
	if scanError != nil {
		writeError(responseWriter, http.StatusInternalServerError, scanError.Error())
		return
	}
	if failure := application.enqueue(httpRequest.Context(), record); failure != nil {
		// A row nobody will ever process is worse than no row: roll it back so
		// the caller sees the queue failure instead of a stuck item.
		if _, deleteFailure := application.Database.Exec(httpRequest.Context(),
			`DELETE FROM items WHERE id = $1`, record.ID); deleteFailure != nil {
			application.Logger.Printf("rollback item %d: %v", record.ID, deleteFailure)
		}
		writeError(responseWriter, http.StatusInternalServerError, "queue unavailable: "+failure.Error())
		return
	}
	application.Logger.Printf("queued item %d", record.ID)
	writeJSON(responseWriter, http.StatusCreated, record)
}

func (application *Application) handleGetItem(responseWriter http.ResponseWriter, httpRequest *http.Request) {
	itemID, valid := parseItemID(responseWriter, httpRequest)
	if !valid {
		return
	}
	record, scanError := scanItem(application.Database.QueryRow(httpRequest.Context(),
		`SELECT `+itemColumns+` FROM items WHERE id = $1`, itemID))
	if errors.Is(scanError, pgx.ErrNoRows) {
		writeError(responseWriter, http.StatusNotFound, "item not found")
		return
	}
	if scanError != nil {
		writeError(responseWriter, http.StatusInternalServerError, scanError.Error())
		return
	}
	writeJSON(responseWriter, http.StatusOK, record)
}

func (application *Application) handleUpdateItem(responseWriter http.ResponseWriter, httpRequest *http.Request) {
	itemID, valid := parseItemID(responseWriter, httpRequest)
	if !valid {
		return
	}
	name, valid := readItemName(responseWriter, httpRequest)
	if !valid {
		return
	}
	record, scanError := scanItem(application.Database.QueryRow(httpRequest.Context(),
		`UPDATE items SET name = $2 WHERE id = $1 RETURNING `+itemColumns, itemID, name))
	if errors.Is(scanError, pgx.ErrNoRows) {
		writeError(responseWriter, http.StatusNotFound, "item not found")
		return
	}
	if scanError != nil {
		writeError(responseWriter, http.StatusInternalServerError, scanError.Error())
		return
	}
	writeJSON(responseWriter, http.StatusOK, record)
}

func (application *Application) handleDeleteItem(responseWriter http.ResponseWriter, httpRequest *http.Request) {
	itemID, valid := parseItemID(responseWriter, httpRequest)
	if !valid {
		return
	}
	result, deleteError := application.Database.Exec(httpRequest.Context(),
		`DELETE FROM items WHERE id = $1`, itemID)
	if deleteError != nil {
		writeError(responseWriter, http.StatusInternalServerError, deleteError.Error())
		return
	}
	if result.RowsAffected() == 0 {
		writeError(responseWriter, http.StatusNotFound, "item not found")
		return
	}
	responseWriter.WriteHeader(http.StatusNoContent)
}

// handleProcessItem is the worker's callback: queued -> processed. It is
// idempotent so a worker that retries after a timeout does not fail.
func (application *Application) handleProcessItem(responseWriter http.ResponseWriter, httpRequest *http.Request) {
	itemID, valid := parseItemID(responseWriter, httpRequest)
	if !valid {
		return
	}
	record, scanError := scanItem(application.Database.QueryRow(httpRequest.Context(),
		`UPDATE items SET status = 'processed', processed_at = now()
		 WHERE id = $1 AND status = 'queued' RETURNING `+itemColumns, itemID))
	if errors.Is(scanError, pgx.ErrNoRows) {
		existing, readError := scanItem(application.Database.QueryRow(httpRequest.Context(),
			`SELECT `+itemColumns+` FROM items WHERE id = $1`, itemID))
		if errors.Is(readError, pgx.ErrNoRows) {
			writeError(responseWriter, http.StatusNotFound, "item not found")
			return
		}
		if readError != nil {
			writeError(responseWriter, http.StatusInternalServerError, readError.Error())
			return
		}
		writeJSON(responseWriter, http.StatusOK, existing)
		return
	}
	if scanError != nil {
		writeError(responseWriter, http.StatusInternalServerError, scanError.Error())
		return
	}
	application.Logger.Printf("processed item %d", record.ID)
	writeJSON(responseWriter, http.StatusOK, record)
}

func readItemName(responseWriter http.ResponseWriter, httpRequest *http.Request) (string, bool) {
	var payload itemPayload
	if decodeError := json.NewDecoder(httpRequest.Body).Decode(&payload); decodeError != nil {
		writeError(responseWriter, http.StatusBadRequest, "invalid JSON body")
		return "", false
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		writeError(responseWriter, http.StatusBadRequest, "name is required")
		return "", false
	}
	if len(name) > maximumNameLength {
		writeError(responseWriter, http.StatusBadRequest, "name is too long")
		return "", false
	}
	return name, true
}

func parseItemID(responseWriter http.ResponseWriter, httpRequest *http.Request) (int64, bool) {
	itemID, parseError := strconv.ParseInt(httpRequest.PathValue("itemID"), 10, 64)
	if parseError != nil || itemID <= 0 {
		writeError(responseWriter, http.StatusBadRequest, "item id must be a positive integer")
		return 0, false
	}
	return itemID, true
}
