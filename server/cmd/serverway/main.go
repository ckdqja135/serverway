// serverway: 게임 서버
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"serverway/internal/game"
	"serverway/internal/sim"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func main() {
	var data []byte
	if p := os.Getenv("SERVERWAY_BALANCE"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			log.Fatal(err)
		}
		data = b
	}
	cfg, err := sim.Parse(data)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	srv := &http.Server{Addr: env("LISTEN", "127.0.0.1:7800"), ReadHeaderTimeout: 5 * time.Second,
		Handler: game.Handler(game.NewManager(ctx, cfg), env("SERVERWAY_WEB", "../web/dist"))}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(c)
	}()
	log.Printf("Serverway http://%s", srv.Addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
