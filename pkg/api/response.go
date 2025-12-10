package api

import "github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"

type ResponseMetadata struct {
	Code        common.StatusCode `json:"code"`
	RequestID   string            `json:"request_id,omitempty"`
	Description string            `json:"description,omitempty"`
}

type APIResponse struct {
	Meta ResponseMetadata `json:"meta"`
	Data interface{}      `json:"data,omitempty"`
}

type apiOrgInput struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type apiOrgOutput struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}
