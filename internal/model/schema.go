package model

// Static model list for the proxy
type ModelObject struct {
	ID            string `json:"id"`
	Object        string `json:"object"`
	OwnedBy       string `json:"owned_by"`
	ContextLength int    `json:"context_length"`
}

type ModelListResponse struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
}

func DefaultModels() ModelListResponse {
	return ModelListResponse{
		Object: "list",
		Data: []ModelObject{
			{
				ID:            "mimo/mimo-auto",
				Object:        "model",
				OwnedBy:       "mimo",
				ContextLength: 1000000,
			},
		},
	}
}