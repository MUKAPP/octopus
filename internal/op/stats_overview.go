package op

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
)

type statsUsageSeries struct {
	summary       model.StatsUsage
	daily         map[string]model.StatsUsage
	availableFrom string
}

// statsUsageSeriesLocked 是主页与分析页共用的日聚合；调用方持有 usage 读锁，
// 数据库历史与尚未落库的增量只合并一次，日期沿用请求开始时的服务器业务日。
func statsUsageSeriesLocked(ctx context.Context, filter StatsAnalyticsFilter) (statsUsageSeries, error) {
	series := statsUsageSeries{daily: make(map[string]model.StatsUsage)}
	var rows []model.StatsUsage
	if err := db.GetDB().WithContext(ctx).Table("stats_usages").Scopes(statsUsageFilterScope(filter)).
		Select("date," + statsUsageSumSelect).Group("date").Scan(&rows).Error; err != nil {
		return series, fmt.Errorf("usage daily aggregates: %w", err)
	}
	for _, row := range rows {
		series.daily[row.Date] = row
	}
	for _, delta := range statsUsageDeltaFiltered(filter) {
		row := series.daily[delta.Date]
		row.Date = delta.Date
		row.Add(delta)
		series.daily[delta.Date] = row
	}
	for date, row := range series.daily {
		series.summary.Add(row)
		if series.availableFrom == "" || date < series.availableFrom {
			series.availableFrom = date
		}
	}
	return series, nil
}

// StatsUsageOverview 仅包含主页所需的汇总与日趋势，不计算性能分位数或实体拆分。
type StatsUsageOverview struct {
	Summary model.StatsMetrics
	Daily   []model.StatsDaily
}

func StatsUsageOverviewGet(ctx context.Context, filter StatsAnalyticsFilter) (StatsUsageOverview, error) {
	statsUsageDeltaCacheLock.RLock()
	defer statsUsageDeltaCacheLock.RUnlock()
	series, err := statsUsageSeriesLocked(ctx, filter)
	if err != nil {
		return StatsUsageOverview{}, err
	}
	result := StatsUsageOverview{Summary: series.summary.StatsMetrics, Daily: make([]model.StatsDaily, 0, len(series.daily))}
	for date, row := range series.daily {
		result.Daily = append(result.Daily, model.StatsDaily{Date: date, StatsMetrics: row.StatsMetrics})
	}
	slices.SortFunc(result.Daily, func(a, b model.StatsDaily) int {
		return cmp.Compare(a.Date, b.Date)
	})
	return result, nil
}

// StatsUsageHourlyGet 与分析页共用请求元数据的小时分布；请求按开始时间归属。
// 只补齐今天已经到达的小时，未来小时由前端留空。
func StatsUsageHourlyGet(ctx context.Context) ([]model.StatsHourly, error) {
	now := time.Now()
	today := now.Format("20060102")
	rows, err := statsAnalyticsHourlyGet(ctx, StatsAnalyticsFilter{StartDate: &today, EndDate: &today})
	if err != nil {
		return nil, err
	}
	result := make([]model.StatsHourly, now.Hour()+1)
	for hour := range result {
		result[hour] = model.StatsHourly{Hour: hour, Date: today}
	}
	for _, row := range rows {
		startedAt, err := time.Parse(time.RFC3339, row.Hour)
		if err != nil {
			return nil, fmt.Errorf("invalid request hour %q: %w", row.Hour, err)
		}
		hour := startedAt.Hour()
		if hour >= len(result) {
			// 夏令时回拨后，今天可能已经有当前墙钟小时之后的记录。
			for next := len(result); next <= hour; next++ {
				result = append(result, model.StatsHourly{Hour: next, Date: today})
			}
		}
		result[hour].StatsMetrics.Add(row.StatsMetrics)
	}
	return result, nil
}
