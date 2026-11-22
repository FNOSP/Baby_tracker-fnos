package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/flosch/pongo2/v6"
)

type gatewayConnKey struct{}

func (a *App) gatewayMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := a.cfg.GatewayPrefix
		originalPath := path.Clean("/" + r.URL.Path)
		trustGateway := false
		if value, _ := r.Context().Value(gatewayConnKey{}).(bool); value {
			trustGateway = true
		}
		if strings.HasPrefix(r.RemoteAddr, "@") || strings.HasPrefix(r.RemoteAddr, "unix") {
			trustGateway = true
		}

		basePath := ""
		gatewayPath := originalPath == prefix || strings.HasPrefix(originalPath, prefix+"/")
		if gatewayPath {
			basePath = prefix
			for originalPath == prefix || strings.HasPrefix(originalPath, prefix+"/") {
				originalPath = strings.TrimPrefix(originalPath, prefix)
				if originalPath == "" {
					originalPath = "/"
				}
			}
			r.URL.Path = originalPath
		} else if trustGateway {
			basePath = prefix
		}

		state := &requestState{
			BasePath: basePath,
			Gateway:  gatewayPath || trustGateway,
		}
		r = r.WithContext(context.WithValue(r.Context(), requestStateKey{}, state))
		// A prefixed URL can be reached directly over TCP. Identity headers are
		// authoritative only when the request arrived through the trusted socket.
		if trustGateway {
			state.GatewayIdentity = a.gatewayHeaderIdentity(r)
		}
		writer := &htmlRewriteWriter{ResponseWriter: w, basePath: basePath}
		defer writer.finish()
		next.ServeHTTP(writer, r)
	})
}

func (a *App) appURL(r *http.Request, target string) string {
	state := requestStateFrom(r.Context())
	if state == nil || state.BasePath == "" {
		return target
	}
	if !strings.HasPrefix(target, "/") {
		target = "/" + target
	}
	return state.BasePath + target
}

func (a *App) renderPage(w http.ResponseWriter, r *http.Request, page string, values pongo2.Context) {
	state := requestStateFrom(r.Context())
	if state == nil {
		state = &requestState{}
	}
	values["app_base"] = state.BasePath
	values["current_user"] = state.User
	values["initial"] = userInitial(state.User)
	values["gateway_login_available"] = state.GatewayIdentity != nil
	if _, ok := values["active_page"]; !ok {
		values["active_page"] = ""
	}
	template, err := a.templates.FromCache(page)
	if err != nil {
		a.logger.Error("load template", "page", page, "error", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	body, err := template.Execute(values)
	if err != nil {
		a.logger.Error("render template", "page", page, "error", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, body)
}

type htmlRewriteWriter struct {
	http.ResponseWriter
	basePath string
	status   int
	buffer   bytes.Buffer
	rewrite  bool
}

func (w *htmlRewriteWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	contentType := w.Header().Get("Content-Type")
	w.rewrite = w.basePath != "" && strings.HasPrefix(contentType, "text/html")
	if !w.rewrite {
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *htmlRewriteWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if !w.rewrite {
		return w.ResponseWriter.Write(data)
	}
	return w.buffer.Write(data)
}

func (w *htmlRewriteWriter) finish() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if !w.rewrite {
		return
	}
	body := w.buffer.Bytes()
	for _, attribute := range [][]byte{[]byte(`href="/`), []byte(`src="/`), []byte(`action="/`)} {
		replacement := append([]byte{}, attribute[:len(attribute)-2]...)
		replacement = append(replacement, []byte(`"`+w.basePath+`/`)...)
		body = bytes.ReplaceAll(body, attribute, replacement)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.ResponseWriter.WriteHeader(w.status)
	_, _ = w.ResponseWriter.Write(body)
}

func (w *htmlRewriteWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (a *App) staticHandler() http.Handler {
	staticDir := filepath.Join(a.cfg.WebDir, "static")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/static/")
		if cleanPath == "." || strings.HasPrefix(cleanPath, "../") {
			http.NotFound(w, r)
			return
		}
		if cleanPath == "manifest.json" {
			a.serveManifest(w, r)
			return
		}
		if cleanPath == "sw.js" {
			state := requestStateFrom(r.Context())
			scope := "/"
			if state != nil && state.BasePath != "" {
				scope = state.BasePath + "/"
			}
			w.Header().Set("Service-Worker-Allowed", scope)
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFile(w, r, filepath.Join(staticDir, filepath.FromSlash(cleanPath)))
	})
}

func (a *App) serveManifest(w http.ResponseWriter, r *http.Request) {
	state := requestStateFrom(r.Context())
	base := ""
	if state != nil {
		base = state.BasePath
	}
	manifest := map[string]any{
		"name":             "Baby Tracker - 新生儿出入量记录",
		"short_name":       "Baby Tracker",
		"description":      "记录新生儿喂养、排泄、症状，支持奶量估算与统计趋势",
		"start_url":        base + "/",
		"display":          "standalone",
		"background_color": "#00e5a0",
		"theme_color":      "#00e5a0",
		"orientation":      "portrait-primary",
		"icons": []map[string]any{
			{"src": base + "/static/icons/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any"},
			{"src": base + "/static/icons/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any"},
		},
	}
	writeJSON(w, http.StatusOK, manifest)
}
