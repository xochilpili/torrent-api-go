package webserver

func (w *WebServer) loadRoutes() {
	api := w.ginger.Group("/")
	api.GET("/ping", w.PingHandler)
	search := w.ginger.Group("/search")
	{
		search.GET("/:provider/", w.SearchByProvider)
		search.GET("/all/", w.SearchAll)
	}
}
