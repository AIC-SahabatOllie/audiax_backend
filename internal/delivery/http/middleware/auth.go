package middleware

import (
	"strings"

	"audiax/internal/apperr"
	"audiax/internal/constants"
	"audiax/internal/model"
	"audiax/internal/usecase"

	"github.com/gofiber/fiber/v2"
)

func NewAuth(userUseCase *usecase.UserUseCase) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		token := bearerToken(ctx.Get(fiber.HeaderAuthorization))

		userID, err := userUseCase.Verify(ctx.UserContext(), token)
		if err != nil {
			return apperr.ErrUnauthorized
		}

		ctx.Locals(constants.AuthLocalsKey, &model.Auth{UserID: userID, Token: token})
		return ctx.Next()
	}
}

// GetAuth is only valid on routes behind NewAuth.
func GetAuth(ctx *fiber.Ctx) *model.Auth {
	auth, _ := ctx.Locals(constants.AuthLocalsKey).(*model.Auth)
	return auth
}

func bearerToken(header string) string {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, constants.AuthScheme) {
		return ""
	}
	return strings.TrimSpace(token)
}
