package route

import (
	"audiax/internal/delivery/http"

	"github.com/gofiber/fiber/v2"
)

type RouteConfig struct {
	App            *fiber.App
	UserController *http.UserController
	AuthMiddleware fiber.Handler
}

func (c *RouteConfig) Setup() {
	c.App.Get("/healthz", func(ctx *fiber.Ctx) error {
		return ctx.JSON(fiber.Map{"status": "ok"})
	})

	api := c.App.Group("/api")
	c.setupGuestRoutes(api)
	c.setupAuthRoutes(api)
}

func (c *RouteConfig) setupGuestRoutes(api fiber.Router) {
	api.Post("/users", c.UserController.Register)
	api.Post("/users/_login", c.UserController.Login)
}

// setupAuthRoutes scopes the auth middleware to its own group. Calling
// App.Use() instead would apply it to every route registered afterwards,
// which silently locks down anything added below it later.
func (c *RouteConfig) setupAuthRoutes(api fiber.Router) {
	authed := api.Group("", c.AuthMiddleware)

	authed.Get("/users/_current", c.UserController.Current)
	authed.Patch("/users/_current", c.UserController.Update)
	authed.Delete("/users/_current", c.UserController.Logout)
}
