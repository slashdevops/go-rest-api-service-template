package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireJSONBody(t *testing.T) {
	t.Parallel()

	handler := RequireJSONBody(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	cases := []struct {
		name        string
		method      string
		body        string
		contentType string
		want        int
	}{
		{"json_body_passes", http.MethodPost, `{}`, "application/json", http.StatusOK},
		{"json_with_charset_passes", http.MethodPut, `{}`, "application/json; charset=utf-8", http.StatusOK},
		{"form_body_is_415", http.MethodPost, `a=1`, "application/x-www-form-urlencoded", http.StatusUnsupportedMediaType},
		{"text_body_is_415", http.MethodPost, `{}`, "text/plain", http.StatusUnsupportedMediaType},
		{"undeclared_body_is_415", http.MethodPost, `{}`, "", http.StatusUnsupportedMediaType},
		{"delete_with_json_body_passes", http.MethodDelete, `{"ids":[]}`, "application/json", http.StatusOK},
		{"post_without_body_is_not_asked", http.MethodPost, ``, "", http.StatusOK},
		{"get_without_body_is_not_asked", http.MethodGet, ``, "", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var body *strings.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}

			var req *http.Request
			if body != nil {
				req = httptest.NewRequest(tc.method, "/roles", body)
			} else {
				req = httptest.NewRequest(tc.method, "/roles", nil)
			}

			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}
