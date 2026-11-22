package app

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/flosch/pongo2/v6"
)

// App owns the HTTP API, database, and in-memory Home Assistant switch state.
type App struct {
	cfg       Config
	db        *sql.DB
	templates *pongo2.TemplateSet
	mux       *http.ServeMux
	logger    *slog.Logger
	mcp       http.Handler

	haMu     sync.Mutex
	haStates map[int64]string
	haTimers map[int64]*time.Timer
}

// New creates an application, opens the SQLite database, and applies migrations.
func New(ctx context.Context, cfg Config) (*App, error) {
	if cfg.SessionSecret == "" {
		cfg.SessionSecret = randomToken(48)
	}
	dbPath := filepath.Join(cfg.DataDir, "baby.db")
	db, err := openDatabase(dbPath)
	if err != nil {
		return nil, err
	}
	templateDir := filepath.Join(cfg.WebDir, "templates")
	if _, err := os.Stat(templateDir); err != nil {
		_ = db.Close()
		return nil, err
	}
	application := &App{
		cfg:       cfg,
		db:        db,
		templates: pongo2.NewSet("baby-tracker", pongo2.MustNewLocalFileSystemLoader(templateDir)),
		mux:       http.NewServeMux(),
		logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
		haStates:  make(map[int64]string),
		haTimers:  make(map[int64]*time.Timer),
	}
	if err := application.initDatabase(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	application.mcp = application.newMCPHandler()
	application.registerRoutes()
	return application, nil
}

// Close releases the database and cancels pending Home Assistant timers.
func (a *App) Close() error {
	a.haMu.Lock()
	for _, timer := range a.haTimers {
		timer.Stop()
	}
	a.haTimers = make(map[int64]*time.Timer)
	a.haStates = make(map[int64]string)
	a.haMu.Unlock()
	return a.db.Close()
}

// Handler returns the complete middleware and route stack.
func (a *App) Handler() http.Handler {
	return a.gatewayMiddleware(a.authMiddleware(a.mux))
}

// ConnContext marks Unix domain socket connections so gateway headers are only
// trusted when they arrive through the fnOS socket.
func (a *App) ConnContext(ctx context.Context, conn net.Conn) context.Context {
	if conn.LocalAddr() != nil && conn.LocalAddr().Network() == "unix" {
		return context.WithValue(ctx, gatewayConnKey{}, true)
	}
	return ctx
}

// DB exposes the underlying database for command-line maintenance.
func (a *App) DB() *sql.DB {
	return a.db
}
