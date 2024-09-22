package webserver

import (
	"net/http"

	ginlogger "github.com/gin-contrib/logger"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/xochilpili/torrent-api-go/internal/config"
	"github.com/xochilpili/torrent-api-go/internal/providers"
)

type WebServer struct {
	config  *config.Config
	logger  *zerolog.Logger
	Web     *http.Server
	ginger  *gin.Engine
	manager *providers.TorrentManager
}

func New(config *config.Config, logger *zerolog.Logger) *WebServer {
	ginger := gin.New()
	ginger.Use(gin.Recovery())
	ginger.Use(ginlogger.SetLogger(
		ginlogger.WithSkipPath([]string{"/ping"}),
		ginlogger.WithLogger(func(ctx *gin.Context, l zerolog.Logger) zerolog.Logger {
			return logger.Output(gin.DefaultWriter).With().Logger()
		}),
	))

	httpSrv := &http.Server{
		Addr:    config.Host + ":" + config.Port,
		Handler: ginger,
	}

	manager := providers.NewTorrentManager(logger)
	srv := &WebServer{
		config:  config,
		logger:  logger,
		Web:     httpSrv,
		ginger:  ginger,
		manager: manager,
	}
	srv.loadRoutes()
	return srv
}
