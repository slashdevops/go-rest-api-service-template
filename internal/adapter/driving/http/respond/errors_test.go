package respond

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteInternalErrorNeverForwardsTheCause(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/roles", nil)
	req = req.WithContext(WithRequestID(req.Context(), "01a0-req"))
	rec := httptest.NewRecorder()

	WriteInternalError(rec, req, errors.New(`ERROR: relation "roles" does not exist (SQLSTATE 42P01)`))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, "SQLSTATE") || strings.Contains(body, "relation") {
		t.Fatalf("the cause reached the client: %s", body)
	}

	var msg struct {
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
		t.Fatal(err)
	}

	if msg.Message != InternalServerErrorMessage || msg.RequestID != "01a0-req" {
		t.Fatalf("body = %+v; want the fixed message and the request id", msg)
	}
}

func TestWriteDecodeError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		err       error
		want      int
		body      string
		forbidden string // the library's words, which must not reach the client
	}{
		{"syntax_error_is_one_wording", errors.New("invalid character 'x' looking for beginning of value"), http.StatusBadRequest, DecodeErrorMessage, "invalid character"},
		{"empty_body_is_the_same_wording", errors.New("EOF"), http.StatusBadRequest, DecodeErrorMessage, "EOF"},
		{"body_cut_by_the_bound_is_413", &http.MaxBytesError{Limit: 1024}, http.StatusRequestEntityTooLarge, BodyTooLargeMessage, "1024"},
		{"wrong_type_names_the_field", &json.UnmarshalTypeError{Field: "name", Value: "number"}, http.StatusBadRequest, "field 'name' has the wrong type", "Go struct"},
		{"bad_uuid_says_so", errors.New("invalid uuid"), http.StatusBadRequest, DecodeErrorMessage + ": invalid uuid", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			WriteDecodeError(rec, httptest.NewRequest(http.MethodPost, "/roles", nil), tc.err)

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}

			body := rec.Body.String()
			if !strings.Contains(body, tc.body) {
				t.Fatalf("body = %s; want %q", body, tc.body)
			}

			if tc.forbidden != "" && strings.Contains(body, tc.forbidden) {
				t.Fatalf("body = %s; the library's text %q reached the client", body, tc.forbidden)
			}
		})
	}
}
