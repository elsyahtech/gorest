package view

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/elsyahtech/gorest"
)

func (res *JSON) WithMessage(message string, errInput any, code ...int) *JSON {
	res.Success = false
	res.HTTPCode = http.StatusBadRequest

	var errString string

	switch val := errInput.(type) {
	case string:
		errString = val
	case error:
		if val != nil {
			errString = val.Error()
		}
	default:
		errString = fmt.Sprintf("%v", val)
	}

	if message != "" && errString != "" {
		res.Error = fmt.Sprintf("%s: %s", message, errString)
	} else if errString != "" {
		res.Error = errString
	} else {
		res.Error = message
	}

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

func (*JSON) NewResponse(fields map[string]any) *JSON {
	viewFields := fields

	if viewFields == nil {
		viewFields = make(map[string]any)
	}

	return view(&jsonField{
		Fields: viewFields,
	})
}

func (*JSON) SendJSON(ctx *gorest.Context, response *JSON, err error) error {
	if err = ctx.Status(response.HTTPCode).JSON(response); err != nil {
		return fmt.Errorf("failed to send JSON Response: %w", err)
	}

	return nil
}
