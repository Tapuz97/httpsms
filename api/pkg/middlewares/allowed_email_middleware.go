package middlewares

import (
	"strings"

	"github.com/NdoleStudio/httpsms/pkg/entities"
	"github.com/NdoleStudio/httpsms/pkg/telemetry"
	"github.com/gofiber/fiber/v3"
)

// AllowedEmail restricts browser-facing routes to an explicit list of Firebase users.
func AllowedEmail(tracer telemetry.Tracer, allowedEmails []string) fiber.Handler {
	allowed := map[string]struct{}{}
	for _, email := range allowedEmails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email != "" {
			allowed[email] = struct{}{}
		}
	}

	return func(c fiber.Ctx) error {
		_, span := tracer.StartFromFiberCtx(c, "middlewares.AllowedEmail")
		defer span.End()

		if len(allowed) == 0 {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"status":  "error",
				"message": "No Firebase users are allowed to use this app.",
			})
		}

		tokenUser, ok := c.Locals(ContextKeyAuthUserID).(entities.AuthContext)
		if !ok || tokenUser.IsNoop() {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"status":  "error",
				"message": "You are not authorized to carry out this request.",
			})
		}

		if _, ok := allowed[strings.ToLower(strings.TrimSpace(tokenUser.Email))]; !ok {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"status":  "error",
				"message": "This Firebase user is not allowed to use this app.",
			})
		}

		return c.Next()
	}
}
