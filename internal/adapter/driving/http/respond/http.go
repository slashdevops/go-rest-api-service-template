package respond

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
)

// WriteJSONData writes the given data to the client as a JSON response.
func WriteJSONData(w http.ResponseWriter, statusCode int, data any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		return err
	}

	return nil
}

// httpMessagePool is a sync.Pool for HTTP messages to reduce memory allocations.
// in the case of high load, this can help reduce the number of allocations and improve performance.
var httpMessagePool = sync.Pool{
	New: func() any {
		return new(payload.HTTPMessage)
	},
}

// WriteJSONMessage writes a success log and response to the client with the given status code and message.
func WriteJSONMessage(w http.ResponseWriter, r *http.Request, statusCode int, message string) {
	writeJSONMessage(w, r, statusCode, "", message)
}

// WriteJSONMessageWithCode is WriteJSONMessage plus a stable, machine-readable
// reason in the "code" field.
//
// Use it only where a client genuinely has to branch on the reason — see
// [payload.HTTPMessage.Code]. A code on every response would be a second error
// vocabulary to keep in step with the first.
func WriteJSONMessageWithCode(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
	writeJSONMessage(w, r, statusCode, code, message)
}

func writeJSONMessage(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")

	mgs := httpMessagePool.Get().(*payload.HTTPMessage)
	mgs.Timestamp = time.Now()
	mgs.StatusCode = statusCode
	mgs.Message = message
	mgs.Code = code
	mgs.Method = r.Method
	mgs.Path = r.URL.Path
	mgs.RequestID = RequestIDFrom(r.Context())

	// Set the status code before writing the response body
	w.WriteHeader(statusCode)

	// Now try to write the data
	if err := json.NewEncoder(w).Encode(mgs); err != nil {
		slog.Error("failed to write JSON response", "error", err)
		// Cannot change status code after headers are sent
	}

	// Reset fields before returning to pool to avoid stale data
	mgs.Timestamp = time.Time{}
	mgs.StatusCode = 0
	mgs.Message = ""
	mgs.Code = ""
	mgs.Method = ""
	mgs.Path = ""
	mgs.RequestID = ""
	httpMessagePool.Put(mgs)

	slog.Debug(
		message,
		"status_code", statusCode,
		"method", r.Method,
		"url", r.URL.Path,
		"user_agent", r.UserAgent(),
		"remote_addr", r.RemoteAddr,
	)
}
