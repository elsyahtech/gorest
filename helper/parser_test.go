package helper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestBodyRequest(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	tests := []struct {
		name       string
		body       string
		content    string
		wantStatus int
		wantName   string
	}{
		{name: "valid JSON", body: `{"name":"Ada"}`, content: fiber.MIMEApplicationJSON, wantStatus: http.StatusOK, wantName: "Ada"},
		{name: "invalid JSON", body: `{"name":`, content: fiber.MIMEApplicationJSON, wantStatus: http.StatusBadRequest},
		{name: "wrong content type", body: `{"name":"Ada"}`, content: fiber.MIMETextPlain, wantStatus: http.StatusBadRequest},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := fiber.New()
			var got payload
			var message string
			var status int
			var gotErr error
			app.Post("/", func(ctx fiber.Ctx) error {
				message, status, gotErr = BodyRequest(ctx, &got)
				return ctx.SendStatus(status)
			})

			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			request.Header.Set(fiber.HeaderContentType, test.content)
			response, err := app.Test(request)
			if err != nil {
				t.Fatalf("app.Test() error = %v", err)
			}
			defer response.Body.Close()

			if response.StatusCode != test.wantStatus || status != test.wantStatus {
				t.Fatalf("status = (%d, %d), want %d", response.StatusCode, status, test.wantStatus)
			}
			if test.wantStatus == http.StatusOK {
				if gotErr != nil || message != "" || got.Name != test.wantName {
					t.Fatalf("BodyRequest() = (%q, %d, %v), payload = %#v", message, status, gotErr, got)
				}
				return
			}
			if gotErr == nil || message == "" {
				t.Fatalf("BodyRequest() = (%q, %d, %v), want parse error and guidance", message, status, gotErr)
			}
		})
	}
}
