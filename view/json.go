package view

import "net/http"

type JSON struct {
	ExtraFields map[string]any `json:"-"`
	Data        any            `json:"data"`
	Error       string         `json:"error,omitempty"`
	Message     string         `json:"message,omitempty"`
	Success     bool           `json:"success,omitempty"`
	HTTPCode    int            `json:"httpCode,omitempty"`
}

type jsonField struct {
	Fields map[string]any
}

func view(json *jsonField) *JSON {
	return &JSON{
		ExtraFields: json.Fields,
		Success:     true,
		HTTPCode:    http.StatusOK,
	}
}
