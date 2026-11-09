package model

// RequestAttemptMetadata is a body-free snapshot of a scheduling decision.
// Free-form upstream errors are deliberately excluded: they can contain response bodies or secrets.
type RequestAttemptMetadata struct {
	ChannelID        int           `json:"channel_id"`
	ChannelName      string        `json:"channel_name"`
	ChannelKeyID     int           `json:"channel_key_id"`
	ChannelKeyRemark string        `json:"channel_key_remark"`
	ModelName        string        `json:"model_name"`
	RateMultiplier   float64       `json:"rate_multiplier"`
	AttemptNum       int           `json:"attempt_num"`
	Status           AttemptStatus `json:"status"`
	StartedAtMS      int64         `json:"started_at_ms"`
	DurationMS       int64         `json:"duration_ms"`
	StatusCode       int           `json:"status_code"`
	Sticky           bool          `json:"sticky"`
}

// RequestMetadata is retained indefinitely, independently of the bounded relay log store.
// Only explicitly selected scalar metadata and typed attempts belong here; never add bodies,
// headers, URLs, raw errors, parameter overrides or API key values.
type RequestMetadata struct {
	ID                  string                   `json:"id" gorm:"primaryKey;size:32"`
	RequestID           int64                    `json:"request_id" gorm:"index"`
	StartedAtMS         int64                    `json:"started_at_ms" gorm:"index"`
	EndedAtMS           int64                    `json:"ended_at_ms"`
	Date                string                   `json:"date" gorm:"size:8;index"`
	Hour                string                   `json:"hour" gorm:"size:32;index"`
	Minute              string                   `json:"minute" gorm:"size:32;index"`
	RequestModelName    string                   `json:"request_model_name"`
	ActualModelName     string                   `json:"actual_model_name"`
	UpstreamModelName   string                   `json:"upstream_model_name"`
	ResponseModelName   string                   `json:"response_model_name"`
	APIKeyID            int                      `json:"api_key_id" gorm:"index"`
	APIKeyName          string                   `json:"api_key_name"`
	ChannelID           int                      `json:"channel_id"`
	ChannelName         string                   `json:"channel_name"`
	ChannelKeyID        int                      `json:"channel_key_id"`
	ChannelKeyRemark    string                   `json:"channel_key_remark"`
	RateMultiplier      float64                  `json:"rate_multiplier"`
	ClientProtocol      string                   `json:"client_protocol"`
	TaskType            string                   `json:"task_type"`
	Stream              bool                     `json:"stream"`
	ReasoningEffort     string                   `json:"reasoning_effort"`
	Success             bool                     `json:"success"`
	Status              string                   `json:"status"`
	StatusCode          int                      `json:"status_code"`
	ErrorCategory       string                   `json:"error_category"`
	UsagePresent        bool                     `json:"usage_present"`
	InputToken          int64                    `json:"input_token"`
	OutputToken         int64                    `json:"output_token"`
	CacheReadToken      *int64                   `json:"cache_read_token"`
	CacheReadInputToken *int64                   `json:"cache_read_input_token"` // Normalized input paired with cache reads; nil if only partial raw usage was received.
	CacheWriteToken     *int64                   `json:"cache_write_token"`
	ReasoningToken      *int64                   `json:"reasoning_token"`
	InputCost           float64                  `json:"input_cost"`
	OutputCost          float64                  `json:"output_cost"`
	TotalDurationMS     int64                    `json:"total_duration_ms" gorm:"index"`
	TTFTMS              *int64                   `json:"ttft_ms" gorm:"column:ttft_ms;index"`
	ExecutedAttempts    int64                    `json:"executed_attempts"`
	RetryAttempts       int64                    `json:"retry_attempts"`
	RetryRequest        bool                     `json:"retry_request"`
	Recovered           bool                     `json:"recovered"`
	CircuitBreakSkips   int64                    `json:"circuit_break_skips"`
	OtherSkips          int64                    `json:"other_skips"`
	Attempts            []RequestAttemptMetadata `json:"attempts" gorm:"serializer:json"`
}

func (RequestMetadata) TableName() string { return "request_metadata" }
