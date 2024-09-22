package webserver

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xochilpili/torrent-api-go/internal/providers"
)

func (w *WebServer) SearchAll(c *gin.Context) {
	query := c.Query("term")
	queryString := url.PathEscape(query)
	res := c.Query("res")
	group := c.Query("group")

	params := providers.SearchParams{
		Query:      queryString,
		Resolution: "",
		Group:      "",
	}

	if res != "" {
		params.Resolution = res
	}
	if group != "" {
		params.Group = group
	}

	w.logger.Info().Msgf("searching %s with filters: %s", queryString, strings.Join([]string{params.Resolution, params.Group}, ","))
	torrents, err := w.manager.FetchAllActive(c.Request.Context(), params)
	if err != nil {
		w.logger.Err(err).Msgf("error while fetching torrents: %v", err)
		c.JSON(http.StatusInternalServerError, &gin.H{"message": "error", "error": err})
	}
	w.logger.Info().Msgf("resolved %d torrents", len(torrents))
	c.JSON(http.StatusOK, &gin.H{"message": "ok", "torrents": torrents})
}

func (w *WebServer) SearchByProvider(c *gin.Context) {
	provider := c.Param("provider")
	query := c.Query("term")
	queryString := url.PathEscape(query)
	res := c.Query("res")
	group := c.Query("group")

	params := providers.SearchParams{
		Query:      queryString,
		Resolution: "",
		Group:      "",
	}

	if res != "" {
		params.Resolution = res
	}

	if group != "" {
		params.Group = group
	}

	w.logger.Info().Msgf("searching %s to provider: %s with filters: %s", queryString, provider, strings.Join([]string{params.Group, params.Resolution}, ","))
	torrents, err := w.manager.FetchByProvider(c.Request.Context(), provider, params)

	if err != nil {
		w.logger.Err(err).Msgf("error while fetching torrents: %v", err)
		c.JSON(http.StatusInternalServerError, &gin.H{"message": "error", "error": err})
	}
	w.logger.Info().Msgf("resolved %d torrents for provider: %s", len(torrents), provider)
	c.JSON(http.StatusOK, &gin.H{"message": "ok", "torrents": torrents})
}
