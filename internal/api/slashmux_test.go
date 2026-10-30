package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSlashMux(t *testing.T) {
	m := newSlashMux()
	m.HandleFunc("GET /q/{vhost}/{name}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.PathValue("vhost") + "|" + r.PathValue("name") + "|" + r.URL.EscapedPath()))
	})
	m.HandleFunc("GET /rest/{path...}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.PathValue("path")))
	})
	cases := []struct{ path, want string }{
		{"/q/%2F/tasks", "/|tasks|/q/%2F/tasks"},
		{"/q/%2f/%2F", "/|/|/q/%2f/%2F"},
		{"/q/a%2Fb/tasks", "a/b|tasks|/q/a%2Fb/tasks"},
		{"/q/shop/tasks", "shop|tasks|/q/shop/tasks"},
		{"/rest/%2F/x", "//x"},
	}
	for _, c := range cases {
		rr := httptest.NewRecorder()
		m.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rr.Code != http.StatusOK || rr.Body.String() != c.want {
			t.Errorf("%s: got %d %q, want %q", c.path, rr.Code, rr.Body.String(), c.want)
		}
	}
}
