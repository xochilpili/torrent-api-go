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
		Query: queryString,
		Filters: providers.ParamFilters{
			Resolution: "",
			Group:      "",
		},
	}

	if res != "" {
		params.Filters.Resolution = strings.ToLower(res)
	}

	if group != "" {
		params.Filters.Group = strings.ToLower(group)
	}

	w.logger.Info().Msgf("searching %s with filters: %s", queryString, strings.Join([]string{params.Filters.Resolution, params.Filters.Group}, ","))
	torrents, err := w.manager.FetchAllActive(c.Request.Context(), params)
	if err != nil {
		w.logger.Err(err).Msgf("error while fetching torrents: %v", err)
		c.JSON(http.StatusInternalServerError, &gin.H{"message": "error", "error": err})
		return
	}
	w.logger.Info().Msgf("resolved %d torrents", len(torrents))
	c.JSON(http.StatusOK, &gin.H{"message": "ok", "total": len(torrents), "data": torrents})
}

func (w *WebServer) SearchByProvider(c *gin.Context) {
	provider := c.Param("provider")
	query := c.Query("term")
	queryString := url.PathEscape(query)
	res := c.Query("res")
	group := c.Query("group")

	params := providers.SearchParams{
		Query: queryString,
		Filters: providers.ParamFilters{
			Resolution: "",
			Group:      "",
		},
	}

	if res != "" {
		params.Filters.Resolution = res
	}

	if group != "" {
		params.Filters.Group = group
	}

	w.logger.Info().Msgf("searching %s to provider: %s with filters: %s", queryString, provider, strings.Join([]string{params.Filters.Group, params.Filters.Resolution}, ","))
	torrents, err := w.manager.FetchByProvider(c.Request.Context(), provider, params)

	if err != nil {
		w.logger.Err(err).Msgf("error while fetching torrents: %v", err)
		c.JSON(http.StatusInternalServerError, &gin.H{"message": "error", "error": err})
		return
	}
	w.logger.Info().Msgf("resolved %d torrents for provider: %s", len(torrents), provider)
	c.JSON(http.StatusOK, &gin.H{"message": "ok", "total": len(torrents), "data": torrents})
}
