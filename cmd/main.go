package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/xochilpili/torrent-api-go/internal/config"
	"github.com/xochilpili/torrent-api-go/internal/logger"
	"github.com/xochilpili/torrent-api-go/internal/webserver"
)

func main() {

	// load Provider config
	config := config.New()
	logger := logger.New()

	srv := webserver.New(config, logger)

	go func() {
		logger.Info().Msgf("server runnning at %s:%s", config.Host, config.Port)
		if err := srv.Web.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msgf("error while loading server: %v", err)
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	<-shutdown
	logger.Info().Msg("shutting down server.")

	if err := srv.Web.Shutdown(context.Background()); err != nil {
		logger.Fatal().Err(err).Msg("couldnt stop server")
	}
}
