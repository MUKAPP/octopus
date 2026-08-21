package relay

import (
	"context"
	"sync"

	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/transformer"
)

type relayAttemptContextKey struct{}
type relayRun struct {
	c               *gin.Context
	inAdapter       transformer.Inbound
	inboundType     llm.APIFormat
	internalRequest *llm.Request
	metrics         *RelayMetrics
	iter            *balancer.Iterator
	group           dbmodel.Group

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

	// responseCommitted is true once the downstream status/headers have been written.
	responseCommitted bool

	// streamEventWritten 表示至少一个真实模型事件已经写入客户端。
	streamEventWritten bool

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
