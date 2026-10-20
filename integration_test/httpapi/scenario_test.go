//go:build integration && integrationcontainers

package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	gorest "github.com/elsyahtech/gorest"
	"github.com/elsyahtech/gorest/orm"
	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
)

type user struct {
	ID    string `gorest:"id, primary_key, required" json:"id"`
	Email string `gorest:"email, required" json:"email"`
	Name  string `gorest:"name, required" json:"name"`
}

type apiResponse struct {
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type userHandler struct {
	db    *gorest.DBSession
	table string
}

type httpClient struct {
	t   *testing.T
	app *fiber.App
}

func runCRUDHTTPScenario(t *testing.T, db *gorest.DBSession, table, createSQL, dropSQL string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, message, code, err := db.ExecSQL(ctx, createSQL)
	if err != nil {
		t.Fatalf("create test table (HTTP %d, %s): %v", code, message, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, _, _, err := db.ExecSQL(cleanupCtx, dropSQL); err != nil {
			t.Errorf("drop test table: %v", err)
		}
	})

	httpServer, _, err := server.New(&server.Config{
		Limiter: &limiter.Config{Max: 1000, Expiration: time.Minute},
	})
	if err != nil {
		t.Fatalf("create GOREST HTTP server: %v", err)
	}
	handler := &userHandler{db: db, table: table}
	httpServer.Post("/users", handler.create)
	httpServer.Get("/users/search", handler.search)
	httpServer.Get("/users/:id", handler.getByID)
	httpServer.Put("/users/:id", handler.update)
	httpServer.Delete("/users/:id", handler.delete)
	client := httpClient{t: t, app: httpServer.SRV}

	client.request(http.MethodPost, "/users", `{"id":"user-1","email":"alice@example.test","name":"Alice"}`, http.StatusCreated)
	client.request(http.MethodPost, "/users", `{"id":"user-2","email":"bob@example.test","name":"Bob"}`, http.StatusCreated)

	// This value resembles an injected SQL predicate. It must remain one bound
	// value and must not make the handler return either stored user.
	injection := "alice@example.test' OR '1'='1"
	searchPath := "/users/search?email=" + url.QueryEscape(injection)
	searchResponse := client.request(http.MethodGet, searchPath, "", http.StatusOK)
	var searchResult apiResponse
	if err := json.Unmarshal(searchResponse, &searchResult); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	encodedData, err := json.Marshal(searchResult.Data)
	if err != nil {
		t.Fatalf("encode search data: %v", err)
	}
	var searched []user
	if err := json.Unmarshal(encodedData, &searched); err != nil {
		t.Fatalf("decode search data: %v", err)
	}
	if len(searched) != 0 {
		t.Fatalf("injection-shaped email matched records: %#v", searched)
	}

	client.request(http.MethodPost, "/users", `{"id":"user-3","name":"Missing email"}`, http.StatusBadRequest)
	client.request(http.MethodPost, "/users", `{"id":"user-4","email":"alice@example.test","name":"Duplicate"}`, http.StatusConflict)
	client.request(http.MethodPut, "/users/user-1", `{"email":"alice-updated@example.test","name":"Alice Updated"}`, http.StatusOK)

	getResponse := client.request(http.MethodGet, "/users/user-1", "", http.StatusOK)
	var loaded apiResponse
	if err := json.Unmarshal(getResponse, &loaded); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	loadedJSON, err := json.Marshal(loaded.Data)
	if err != nil {
		t.Fatalf("encode GET data: %v", err)
	}
	var loadedUser user
	if err := json.Unmarshal(loadedJSON, &loadedUser); err != nil {
		t.Fatalf("decode GET data: %v", err)
	}
	if loadedUser.Email != "alice-updated@example.test" || loadedUser.Name != "Alice Updated" {
		t.Fatalf("updated user = %#v", loadedUser)
	}

	client.request(http.MethodDelete, "/users/user-1", "", http.StatusOK)
	client.request(http.MethodGet, "/users/user-1", "", http.StatusNotFound)

	var remaining []user
	query := db.Table(table).Find(&remaining)
	if query.Error != nil {
		t.Fatalf("verify persisted rows (HTTP %d): %v", query.HTTPCode, query.Error)
	}
	if len(remaining) != 1 || remaining[0].ID != "user-2" {
		t.Fatalf("remaining rows = %#v, want only user-2", remaining)
	}

	runConcurrentHTTPCRUDScenario(t, client)
	remaining = nil
	query = db.Table(table).Find(&remaining)
	if query.Error != nil {
		t.Fatalf("verify rows after concurrent HTTP requests (HTTP %d): %v", query.HTTPCode, query.Error)
	}
	if len(remaining) != 1 || remaining[0].ID != "user-2" {
		t.Fatalf("concurrent HTTP requests left unexpected rows: %#v", remaining)
	}
}

func runConcurrentHTTPCRUDScenario(t *testing.T, client httpClient) {
	t.Helper()
	const requestCount = 12
	type result struct {
		index  int
		status int
		body   []byte
		err    error
	}

	requestAll := func(method string, pathFor func(int) string, bodyFor func(int) string, wantStatus int) [][]byte {
		t.Helper()
		results := make(chan result, requestCount)
		start := make(chan struct{})
		for i := 0; i < requestCount; i++ {
			i := i
			go func() {
				<-start
				status, body, err := client.do(method, pathFor(i), bodyFor(i))
				results <- result{index: i, status: status, body: body, err: err}
			}()
		}
		close(start)

		bodies := make([][]byte, requestCount)
		for range bodies {
			got := <-results
			if got.err != nil {
				t.Errorf("concurrent %s request failed: %v", method, got.err)
				continue
			}
			if got.status != wantStatus {
				t.Errorf("concurrent %s status = %d, want %d; body=%s", method, got.status, wantStatus, got.body)
				continue
			}
			bodies[got.index] = got.body
		}
		return bodies
	}

	pathFor := func(i int) string { return "/users/parallel-" + fmt.Sprintf("%02d", i) }
	bodyFor := func(i int) string {
		id := "parallel-" + fmt.Sprintf("%02d", i)
		payload, _ := json.Marshal(user{ID: id, Email: id + "@example.test", Name: "Parallel"})
		return string(payload)
	}

	requestAll(http.MethodPost, func(int) string { return "/users" }, bodyFor, http.StatusCreated)
	getBodies := requestAll(http.MethodGet, pathFor, func(int) string { return "" }, http.StatusOK)
	for i, body := range getBodies {
		if len(body) == 0 {
			continue
		}
		var response struct {
			Data user `json:"data"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			t.Errorf("decode concurrent GET %d response: %v", i, err)
			continue
		}
		id := "parallel-" + fmt.Sprintf("%02d", i)
		if response.Data.ID != id || response.Data.Email != id+"@example.test" {
			t.Errorf("concurrent GET %d returned %#v", i, response.Data)
		}
	}
	requestAll(http.MethodDelete, pathFor, func(int) string { return "" }, http.StatusOK)
}

func (handler *userHandler) create(ctx *server.Context) error {
	var payload user
	if err := ctx.Bind().Body(&payload); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(apiResponse{Error: "invalid JSON payload"})
	}
	query := handler.db.Table(handler.table).Create(&payload)
	if query.Error != nil {
		return writeORMError(ctx, query)
	}
	return ctx.Status(http.StatusCreated).JSON(apiResponse{Data: payload})
}

func (handler *userHandler) search(ctx *server.Context) error {
	var users []user
	query := handler.db.Table(handler.table).
		Select("id, email, name").
		Where("email = ?", ctx.Query("email")).
		Find(&users)
	if query.Error != nil {
		return writeORMError(ctx, query)
	}
	return ctx.Status(http.StatusOK).JSON(apiResponse{Data: users})
}

func (handler *userHandler) getByID(ctx *server.Context) error {
	var found user
	query := handler.db.Table(handler.table).
		Where("id = ?", ctx.Params("id")).
		Find(&found)
	if query.Error != nil {
		return writeORMError(ctx, query)
	}
	return ctx.Status(http.StatusOK).JSON(apiResponse{Data: found})
}

func (handler *userHandler) update(ctx *server.Context) error {
	var payload user
	if err := ctx.Bind().Body(&payload); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(apiResponse{Error: "invalid JSON payload"})
	}
	payload.ID = ctx.Params("id")
	query := handler.db.Table(handler.table).
		Where("id = ?", payload.ID).
		Select("email, name").
		Update(&payload)
	if query.Error != nil {
		return writeORMError(ctx, query)
	}
	return ctx.Status(http.StatusOK).JSON(apiResponse{Data: payload})
}

func (handler *userHandler) delete(ctx *server.Context) error {
	id := ctx.Params("id")
	query := handler.db.Table(handler.table).
		Where("id = ?", id).
		Delete(&user{ID: id})
	if query.Error != nil {
		return writeORMError(ctx, query)
	}
	return ctx.Status(http.StatusOK).JSON(apiResponse{Message: "user deleted"})
}

func writeORMError(ctx *server.Context, query *orm.ORM) error {
	status := query.HTTPCode
	if status < 400 {
		status = http.StatusInternalServerError
	}
	return ctx.Status(status).JSON(apiResponse{Error: query.Message})
}

func (client httpClient) request(method, path, body string, wantStatus int) []byte {
	client.t.Helper()
	status, responseBody, err := client.do(method, path, body)
	if err != nil {
		client.t.Fatalf("send %s %s request: %v", method, path, err)
	}
	if status != wantStatus {
		client.t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, status, wantStatus, responseBody)
	}
	return responseBody
}

func (client httpClient) do(method, path, body string) (int, []byte, error) {
	var requestBody io.Reader
	if body != "" {
		requestBody = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, "http://gorest.test"+path, requestBody)
	if err != nil {
		return 0, nil, fmt.Errorf("build %s %s request: %w", method, path, err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.app.Test(request, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		return 0, nil, fmt.Errorf("send %s %s request: %w", method, path, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, nil, fmt.Errorf("read %s %s response: %w", method, path, err)
	}
	return response.StatusCode, responseBody, nil
}
