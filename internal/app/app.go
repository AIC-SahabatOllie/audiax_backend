// Package app is the composition root: the single place that knows which
// concrete implementation satisfies which interface, and how the HTTP surface
// is assembled. It is deliberately separate from internal/config, which only
// builds infrastructure and must stay a leaf package that everything can import.
package app

import (
	"log/slog"

	"audiax/internal/config"
	"audiax/internal/constants"
	deliveryhttp "audiax/internal/delivery/http"
	"audiax/internal/delivery/http/middleware"
	"audiax/internal/delivery/http/route"
	"audiax/internal/repository"
	"audiax/internal/usecase"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Dependencies struct {
	Config *config.Config
	Log    *slog.Logger
	DB     *gorm.DB
	Redis  *redis.Client
}

// New builds the fully wired HTTP application.
func New(deps Dependencies) *fiber.App {
	cfg := deps.Config

	app := fiber.New(fiber.Config{
		AppName:               cfg.AppName,
		ErrorHandler:          deliveryhttp.NewErrorHandler(deps.Log),
		DisableStartupMessage: true,
		ReadTimeout:           constants.HTTPReadTimeout,
		WriteTimeout:          constants.HTTPWriteTimeout,
		IdleTimeout:           constants.HTTPIdleTimeout,
		BodyLimit:             constants.HTTPBodyLimit,
	})

	app.Use(requestid.New())
	app.Use(recover.New(recover.Config{EnableStackTrace: !cfg.IsProduction()}))
	app.Use(cors.New(cors.Config{AllowOrigins: cfg.CORSOrigins}))

	userRepository := repository.NewUserRepository()
	sessionRepository := repository.NewSessionRepository(deps.Redis)
	machineRepository := repository.NewMachineRepository()
	baselineRepository := repository.NewBaselineRepository()
	inspectionRepository := repository.NewInspectionRepository()
	aiService := config.NewAIService(cfg.AIServiceURL, cfg.AITimeout)
	objectStore := config.NewObjectStore(cfg.SupabaseURL, cfg.SupabaseServiceKey, cfg.StorageBucket, constants.StorageTimeout)
	llmService := config.NewLLMService(cfg.OllamaURL, constants.OllamaModelName, cfg.AdvisoryTimeout)

	userUseCase := usecase.NewUserUseCase(
		deps.DB, deps.Log, config.NewValidator(),
		userRepository, sessionRepository,
		cfg.SessionTTL, cfg.BcryptCost,
	)
	machineUseCase := usecase.NewMachineUseCase(
		deps.DB, deps.Log, config.NewValidator(), machineRepository,
	)
	baselineUseCase := usecase.NewBaselineUseCase(
		deps.DB, deps.Log, config.NewValidator(),
		machineRepository, baselineRepository, aiService, objectStore,
	)
	inspectionUseCase := usecase.NewInspectionUseCase(
		deps.DB, deps.Log, config.NewValidator(),
		machineRepository, baselineRepository, inspectionRepository,
		aiService, objectStore,
	)
	advisoryUseCase := usecase.NewAdvisoryUseCase(
		deps.DB, deps.Log, config.NewValidator(),
		machineRepository, baselineRepository, inspectionRepository,
		llmService, cfg.AdvisoryTimeout,
	)

	routes := route.RouteConfig{
		App:                  app,
		UserController:       deliveryhttp.NewUserController(userUseCase),
		MachineController:    deliveryhttp.NewMachineController(machineUseCase),
		BaselineController:   deliveryhttp.NewBaselineController(baselineUseCase),
		InspectionController: deliveryhttp.NewInspectionController(inspectionUseCase),
		AdvisoryController:   deliveryhttp.NewAdvisoryController(advisoryUseCase),
		AuthMiddleware:       middleware.NewAuth(userUseCase),
	}
	routes.Setup()

	return app
}
