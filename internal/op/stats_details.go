package op

import (
	"context"
	"fmt"
	"math"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"gorm.io/gorm"
)

// StatsUsageRecordRequest makes a completed request indivisible to usage backups/imports.
// Preserve the aggregate delta even if metadata persistence fails, and report the error to the relay.
func StatsUsageRecordRequest(ctx context.Context, usage model.StatsUsage, row *model.RequestMetadata) error {
	statsUsageDeltaCacheLock.Lock()
	defer statsUsageDeltaCacheLock.Unlock()
	if err := statsUsageUpdateLocked(usage); err != nil {
		return fmt.Errorf("update usage stats: %w", err)
	}
	if err := db.GetDB().WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("persist request metadata: %w", err)
	}
	return nil
}

type StatsDistribution struct {
	SampleCount int64    `json:"sample_count"`
	P50         *float64 `json:"p50"`
	P95         *float64 `json:"p95"`
}

type StatsStreamSpeed struct {
	StatsDistribution
	Value *float64 `json:"value"`
}

type StatsPerformance struct {
	TTFTMS                      StatsDistribution `json:"ttft_ms" gorm:"-"`
	TotalDurationMS             StatsDistribution `json:"total_duration_ms" gorm:"-"`
	StreamOutputTokensPerSecond StatsStreamSpeed  `json:"stream_output_tokens_per_second" gorm:"-"`
}

type StatsCoverage struct {
	AvailableFrom      string `json:"available_from"`
	AvailableFromMS    *int64 `json:"available_from_ms"`
	SampleCount        int64  `json:"sample_count"`
	LegacyRequestCount int64  `json:"legacy_request_count"`
	SummarySource      string `json:"summary_source"`
}

type StatsDetailTokens struct {
	UsageSampleCount      int64  `json:"usage_sample_count"`
	CacheReadSampleCount  int64  `json:"cache_read_sample_count"`
	CacheWriteSampleCount int64  `json:"cache_write_sample_count"`
	ReasoningSampleCount  int64  `json:"reasoning_sample_count"`
	CacheReadToken        *int64 `json:"cache_read_token"`
	CacheWriteToken       *int64 `json:"cache_write_token"`
	ReasoningToken        *int64 `json:"reasoning_token"`
}

type StatsReliability struct {
	ExecutedAttempts  int64 `json:"executed_attempts"`
	RetryRequests     int64 `json:"retry_requests"`
	RetryAttempts     int64 `json:"retry_attempts"`
	RecoveredRequests int64 `json:"recovered_requests"`
	CircuitBreakSkips int64 `json:"circuit_break_skips"`
	OtherSkips        int64 `json:"other_skips"`
}

type StatsCategoryCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}
type StatsStatusCodeCount struct {
	StatusCode int   `json:"status_code"`
	Count      int64 `json:"count"`
}

type StatsHourlyDetail struct {
	Hour         string `json:"hour"`
	RequestCount int64  `json:"request_count"`
	model.StatsMetrics
}

// Peaks use server-local calendar minutes of request start; TPM attributes reported
// request usage to that minute, rather than estimating per-token emission timestamps.
type StatsPeaks struct {
	RPM   *int64 `json:"rpm"`
	RPMAt string `json:"rpm_at"`
	TPM   *int64 `json:"tpm"`
	TPMAt string `json:"tpm_at"`
}

type StatsDetailBreakdown struct {
	Name            string  `json:"name"`
	SampleCount     int64   `json:"sample_count"`
	InputToken      int64   `json:"input_token"`
	OutputToken     int64   `json:"output_token"`
	InputCost       float64 `json:"input_cost"`
	OutputCost      float64 `json:"output_cost"`
	RequestSuccess  int64   `json:"request_success"`
	RequestFailed   int64   `json:"request_failed"`
	CacheWriteToken *int64  `json:"cache_write_token"`
	StatsPerformance
}

type StatsAnalyticsDetails struct {
	Coverage          StatsCoverage          `json:"coverage"`
	Performance       StatsPerformance       `json:"performance"`
	Tokens            StatsDetailTokens      `json:"tokens"`
	Reliability       StatsReliability       `json:"reliability"`
	ByErrorCategory   []StatsCategoryCount   `json:"by_error_category"`
	ByStatusCode      []StatsStatusCodeCount `json:"by_status_code"`
	Hourly            []StatsHourlyDetail    `json:"hourly"`
	Peaks             StatsPeaks             `json:"peaks"`
	ByStream          []StatsDetailBreakdown `json:"by_stream"`
	ByReasoningEffort []StatsDetailBreakdown `json:"by_reasoning_effort"`
	ByClientProtocol  []StatsDetailBreakdown `json:"by_client_protocol"`
	ByTaskType        []StatsDetailBreakdown `json:"by_task_type"`
}

func (f StatsAnalyticsFilter) hasDetailFilters() bool {
	return f.Stream != nil || f.ReasoningEffort != nil || f.TaskType != nil || f.ClientProtocol != nil || f.ErrorCategory != nil
}

func metadataQuery(ctx context.Context, f StatsAnalyticsFilter, detailFilters bool) *gorm.DB {
	q := db.GetDB().WithContext(ctx).Model(&model.RequestMetadata{}).Scopes(statsUsageFilterScope(f))
	if detailFilters {
		if f.Stream != nil {
			q = q.Where("stream = ?", *f.Stream)
		}
		for _, condition := range []struct {
			column string
			value  *string
		}{
			{"reasoning_effort", f.ReasoningEffort}, {"task_type", f.TaskType}, {"client_protocol", f.ClientProtocol}, {"error_category", f.ErrorCategory},
		} {
			if condition.value != nil {
				q = q.Where(condition.column+" = ?", *condition.value)
			}
		}
	}
	return q
}

// Each builder returns a fresh GORM query: aggregate selections, grouping and offsets never leak to another query.
type metadataQueryBuilder func() *gorm.DB

// Quantiles use nearest rank over the complete filtered request population. Only two scalar
// rows are read, even for all-time queries; never average daily percentiles or load attempts.
func metadataDistribution(query metadataQueryBuilder, expression string) (StatsDistribution, error) {
	var out StatsDistribution
	if err := query().Count(&out.SampleCount).Error; err != nil {
		return out, err
	}
	if out.SampleCount == 0 {
		return out, nil
	}
	for _, point := range []struct {
		p      float64
		target **float64
	}{{0.5, &out.P50}, {0.95, &out.P95}} {
		offset := int(math.Ceil(float64(out.SampleCount)*point.p)) - 1
		var row struct{ Value float64 }
		if err := query().Select(expression + " AS value").Order(expression + " ASC").Offset(offset).Limit(1).Scan(&row).Error; err != nil {
			return out, err
		}
		value := row.Value
		*point.target = &value
	}
	return out, nil
}

func metadataPerformance(query metadataQueryBuilder) (StatsPerformance, error) {
	var out StatsPerformance
	var err error
	if out.TotalDurationMS, err = metadataDistribution(query, "total_duration_ms"); err != nil {
		return out, err
	}
	ttftQuery := func() *gorm.DB {
		return query().Where("success = ? AND stream = ? AND ttft_ms IS NOT NULL AND ttft_ms >= 0", true, true)
	}
	if out.TTFTMS, err = metadataDistribution(ttftQuery, "ttft_ms"); err != nil {
		return out, err
	}
	speedQuery := func() *gorm.DB {
		return ttftQuery().Where("usage_present = ? AND output_token > 0 AND total_duration_ms > ttft_ms", true)
	}
	if out.StreamOutputTokensPerSecond.StatsDistribution, err = metadataDistribution(speedQuery, "(1000.0 * output_token / (total_duration_ms - ttft_ms))"); err != nil {
		return out, err
	}
	if out.StreamOutputTokensPerSecond.SampleCount > 0 {
		var row struct {
			OutputToken float64
			Duration    float64
		}
		if err := speedQuery().Select("SUM(output_token) AS output_token, SUM(total_duration_ms - ttft_ms) AS duration").Scan(&row).Error; err != nil {
			return out, err
		}
		value := 1000 * row.OutputToken / row.Duration
		out.StreamOutputTokensPerSecond.Value = &value
	}
	return out, nil
}

const metadataTotalsSelect = "COALESCE(SUM(input_token),0) AS input_token,COALESCE(SUM(output_token),0) AS output_token," +
	"COALESCE(SUM(input_cost),0) AS input_cost,COALESCE(SUM(output_cost),0) AS output_cost," +
	"COALESCE(SUM(CASE WHEN success THEN 1 ELSE 0 END),0) AS request_success," +
	"COALESCE(SUM(CASE WHEN success THEN 0 ELSE 1 END),0) AS request_failed"

const metadataAnalyticsSelect = metadataTotalsSelect + ",COALESCE(SUM(total_duration_ms),0) AS wait_time,COALESCE(SUM(cache_read_token),0) AS cached_token"

// statsAnalyticsHourlyGet 供主页与分析页复用开始时间小时分布。
func statsAnalyticsHourlyGet(ctx context.Context, filter StatsAnalyticsFilter) ([]StatsHourlyDetail, error) {
	rows := []StatsHourlyDetail{}
	err := metadataQuery(ctx, filter, true).
		Select("hour,COUNT(*) AS request_count," + metadataTotalsSelect + ",COALESCE(SUM(total_duration_ms),0) AS wait_time").
		Group("hour").Order("hour").Scan(&rows).Error
	return rows, err
}

func statsAnalyticsDetailsGet(ctx context.Context, f StatsAnalyticsFilter, legacyCount int64) (StatsAnalyticsDetails, error) {
	var out StatsAnalyticsDetails
	out.ByErrorCategory = []StatsCategoryCount{}
	out.ByStatusCode = []StatsStatusCodeCount{}
	out.Hourly = []StatsHourlyDetail{}
	query := func() *gorm.DB { return metadataQuery(ctx, f, true) }
	var earliest struct {
		Date        string
		StartedAtMS *int64
	}
	if err := db.GetDB().WithContext(ctx).Model(&model.RequestMetadata{}).Select("COALESCE(MIN(date),'') AS date, MIN(started_at_ms) AS started_at_ms").Scan(&earliest).Error; err != nil {
		return out, err
	}
	out.Coverage.AvailableFrom, out.Coverage.AvailableFromMS = earliest.Date, earliest.StartedAtMS
	if err := query().Count(&out.Coverage.SampleCount).Error; err != nil {
		return out, err
	}
	var baseCount int64
	if err := metadataQuery(ctx, f, false).Count(&baseCount).Error; err != nil {
		return out, err
	}
	out.Coverage.LegacyRequestCount = max(int64(0), legacyCount-baseCount)
	out.Coverage.SummarySource = "daily_aggregates"
	if f.hasDetailFilters() {
		out.Coverage.SummarySource = "request_metadata"
	}
	var err error
	if out.Performance, err = metadataPerformance(query); err != nil {
		return out, err
	}
	if err := query().Select("COALESCE(SUM(CASE WHEN usage_present THEN 1 ELSE 0 END),0) AS usage_sample_count," +
		"COUNT(cache_read_token) AS cache_read_sample_count,COUNT(cache_write_token) AS cache_write_sample_count,COUNT(reasoning_token) AS reasoning_sample_count," +
		"SUM(cache_read_token) AS cache_read_token,SUM(cache_write_token) AS cache_write_token,SUM(reasoning_token) AS reasoning_token").Scan(&out.Tokens).Error; err != nil {
		return out, err
	}
	if err := query().Select("COALESCE(SUM(executed_attempts),0) AS executed_attempts,COALESCE(SUM(retry_attempts),0) AS retry_attempts," +
		"COALESCE(SUM(CASE WHEN retry_request THEN 1 ELSE 0 END),0) AS retry_requests,COALESCE(SUM(CASE WHEN recovered THEN 1 ELSE 0 END),0) AS recovered_requests," +
		"COALESCE(SUM(circuit_break_skips),0) AS circuit_break_skips,COALESCE(SUM(other_skips),0) AS other_skips").Scan(&out.Reliability).Error; err != nil {
		return out, err
	}
	if err := query().Where("success = ?", false).Select("error_category AS name,COUNT(*) AS count").Group("error_category").Order("error_category").Scan(&out.ByErrorCategory).Error; err != nil {
		return out, err
	}
	if err := query().Select("status_code,COUNT(*) AS count").Group("status_code").Order("status_code").Scan(&out.ByStatusCode).Error; err != nil {
		return out, err
	}
	if out.Hourly, err = statsAnalyticsHourlyGet(ctx, f); err != nil {
		return out, err
	}
	for _, peak := range []struct {
		expression string
		target     **int64
		at         *string
		usageOnly  bool
	}{
		{"COUNT(*)", &out.Peaks.RPM, &out.Peaks.RPMAt, false}, {"SUM(input_token + output_token)", &out.Peaks.TPM, &out.Peaks.TPMAt, true},
	} {
		var rows []struct {
			Minute string
			Value  int64
		}
		q := query()
		if peak.usageOnly {
			q = q.Where("usage_present = ?", true)
		}
		if err := q.Select("minute," + peak.expression + " AS value").Group("minute").Order("value DESC,minute ASC").Limit(1).Scan(&rows).Error; err != nil {
			return out, err
		}
		if len(rows) > 0 {
			value := rows[0].Value
			*peak.target, *peak.at = &value, rows[0].Minute
		}
	}
	for _, dimension := range []struct {
		column string
		target *[]StatsDetailBreakdown
	}{
		{"stream", &out.ByStream}, {"reasoning_effort", &out.ByReasoningEffort}, {"client_protocol", &out.ByClientProtocol}, {"task_type", &out.ByTaskType},
	} {
		if *dimension.target, err = metadataBreakdown(query, dimension.column); err != nil {
			return out, err
		}
	}
	return out, nil
}

func metadataBreakdown(query metadataQueryBuilder, column string) ([]StatsDetailBreakdown, error) {
	rows := []StatsDetailBreakdown{}
	name := column
	if column == "stream" {
		name = "CASE WHEN stream THEN 'true' ELSE 'false' END"
	}
	if err := query().Select(name + " AS name,COUNT(*) AS sample_count,SUM(cache_write_token) AS cache_write_token," + metadataTotalsSelect).
		Group(column).Order(column).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		value := rows[i].Name
		rowQuery := func() *gorm.DB {
			if column == "stream" {
				return query().Where("stream = ?", value == "true")
			}
			return query().Where(column+" = ?", value)
		}
		performance, err := metadataPerformance(rowQuery)
		if err != nil {
			return nil, err
		}
		rows[i].StatsPerformance = performance
	}
	return rows, nil
}

// Existing aggregate fields adopt metadata only when a new filter cannot be evaluated on daily rows.
func statsAnalyticsMetadataSummary(ctx context.Context, f StatsAnalyticsFilter, result *StatsAnalyticsResponse) error {
	query := func() *gorm.DB { return metadataQuery(ctx, f, true) }
	if err := query().Select(metadataAnalyticsSelect).Scan(&result.Summary).Error; err != nil {
		return err
	}
	if err := query().Select("COALESCE(MIN(date),'')").Scan(&result.AvailableFrom).Error; err != nil {
		return err
	}
	result.Trend = []StatsAnalyticsTrendPoint{}
	result.ByModel = []StatsAnalyticsModelRow{}
	result.ByActualModel = []StatsAnalyticsModelRow{}
	result.ByAPIKey = []StatsAnalyticsEntityRow{}
	result.ByChannel = []StatsAnalyticsEntityRow{}
	if err := query().Select("date," + metadataAnalyticsSelect).Group("date").Order("date").Scan(&result.Trend).Error; err != nil {
		return err
	}
	if err := query().Select("request_model_name AS name," + metadataAnalyticsSelect).Group("request_model_name").Order("request_model_name").Scan(&result.ByModel).Error; err != nil {
		return err
	}
	if err := query().Select("actual_model_name AS name," + metadataAnalyticsSelect).Group("actual_model_name").Order("actual_model_name").Scan(&result.ByActualModel).Error; err != nil {
		return err
	}
	if err := query().Select("api_key_id AS id,api_key_name AS name," + metadataAnalyticsSelect).Group("api_key_id,api_key_name").Order("api_key_name,api_key_id").Scan(&result.ByAPIKey).Error; err != nil {
		return err
	}
	if err := query().Select("channel_id AS id,channel_name AS name," + metadataAnalyticsSelect).Group("channel_id,channel_name").Order("channel_name,channel_id").Scan(&result.ByChannel).Error; err != nil {
		return err
	}
	return nil
}

func statsAnalyticsDetailDimensions(ctx context.Context, dims *StatsAnalyticsDimensions) error {
	for _, dimension := range []struct {
		column string
		target *[]string
	}{
		{"reasoning_effort", &dims.ReasoningEfforts}, {"client_protocol", &dims.ClientProtocols}, {"task_type", &dims.TaskTypes}, {"error_category", &dims.ErrorCategories},
	} {
		*dimension.target = []string{}
		if err := db.GetDB().WithContext(ctx).Model(&model.RequestMetadata{}).Distinct().Order(dimension.column).Pluck(dimension.column, dimension.target).Error; err != nil {
			return fmt.Errorf("analytics %s: %w", dimension.column, err)
		}
	}
	dims.Streams = []string{}
	var values []bool
	if err := db.GetDB().WithContext(ctx).Model(&model.RequestMetadata{}).Distinct().Order("stream").Pluck("stream", &values).Error; err != nil {
		return err
	}
	for _, value := range values {
		if value {
			dims.Streams = append(dims.Streams, "true")
		} else {
			dims.Streams = append(dims.Streams, "false")
		}
	}
	return nil
}
