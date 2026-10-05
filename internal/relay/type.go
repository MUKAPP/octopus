package relay

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"

	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/transformer"
)

type relayAttemptContextKey struct{}

type relaySelectionKey struct {
	channelID int
	keyID     int
	modelName string
}

type relayRun struct {
	c               *gin.Context
	inAdapter       transformer.Inbound
	inboundType     llm.APIFormat
	internalRequest *llm.Request
	metrics         *RelayMetrics
	iter            *balancer.Iterator
	group           dbmodel.Group
	probe           bool

	// clientHeaders 保存受支持压缩编码解码前的入站头快照，供 {client_header:NAME} 占位符使用；
	// nil 表示普通请求，占位符直接读取现有 Gin 入站头。
	clientHeaders http.Header

	tried    map[relaySelectionKey]struct{}
	deferred map[relaySelectionKey]struct{}

	candidateStarted   bool
	candidateItemIndex int
	candidateLoaded    bool
	candidateDone      bool
	candidateKeyIndex  int
	candidateChannel   *dbmodel.Channel

	attemptCancelMu sync.Mutex
	attemptCancel   context.CancelFunc
	attemptContext  context.Context
}

// relayAttempt 保存一次上游通道尝试的状态。
type relayAttempt struct {
	*relayRun

	outAdapter   transformer.Outbound
	channel      *dbmodel.Channel
	usedKey      dbmodel.ChannelKey
	attemptIndex int

	// 原始模型名只归属当前尝试，取消后晚到的流事件不能写入共享 metrics。
	modelNamesMu      sync.Mutex
	upstreamModelName string
	responseModelName string

	// 原始 HTTP 响应和错误只归属当前尝试，避免取消后的晚到响应污染下一次尝试。
	upstreamStatusCode atomic.Int32
	upstreamErrorMu    sync.Mutex
	upstreamError      *relayUpstreamError

	// responseCommitted is true once the downstream status/headers have been written.
	responseCommitted bool

	// streamEventWritten 表示至少一个真实模型事件已经写入客户端。
	streamEventWritten bool
	streamErrorWritten bool

	// streamTerminated 表示终止帧已经写入客户端；它不计入首 token。
	streamTerminated        bool
	clientStreamWriteFailed bool
}

func (ra *relayAttempt) responseFinalized() bool {
	return ra.streamEventWritten || ra.streamTerminated || ra.clientStreamWriteFailed
}

func (r *relayRun) beginAttemptContext() (context.Context, context.CancelFunc) {
	r.attemptCancelMu.Lock()
	if r.attemptCancel != nil {
		r.attemptCancel()
	}
	ctx, cancel := context.WithCancel(context.WithValue(r.c.Request.Context(), relayAttemptContextKey{}, true))
	r.attemptContext = ctx
	r.attemptCancel = cancel
	r.attemptCancelMu.Unlock()
	return ctx, cancel
}
func (r *relayRun) endAttemptContext() {
	r.attemptCancelMu.Lock()
	if r.attemptCancel != nil {
		r.attemptCancel()
	}
	r.attemptCancel = nil
	r.attemptContext = nil
	r.attemptCancelMu.Unlock()
}
