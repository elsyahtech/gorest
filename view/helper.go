package view

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/elsyahtech/gorest"
)

func (res *JSON) WithMessage(message string, errInput any, code ...int) *JSON {
	var errString string

	// Check if an error was sent
	switch val := errInput.(type) {
	case string:
		errString = val
	case error:
		if val != nil {
			errString = val.Error()
		}
	default:
		// If errInput is nil or another empty type
		if val != nil {
			errString = fmt.Sprintf("%v", val)
		}
	}

	// If an error occurs (go to the failure path)
	if errString != "" && errString != "<nil>" {
		res.Success = false
		res.HTTPCode = http.StatusBadRequest

		if message != "" {
			res.Error = fmt.Sprintf("%s: %s", message, errString)
		} else {
			res.Error = errString
		}
	} else {
		// If there are NO errors (go to the success path)
		res.Success = true
		res.HTTPCode = http.StatusOK
		res.Message = message
	}

	// Override HTTP code if manually provided in the parameter
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
