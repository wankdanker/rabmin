package api

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// slashMux is an http.ServeMux that can route a path segment that is exactly
// "%2F". ServeMux unescapes each segment before matching and never lets a
// wildcard match "/", treating it as a trailing slash, so every route for
// RabbitMQ's default vhost "/" (sent as %2F) answered 404. ServeHTTP swaps such
// segments for an escaped NUL before routing; the handlers registered with
// HandleFunc turn it back into "/" and see the original URL.
type slashMux struct{ *http.ServeMux }

const slashPlaceholder = "\x00"

type origURLKey struct{}

var wildcardRe = regexp.MustCompile(`\{([^}.]+)(?:\.\.\.)?\}`)

func newSlashMux() slashMux { return slashMux{http.NewServeMux()} }

func (m slashMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	escaped := r.URL.EscapedPath()
	if strings.Contains(strings.ToUpper(escaped+"/"), "/%2F/") {
		segs := strings.Split(escaped, "/")
		for i, s := range segs {
			if strings.EqualFold(s, "%2F") {
				segs[i] = "%00"
			}
		}
		raw := strings.Join(segs, "/")
		if path, err := url.PathUnescape(raw); err == nil {
			orig := r.URL
			u := *r.URL
			u.Path, u.RawPath = path, raw
			r = r.WithContext(context.WithValue(r.Context(), origURLKey{}, orig))
			r.URL = &u
		}
	}
	m.ServeMux.ServeHTTP(w, r)
}

func (m slashMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	m.ServeMux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if orig, ok := r.Context().Value(origURLKey{}).(*url.URL); ok {
			for _, wc := range wildcardRe.FindAllStringSubmatch(r.Pattern, -1) {
				if v := r.PathValue(wc[1]); strings.Contains(v, slashPlaceholder) {
					r.SetPathValue(wc[1], strings.ReplaceAll(v, slashPlaceholder, "/"))
				}
			}
			r.URL = orig
		}
		handler(w, r)
	})
}
