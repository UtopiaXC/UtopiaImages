package http

import (
	"net/http"

	"github.com/UtopiaXC/UtopiaImages/internal/config"
	"github.com/gin-gonic/gin"
)

type Server struct {
	cfg    *config.Config
	engine *gin.Engine
}

func NewServer(cfg *config.Config) *Server {
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	s := &Server{
		cfg:    cfg,
		engine: r,
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	// 1. Health check
	s.engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"version": "v0.1.0-alpha",
		})
	})

	// 2. API v1 base route group
	api := s.engine.Group("/api/v1")
	{
		api.GET("/site", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"site_name":   "UtopiaImages",
				"description": "Next-generation self-hosted image & static asset host",
				"version":     "v0.1.0-alpha",
			})
		})
	}
}

func (s *Server) Engine() *gin.Engine {
	return s.engine
}
