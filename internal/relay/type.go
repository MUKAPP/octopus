package relay

import (
	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/transformer"
)

// relayRun 保存一次客户端请求在负载均衡循环中共享的状态。
type relayRun struct {
	c               *gin.Context
	inAdapter       transformer.Inbound
	internalRequest *llm.Request
	metrics         *RelayMetrics
	iter            *balancer.Iterator
	group           dbmodel.Group
}

// relayAttempt 保存一次上游通道尝试的状态。
type relayAttempt struct {
	*relayRun

	outAdapter transformer.Outbound
	channel    *dbmodel.Channel
	usedKey    dbmodel.ChannelKey

	// streamEventWritten 表示至少一个真实模型事件已经写入客户端。
	streamEventWritten      bool
	clientStreamWriteFailed bool
}

func (ra *relayAttempt) responseFinalized() bool {
	return ra.streamEventWritten || ra.clientStreamWriteFailed
}
