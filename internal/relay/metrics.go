package relay

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"time"
	"unicode/utf8"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/price"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/looplj/axonhub/llm"
)

// RelayMetrics 负责最终的日志收集与持久化
type RelayMetrics struct {
	ID           int64 // Snowflake ID，进行中请求与最终日志共用
	APIKeyID     int
	RequestModel string
	StartTime    time.Time

	// 首 Token 时间
	FirstTokenTime time.Time

	// 请求和最终响应体；InternalResponse 保存实际写回客户端或流式聚合后的 body，不再强制转换成 llm.Response。
	InternalRequest  *llm.Request
	InternalResponse []byte

	// 统计指标
	ActualModel      string
	Stats            model.StatsMetrics
	CachedTokens     int64
	CacheWriteTokens int64

	// 参数覆盖 / 参数追加
	ParamOverride string
	ParamAppend   string
}

func (m *RelayMetrics) RecordUsage(usage *llm.Usage) {
	if usage == nil {
		return
	}

	// usage 已由 axonhub/llm 标准化；octopus 仍使用本地模型价格表计算成本，所以这里只做用量落点和价格换算。
	m.Stats.InputToken = usage.PromptTokens
	m.Stats.OutputToken = usage.CompletionTokens

	tokenDetails := usage.PromptTokensDetails
	m.CachedTokens = 0
	m.CacheWriteTokens = 0
	if tokenDetails != nil {
		m.CachedTokens = tokenDetails.CachedTokens
		m.CacheWriteTokens = tokenDetails.WriteCachedTokens
	}

	modelPrice := price.GetLLMPrice(m.ActualModel)
	if modelPrice == nil {
		return
	}
	if tokenDetails == nil {
		tokenDetails = &llm.PromptTokensDetails{}
	}
	// 缓存读、缓存写和普通输入的单价不同；如果上游返回的缓存明细超过总输入 token，就退回按全部输入 token 计费，避免出现负成本。
	nonCachedTokens := usage.PromptTokens - tokenDetails.CachedTokens - tokenDetails.WriteCachedTokens
	if nonCachedTokens < 0 {
		nonCachedTokens = usage.PromptTokens
	}
	m.Stats.InputCost = (float64(tokenDetails.CachedTokens)*modelPrice.CacheRead +
		float64(tokenDetails.WriteCachedTokens)*modelPrice.CacheWrite +
		float64(nonCachedTokens)*modelPrice.Input) * 1e-6
	m.Stats.OutputCost = float64(usage.CompletionTokens) * modelPrice.Output * 1e-6
}

func (m *RelayMetrics) Save(ctx context.Context, success bool, err error, attempts []model.ChannelAttempt) {
	duration := time.Since(m.StartTime)

	globalStats := model.StatsMetrics{
		WaitTime:    duration.Milliseconds(),
		InputToken:  m.Stats.InputToken,
		OutputToken: m.Stats.OutputToken,
		InputCost:   m.Stats.InputCost,
		OutputCost:  m.Stats.OutputCost,
	}
	if success {
		globalStats.RequestSuccess = 1
	} else {
		globalStats.RequestFailed = 1
	}

	channelID, channelName, rateMultiplier := finalChannel(attempts)

	// 客户端断开或请求上下文取消后仍要保存最终审计日志与统计，因此持久化阶段主动脱离请求取消信号。
	persistenceCtx := context.WithoutCancel(ctx)

	var apiKeyName string
	if apiKey, getErr := op.APIKeyGet(m.APIKeyID, persistenceCtx); getErr == nil {
		apiKeyName = apiKey.Name
	}

	op.StatsTotalUpdate(globalStats)
	op.StatsHourlyUpdate(globalStats)
	op.StatsDailyUpdate(context.Background(), globalStats)
	op.StatsAPIKeyUpdate(m.APIKeyID, globalStats)
	if channelID > 0 {
		// 通道成功/失败和等待时间在每次 attempt 结束时已记录；这里仅把最终响应的用量成本归到实际通道，避免重复计数。
		op.StatsChannelUpdate(channelID, model.StatsMetrics{
			InputToken:  m.Stats.InputToken,
			OutputToken: m.Stats.OutputToken,
			InputCost:   m.Stats.InputCost,
			OutputCost:  m.Stats.OutputCost,
		})
	}

	// 多维日聚合：请求模型、实际上游模型、API Key、最终渠道与成功状态组合。
	// 没有最终渠道或实际上游模型的失败请求照常保存（空身份），不能丢弃失败量。
	usage := model.StatsUsage{
		Date:             m.StartTime.In(time.Local).Format("20060102"),
		RequestModelName: m.RequestModel,
		ActualModelName:  m.ActualModel,
		APIKeyID:         m.APIKeyID,
		APIKeyName:       apiKeyName,
		ChannelID:        channelID,
		ChannelName:      channelName,
		Success:          success,
		CachedToken:      m.CachedTokens,
		StatsMetrics:     globalStats,
	}
	if err := op.StatsUsageUpdate(usage); err != nil {
		log.Warnf("failed to update usage stats: %v", err)
	}

	log.Infof("relay complete: model=%s, channel=%d(%s), success=%t, duration=%dms, input_token=%d, output_token=%d, input_cost=%f, output_cost=%f, total_cost=%f, attempts=%d",
		m.RequestModel, channelID, channelName, success, duration.Milliseconds(),
		m.Stats.InputToken, m.Stats.OutputToken,
		m.Stats.InputCost, m.Stats.OutputCost, m.Stats.InputCost+m.Stats.OutputCost,
		len(attempts))

	m.saveLog(persistenceCtx, err, duration, attempts, channelID, channelName, rateMultiplier, apiKeyName)
}

func finalChannel(attempts []model.ChannelAttempt) (int, string, float64) {
	var lastID int
	var lastName string
	var lastRate float64
	for i := len(attempts) - 1; i >= 0; i-- {
		a := attempts[i]
		if a.Status == model.AttemptSuccess {
			return a.ChannelID, a.ChannelName, a.RateMultiplier
		}
		if a.Status == model.AttemptFailed && lastID == 0 {
			lastID = a.ChannelID
			lastName = a.ChannelName
			lastRate = a.RateMultiplier
		}
	}
	return lastID, lastName, lastRate
}

func (m *RelayMetrics) buildRelayLog(err error, duration time.Duration, attempts []model.ChannelAttempt, channelID int, channelName string, rateMultiplier float64, apiKeyName string) model.RelayLog {
	cachedTokens := int(m.CachedTokens)
	relayLog := model.RelayLog{
		ID:               m.ID,
		Time:             m.StartTime.Unix(),
		RequestModelName: m.RequestModel,
		ChannelName:      channelName,
		ChannelId:        channelID,
		RateMultiplier:   rateMultiplier,
		ActualModelName:  m.ActualModel,
		UseTime:          int(duration.Milliseconds()),
		CachedTokens:     &cachedTokens,
		Attempts:         append([]model.ChannelAttempt(nil), attempts...),
		TotalAttempts:    len(attempts),
	}

	if apiKeyName != "" {
		relayLog.RequestAPIKeyName = apiKeyName
	}

	// 首字时间
	if !m.FirstTokenTime.IsZero() {
		relayLog.Ftut = int(m.FirstTokenTime.Sub(m.StartTime).Milliseconds())
	}

	// 用量
	if m.Stats.InputToken > 0 || m.Stats.OutputToken > 0 {
		relayLog.InputTokens = int(m.Stats.InputToken)
		relayLog.OutputTokens = int(m.Stats.OutputToken)
		relayLog.Cost = m.Stats.InputCost + m.Stats.OutputCost
	}

	relayLog.RequestContent, relayLog.RequestContentTruncated, relayLog.ResponseContent, relayLog.ResponseContentTruncated = m.logContent()
	if err != nil {
		relayLog.Error = err.Error()
	}
	return relayLog
}

func (m *RelayMetrics) saveLog(ctx context.Context, err error, duration time.Duration, attempts []model.ChannelAttempt, channelID int, channelName string, rateMultiplier float64, apiKeyName string) {
	relayLog := m.buildRelayLog(err, duration, attempts, channelID, channelName, rateMultiplier, apiKeyName)
	state := op.RelayLogStateFailed
	if err == nil {
		state = op.RelayLogStateSuccess
	} else if errors.Is(err, context.Canceled) {
		state = op.RelayLogStateCanceled
	}
	// 生产日志只进入并发安全的内存 store，不写入 SQLite。
	op.RelayLogStoreComplete(m.ID, state, relayLog, err, m.CachedTokens, m.CacheWriteTokens)
}

// logContent 构造请求/响应正文字段。请求正文完整保留；响应正文仍使用固定上限，避免单条内存日志无限增长。
func (m *RelayMetrics) logContent() (request string, requestTruncated bool, response string, responseTruncated bool) {
	const responseMaxBytes = 262144
	if reqBytes := m.requestContent(); reqBytes != nil {
		request = string(reqBytes)
	}
	if len(m.InternalResponse) > 0 {
		response, responseTruncated = truncateLogContent(m.InternalResponse, responseMaxBytes)
	}
	return request, false, response, responseTruncated
}

// truncateLogContent 按 UTF-8 字节计数保留 content 前缀，结果绝不超过 maxBytes 且不切断码点。
// 仅当原始内容超限或遇到无效 UTF-8 时返回 truncated=true；无效序列只保留其前面的有效前缀。
func truncateLogContent(content []byte, maxBytes int) (string, bool) {
	if maxBytes <= 0 {
		return "", len(content) > 0
	}
	if len(content) <= maxBytes && utf8.Valid(content) {
		return string(content), false
	}
	offset := 0
	for offset < len(content) && offset < maxBytes {
		r, size := utf8.DecodeRune(content[offset:])
		if r == utf8.RuneError && size == 1 {
			// 无效 UTF-8 或末尾截断的多字节序列，只保留其前的有效前缀
			break
		}
		if offset+size > maxBytes {
			break
		}
		offset += size
	}
	return string(content[:offset]), true
}

func (m *RelayMetrics) requestContent() []byte {
	if m.InternalRequest == nil {
		return nil
	}

	reqJSON, err := json.Marshal(filterRequestForLog(m.InternalRequest))
	if err != nil {
		return nil
	}
	if m.ParamOverride == "" && m.ParamAppend == "" {
		return reqJSON
	}

	var reqMap map[string]any
	if err := json.Unmarshal(reqJSON, &reqMap); err != nil {
		return reqJSON
	}

	// 日志里的请求体要反映本次实际发给上游的参数覆盖与追加，但失败解析时保留原始可审计内容。
	if m.ParamOverride != "" {
		var override map[string]any
		if err := json.Unmarshal([]byte(m.ParamOverride), &override); err != nil {
			return reqJSON
		}
		maps.Copy(reqMap, override)
	}
	if m.ParamAppend != "" {
		var appendParams map[string]any
		if err := json.Unmarshal([]byte(m.ParamAppend), &appendParams); err != nil {
			return reqJSON
		}
		mergeParamAppend(reqMap, appendParams)
	}

	finalJSON, err := json.Marshal(reqMap)
	if err != nil {
		return reqJSON
	}
	return finalJSON
}

// filterRequestForLog 去掉 RawRequest 和图片二进制字段，避免 multipart 原始 body 或图片内容落库。
func filterRequestForLog(req *llm.Request) *llm.Request {
	if req == nil {
		return nil
	}
	filtered := *req
	filtered.RawRequest = nil
	if req.Image != nil {
		img := *req.Image
		if len(img.Images) > 0 {
			img.Images = nil
		}
		if len(img.Mask) > 0 {
			img.Mask = nil
		}
		filtered.Image = &img
	}
	return &filtered
}
