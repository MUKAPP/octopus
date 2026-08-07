package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/gin-gonic/gin"
)

func parseTestContext(t *testing.T, rawQuery string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req, err := http.NewRequest(http.MethodGet, "/api/v1/stats/analytics?"+rawQuery, nil)
	if err != nil {
		t.Fatalf("构造请求失败：%v", err)
	}
	c.Request = req
	return c
}

func TestParseStatsAnalyticsFilterPresets(t *testing.T) {
	today := time.Now().Format("20060102")

	c := parseTestContext(t, "period=7d")
	f, err := parseStatsAnalyticsFilter(c)
	if err != nil {
		t.Fatalf("7d 解析失败：%v", err)
	}
	if f.StartDate == nil || f.EndDate == nil {
		t.Fatal("7d 应生成日期范围")
	}
	if *f.EndDate != today {
		t.Fatalf("7d end = %q, want %q", *f.EndDate, today)
	}
	if *f.StartDate != time.Now().AddDate(0, 0, -6).Format("20060102") {
		t.Fatalf("7d start = %q", *f.StartDate)
	}

	// 无参数缺省按 30d
	c2 := parseTestContext(t, "")
	f2, err := parseStatsAnalyticsFilter(c2)
	if err != nil {
		t.Fatalf("缺省解析失败：%v", err)
	}
	if *f2.EndDate != today || *f2.StartDate != time.Now().AddDate(0, 0, -29).Format("20060102") {
		t.Fatalf("缺省 30d 范围错误：%s..%s", *f2.StartDate, *f2.EndDate)
	}

	c3 := parseTestContext(t, "period=all")
	f3, err := parseStatsAnalyticsFilter(c3)
	if err != nil {
		t.Fatalf("all 解析失败：%v", err)
	}
	if f3.StartDate != nil || f3.EndDate != nil {
		t.Fatal("all 不应生成日期范围")
	}

	c4 := parseTestContext(t, "period=custom&start_date=20260801&end_date=20260805")
	f4, err := parseStatsAnalyticsFilter(c4)
	if err != nil {
		t.Fatalf("custom 解析失败：%v", err)
	}
	if *f4.StartDate != "20260801" || *f4.EndDate != "20260805" {
		t.Fatalf("custom 范围错误：%s..%s", *f4.StartDate, *f4.EndDate)
	}
}

func TestParseStatsAnalyticsFilterExplicitEmptyActualModel(t *testing.T) {
	c := parseTestContext(t, "period=30d&actual_model=")
	f, err := parseStatsAnalyticsFilter(c)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if f.ActualModelName == nil || *f.ActualModelName != "" {
		t.Fatalf("显式空 actual_model 应生成指向空字符串的指针：%+v", f.ActualModelName)
	}
	if f.RequestModelName != nil {
		t.Fatal("未提供的 model 应为 nil")
	}
	if f.Status != "" {
		t.Fatalf("未提供的 status 应为空：%q", f.Status)
	}
}

func TestParseStatsAnalyticsFilterIDNamePairs(t *testing.T) {
	c := parseTestContext(t, "period=custom&start_date=20260801&end_date=20260802&model=&api_key_id=1&api_key_name=&channel_id=10&channel_name=openai&status=success")
	f, err := parseStatsAnalyticsFilter(c)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if f.APIKeyID == nil || *f.APIKeyID != 1 || f.APIKeyName == nil || *f.APIKeyName != "" {
		t.Fatalf("api key 对解析错误：%+v %+v", f.APIKeyID, f.APIKeyName)
	}
	if f.ChannelID == nil || *f.ChannelID != 10 || f.ChannelName == nil || *f.ChannelName != "openai" {
		t.Fatalf("channel 对解析错误：%+v %+v", f.ChannelID, f.ChannelName)
	}
	if f.Status != "success" {
		t.Fatalf("status = %q, want success", f.Status)
	}
	if f.RequestModelName == nil || *f.RequestModelName != "" {
		t.Fatalf("显式空 model 应保留：%+v", f.RequestModelName)
	}
}

func TestParseStatsAnalyticsFilterErrors(t *testing.T) {
	cases := []string{
		"period=today&start_date=20260801",
		"period=all&end_date=20260801",
		"period=7d&start_date=20260801&end_date=20260805",
		"period=custom&start_date=20260801",
		"period=custom&end_date=20260805",
		"period=custom&start_date=20260805&end_date=20260801",
		"period=custom&start_date=2026-08-01&end_date=20260805",
		"period=custom&start_date=202608011&end_date=20260805",
		"period=custom&start_date=&end_date=20260805",
		"period=custom&start_date=20260231&end_date=20260301",
		"period=custom&start_date=20260101&end_date=20260230",
		"period=custom&start_date=20261301&end_date=20261302",
		"period=custom&start_date=20260000&end_date=20260101",
		"period=weekly",
		"status=ok",
		"api_key_id=1",
		"api_key_name=prod",
		"channel_id=10",
		"channel_name=openai",
		"api_key_id=abc&api_key_name=x",
		"channel_id=abc&channel_name=x",
	}
	for _, q := range cases {
		c := parseTestContext(t, q)
		if _, err := parseStatsAnalyticsFilter(c); err == nil {
			t.Fatalf("query %q 应返回解析错误", q)
		}
	}
}

func TestDecodeDBDumpStatsUsageBareAndWrapper(t *testing.T) {
	row := model.StatsUsage{
		KeyHash:          strings.Repeat("a", 64),
		Date:             "20260805",
		RequestModelName: "gpt-4o",
	}

	// 纯 stats_usage 裸对象
	bare, err := json.Marshal(map[string]any{"stats_usage": []model.StatsUsage{row}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dump model.DBDump
	if err := decodeDBDump(bare, &dump); err != nil {
		t.Fatalf("decodeDBDump 裸对象：%v", err)
	}
	if len(dump.StatsUsage) != 1 || dump.StatsUsage[0].Date != "20260805" {
		t.Fatalf("裸对象解析错误：%+v", dump.StatsUsage)
	}

	// {data:{stats_usage:...}} wrapper
	inner, err := json.Marshal(map[string]any{"stats_usage": []model.StatsUsage{row}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	wrapped, err := json.Marshal(map[string]any{"code": 0, "message": "ok", "data": json.RawMessage(inner)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dump2 model.DBDump
	if err := decodeDBDump(wrapped, &dump2); err != nil {
		t.Fatalf("decodeDBDump wrapper：%v", err)
	}
	if len(dump2.StatsUsage) != 1 || dump2.StatsUsage[0].RequestModelName != "gpt-4o" {
		t.Fatalf("wrapper 解析错误：%+v", dump2.StatsUsage)
	}
}
