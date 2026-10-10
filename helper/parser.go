package helper

import (
	"fmt"
	"net/http"

	"github.com/gofiber/fiber/v3"
)

// BodyRequest parses the incoming HTTP request body into the provided struct pointer.
func BodyRequest(ctx fiber.Ctx, request any) (string, int, error) {
	var message string

	if err := ctx.Bind().Body(request); err != nil {
		message := "Please ensure that: " +
			"1) The Content-Type header is set to application/json, " +
			"2) The JSON payload is well-formatted and valid, and " +
			"3) The data types match the expected request structure."

		return message, http.StatusBadRequest, fmt.Errorf("failed to parse incoming HTTP request body: %w", err)
	}

	return message, http.StatusOK, nil
}
