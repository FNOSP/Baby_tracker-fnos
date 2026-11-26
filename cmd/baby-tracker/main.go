package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"baby-tracker/internal/app"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: baby-tracker <serve|bootstrap|reset-password>")
		os.Exit(2)
	}
	cfg := app.LoadConfig()
	switch os.Args[1] {
	case "serve":
		if err := serve(cfg); err != nil {
			slog.Error("server stopped", "error", err)
			os.Exit(1)
		}
	case "bootstrap":
		if err := bootstrap(cfg); err != nil {
			slog.Error("bootstrap failed", "error", err)
			os.Exit(1)
		}
	case "reset-password":
		if err := resetPassword(cfg); err != nil {
			slog.Error("reset password failed", "error", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}
}

func bootstrap(cfg app.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	application, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	return application.Close()
}

func resetPassword(cfg app.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	application, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer application.Close()
	username, password, err := application.ResetAdminPassword(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("管理员 [%s] 密码已重置\n", username)
	fmt.Printf("新密码: %s\n", password)
	return nil
}

func serve(cfg app.Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	application, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer application.Close()

	server := &http.Server{
		Handler:           application.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
		ConnContext:       application.ConnContext,
	}
	listeners := make([]net.Listener, 0, 2)
	cleanup := func() {}
	if cfg.UnixSocket != "" {
		listener, socketCleanup, err := unixListener(cfg.UnixSocket)
		if err != nil {
			return err
		}
		listeners = append(listeners, listener)
		cleanup = socketCleanup
	}
	if cfg.ListenAddress != "" {
		listener, err := net.Listen("tcp", cfg.ListenAddress)
		if err != nil {
			cleanup()
			return fmt.Errorf("listen on %s: %w", cfg.ListenAddress, err)
		}
		listeners = append(listeners, listener)
	}
	if len(listeners) == 0 {
		listener, err := net.Listen("tcp", "127.0.0.1:5000")
		if err != nil {
			return err
		}
		listeners = append(listeners, listener)
		slog.Info("no listener configured; using development fallback", "address", "127.0.0.1:5000")
	}

	errorsChannel := make(chan error, len(listeners))
	for _, listener := range listeners {
		listener := listener
		slog.Info("listening", "network", listener.Addr().Network(), "address", listener.Addr().String())
		go func() {
			if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errorsChannel <- err
			}
		}()
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case sig := <-signals:
		slog.Info("shutdown requested", "signal", sig.String())
	case err := <-errorsChannel:
		cancel()
		_ = server.Close()
		cleanup()
		return err
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	err = server.Shutdown(shutdownCtx)
	cleanup()
	return err
}

func unixListener(socketPath string) (net.Listener, func(), error) {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o750); err != nil {
		return nil, func() {}, err
	}
	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, func() {}, fmt.Errorf("gateway socket path exists and is not a socket: %s", socketPath)
		}
		if err := os.Remove(socketPath); err != nil {
			return nil, func() {}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, func() {}, err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, func() {}, fmt.Errorf("listen on unix socket: %w", err)
	}
	if err := os.Chmod(socketPath, 0o660); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return nil, func() {}, err
	}
	cleanup := func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}
	return listener, cleanup, nil
}
