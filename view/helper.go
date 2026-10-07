package view

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gofiber/fiber/v3"
)

func (res *JSON) SendMessage(msg string) *JSON {
	res.Message = msg

	return res
}

func (res *JSON) WithError(err string, code ...int) *JSON {
	res.Error = err
	res.Success = false
	res.HTTPCode = http.StatusBadRequest

	if len(code) > 0 {
		res.HTTPCode = code[0]
	}

	return res
}

func (res *JSON) MarshalJSON() ([]byte, error) {
	mappingObj := make(map[string]any)

	if res.ExtraFields != nil {
		for k, v := range res.ExtraFields {
			mappingObj[k] = v
		}
	}

	mappingObj["data"] = res.Data

	if res.Error != "" {
		mappingObj["error"] = res.Error
	}

	if res.Message != "" {
		mappingObj["message"] = res.Message
	}

	if res.Success || !res.Success {
		mappingObj["success"] = res.Success
	}

	if res.HTTPCode != 0 {
		mappingObj["httpCode"] = res.HTTPCode
	}

	data, err := json.Marshal(mappingObj)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return data, nil
}

func (*JSON) ToJSON(fields map[string]any) *JSON {
	viewFields := fields

	if viewFields == nil {
		viewFields = make(map[string]any)
	}

	return view(&jsonField{
		Fields: viewFields,
	})
}

func (*JSON) JSONView(ctx fiber.Ctx, response *JSON, err error) error {
	if err = ctx.Status(response.HTTPCode).JSON(response); err != nil {
		return fmt.Errorf("failed to send JSON Response: %w", err)
	}

	return nil
}
