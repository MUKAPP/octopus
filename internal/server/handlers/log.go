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

	logs, err := op.RelayLogList(c.Request.Context(), startTime, endTime, page, pageSize)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	resp.Success(c, logs)
}

func activeLog(c *gin.Context) {
	resp.Success(c, op.RelayActiveList())
}

func clearLog(c *gin.Context) {
	if err := op.RelayLogClear(c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
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

func streamLog(c *gin.Context) {
	token := c.Query("token")
	if token == "" || !op.RelayLogStreamTokenVerify(token) {
		resp.Error(c, http.StatusUnauthorized, "invalid stream token")
		return
	}

	op.RelayLogStreamTokenRevoke(token)

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
