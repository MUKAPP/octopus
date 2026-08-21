package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/gin-gonic/gin"
)

func init() {
	router.NewGroupRouter("/api/v1/log").
		Use(middleware.Auth()).
		AddRoute(
			router.NewRoute("/list", http.MethodGet).
				Handle(listLog),
		).
		AddRoute(
			router.NewRoute("/active", http.MethodGet).
				Handle(activeLog),
		).
		AddRoute(
			router.NewRoute("/clear", http.MethodDelete).
				Handle(clearLog),
		).
		AddRoute(
			router.NewRoute("/stream-token", http.MethodGet).
				Handle(getStreamToken),
		).
		AddRoute(
			router.NewRoute("/:request_id/request-body", http.MethodGet).
				Handle(requestBody),
		).
		AddRoute(
			router.NewRoute("/:request_id/response-body", http.MethodGet).
				Handle(responseBody),
		).
		AddRoute(
			router.NewRoute("/:request_id/:attempt_index/stop", http.MethodPost).
				Handle(stopAttempt),
		)

	router.NewGroupRouter("/api/v1/log").
		AddRoute(
			router.NewRoute("/overview/stream", http.MethodGet).
				Handle(streamLogOverview),
		).
		AddRoute(
			router.NewRoute("/:request_id/stream", http.MethodGet).
				Handle(streamLogDetail),
		)

	router.NewGroupRouter("/api/v1/log").
		AddRoute(
			router.NewRoute("/stream", http.MethodGet).
				Handle(streamLog),
		)
}

func listLog(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	startTimeStr := c.Query("start_time")
	endTimeStr := c.Query("end_time")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var startTime, endTime *int
	if startTimeStr != "" && endTimeStr != "" {
		st, err := strconv.Atoi(startTimeStr)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		et, err := strconv.Atoi(endTimeStr)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		startTime = &st
		endTime = &et
	}
	resp.Success(c, op.RelayLogStoreList(startTime, endTime, page, pageSize))
}

func activeLog(c *gin.Context) {
	resp.Success(c, op.RelayActiveList())
}

func clearLog(c *gin.Context) {
	op.RelayLogStoreClear()
	resp.Success(c, nil)
}
func getStreamToken(c *gin.Context) {
	token, err := op.RelayLogStreamTokenCreate()
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, gin.H{"token": token})
}

func parseRelayLogID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("request_id"), 10, 64)
	if err != nil || id <= 0 {
		resp.Error(c, http.StatusBadRequest, "invalid request id")
		return 0, false
	}
	return id, true
}

func requestBody(c *gin.Context) {
	id, ok := parseRelayLogID(c)
	if !ok {
		return
	}
	body, truncated, exists := op.RelayLogStoreBody(id, false)
	if !exists {
		resp.Error(c, http.StatusNotFound, "log not found")
		return
	}
	resp.Success(c, gin.H{"body": body, "content": body, "truncated": truncated})
}

func responseBody(c *gin.Context) {
	id, ok := parseRelayLogID(c)
	if !ok {
		return
	}
	body, truncated, exists := op.RelayLogStoreBody(id, true)
	if !exists {
		resp.Error(c, http.StatusNotFound, "log not found")
		return
	}
	resp.Success(c, gin.H{"body": body, "content": body, "truncated": truncated})
}

func stopAttempt(c *gin.Context) {
	id, ok := parseRelayLogID(c)
	if !ok {
		return
	}
	attemptIndex, err := strconv.Atoi(c.Param("attempt_index"))
	if err != nil || attemptIndex < 0 {
		resp.Error(c, http.StatusBadRequest, "invalid attempt index")
		return
	}
	if !op.RelayLogStoreStopAttempt(id, attemptIndex) {
		resp.Error(c, http.StatusConflict, "attempt is not running")
		return
	}
	resp.Success(c, gin.H{"stopped": true})
}

func authorizeLogStream(c *gin.Context) bool {
	if !op.RelayLogStreamTokenConsume(c.Query("token")) {
		resp.Error(c, http.StatusUnauthorized, "invalid stream token")
		return false
	}
	return true
}

func writeLogEvent(c *gin.Context, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

func setupLogSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	_, _ = fmt.Fprint(c.Writer, ": connected\n\n")
	c.Writer.Flush()
}

func streamLogOverview(c *gin.Context) {
	if !authorizeLogStream(c) {
		return
	}
	setupLogSSE(c)
	sub := op.RelayLogStoreSubscribeOverview()
	defer op.RelayLogStoreUnsubscribeOverview(sub)
	for _, snapshot := range op.RelayLogStoreList(nil, nil, 1, 100) {
		if err := writeLogEvent(c, op.RelayLogEventOverview, snapshot); err != nil {
			return
		}
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		case snapshot, ok := <-sub:
			if !ok {
				return
			}
			if err := writeLogEvent(c, op.RelayLogEventOverview, snapshot); err != nil {
				return
			}
		}
	}
}

func streamLogDetail(c *gin.Context) {
	if !authorizeLogStream(c) {
		return
	}
	id, ok := parseRelayLogID(c)
	if !ok {
		return
	}
	sub, exists := op.RelayLogStoreSubscribeDetail(id)
	if !exists {
		resp.Error(c, http.StatusNotFound, "log not found")
		return
	}
	defer op.RelayLogStoreUnsubscribeDetail(id, sub)
	setupLogSSE(c)
	snapshot, exists := op.RelayLogStoreGet(id)
	if !exists {
		return
	}
	if err := writeLogEvent(c, op.RelayLogEventOverview, snapshot); err != nil {
		return
	}
	for _, attempt := range snapshot.History {
		event := op.RelayLogEventAttemptFinished
		if attempt.Status == "running" {
			event = op.RelayLogEventAttemptStarted
		}
		if err := writeLogEvent(c, event, attempt); err != nil {
			return
		}
	}
	if snapshot.ResponseCommitted {
		if err := writeLogEvent(c, op.RelayLogEventResponseCommitted, snapshot); err != nil {
			return
		}
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		case event, ok := <-sub:
			if !ok {
				return
			}
			switch event.Type {
			case op.RelayLogEventAttemptStarted, op.RelayLogEventAttemptFinished:
				if event.Attempt != nil {
					if err := writeLogEvent(c, event.Type, *event.Attempt); err != nil {
						return
					}
				}
			case op.RelayLogEventResponseCommitted:
				if event.Overview != nil {
					if err := writeLogEvent(c, event.Type, *event.Overview); err != nil {
						return
					}
				}
			}
		}
	}
}

func streamLog(c *gin.Context) {
	if !op.RelayLogStreamTokenConsume(c.Query("token")) {
		resp.Error(c, http.StatusUnauthorized, "invalid stream token")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 定期发送注释心跳，避免 Cloudflare 等反向代理因长时间无数据返回 524
	const sseHeartbeatInterval = 15 * time.Second
	heartbeatTicker := time.NewTicker(sseHeartbeatInterval)
	defer heartbeatTicker.Stop()

	logChan := op.RelayLogSubscribe()
	defer op.RelayLogUnsubscribe(logChan)
	activeChan := op.RelayActiveSubscribe()
	defer op.RelayActiveUnsubscribe(activeChan)

	ctx := c.Request.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeatTicker.C:
			if _, err := c.Writer.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			c.Writer.Flush()
		case active, ok := <-activeChan:
			if !ok {
				return
			}
			var event []byte
			switch active.Type {
			case op.RelayActiveEventStart:
				data, err := json.Marshal(active.Request)
				if err != nil {
					continue
				}
				event = []byte(fmt.Sprintf("event: active_start\ndata: %s\n\n", data))
			case op.RelayActiveEventUpdate:
				data, err := json.Marshal(active.Request)
				if err != nil {
					continue
				}
				event = []byte(fmt.Sprintf("event: active_update\ndata: %s\n\n", data))
			case op.RelayActiveEventEnd:
				data, _ := json.Marshal(struct {
					ID int64 `json:"id"`
				}{ID: active.ID})
				event = []byte(fmt.Sprintf("event: active_end\ndata: %s\n\n", data))
			default:
				continue
			}
			if _, err := c.Writer.Write(event); err != nil {
				return
			}
			c.Writer.Flush()
		case log, ok := <-logChan:
			if !ok {
				return
			}
			data, err := json.Marshal(log)
			if err != nil {
				continue
			}
			if _, err := c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", data))); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}
