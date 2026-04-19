package domain

type ToolDefinition struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	Parameters  map[string]any    `json:"parameters"`
	Cache       ToolCacheConfig   `json:"cache"`
	Strict      bool              `json:"strict"`
	Active      bool              `json:"active"`
}

type ToolDefinitionPatch struct {
	Name        *string
	Description *string
	Method      *string
	URL         *string
	Headers     *map[string]string
	Parameters  *map[string]any
	Cache       *ToolCacheConfig
	Strict      *bool
}

type ToolCacheConfig struct {
	Enabled    bool `json:"enabled"`
	TTLSeconds int  `json:"ttl_seconds"`
	MaxEntries int  `json:"max_entries"`
}

type ToolFunctionDefinition struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
	Strict      bool           `json:"strict,omitempty"`
}

type FunctionCallOutput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id,omitempty"`
	Output string `json:"output"`
}
