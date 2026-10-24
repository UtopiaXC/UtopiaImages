package http

import (
	"fmt"
	"net/http"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/cache"
	"github.com/UtopiaXC/UtopiaImages/internal/config"
	"github.com/UtopiaXC/UtopiaImages/internal/imaging"
	"github.com/UtopiaXC/UtopiaImages/internal/repository"
	"github.com/UtopiaXC/UtopiaImages/internal/service"
	"github.com/UtopiaXC/UtopiaImages/internal/storage"
	"github.com/UtopiaXC/UtopiaImages/web"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Server struct {
	cfg             *config.Config
	db              *gorm.DB
	engine          *gin.Engine
	dispatchService *service.DispatchService
}

func NewServer(cfg *config.Config) (*Server, error) {
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 1. Initialize Database
	db, err := repository.InitDB(cfg)
	if err != nil {
		return nil, fmt.Errorf("database init failed: %w", err)
	}

	// 2. Initialize Storage Driver
	stDriver, err := storage.NewLocalStorageDriver(cfg.Storage.Local.BasePath)
	if err != nil {
		return nil, fmt.Errorf("storage driver init failed: %w", err)
	}

	// 3. Initialize Image Processor & Cache
	processor := imaging.NewPureGoProcessor()
	derivedCache, err := cache.NewDerivedCache(cfg.Cache.Dir)
	if err != nil {
		return nil, fmt.Errorf("derived cache init failed: %w", err)
	}

	// 4. Initialize Auth components
	sessionManager := auth.NewSessionManager(db)
	totpManager := auth.NewTOTPManager(cfg.Security.SecretKey)
	captchaManager := auth.NewCaptchaManager(cfg.Security.SecretKey, cfg.Security.Captcha.TurnstileSecretKey)

	// 5. Initialize Services
	authService := service.NewAuthService(db, sessionManager, totpManager, captchaManager)
	uploadService := service.NewUploadService(db, stDriver, processor)
	dispatchService := service.NewDispatchService(db, stDriver, processor, derivedCache)
	importService := service.NewImportService(db, uploadService)

	// 6. Initialize Controllers
	authController := NewAuthController(authService)
	fileController := NewFileController(db, uploadService, dispatchService, importService, cfg.Server.BaseURL)
	adminController := NewAdminController(db)
	watermarkController := NewWatermarkController(db, processor)
	settingsController := NewSettingsController(db, derivedCache)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	// Attach authentication context on every request
	r.Use(auth.AuthMiddleware(sessionManager))

	s := &Server{
		cfg:             cfg,
		db:              db,
		engine:          r,
		dispatchService: dispatchService,
	}

	s.setupRoutes(authController, fileController, adminController, watermarkController, settingsController)
	return s, nil
}

func (s *Server) setupRoutes(
	authCtrl *AuthController,
	fileCtrl *FileController,
	adminCtrl *AdminController,
	wmCtrl *WatermarkController,
	setCtrl *SettingsController,
) {
	// Health check
	s.engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"version": "v0.3.0",
		})
	})

	// API v1 route group
	api := s.engine.Group("/api/v1")
	{
		api.GET("/site", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"site_name":   "Utopia Images",
				"description": "Next-generation self-hosted image & static asset host",
				"version":     "v0.3.0",
				"base_url":    s.cfg.Server.BaseURL,
			})
		})

		authCtrl.RegisterRoutes(api)
		fileCtrl.RegisterRoutes(api)
		adminCtrl.RegisterRoutes(api)
		wmCtrl.RegisterRoutes(api)
		setCtrl.RegisterRoutes(api)
	}

	// 1. Static assets from embedded Vue 3 SPA
	s.engine.StaticFS("/assets", web.AssetsFS())

	// 2. SPA routes
	serveIndex := func(c *gin.Context) {
		html, err := web.ReadIndexHTML()
		if err != nil {
			c.String(http.StatusInternalServerError, "Frontend build artifact not found")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", html)
	}

	s.engine.GET("/", serveIndex)
	s.engine.GET("/setup", serveIndex)
	s.engine.GET("/login", serveIndex)
	s.engine.GET("/console/*filepath", serveIndex)
	s.engine.GET("/favicon.ico", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	// 3. NoRoute handler:
	// Matches any URL that doesn't start with /api or SPA pages,
	// checking against the public_path namespace (e.g. /upload/2026/05/23/a.jpg or /alice/upload/2026/05/23/a.jpg)
	s.engine.NoRoute(s.dispatchService.ServeFile)
}

func (s *Server) Engine() *gin.Engine {
	return s.engine
}
