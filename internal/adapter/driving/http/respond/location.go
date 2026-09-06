package respond

import (
	"net/http"
	"net/url"
	"strings"
)

// publicBaseURL is the operator-declared URL clients reach the service on,
// with no trailing slash, or empty for a path-only Location.
var publicBaseURL string

// SetPublicBaseURL declares the URL clients use to reach this service. Called
// once from the composition root, from http.server.public.url.
func SetPublicBaseURL(u string) {
	publicBaseURL = strings.TrimRight(u, "/")
}

// LocationFor builds the Location of a resource beneath the request's path:
// the request path (query dropped) followed by the given segments.
//
// It used to be built from the request's Origin header, so the caller chose
// the host: measured 2026-09-06, POST /roles with "Origin: http://evil.example"
// answered 201 with "Location: http://evil.example/api/v1/roles/<id>". A client
// that follows Location was sent wherever the caller said. It also kept the
// query string, so a create through "?x=1" produced ".../roles?x=1/<id>".
//
// With http.server.public.url unset the result is a path reference, which
// RFC 9110 §10.2.2 allows and every client resolves against the request.
func LocationFor(r *http.Request, segments ...string) string {
	// RequestURI is the path the client sent, prefix included; r.URL.Path has
	// had the API prefix stripped by the time a handler runs.
	path, _, _ := strings.Cut(r.RequestURI, "?")
	path = strings.TrimRight(path, "/")

	for _, s := range segments {
		path += "/" + url.PathEscape(s)
	}

	return publicBaseURL + path
}

// SetLocation sets the Location header to [LocationFor].
func SetLocation(w http.ResponseWriter, r *http.Request, segments ...string) {
	w.Header().Set("Location", LocationFor(r, segments...))
}
