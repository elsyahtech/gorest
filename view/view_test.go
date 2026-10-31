package view

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3"
)

type statusError struct {
	message string
	code    int
}

func (err statusError) Error() string {
	return err.message
}

func (err statusError) HTTPStatusCode() int {
	return err.code
}

func TestWithMessageBuildsSuccessAndErrorResponses(t *testing.T) {
	var response JSON
	if got := response.WithMessage("saved", nil); !got.Success ||
		got.HTTPCode != http.StatusOK || got.Message != "saved" {
		t.Fatalf("success response = %#v", got)
	}

	for _, test := range []struct {
		name     string
		input    any
		message  string
		want     string
		code     int
		override []int
	}{
		{name: "string error", input: "invalid", message: "request", want: "request: invalid", code: http.StatusBadRequest},
		{name: "error status", input: statusError{message: "missing", code: http.StatusNotFound}, want: "missing", code: http.StatusNotFound},
		{name: "wrapped error status", input: errors.Join(errors.New("context"), statusError{message: "conflict", code: http.StatusConflict}), want: "context\nconflict", code: http.StatusConflict},
		{name: "other value", input: 17, want: "17", code: http.StatusBadRequest},
		{name: "manual status override", input: "invalid", want: "invalid", code: http.StatusUnprocessableEntity, override: []int{http.StatusUnprocessableEntity}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var result JSON
			got := result.WithMessage(test.message, test.input, test.override...)
			if got.Success || got.Error != test.want || got.HTTPCode != test.code {
				t.Fatalf("WithMessage() = %#v, want error=%q code=%d", got, test.want, test.code)
			}
		})
	}

	var overridden JSON
	if got := overridden.WithMessage("", "invalid", http.StatusUnprocessableEntity); got.HTTPCode != http.StatusUnprocessableEntity {
		t.Fatalf("explicit status = %d, want %d", got.HTTPCode, http.StatusUnprocessableEntity)
	}
}

func TestJSONMarshalingAndResponseConstruction(t *testing.T) {
	response := (&JSON{}).NewResponse(map[string]any{"traceId": "trace-1"})
	response.Data = map[string]string{"name": "Ada"}
	response.WithMessage("loaded", nil)

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for key, want := range map[string]any{
		"traceId":  "trace-1",
		"message":  "loaded",
		"success":  true,
		"httpCode": float64(http.StatusOK),
	} {
		if decoded[key] != want {
			t.Errorf("decoded[%q] = %v, want %v", key, decoded[key], want)
		}
	}
	if decoded["data"] == nil {
		t.Fatal("data field missing from JSON response")
	}

	if got := (&JSON{}).NewResponse(nil); got.ExtraFields == nil || !got.Success ||
		got.HTTPCode != http.StatusOK {
		t.Fatalf("NewResponse(nil) = %#v, want initialized success response", got)
	}

	if _, err := json.Marshal(&JSON{Data: make(chan int)}); err == nil {
		t.Fatal("MarshalJSON() accepted a value unsupported by encoding/json")
	}
}

func TestSendJSONOverHTTP(t *testing.T) {
	app, _, err := server.New(nil)
	if err != nil {
		t.Fatalf("server.New() error = %v", err)
	}
	app.SRV.Get("/", func(ctx fiber.Ctx) error {
		response := (&JSON{}).NewResponse(nil).WithMessage("ok", nil)
		return (&JSON{}).SendJSON(&server.Context{Ctx: ctx}, response, nil)
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	result, err := app.SRV.Test(request)
	if err != nil {
		t.Fatalf("server.Test() error = %v", err)
	}
	defer result.Body.Close()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", result.StatusCode, http.StatusOK)
	}
}
