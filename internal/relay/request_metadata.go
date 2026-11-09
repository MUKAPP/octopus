package relay

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/utils/log"
)

// metadataErrorCategory uses typed error facts; raw upstream text never enters durable storage.
func metadataErrorCategory(success bool, err error, code int, executed int64) string {
	if success {
		return "none"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkError) && networkError.Timeout() || err != nil && strings.Contains(err.Error(), "first token timeout (") {
		return "timeout"
	}
	switch {
	case code == 429:
		return "rate_limit"
	case code == 401 || code == 403:
		return "authentication"
	case code >= 500:
		return "upstream_server_error"
	case code >= 400:
		return "upstream_client_error"
	}
	if relayErrorSource(err) != nil {
		return "upstream_error"
	}
	if networkError != nil {
		return "network_error"
	}
	if executed == 0 {
		return "unavailable"
	}
	return "internal_error"
}

func (m *RelayMetrics) saveMetadata(ctx context.Context, success bool, relayErr error, duration time.Duration, attempts []model.ChannelAttempt, channelID int, channelName string, rate float64, apiKeyName string) {
	start := m.StartTime.In(time.Local)
	row := model.RequestMetadata{
		ID: m.ID, StartedAtMS: m.StartTime.UnixMilli(), EndedAtMS: m.StartTime.Add(duration).UnixMilli(),
		Date: start.Format("20060102"), Hour: start.Format("2006-01-02T15:00:00Z07:00"), Minute: start.Format("2006-01-02T15:04:00Z07:00"),
		RequestModelName: m.RequestModel, ActualModelName: m.ActualModel, UpstreamModelName: m.UpstreamModel, ResponseModelName: m.ResponseModel,
		APIKeyID: m.APIKeyID, APIKeyName: apiKeyName, ChannelID: channelID, ChannelName: channelName, RateMultiplier: rate,
		ClientProtocol: m.ClientProtocol, Success: success, StatusCode: m.UpstreamStatusCode,
		UsagePresent: m.usageMetadata.Present, InputToken: m.Stats.InputToken, OutputToken: m.Stats.OutputToken,
		InputCost: m.Stats.InputCost, OutputCost: m.Stats.OutputCost, TotalDurationMS: duration.Milliseconds(), ReasoningToken: m.usageMetadata.ReasoningToken,
		CacheReadToken: m.usageMetadata.CacheReadToken, CacheWriteToken: m.usageMetadata.CacheWriteToken,
		Attempts: make([]model.RequestAttemptMetadata, 0, len(attempts)),
	}
	if m.InternalRequest != nil {
		row.Stream = m.InternalRequest.Stream != nil && *m.InternalRequest.Stream
		row.ReasoningEffort = m.InternalRequest.ReasoningEffort
		row.TaskType = string(m.InternalRequest.RequestType)
	}
	if !m.FirstTokenTime.IsZero() {
		value := m.FirstTokenTime.Sub(m.StartTime).Milliseconds()
		row.TTFTMS = &value
	}
	for _, a := range attempts {
		row.Attempts = append(row.Attempts, model.RequestAttemptMetadata{
			ChannelID: a.ChannelID, ChannelName: a.ChannelName, ChannelKeyID: a.ChannelKeyID, ChannelKeyRemark: a.ChannelKeyRemark,
			ModelName: a.ModelName, RateMultiplier: a.RateMultiplier, AttemptNum: a.AttemptNum, Status: a.Status,
			StartedAtMS: a.StartedAtMS, DurationMS: int64(a.Duration), StatusCode: a.UpstreamStatusCode, Sticky: a.Sticky,
		})
		switch a.Status {
		case model.AttemptSuccess, model.AttemptFailed:
			row.ExecutedAttempts++
			row.ChannelKeyID, row.ChannelKeyRemark = a.ChannelKeyID, a.ChannelKeyRemark
		case model.AttemptCircuitBreak:
			row.CircuitBreakSkips++
		case model.AttemptSkipped:
			row.OtherSkips++
		}
	}
	if row.ExecutedAttempts > 1 {
		row.RetryAttempts = row.ExecutedAttempts - 1
		row.RetryRequest = true
		row.Recovered = success
	}
	row.ErrorCategory = metadataErrorCategory(success, relayErr, row.StatusCode, row.ExecutedAttempts)
	row.Status = "failed"
	if success {
		row.Status = "success"
	} else if row.ErrorCategory == "canceled" {
		row.Status = "canceled"
	}
	if err := op.RequestMetadataCreate(ctx, &row); err != nil {
		log.Warnf("failed to persist request metadata: %v", err)
	}
}
