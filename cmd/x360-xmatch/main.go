// Command x360-xmatch serves Xbox Live matchmaking, Live service 6 (see internal/xmatch).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openpak/x360-common/httplog"
	"github.com/openpak/x360-common/wire"
	"github.com/openpak/x360-xmatch/internal/xmatch"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	addr := os.Getenv("X360_XMATCH_ADDR")
	if addr == "" {
		addr = ":10006"
	}
	h := &xmatch.Handler{Store: xmatch.NewStore(), Log: log}
	srv := &http.Server{Addr: addr, Handler: httplog.Wrap(wire.Guard(os.Getenv("X360_GATEWAY_KEY"), h)), ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(sc)
	}()
	log.Info("x360-xmatch listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}
