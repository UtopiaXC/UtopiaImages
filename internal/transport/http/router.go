package http

import (
	"fmt"
	"net/http"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/cache"
	"github.com/UtopiaXC/UtopiaImages/internal/config"
	"github.com/UtopiaXC/UtopiaImages/internal/imaging"
	appLogger "github.com/UtopiaXC/UtopiaImages/internal/logger"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/UtopiaXC/UtopiaImages/internal/repository"
	"github.com/UtopiaXC/UtopiaImages/internal/service"
	"github.com/UtopiaXC/UtopiaImages/internal/storage"
	"github.com/UtopiaXC/UtopiaImages/web"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const defaultFaviconSVG = `<svg t="1793016306889" class="icon" viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" p-id="1817" width="200" height="200"><circle cx="512" cy="512" r="512" fill="#FFFFFF"/><g transform="matrix(0.82 0 0 0.82 92.16 92.16)"><path d="M836.8 912H555.2c-86.4 0-155.2-70.4-155.2-155.2v-46.4c0-86.4 70.4-155.2 155.2-155.2h281.6c86.4 0 155.2 70.4 155.2 155.2v46.4c0 84.8-70.4 155.2-155.2 155.2z" fill="#96E8BA" p-id="1818"></path><path d="M790.4 846.4H169.6c-78.4 0-144-64-144-144V273.6c0-78.4 64-144 144-144h620.8c78.4 0 144 64 144 144v428.8c0 78.4-65.6 144-144 144z m-620.8-672c-54.4 0-99.2 44.8-99.2 99.2v428.8c0 54.4 44.8 99.2 99.2 99.2h620.8c54.4 0 99.2-44.8 99.2-99.2V273.6c0-54.4-44.8-99.2-99.2-99.2H169.6z" fill="#103E26" p-id="1819"></path><path d="M424 830.4c-6.4 0-14.4-3.2-17.6-9.6L249.6 608 123.2 804.8c-6.4 11.2-20.8 12.8-30.4 6.4-11.2-6.4-12.8-20.8-6.4-30.4l144-224c3.2-6.4 11.2-9.6 17.6-9.6 8 0 14.4 3.2 19.2 9.6L422.4 768l238.4-372.8c4.8-6.4 11.2-11.2 20.8-9.6 8 0 16 4.8 19.2 12.8l192 384c4.8 11.2 1.6 24-9.6 30.4-11.2 4.8-24 1.6-30.4-9.6L678.4 452.8l-235.2 368c-4.8 4.8-11.2 9.6-19.2 9.6zM283.2 406.4c-19.2 0-36.8-6.4-51.2-17.6-17.6-14.4-27.2-33.6-30.4-54.4-1.6-22.4 3.2-43.2 17.6-59.2s33.6-27.2 54.4-30.4c22.4-1.6 43.2 3.2 59.2 17.6s27.2 33.6 30.4 54.4c1.6 22.4-3.2 43.2-17.6 59.2s-33.6 27.2-54.4 30.4h-8z m0-118.4h-4.8c-9.6 1.6-19.2 6.4-24 12.8-6.4 8-9.6 17.6-8 27.2 1.6 9.6 6.4 19.2 12.8 24 8 6.4 17.6 9.6 27.2 8 9.6-1.6 19.2-6.4 24-12.8 6.4-8 9.6-17.6 8-27.2-1.6-9.6-6.4-19.2-12.8-24-6.4-4.8-14.4-8-22.4-8zM824 260.8H768c-9.6 0-16-6.4-16-16s6.4-16 16-16h56c9.6 0 16 6.4 16 16s-6.4 16-16 16z" fill="#103E26" p-id="1820"></path><path d="M796.8 288c-9.6 0-16-6.4-16-16v-56c0-9.6 6.4-16 16-16s16 6.4 16 16V272c0 9.6-8 16-16 16z" fill="#103E26" p-id="1821"></path></g></svg>`

type Server struct {
	cfg             *config.Config
	db              *gorm.DB
	engine          *gin.Engine
	dispatchService *service.DispatchService
}

func NewServer(cfg *config.Config, configPath string) (*Server, error) {
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
	passkeyManager, _ := auth.NewPasskeyManager(db, cfg.Server.BaseURL)

	// 5. Initialize Services
	authService := service.NewAuthService(db, sessionManager, totpManager, captchaManager, passkeyManager)
	uploadService := service.NewUploadService(db, stDriver, processor, derivedCache)
	dispatchService := service.NewDispatchService(db, stDriver, processor, derivedCache)
	importService := service.NewImportService(db, uploadService)

	// 6. Initialize Controllers
	authController := NewAuthController(authService, db, cfg, configPath)
	fileController := NewFileController(db, uploadService, dispatchService, importService, cfg.Server.BaseURL, derivedCache)
	adminController := NewAdminController(db, uploadService, cfg.Server.BaseURL, derivedCache)
	watermarkController := NewWatermarkController(db, processor, derivedCache)
	settingsController := NewSettingsController(db, derivedCache, cfg, configPath)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(appLogger.GinLoggerMiddleware())

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
			siteName := s.cfg.Site.Name
			if siteName == "" {
				siteName = "Utopia Images"
			}
			siteDesc := s.cfg.Site.Description
			if siteDesc == "" {
				siteDesc = "又一个Utopia Images自部署图床"
			}

			var allowRegSetting model.Setting
			s.db.Where("key = ?", "allow_registration").First(&allowRegSetting)
			allowReg := allowRegSetting.Value == "true"

			var captchaProviderSetting model.Setting
			s.db.Where("key = ?", "captcha_provider").First(&captchaProviderSetting)
			provider := captchaProviderSetting.Value
			if provider == "" {
				provider = s.cfg.Security.Captcha.Provider
			}

			var favSetting model.Setting
			fav := ""
			if err := s.db.Where("key = 'site_favicon'").First(&favSetting).Error; err == nil {
				fav = favSetting.Value
			}
			if fav == "" {
				fav = defaultFaviconSVG
			}

			c.JSON(http.StatusOK, gin.H{
				"site_name":          siteName,
				"description":        siteDesc,
				"version":            "v0.3.0",
				"base_url":           s.cfg.Server.BaseURL,
				"allow_registration": allowReg,
				"captcha_provider":   provider,
				"favicon":            fav,
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
	serveFavicon := func(c *gin.Context) {
		var favBinarySetting model.Setting
		if err := s.db.Where("key = 'site_favicon_binary'").First(&favBinarySetting).Error; err == nil && len(favBinarySetting.BinaryValue) > 0 {
			contentType := favBinarySetting.ContentType
			if contentType == "" {
				contentType = "image/x-icon"
			}
			c.Header("Cache-Control", "public, max-age=86400, must-revalidate")
			c.Data(http.StatusOK, contentType, favBinarySetting.BinaryValue)
			return
		}

		c.Header("Cache-Control", "public, max-age=86400, must-revalidate")
		c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(defaultFaviconSVG))
	}

	s.engine.GET("/favicon.ico", serveFavicon)
	s.engine.GET("/favicon.svg", serveFavicon)

	// 3. NoRoute handler:
	// Matches any URL that doesn't start with /api or SPA pages,
	// checking against the public_path namespace (e.g. /upload/2026/05/23/a.jpg or /alice/upload/2026/05/23/a.jpg)
	s.engine.NoRoute(s.dispatchService.ServeFile)
}

func (s *Server) Engine() *gin.Engine {
	return s.engine
}
