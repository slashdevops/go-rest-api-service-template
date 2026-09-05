package respond

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
)

func TestWriteJSONData_Success(t *testing.T) {
	rec := httptest.NewRecorder()
	data := map[string]string{"foo": "bar"}

	err := WriteJSONData(rec, http.StatusOK, data)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp map[string]string
	err = json.NewDecoder(rec.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Equal(t, data, resp)
}

func TestWriteJSONData_EncodeError(t *testing.T) {
	rec := httptest.NewRecorder()
	// json.Encoder will fail on channel type
	ch := make(chan int)
	err := WriteJSONData(rec, http.StatusOK, ch)
	assert.Error(t, err)
}

func TestWriteJSONData_HeaderAndStatusOnError(t *testing.T) {
	rec := httptest.NewRecorder()
	// Use a type that cannot be marshaled to JSON
	invalid := make(chan int)
	err := WriteJSONData(rec, http.StatusTeapot, invalid)
	assert.Error(t, err)
	// Even on error, headers and status should be set
	assert.Equal(t, http.StatusTeapot, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}

func TestWriteJSONMessage_Success(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.Header.Set("User-Agent", "test-agent")

	WriteJSONMessage(rec, req, http.StatusCreated, "created successfully")
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var msg payload.HTTPMessage
	body, err := io.ReadAll(rec.Body)
	assert.NoError(t, err)

	err = json.Unmarshal(body, &msg)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, msg.StatusCode)
	assert.Equal(t, "created successfully", msg.Message)
	assert.Equal(t, req.Method, msg.Method)
	assert.Equal(t, req.URL.Path, msg.Path)
	assert.WithinDuration(t, time.Now(), msg.Timestamp, time.Second)
}

func TestWriteJSONMessage_PoolReuse(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/pool", nil)
	WriteJSONMessage(rec, req, http.StatusOK, "pool test")
	// The object should be returned to the pool and reused
	msg1 := httpMessagePool.Get().(*payload.HTTPMessage)
	httpMessagePool.Put(msg1)
	msg2 := httpMessagePool.Get().(*payload.HTTPMessage)
	assert.Equal(t, msg1, msg2)
	httpMessagePool.Put(msg2)
}

func TestWriteJSONMessage_AllFieldsSet(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/allfields?foo=bar", nil)
	req.Header.Set("User-Agent", "coverage-agent")
	req.RemoteAddr = "127.0.0.1:12345"
	WriteJSONMessage(rec, req, http.StatusAccepted, "all fields set")
	var msg payload.HTTPMessage
	err := json.NewDecoder(rec.Body).Decode(&msg)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, msg.StatusCode)
	assert.Equal(t, "all fields set", msg.Message)
	assert.Equal(t, req.Method, msg.Method)
	assert.Equal(t, req.URL.Path, msg.Path)
	assert.WithinDuration(t, time.Now(), msg.Timestamp, time.Second)
}
