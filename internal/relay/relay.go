package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/helper"
	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/internal/utils/snowflake"
	"github.com/gin-contrib/sse"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/pipeline"
	"github.com/looplj/axonhub/llm/pipeline/stream"
	"github.com/looplj/axonhub/llm/streams"
	"github.com/looplj/axonhub/llm/transformer"
)

const relaySSEHeartbeatInterval = 15 * time.Second

var relaySSEHeartbeatComment = []byte(": ping\n\n")

// clientHeaderPlaceholderPattern 匹配自定义头中的 {client_header:NAME} 占位符。
var clientHeaderPlaceholderPattern = regexp.MustCompile(`\{client_header:[^}]+\}`)

type relaySSEHeartbeatTicker interface {
	Chan() <-chan time.Time
	Stop()
}

type relaySSEHeartbeatTimeTicker struct {
	ticker *time.Ticker
}

func newRelaySSEHeartbeatTicker(interval time.Duration) relaySSEHeartbeatTicker {
	return &relaySSEHeartbeatTimeTicker{ticker: time.NewTicker(interval)}
}

func (t *relaySSEHeartbeatTimeTicker) Chan() <-chan time.Time {
	return t.ticker.C
}

func (t *relaySSEHeartbeatTimeTicker) Stop() {
	t.ticker.Stop()
}

// Handler 返回处理入站请求并转发到上游服务的 Gin handler。
func Handler(inboundType llm.APIFormat) gin.HandlerFunc {
	inAdapter := newInbound(inboundType)
	return func(c *gin.Context) {
		run, err := newRelayRun(c, inboundType, inAdapter)
		if err != nil {
			return
		}
		run.run()
	}
}

func newRelayRun(c *gin.Context, inboundType llm.APIFormat, inAdapter transformer.Inbound) (*relayRun, error) {
	// 受支持的压缩编码在 parseRequest 内被解码时会就地删除共享头，需在解码前留一份原始入站头快照。
	var clientHeaders http.Header
	switch strings.ToLower(strings.TrimSpace(c.Request.Header.Get("Content-Encoding"))) {
	case "gzip", "x-gzip", "deflate", "zstd":
		clientHeaders = c.Request.Header.Clone()
	}

	internalRequest, err := parseRequest(c, inboundType, inAdapter)
	if err != nil {
		return nil, err
	}

	if supportedModels := c.GetString("supported_models"); supportedModels != "" {
		if !slices.Contains(strings.Split(supportedModels, ","), internalRequest.Model) {
			err := errors.New("model not supported")
			resp.Error(c, http.StatusBadRequest, err.Error())
			return nil, err
		}
	}

	group, err := op.GroupGetEnabledMap(internalRequest.Model, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "model not found")
		return nil, err
	}

	apiKeyID := c.GetInt("api_key_id")
	iter := balancer.NewIterator(group, apiKeyID, internalRequest.Model)
	if iter.Len() == 0 {
		err := errors.New("no available channel")
		resp.Error(c, http.StatusServiceUnavailable, err.Error())
		return nil, err
	}

	return &relayRun{
		c:               c,
		inAdapter:       inAdapter,
		inboundType:     inboundType,
		internalRequest: internalRequest,
		clientHeaders:   clientHeaders,
		metrics: &RelayMetrics{
			APIKeyID:        apiKeyID,
			RequestModel:    internalRequest.Model,
			StartTime:       time.Now(),
			InternalRequest: internalRequest,
		},
		iter:  iter,
		group: group,
		tried: make(map[relaySelectionKey]struct{}),
	}, nil
}

func (r *relayRun) run() {
	ctx := r.c.Request.Context()
	r.registerActive()
	var lastErr error
	for pass := range 2 {
		if pass == 1 && len(r.deferred) == 0 {
			break
		}
		r.probe = pass == 1
		r.iter.Rewind()
		for r.iter.Next() {
			r.resetCandidate()
			for !r.candidateDone {
				select {
				case <-ctx.Done():
					log.Infof("request context canceled, stopping retry")
					r.removeActive()
					r.metrics.Save(ctx, false, context.Canceled, r.iter.Attempts())
					return
				default:
				}

				attempt, err := r.prepareAttempt()
				if err != nil {
					lastErr = err
				}
				if attempt == nil {
					continue
				}

				responseFinalized, err := attempt.run()
				if err == nil {
					r.removeActive()
					r.metrics.Save(ctx, true, nil, r.iter.Attempts())
					return
				}
				if responseFinalized {
					if attempt.canWriteCommittedSSEError(ctx, err) {
						if r.writeCommittedSSEError(ctx, err) == nil {
							attempt.streamErrorWritten = true
							attempt.streamTerminated = true
						} else {
							attempt.clientStreamWriteFailed = true
						}
					}
					r.removeActive()
					r.metrics.Save(ctx, false, err, r.iter.Attempts())
					return
				}
				lastErr = err
			}
		}
	}
	r.probe = false
	if lastErr == nil {
		lastErr = errors.New("all channels failed")
	}
	r.removeActive()
	r.writeFinalError(ctx, lastErr)
	r.metrics.Save(ctx, false, lastErr, r.iter.Attempts())
}

// registerActive 在请求开始时登记为进行中状态；ID 同时用于最终日志，便于前端关联。
func (r *relayRun) registerActive() {
	apiKeyName := ""
	if apiKey, err := op.APIKeyGet(r.metrics.APIKeyID, r.c.Request.Context()); err == nil {
		apiKeyName = apiKey.Name
	}
	r.metrics.ID = snowflake.GenerateID()
	initialLog := r.metrics.buildRelayLog(nil, 0, nil, 0, "", 0, apiKeyName)
	stream := r.internalRequest.Stream != nil && *r.internalRequest.Stream
	op.RelayLogStoreStart(initialLog, r.metrics.StartTime, string(r.inboundType), stream)
	op.RelayActiveAdd(op.ActiveRelayRequest{
		ID:                r.metrics.ID,
		Time:              r.metrics.StartTime.Unix(),
		RequestModelName:  r.metrics.RequestModel,
		RequestAPIKeyName: apiKeyName,
	})
}

func (r *relayRun) removeActive() {
	op.RelayActiveRemove(r.metrics.ID)
}

// writeFinalError 以客户端 API 格式返回最后一次上游 HTTP 错误。
// 已提交 SSE 响应时发送客户端格式的 error 事件，避免静默结束。
// 网络错误和中继内部错误没有可保留的上游状态码，使用 424 避免被 5xx 错误页改写。
func (r *relayRun) writeFinalError(ctx context.Context, err error) {
	if r.c.Writer.Written() {
		if strings.HasPrefix(strings.ToLower(r.c.Writer.Header().Get("Content-Type")), "text/event-stream") {
			r.writeCommittedSSEError(ctx, err)
			return
		}
		log.Warnf("all channels failed after response started: %v", err)
		return
	}
	clientErr := r.clientError(ctx, err)
	for key, values := range clientErr.Headers {
		r.c.Writer.Header()[key] = values
	}
	r.metrics.InternalResponse = clientErr.Body
	r.c.Abort()
	r.c.Data(clientErr.StatusCode, clientErr.Headers.Get("Content-Type"), clientErr.Body)
}

func (r *relayRun) writeCommittedSSEError(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	detail := llm.ErrorDetail{Message: err.Error(), Type: "internal_server_error"}
	if source := relayErrorSource(err); source != nil {
		detail = relayClientErrorDetail(source)
	}
	body := relaySSEErrorBody(r.inboundType, detail)
	if writeErr := r.writeSSEEvent(&httpclient.StreamEvent{Type: "error", Data: body}); writeErr != nil {
		return writeErr
	}
	r.metrics.InternalResponse = body
	log.Warnf("request failed after SSE response started: %v", err)
	return nil
}

func (ra *relayAttempt) canWriteCommittedSSEError(ctx context.Context, err error) bool {
	return ra.responseCommitted && ra.c.Writer.Written() &&
		strings.HasPrefix(strings.ToLower(ra.c.Writer.Header().Get("Content-Type")), "text/event-stream") &&
		ra.streamEventWritten && !ra.streamErrorWritten && !ra.streamTerminated &&
		!ra.clientStreamWriteFailed && ctx.Err() == nil && !errors.Is(err, context.Canceled)
}

// relaySSEWriter 补足现有 SSE encoder 对 []byte 写入错误的忽略，失败后不再写入。
type relaySSEWriter struct {
	gin.ResponseWriter
	err error
}

func (w *relaySSEWriter) Write(body []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.ResponseWriter.Write(body)
	w.err = err
	return n, err
}

func (w *relaySSEWriter) WriteString(body string) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.ResponseWriter.WriteString(body)
	w.err = err
	return n, err
}

func (r *relayRun) writeSSEEvent(event *httpclient.StreamEvent) error {
	writer := relaySSEWriter{ResponseWriter: r.c.Writer}
	if err := sse.Encode(&writer, sse.Event{Event: event.Type, Data: event.Data}); err != nil {
		return err
	}
	if writer.err != nil {
		return writer.err
	}
	r.c.Writer.Flush()
	return nil
}

func (r *relayRun) emitAttemptsSince(start int) {
	attempts := r.iter.Attempts()
	if start < 0 || start >= len(attempts) {
		return
	}
	for index := start; index < len(attempts); index++ {
		op.RelayLogStoreAttemptFinished(r.metrics.ID, index, attempts[index])
	}
}

func (r *relayRun) resetCandidate() {
	r.candidateStarted = true
	r.candidateItemIndex = r.iter.Index()
	r.candidateLoaded = false
	r.candidateDone = false
	r.candidateKeyIndex = 0
	r.candidateChannel = nil
}

func (r *relayRun) hasDeferredItem(item dbmodel.GroupItem) bool {
	for key := range r.deferred {
		if key.channelID == item.ChannelID && key.modelName == item.ModelName {
			return true
		}
	}
	return false
}

func (r *relayRun) deferSelection(key relaySelectionKey) {
	if r.deferred == nil {
		r.deferred = make(map[relaySelectionKey]struct{})
	}
	r.deferred[key] = struct{}{}
}

func (r *relayRun) markTried(key relaySelectionKey) {
	if r.tried == nil {
		r.tried = make(map[relaySelectionKey]struct{})
	}
	r.tried[key] = struct{}{}
}

func relayKeySortRank(key dbmodel.ChannelKey, nowSec int64) int {
	if !key.Enabled || key.ChannelKey == "" {
		return 2
	}
	if key.IsCoolingDown(nowSec) {
		return 1
	}
	return 0
}

func sortRelayChannelKeys(keys []dbmodel.ChannelKey) {
	nowSec := time.Now().Unix()
	slices.SortStableFunc(keys, func(left, right dbmodel.ChannelKey) int {
		leftRank := relayKeySortRank(left, nowSec)
		rightRank := relayKeySortRank(right, nowSec)
		if leftRank != rightRank {
			if leftRank < rightRank {
				return -1
			}
			return 1
		}
		if left.TotalCost < right.TotalCost {
			return -1
		}
		if left.TotalCost > right.TotalCost {
			return 1
		}
		return 0
	})
}

func (r *relayRun) preparedAttempt(item dbmodel.GroupItem, channel *dbmodel.Channel, usedKey dbmodel.ChannelKey, outAdapter transformer.Outbound) *relayAttempt {
	r.markTried(relaySelectionKey{
		channelID: channel.ID,
		keyID:     usedKey.ID,
		modelName: item.ModelName,
	})
	// 每次尝试都把客户端模型改成本次候选的实际上游模型；重试时会被下一候选覆盖。
	r.internalRequest.Model = item.ModelName
	r.metrics.ActualModel = item.ModelName
	r.metrics.ParamOverride = ""
	r.metrics.ParamAppend = ""
	r.metrics.UpstreamModel = ""
	r.metrics.ResponseModel = ""
	log.Infof("request model %s, mode: %d, forwarding to channel: %s model: %s (attempt %d/%d, sticky=%t)",
		r.metrics.RequestModel, r.group.Mode, channel.Name, item.ModelName,
		r.iter.Index()+1, r.iter.Len(), r.iter.IsSticky())
	op.RelayActiveUpdate(r.metrics.ID, channel.Name, item.ModelName)
	return &relayAttempt{
		relayRun:   r,
		outAdapter: outAdapter,
		channel:    channel,
		usedKey:    usedKey,
	}
}

func (r *relayRun) prepareAttempt() (*relayAttempt, error) {
	item := r.iter.Item()
	if !r.candidateStarted || r.candidateItemIndex != r.iter.Index() {
		r.resetCandidate()
	}
	if r.candidateDone {
		return nil, nil
	}
	if r.probe && !r.hasDeferredItem(item) {
		r.candidateDone = true
		return nil, nil
	}

	if !r.candidateLoaded {
		r.candidateLoaded = true
		channel, err := op.ChannelGet(item.ChannelID, r.c.Request.Context())
		if err != nil {
			log.Warnf("failed to get channel %d: %v", item.ChannelID, err)
			start := len(r.iter.Attempts())
			r.iter.Skip(item.ChannelID, 0, fmt.Sprintf("channel_%d", item.ChannelID), fmt.Sprintf("channel not found: %v", err))
			r.emitAttemptsSince(start)
			r.candidateDone = true
			return nil, err
		}
		r.candidateChannel = channel
		if !channel.Enabled {
			start := len(r.iter.Attempts())
			r.iter.Skip(channel.ID, 0, channel.Name, "channel disabled")
			r.emitAttemptsSince(start)
			r.candidateDone = true
			return nil, nil
		}

		sortRelayChannelKeys(channel.Keys)
		hasKey := false
		for _, key := range channel.Keys {
			if key.Enabled && key.ChannelKey != "" {
				hasKey = true
				break
			}
		}
		if !hasKey {
			start := len(r.iter.Attempts())
			r.iter.Skip(channel.ID, 0, channel.Name, "no available key")
			r.emitAttemptsSince(start)
			r.candidateDone = true
			return nil, nil
		}
	}

	channel := r.candidateChannel
	nowSec := time.Now().Unix()
	for r.candidateKeyIndex < len(channel.Keys) {
		usedKey := channel.Keys[r.candidateKeyIndex]
		r.candidateKeyIndex++
		if !usedKey.Enabled || usedKey.ChannelKey == "" {
			continue
		}
		selection := relaySelectionKey{
			channelID: channel.ID,
			keyID:     usedKey.ID,
			modelName: item.ModelName,
		}
		if _, tried := r.tried[selection]; tried {
			continue
		}

		if r.probe {
			if _, deferred := r.deferred[selection]; !deferred {
				continue
			}
		} else {
			state, remaining := balancer.GetCircuitState(channel.ID, usedKey.ID, item.ModelName)
			if state == balancer.StateHalfOpen || (state == balancer.StateOpen && remaining > 0) {
				r.deferSelection(selection)
				start := len(r.iter.Attempts())
				r.iter.SkipCircuitBreak(channel.ID, usedKey.ID, channel.Name)
				r.emitAttemptsSince(start)
				continue
			}
			if usedKey.IsCoolingDown(nowSec) {
				r.deferSelection(selection)
				start := len(r.iter.Attempts())
				r.iter.Skip(channel.ID, usedKey.ID, channel.Name, "key is in 429 cooldown")
				r.emitAttemptsSince(start)
				continue
			}
		}

		// 适配器准备完成后才占用探针，避免构造失败留下无人完成的 HalfOpen。
		outAdapter, err := newOutbound(channel.Type, r.internalRequest, channel.GetBaseUrl(), usedKey.ChannelKey)
		if err != nil {
			start := len(r.iter.Attempts())
			r.iter.Skip(channel.ID, usedKey.ID, channel.Name, err.Error())
			r.emitAttemptsSince(start)
			continue
		}
		if !r.probe || !balancer.Probe(channel.ID, usedKey.ID, item.ModelName) {
			if tripped, _ := balancer.IsTripped(channel.ID, usedKey.ID, item.ModelName); tripped {
				r.deferSelection(selection)
				start := len(r.iter.Attempts())
				r.iter.SkipCircuitBreak(channel.ID, usedKey.ID, channel.Name)
				r.emitAttemptsSince(start)
				continue
			}
		}
		return r.preparedAttempt(item, channel, usedKey, outAdapter), nil
	}
	r.candidateDone = true
	return nil, nil
}

// run 统一管理一次通道尝试的完整生命周期。
func (ra *relayAttempt) run() (bool, error) {
	attemptIndex := len(ra.iter.Attempts())
	ra.attemptIndex = attemptIndex
	_, cancel := ra.beginAttemptContext()
	defer func() {
		op.RelayLogStoreClearAttemptCancel(ra.metrics.ID, attemptIndex)
		ra.endAttemptContext()
	}()
	span := ra.iter.StartAttempt(ra.channel.ID, ra.usedKey.ID, ra.channel.Name, strings.TrimSpace(ra.usedKey.Remark))
	op.RelayLogStoreAttemptStarted(ra.metrics.ID, attemptIndex, span.Attempt())
	op.RelayLogStoreRegisterAttemptCancel(ra.metrics.ID, attemptIndex, cancel)

	upstreamStatusCode, fwdErr := ra.forward()
	fwdErr = ra.normalizeUpstreamError(fwdErr)
	observedStatusCode := int(ra.upstreamStatusCode.Load())
	ra.metrics.UpstreamStatusCode = observedStatusCode
	ra.modelNamesMu.Lock()
	ra.metrics.UpstreamModel = ra.upstreamModelName
	ra.metrics.ResponseModel = ra.responseModelName
	ra.modelNamesMu.Unlock()
	if fwdErr == nil && upstreamStatusCode == 0 {
		upstreamStatusCode = http.StatusOK
	}
	ra.usedKey.StatusCode = upstreamStatusCode
	ra.usedKey.LastUseTimeStamp = time.Now().Unix()

	if fwdErr == nil {
		op.ChannelKeyUpdate(ra.usedKey, ra.metrics.Stats.InputCost+ra.metrics.Stats.OutputCost)

		span.End(dbmodel.AttemptSuccess, "", observedStatusCode)
		op.RelayLogStoreAttemptFinished(ra.metrics.ID, attemptIndex, span.Attempt())
		op.StatsChannelUpdate(ra.channel.ID, dbmodel.StatsMetrics{
			WaitTime:       span.Duration().Milliseconds(),
			RequestSuccess: 1,
		})
		balancer.RecordSuccess(ra.channel.ID, ra.usedKey.ID, ra.internalRequest.Model)
		balancer.SetSticky(ra.metrics.APIKeyID, ra.metrics.RequestModel, ra.channel.ID, ra.usedKey.ID)
		return false, nil
	}

	op.ChannelKeyUpdate(ra.usedKey, 0)
	span.End(dbmodel.AttemptFailed, fwdErr.Error(), observedStatusCode)
	op.RelayLogStoreAttemptFinished(ra.metrics.ID, attemptIndex, span.Attempt())
	op.StatsChannelUpdate(ra.channel.ID, dbmodel.StatsMetrics{
		WaitTime:      span.Duration().Milliseconds(),
		RequestFailed: 1,
	})
	balancer.RecordFailure(ra.channel.ID, ra.usedKey.ID, ra.internalRequest.Model)
	finalized := ra.responseFinalized()
	if errors.Is(fwdErr, context.Canceled) && ra.responseCommitted {
		finalized = true
	}
	return finalized, fmt.Errorf("channel %s failed: %w", ra.channel.Name, fwdErr)
}

// parseRequest 解析并验证入站请求
func parseRequest(c *gin.Context, inboundType llm.APIFormat, inAdapter transformer.Inbound) (*llm.Request, error) {
	if inAdapter == nil {
		err := fmt.Errorf("unsupported inbound type: %s", inboundType)
		resp.Error(c, http.StatusBadRequest, err.Error())
		return nil, err
	}

	httpRequest, err := httpclient.ReadHTTPRequest(c.Request)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return nil, err
	}

	internalRequest, err := inAdapter.TransformRequest(c.Request.Context(), httpRequest)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if errors.Is(err, transformer.ErrInvalidRequest) {
			statusCode = http.StatusBadRequest
		}
		resp.Error(c, statusCode, err.Error())
		return nil, err
	}
	if internalRequest.RawRequest == nil {
		internalRequest.RawRequest = httpRequest
	}

	return internalRequest, nil
}

// forward 转发请求到上游服务。流请求在 pipeline 首事件预读期间也保持 SSE 心跳。
func (ra *relayAttempt) forward() (int, error) {
	ctx := ra.attemptContext
	if ctx == nil {
		ctx = ra.c.Request.Context()
	}
	if ra.internalRequest.RawRequest == nil {
		return 0, fmt.Errorf("missing raw request")
	}

	httpClient, err := helper.ChannelHttpClient(ra.channel)
	if err != nil {
		log.Warnf("failed to get http client: %v", err)
		return 0, err
	}
	// 浅拷贝共享 client，仅包裹当前尝试的 transport；保留原连接池、代理和 client 配置。
	attemptClient := *httpClient
	attemptClient.Transport = &relayStatusTransport{
		base:       httpClient.Transport,
		statusCode: &ra.upstreamStatusCode,
	}

	relayMiddleware := &relayPipelineMiddleware{attempt: ra}
	process := func(processCtx context.Context) (*pipeline.Result, error) {
		return pipeline.NewFactory(httpclient.NewHttpClientWithClient(&attemptClient)).
			Pipeline(
				&parsedRequestInbound{Inbound: ra.inAdapter, request: ra.internalRequest},
				ra.outAdapter,
				pipeline.WithMiddlewares(stream.EnsureUsage(), relayMiddleware),
				pipeline.WithEmptyResponseDetection(),
			).
			Process(processCtx, ra.internalRequest.RawRequest)
	}

	if ra.internalRequest.Stream != nil && *ra.internalRequest.Stream {
		heartbeatTicker := newRelaySSEHeartbeatTicker(relaySSEHeartbeatInterval)
		result, heartbeatTicker, processCancel, err := ra.processStreamWithHeartbeat(ctx, heartbeatTicker, process)
		if err != nil {
			return relayMiddleware.upstreamStatusCode, err
		}
		streamErr := ra.writeStreamWithHeartbeatTicker(ctx, result.EventStream, heartbeatTicker)
		processCancel()
		if streamErr != nil {
			return http.StatusOK, streamErr
		}
		return http.StatusOK, nil
	}

	result, err := process(ctx)
	if err != nil {
		return relayMiddleware.upstreamStatusCode, err
	}
	if result == nil {
		return 0, fmt.Errorf("empty pipeline result")
	}
	if result.Stream {
		return 0, fmt.Errorf("unexpected stream pipeline result")
	}
	if result.Response == nil {
		return 0, fmt.Errorf("empty pipeline response")
	}
	ra.metrics.InternalResponse = result.Response.Body
	statusCode := result.Response.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	contentType := "application/json"
	if result.Response.Headers != nil {
		for key, values := range result.Response.Headers {
			for _, value := range values {
				ra.c.Header(key, value)
			}
		}
		if result.Response.Headers.Get("Content-Type") != "" {
			contentType = result.Response.Headers.Get("Content-Type")
		}
	}
	ra.c.Data(statusCode, contentType, result.Response.Body)
	ra.responseCommitted = true
	if ra.metrics != nil && ra.metrics.ID != 0 {
		op.RelayLogStoreResponseCommitted(ra.metrics.ID)
	}
	return statusCode, nil
}

func (ra *relayAttempt) applyChannelRequestOptions(outboundRequest *httpclient.Request) {
	// ParamOverride / ParamAppend 只改 JSON 请求体；multipart 图片编辑等请求不能按 map 合并。
	hasOverride := ra.channel.ParamOverride != nil && *ra.channel.ParamOverride != ""
	hasAppend := ra.channel.ParamAppend != nil && *ra.channel.ParamAppend != ""
	isJSON := strings.Contains(strings.ToLower(outboundRequest.Headers.Get("Content-Type")+" "+outboundRequest.ContentType), "application/json")
	if (hasOverride || hasAppend) && isJSON {
		var bodyMap map[string]any
		if err := json.Unmarshal(outboundRequest.Body, &bodyMap); err != nil {
			log.Warnf("failed to unmarshal request body: %v, skipping param_override/param_append", err)
		} else {
			bodyModified := false
			if hasOverride {
				var override map[string]any
				if err := json.Unmarshal([]byte(*ra.channel.ParamOverride), &override); err != nil {
					log.Warnf("failed to unmarshal param_override: %v, skipping", err)
				} else {
					maps.Copy(bodyMap, override)
					bodyModified = true
					ra.metrics.ParamOverride = *ra.channel.ParamOverride
				}
			}
			if hasAppend {
				var appendParams map[string]any
				if err := json.Unmarshal([]byte(*ra.channel.ParamAppend), &appendParams); err != nil {
					log.Warnf("failed to unmarshal param_append: %v, skipping", err)
				} else {
					mergeParamAppend(bodyMap, appendParams)
					bodyModified = true
					ra.metrics.ParamAppend = *ra.channel.ParamAppend
				}
			}
			if bodyModified {
				modifiedBody, err := json.Marshal(bodyMap)
				if err != nil {
					log.Warnf("failed to marshal modified body: %v, skipping param_override/param_append", err)
				} else {
					outboundRequest.Body = modifiedBody
				}
			}
		}
	}
	for _, header := range ra.channel.CustomHeader {
		// pipeline 在 raw request middleware 前已经写入 Auth；同名敏感头保持认证配置优先，延续旧 BuildHttpRequest 的覆盖顺序。
		if outboundRequest.Headers.Get(header.HeaderKey) != "" && httpclient.IsSensitiveHeader(header.HeaderKey) {
			continue
		}
		if !strings.Contains(header.HeaderValue, "{client_header:") {
			outboundRequest.Headers.Set(header.HeaderKey, header.HeaderValue)
			continue
		}
		// 占位符只做单趟替换，结果不再递归解析；NAME 不 trim，沿用 http.Header.Get 的大小写不敏感与多值取首个语义。
		value := clientHeaderPlaceholderPattern.ReplaceAllStringFunc(header.HeaderValue, func(placeholder string) string {
			name := placeholder[len("{client_header:") : len(placeholder)-1]
			if ra.clientHeaders != nil {
				return ra.clientHeaders.Get(name)
			}
			return ra.c.Request.Header.Get(name)
		})
		outboundRequest.Headers.Set(header.HeaderKey, value)
	}
}

func (ra *relayAttempt) writeSSEHeartbeat() error {
	if _, err := ra.c.Writer.Write(relaySSEHeartbeatComment); err != nil {
		return err
	}
	ra.c.Writer.Flush()
	return nil
}

func isRelayStreamDone(data []byte) bool {
	return len(data) > 0 && bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]"))
}

func isRelayStreamTerminal(event *httpclient.StreamEvent) bool {
	if isRelayStreamDone(event.Data) || event.Type == "message_stop" || event.Type == "response.completed" {
		return true
	}
	if event.Type == "" && (bytes.Contains(event.Data, []byte(`"message_stop"`)) || bytes.Contains(event.Data, []byte(`"response.completed"`))) {
		var payload struct {
			Type string `json:"type"`
		}
		return json.Unmarshal(event.Data, &payload) == nil && (payload.Type == "message_stop" || payload.Type == "response.completed")
	}
	return false
}

func isRelayStreamEventContent(data []byte) bool {
	return len(data) > 0 && !isRelayStreamDone(data)
}

func (ra *relayAttempt) prepareSSEStreamResponse() error {
	ra.c.Header("Content-Type", "text/event-stream")
	ra.c.Header("Cache-Control", "no-cache")
	ra.c.Header("Connection", "keep-alive")
	ra.c.Header("X-Accel-Buffering", "no")
	ra.c.Status(http.StatusOK)
	if err := ra.writeSSEHeartbeat(); err != nil {
		ra.clientStreamWriteFailed = true
		return fmt.Errorf("failed to write initial SSE heartbeat: %w", err)
	}
	ra.responseCommitted = true
	if ra.metrics != nil && ra.metrics.ID != 0 {
		op.RelayLogStoreResponseCommitted(ra.metrics.ID)
	}
	return nil
}

// processStreamWithHeartbeat 在 pipeline 首事件预读期间提交 SSE 响应并持续发送心跳。
// 只有流结果才把 ticker 和 process cancel 的所有权转交给调用方。
func (ra *relayAttempt) processStreamWithHeartbeat(
	ctx context.Context,
	heartbeatTicker relaySSEHeartbeatTicker,
	process func(context.Context) (*pipeline.Result, error),
) (*pipeline.Result, relaySSEHeartbeatTicker, context.CancelFunc, error) {
	if heartbeatTicker == nil {
		return nil, nil, nil, errors.New("nil SSE heartbeat ticker")
	}
	if err := ra.prepareSSEStreamResponse(); err != nil {
		heartbeatTicker.Stop()
		return nil, nil, nil, err
	}

	processCtx, processCancel := context.WithCancel(ctx)
	type processResult struct {
		result *pipeline.Result
		err    error
	}
	results := make(chan processResult, 1)
	go func() {
		result, err := process(processCtx)
		results <- processResult{result: result, err: err}
	}()

	stop := func() {
		heartbeatTicker.Stop()
		processCancel()
	}
	for {
		select {
		case <-heartbeatTicker.Chan():
			if err := ra.writeSSEHeartbeat(); err != nil {
				ra.clientStreamWriteFailed = true
				stop()
				return nil, nil, nil, fmt.Errorf("failed to write SSE heartbeat: %w", err)
			}
		case <-ctx.Done():
			stop()
			return nil, nil, nil, ctx.Err()
		case processed := <-results:
			if processed.err != nil {
				stop()
				return nil, nil, nil, processed.err
			}
			if processed.result == nil || !processed.result.Stream {
				stop()
				return nil, nil, nil, errors.New("stream request returned non-stream result")
			}
			return processed.result, heartbeatTicker, processCancel, nil
		}
	}
}

// writeStreamWithHeartbeatTicker 把客户端格式流写回请求方，并保留首 token 超时切换通道的行为。
func (ra *relayAttempt) writeStreamWithHeartbeatTicker(
	ctx context.Context,
	clientStream streams.Stream[*httpclient.StreamEvent],
	heartbeatTicker relaySSEHeartbeatTicker,
) error {
	if heartbeatTicker != nil {
		defer heartbeatTicker.Stop()
	}
	if clientStream == nil {
		return fmt.Errorf("empty pipeline stream")
	}
	if heartbeatTicker == nil {
		return errors.New("nil SSE heartbeat ticker")
	}

	firstToken := true
	responseEvents := make([]*httpclient.StreamEvent, 0, 8)
	type sseReadResult struct {
		event *httpclient.StreamEvent
		err   error
	}
	results := make(chan sseReadResult, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		defer close(results)
		defer clientStream.Close()
		defer func() {
			if r := recover(); r != nil {
				log.Warnf("stream reader panic: %v", r)
				select {
				case results <- sseReadResult{err: fmt.Errorf("stream reader panic: %v", r)}:
				case <-done:
				case <-ctx.Done():
				}
			}
		}()
		// Next 可能阻塞等待上游 token；放到协程里让首 token 超时和客户端断开都能及时打断本次通道尝试。
		for clientStream.Next() {
			select {
			case results <- sseReadResult{event: clientStream.Current()}:
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
		if err := clientStream.Err(); err != nil {
			select {
			case results <- sseReadResult{err: err}:
			case <-done:
			case <-ctx.Done():
			}
		}
	}()

	firstTokenTimeoutSec := ra.group.FirstTokenTimeOut
	var firstTokenTimer *time.Timer
	var firstTokenC <-chan time.Time
	if firstTokenTimeoutSec > 0 {
		firstTokenTimer = time.NewTimer(time.Duration(firstTokenTimeoutSec) * time.Second)
		firstTokenC = firstTokenTimer.C
		defer func() {
			if firstTokenTimer != nil {
				firstTokenTimer.Stop()
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			log.Infof("client disconnected, stopping stream")
			_ = clientStream.Close()
			if _, attemptOwned := ctx.Value(relayAttemptContextKey{}).(bool); attemptOwned {
				return ctx.Err()
			}
			return nil
		case <-heartbeatTicker.Chan():
			if err := ra.writeSSEHeartbeat(); err != nil {
				ra.clientStreamWriteFailed = true
				_ = clientStream.Close()
				return fmt.Errorf("failed to write SSE heartbeat: %w", err)
			}
		case <-firstTokenC:
			log.Warnf("first token timeout (%ds), switching channel", firstTokenTimeoutSec)
			_ = clientStream.Close()
			return fmt.Errorf("first token timeout (%ds)", firstTokenTimeoutSec)
		case r, ok := <-results:
			if !ok {
				log.Infof("stream end")
				if len(responseEvents) == 0 {
					return nil
				}
				// 客户端请求流式时，pipeline 只负责边转边写，不会自动生成完整响应体。
				// 这里复用同一个 inbound 聚合器把已经写给客户端的事件合成最终 body，日志只落一次最终响应。
				responseBody, meta, err := ra.inAdapter.AggregateStreamChunks(context.WithoutCancel(ctx), responseEvents)
				if err != nil {
					log.Warnf("failed to aggregate stream response for log: %v", err)
					return nil
				}
				ra.metrics.InternalResponse = responseBody
				ra.metrics.RecordUsage(meta.Usage)
				return nil
			}
			if r.err != nil {
				log.Warnf("failed to read event: %v", r.err)
				return fmt.Errorf("failed to read stream event: %w", r.err)
			}

			if r.event == nil {
				continue
			}
			if failureEvent, failureErr := ra.streamFailure(r.event); failureErr != nil {
				if ctx.Err() != nil {
					_ = clientStream.Close()
					return ra.normalizeUpstreamError(ctx.Err())
				}
				if ra.streamTerminated || ra.streamErrorWritten || ra.clientStreamWriteFailed {
					_ = clientStream.Close()
					return failureErr
				}
				if err := ra.writeSSEEvent(failureEvent); err != nil {
					ra.clientStreamWriteFailed = true
					_ = clientStream.Close()
					return fmt.Errorf("failed to write stream error event: %w", err)
				}
				ra.metrics.InternalResponse = failureEvent.Data
				ra.streamErrorWritten = true
				ra.streamTerminated = true
				_ = clientStream.Close()
				return failureErr
			}
			if len(r.event.Data) == 0 {
				continue
			}
			// 终止帧仍需原样透传并参与最终聚合，但不能被当作首个模型内容事件。
			responseEvents = append(responseEvents, r.event)
			hasContent := isRelayStreamEventContent(r.event.Data)
			if hasContent && firstToken {
				ra.metrics.FirstTokenTime = time.Now()
				firstToken = false
				if firstTokenTimer != nil {
					if !firstTokenTimer.Stop() {
						select {
						case <-firstTokenTimer.C:
						default:
						}
					}
					firstTokenTimer = nil
					firstTokenC = nil
				}
			}

			if err := ra.writeSSEEvent(r.event); err != nil {
				ra.clientStreamWriteFailed = true
				_ = clientStream.Close()
				return fmt.Errorf("failed to write stream event: %w", err)
			}
			if hasContent {
				ra.streamEventWritten = true
			}
			if isRelayStreamTerminal(r.event) {
				ra.streamTerminated = true
			}
		}
	}
}

// relayPipelineMiddleware 承接 octopus 自己的通道级副作用：
// 1. 在 pipeline 发出上游请求前应用渠道参数覆盖和自定义 header；
// 2. 在上游失败时保存 HTTP 状态码，供 key 冷却、熔断和后续选路使用；
// 3. 在非流式响应转成 llm.Response 后记录 usage。
// 4. 从最终出站请求和原始上游响应采集当前尝试的模型声明。
// axonhub/llm 只提供了部分函数式 middleware 构造器，错误状态码和 llm 响应 usage 这两个回调没有公开构造器，
// 所以这里保留一个很薄的结构体实现完整接口，而不是在 relay 主流程里重复 pipeline 的执行逻辑。
type relayPipelineMiddleware struct {
	pipeline.DummyMiddleware
	attempt            *relayAttempt
	upstreamStatusCode int
}

func (m *relayPipelineMiddleware) Name() string {
	return "octopus_relay"
}

func (m *relayPipelineMiddleware) OnOutboundRawRequest(ctx context.Context, request *httpclient.Request) (*httpclient.Request, error) {
	if request.Headers == nil {
		request.Headers = make(http.Header)
	}
	m.attempt.applyChannelRequestOptions(request)
	if m.attempt.channel != nil && m.attempt.internalRequest != nil {
		thinkingEnabled := m.attempt.internalRequest.ReasoningEffort != "none"
		if _, err := ensureDeepSeekChatReasoningContent(
			request,
			m.attempt.channel.Type,
			thinkingEnabled,
		); err != nil {
			log.Warnf("failed to preserve DeepSeek reasoning_content: %v", err)
		}
	}
	m.attempt.recordUpstreamModelName(extractUpstreamModelName(request, m.attempt.channel.Type))
	return request, nil
}

func (m *relayPipelineMiddleware) OnOutboundRawResponse(ctx context.Context, response *httpclient.Response) (*httpclient.Response, error) {
	if response != nil {
		m.attempt.recordResponseModelName(response.Body, false)
		// 普通成功响应只查找字节标记；明确错误候选才进行 JSON 解码。
		if bytes.Contains(response.Body, []byte(`"error"`)) || bytes.Contains(response.Body, []byte(`"errors"`)) || bytes.Contains(response.Body, []byte(`"response.failed"`)) {
			if detail, explicit := decodeRelayErrorDetail(response.Body, response.Headers, ""); explicit {
				source := &relayUpstreamError{
					protocol:   m.attempt.outAdapter.APIFormat(),
					statusCode: response.StatusCode,
					body:       response.Body,
					headers:    response.Headers,
					detail:     detail,
				}
				m.upstreamStatusCode = response.StatusCode
				m.attempt.captureUpstreamError(source)
				return nil, pipeline.WrapUpstreamError(source)
			}
			if m.attempt.outAdapter.APIFormat() == llm.APIFormatOpenAIChatCompletion {
				if body, changed := relayBodyWithoutEmptyError(response.Body); changed {
					converterResponse := *response
					converterResponse.Body = body
					return &converterResponse, nil
				}
			}
		}
	}
	return response, nil
}

func (m *relayPipelineMiddleware) OnOutboundRawStream(ctx context.Context, stream streams.Stream[*httpclient.StreamEvent]) (streams.Stream[*httpclient.StreamEvent], error) {
	return streams.NoNil(streams.MapErr(stream, func(event *httpclient.StreamEvent) (*httpclient.StreamEvent, error) {
		if event == nil {
			return nil, nil
		}
		m.attempt.recordResponseModelName(event.Data, true)
		explicit := event.Type == "error" || event.Type == "response.failed"
		var detail llm.ErrorDetail
		if explicit || bytes.Contains(event.Data, []byte(`"error"`)) || bytes.Contains(event.Data, []byte(`"errors"`)) || bytes.Contains(event.Data, []byte(`"response.failed"`)) {
			var payloadError bool
			detail, payloadError = decodeRelayErrorDetail(event.Data, nil, event.Type)
			explicit = explicit || payloadError
			if !explicit && m.attempt.outAdapter.APIFormat() == llm.APIFormatOpenAIChatCompletion {
				if body, changed := relayBodyWithoutEmptyError(event.Data); changed {
					converterEvent := *event
					converterEvent.Data = body
					return &converterEvent, nil
				}
			}
		}
		if !explicit {
			return event, nil
		}
		if len(bytes.TrimSpace(event.Data)) == 0 {
			detail = llm.ErrorDetail{Message: "upstream stream error", Type: "stream_error"}
		}
		source := &relayUpstreamError{
			protocol:   m.attempt.outAdapter.APIFormat(),
			statusCode: int(m.attempt.upstreamStatusCode.Load()),
			body:       event.Data,
			detail:     detail,
			stream:     true,
		}
		m.attempt.captureUpstreamError(source)
		return nil, source
	})), nil
}

func (m *relayPipelineMiddleware) OnOutboundRawError(ctx context.Context, err error) {
	var upstreamErr *httpclient.Error
	if errors.As(err, &upstreamErr) {
		// pipeline 会把上游错误转换成统一错误返回；这里在转换前记录原始 HTTP 状态码，用于渠道 key 的后续调度决策。
		m.upstreamStatusCode = upstreamErr.StatusCode
		m.attempt.captureUpstreamError(&relayUpstreamError{
			cause:      err,
			protocol:   m.attempt.outAdapter.APIFormat(),
			statusCode: upstreamErr.StatusCode,
			body:       upstreamErr.Body,
			headers:    upstreamErr.Headers,
			detail:     parseRelayErrorDetail(upstreamErr.Body, upstreamErr.Headers),
		})
	}
}

func (m *relayPipelineMiddleware) OnOutboundLlmResponse(ctx context.Context, response *llm.Response) (*llm.Response, error) {
	if response != nil {
		if response.Error != nil {
			return nil, pipeline.WrapUpstreamError(response.Error)
		}
		// 非流式 usage 已由 outbound transformer 标准化到 llm.Response；流式 usage 在最终聚合时记录，避免重复计数。
		m.attempt.metrics.RecordUsage(response.Usage)
	}
	return response, nil
}

// parsedRequestInbound 让 pipeline 复用 relay 在选路前已经解析好的 llm.Request。
// 这样每次候选通道尝试只重新执行 outbound transform 和 HTTP 请求，不会重复读取或解析客户端 body。
type parsedRequestInbound struct {
	transformer.Inbound
	request *llm.Request
}

func (in *parsedRequestInbound) TransformRequest(ctx context.Context, request *httpclient.Request) (*llm.Request, error) {
	if in.request == nil {
		return nil, fmt.Errorf("missing parsed request")
	}
	// relay 已经为选路解析过请求；pipeline 入口复用该结果，避免每次通道尝试再次解析同一份 body。
	in.request.RawRequest = request
	return in.request, nil
}
