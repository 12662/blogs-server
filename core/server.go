package core

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"server/global"
	"server/initialize"
	"syscall"
	"time"

	"go.uber.org/zap"
	// "server/service"
)

type server interface {
	ListenAndServe() error
}

type gracefulServer interface {
	Shutdown(ctx context.Context) error
}

// RunServer starts Gin and waits until the HTTP server exits or the process
// receives SIGINT/SIGTERM. On Windows this gives the normal http.Server the same
// graceful shutdown path that Linux gets from process signals.
func RunServer() {
	addr := global.Config.System.Addr()
	router := initialize.InitRouter()

	// 加载所有的 JWT 黑名单，存入本地缓存
	// TODO service.LoadAll()

	s := initServer(addr, router)
	global.Log.Info("server run success on ", zap.String("address", addr))

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- s.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	select {
	case sig := <-stop:
		global.Log.Info("server shutdown signal received", zap.String("signal", sig.String()))
		if gs, ok := s.(gracefulServer); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := gs.Shutdown(ctx); err != nil {
				global.Log.Error("server graceful shutdown failed", zap.Error(err))
			}
		}
		if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			global.Log.Error("server stopped with error", zap.Error(err))
		}
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			global.Log.Error("server stopped with error", zap.Error(err))
		}
	}
}
