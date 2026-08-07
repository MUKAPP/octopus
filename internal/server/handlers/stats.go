package handlers

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/gin-gonic/gin"
)

func init() {
	router.NewGroupRouter("/api/v1/stats").
		Use(middleware.Auth()).
		AddRoute(
			router.NewRoute("/today", http.MethodGet).
				Handle(getStatsToday),
		).
		AddRoute(
			router.NewRoute("/daily", http.MethodGet).
				Handle(getStatsDaily),
		).
		AddRoute(
			router.NewRoute("/hourly", http.MethodGet).
				Handle(getStatsHourly),
		).
		AddRoute(
			router.NewRoute("/total", http.MethodGet).
				Handle(getStatsTotal),
		).
		AddRoute(
			router.NewRoute("/apikey", http.MethodGet).
				Handle(getStatsAPIKey),
		).
		AddRoute(
			router.NewRoute("/analytics", http.MethodGet).
				Handle(getStatsAnalytics),
		).
		AddRoute(
			router.NewRoute("/analytics/dimensions", http.MethodGet).
				Handle(getStatsAnalyticsDimensions),
		)
}

func getStatsToday(c *gin.Context) {
	resp.Success(c, op.StatsTodayGet())
}

func getStatsDaily(c *gin.Context) {
	statsDaily, err := op.StatsGetDaily(c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, statsDaily)
}

func getStatsHourly(c *gin.Context) {
	resp.Success(c, op.StatsHourlyGet())
}

func getStatsTotal(c *gin.Context) {
	resp.Success(c, op.StatsTotalGet())
}

func getStatsAPIKey(c *gin.Context) {
	resp.Success(c, op.StatsAPIKeyList())
}

func getStatsAnalytics(c *gin.Context) {
	filter, err := parseStatsAnalyticsFilter(c)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	data, err := op.StatsAnalyticsGet(c.Request.Context(), filter)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	// 回显实际采用的服务器日历范围；all-time 时两者为空字符串。
	if filter.StartDate != nil {
		data.ResolvedStartDate = *filter.StartDate
	}
	if filter.EndDate != nil {
		data.ResolvedEndDate = *filter.EndDate
	}
	resp.Success(c, data)
}

func getStatsAnalyticsDimensions(c *gin.Context) {
	dims, err := op.StatsAnalyticsDimensionsGet(c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, dims)
}

var statsAnalyticsDatePattern = regexp.MustCompile(`^\d{8}$`)

// parseStatsAnalyticsFilter 解析并校验分析查询参数。
// period 缺省按 30d；today/7d/30d 由后端按服务器本地日历日计算包含首尾的范围，
// all 产生 nil 日期，custom 必须同时提供两个严格 YYYYMMDD 且开始不晚于结束；
// 其他 period 禁止携带 start/end。API Key 与渠道的 ID/name 必须同时提供。
func parseStatsAnalyticsFilter(c *gin.Context) (op.StatsAnalyticsFilter, error) {
	var filter op.StatsAnalyticsFilter
	query := c.Request.URL.Query()

	period := query.Get("period")
	if period == "" {
		period = "30d"
	}
	hasStart := false
	hasEnd := false
	if _, ok := query["start_date"]; ok {
		hasStart = true
	}
	if _, ok := query["end_date"]; ok {
		hasEnd = true
	}

	now := time.Now()
	today := now.Format("20060102")
	switch period {
	case "today":
		if hasStart || hasEnd {
			return filter, fmt.Errorf("period today must not carry start_date/end_date")
		}
		filter.StartDate = &today
		filter.EndDate = &today
	case "7d":
		if hasStart || hasEnd {
			return filter, fmt.Errorf("period 7d must not carry start_date/end_date")
		}
		start := now.AddDate(0, 0, -6).Format("20060102")
		filter.StartDate = &start
		filter.EndDate = &today
	case "30d":
		if hasStart || hasEnd {
			return filter, fmt.Errorf("period 30d must not carry start_date/end_date")
		}
		start := now.AddDate(0, 0, -29).Format("20060102")
		filter.StartDate = &start
		filter.EndDate = &today
	case "all":
		if hasStart || hasEnd {
			return filter, fmt.Errorf("period all must not carry start_date/end_date")
		}
	case "custom":
		if !hasStart || !hasEnd {
			return filter, fmt.Errorf("period custom requires both start_date and end_date")
		}
		startStr := query.Get("start_date")
		endStr := query.Get("end_date")
		if !statsAnalyticsDatePattern.MatchString(startStr) || !statsAnalyticsDatePattern.MatchString(endStr) {
			return filter, fmt.Errorf("invalid custom date format, want YYYYMMDD")
		}
		// 8 位数字之外还必须是真实存在的日历日（如 20260231 拒绝）。
		if _, err := time.Parse("20060102", startStr); err != nil {
			return filter, fmt.Errorf("invalid custom start_date: %s", startStr)
		}
		if _, err := time.Parse("20060102", endStr); err != nil {
			return filter, fmt.Errorf("invalid custom end_date: %s", endStr)
		}
		if startStr > endStr {
			return filter, fmt.Errorf("custom start_date must not be later than end_date")
		}
		filter.StartDate = &startStr
		filter.EndDate = &endStr
	default:
		return filter, fmt.Errorf("unknown period: %s", period)
	}

	if model, ok := query["model"]; ok {
		modelName := model[0]
		filter.RequestModelName = &modelName
	}
	if actualModel, ok := query["actual_model"]; ok {
		modelName := actualModel[0]
		filter.ActualModelName = &modelName
	}

	apiKeyIDStr, hasAPIKeyID := query["api_key_id"]
	apiKeyName, hasAPIKeyName := query["api_key_name"]
	if hasAPIKeyID != hasAPIKeyName {
		return filter, fmt.Errorf("api_key_id and api_key_name must be provided together")
	}
	if hasAPIKeyID {
		id, err := strconv.Atoi(apiKeyIDStr[0])
		if err != nil {
			return filter, fmt.Errorf("invalid api_key_id: %s", apiKeyIDStr[0])
		}
		name := apiKeyName[0]
		filter.APIKeyID = &id
		filter.APIKeyName = &name
	}

	channelIDStr, hasChannelID := query["channel_id"]
	channelName, hasChannelName := query["channel_name"]
	if hasChannelID != hasChannelName {
		return filter, fmt.Errorf("channel_id and channel_name must be provided together")
	}
	if hasChannelID {
		id, err := strconv.Atoi(channelIDStr[0])
		if err != nil {
			return filter, fmt.Errorf("invalid channel_id: %s", channelIDStr[0])
		}
		name := channelName[0]
		filter.ChannelID = &id
		filter.ChannelName = &name
	}

	status := query.Get("status")
	if status != "" && status != "success" && status != "failed" {
		return filter, fmt.Errorf("invalid status: %s", status)
	}
	filter.Status = status

	return filter, nil
}
