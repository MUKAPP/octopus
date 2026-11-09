package op

import "context"

// Cache ratios use only requests whose upstream explicitly reported cache reads.
// Daily aggregate zeroes cannot distinguish an uncached request from missing usage.
type statsCacheCoverageRow struct {
	ID          int
	Name        string
	SampleCount int64
	CachedToken *int64
	InputToken  int64
}

const statsCacheCoverageSelect = "COUNT(*) AS sample_count,SUM(cache_read_token) AS cached_token,COALESCE(SUM(input_token),0) AS input_token"

func applyStatsCacheCoverage(target *StatsAnalyticsMetrics, row statsCacheCoverageRow) {
	target.ObservedCachedToken = row.CachedToken
	target.CacheReadSampleCount = row.SampleCount
	if row.CachedToken != nil && row.InputToken > 0 {
		value := float64(*row.CachedToken) / float64(row.InputToken) * 100
		target.CacheReadRatio = &value
	}
}

func statsAnalyticsCacheCoverage(ctx context.Context, filter StatsAnalyticsFilter, result *StatsAnalyticsResponse) error {
	var summary statsCacheCoverageRow
	if err := metadataQuery(ctx, filter, true).Where("cache_read_token IS NOT NULL").Select(statsCacheCoverageSelect).Scan(&summary).Error; err != nil {
		return err
	}
	applyStatsCacheCoverage(&result.Summary, summary)
	groups := []struct {
		selectColumns string
		groupColumns  string
		targets       map[statsUsageEntityKey]*StatsAnalyticsMetrics
	}{
		{"date AS name", "date", make(map[statsUsageEntityKey]*StatsAnalyticsMetrics)},
		{"request_model_name AS name", "request_model_name", make(map[statsUsageEntityKey]*StatsAnalyticsMetrics)},
		{"actual_model_name AS name", "actual_model_name", make(map[statsUsageEntityKey]*StatsAnalyticsMetrics)},
		{"api_key_id AS id,api_key_name AS name", "api_key_id,api_key_name", make(map[statsUsageEntityKey]*StatsAnalyticsMetrics)},
		{"channel_id AS id,channel_name AS name", "channel_id,channel_name", make(map[statsUsageEntityKey]*StatsAnalyticsMetrics)},
	}
	for i := range result.Trend {
		groups[0].targets[statsUsageEntityKey{Name: result.Trend[i].Date}] = &result.Trend[i].StatsAnalyticsMetrics
	}
	for i := range result.ByModel {
		groups[1].targets[statsUsageEntityKey{Name: result.ByModel[i].Name}] = &result.ByModel[i].StatsAnalyticsMetrics
	}
	for i := range result.ByActualModel {
		groups[2].targets[statsUsageEntityKey{Name: result.ByActualModel[i].Name}] = &result.ByActualModel[i].StatsAnalyticsMetrics
	}
	for i := range result.ByAPIKey {
		groups[3].targets[statsUsageEntityKey{ID: result.ByAPIKey[i].ID, Name: result.ByAPIKey[i].Name}] = &result.ByAPIKey[i].StatsAnalyticsMetrics
	}
	for i := range result.ByChannel {
		groups[4].targets[statsUsageEntityKey{ID: result.ByChannel[i].ID, Name: result.ByChannel[i].Name}] = &result.ByChannel[i].StatsAnalyticsMetrics
	}
	for _, group := range groups {
		var rows []statsCacheCoverageRow
		if err := metadataQuery(ctx, filter, true).Where("cache_read_token IS NOT NULL").Select(group.selectColumns + "," + statsCacheCoverageSelect).Group(group.groupColumns).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if target := group.targets[statsUsageEntityKey{ID: row.ID, Name: row.Name}]; target != nil {
				applyStatsCacheCoverage(target, row)
			}
		}
	}
	return nil
}
