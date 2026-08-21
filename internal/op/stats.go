package op

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/cache"
	"github.com/bestruirui/octopus/internal/utils/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var statsDailyCache model.StatsDaily
var statsDailyCacheLock sync.RWMutex

var statsTotalCache model.StatsTotal
var statsTotalCacheLock sync.RWMutex

var statsHourlyCache [24]model.StatsHourly
var statsHourlyCacheLock sync.RWMutex

var statsChannelCache = cache.New[int, model.StatsChannel](16)
var statsChannelCacheNeedUpdate = make(map[int]struct{})
var statsChannelCacheNeedUpdateLock sync.Mutex
var statsChannelCacheLock sync.RWMutex

var statsModelCache = cache.New[int, model.StatsModel](16)
var statsModelCacheNeedUpdate = make(map[int]struct{})
var statsModelCacheNeedUpdateLock sync.Mutex

var statsAPIKeyCache = cache.New[int, model.StatsAPIKey](16)
var statsAPIKeyCacheNeedUpdate = make(map[int]struct{})
var statsAPIKeyCacheNeedUpdateLock sync.Mutex

// statsUsageDeltaCache 只保存上次成功落库后的增量，不在启动时载入历史，
// 内存大小受一个保存周期内出现的维度组合数约束。
var statsUsageDeltaCache = make(map[model.StatsUsageKey]model.StatsUsage)
var statsUsageDeltaCacheLock sync.RWMutex

// StatsUsageUpdate 把一行多维日聚合增量并入有界 delta cache。
// hash 生成失败时原样返回且不改动 cache，绝不退化为只累加 metrics 的零值身份。
func StatsUsageUpdate(usage model.StatsUsage) error {
	key := usage.Key()
	keyHash, err := key.Hash()
	if err != nil {
		return err
	}
	usage.KeyHash = keyHash

	statsUsageDeltaCacheLock.Lock()
	defer statsUsageDeltaCacheLock.Unlock()
	if existing, ok := statsUsageDeltaCache[key]; ok {
		existing.Add(usage)
		statsUsageDeltaCache[key] = existing
	} else {
		statsUsageDeltaCache[key] = usage
	}
	return nil
}

// persistStatsUsage 把 delta cache 原子落库；事务成功后清空，失败时完整保留供下次重试。
func persistStatsUsage(ctx context.Context) error {
	statsUsageDeltaCacheLock.Lock()
	defer statsUsageDeltaCacheLock.Unlock()
	return persistStatsUsageLocked(ctx)
}

// persistStatsUsageLocked 必须在持有 statsUsageDeltaCacheLock 写锁时调用；
// 事务成功后清空 delta cache，失败时完整保留供下次重试。
func persistStatsUsageLocked(ctx context.Context) error {
	if err := persistStatsUsageRows(ctx, db.GetDB().WithContext(ctx), statsUsageDeltaCache); err != nil {
		return err
	}
	clear(statsUsageDeltaCache)
	return nil
}

// persistStatsUsageRows 对指定连接执行 delta 的原子累加事务；清理由调用方负责。
// 每行先尝试原子 UPDATE 累加；无行时用 DoNothing 插入，冲突则重试一次 UPDATE，
// 保证并发 flush 同一 key 时最终得到精确总和。
func persistStatsUsageRows(ctx context.Context, conn *gorm.DB, deltas map[model.StatsUsageKey]model.StatsUsage) error {
	if len(deltas) == 0 {
		return nil
	}
	rows := make([]model.StatsUsage, 0, len(deltas))
	for _, delta := range deltas {
		rows = append(rows, delta)
	}
	err := conn.Transaction(func(tx *gorm.DB) error {
		for _, delta := range rows {
			updates := map[string]any{
				"input_token":     gorm.Expr("input_token + ?", delta.InputToken),
				"output_token":    gorm.Expr("output_token + ?", delta.OutputToken),
				"input_cost":      gorm.Expr("input_cost + ?", delta.InputCost),
				"output_cost":     gorm.Expr("output_cost + ?", delta.OutputCost),
				"wait_time":       gorm.Expr("wait_time + ?", delta.WaitTime),
				"request_success": gorm.Expr("request_success + ?", delta.RequestSuccess),
				"request_failed":  gorm.Expr("request_failed + ?", delta.RequestFailed),
				"cached_token":    gorm.Expr("cached_token + ?", delta.CachedToken),
			}
			result := tx.Model(&model.StatsUsage{}).Where("key_hash = ?", delta.KeyHash).Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&delta)
				if insert.Error != nil {
					return insert.Error
				}
				if insert.RowsAffected == 0 {
					// 另一实例刚创建了同 key，重试一次原子 UPDATE。
					if result := tx.Model(&model.StatsUsage{}).Where("key_hash = ?", delta.KeyHash).Updates(updates); result.Error != nil {
						return result.Error
					}
				}
			}
		}
		return nil
	})
	return err
}

func StatsSaveDBTask() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	log.Debugf("stats save db task started")
	startTime := time.Now()
	defer func() {
		log.Debugf("stats save db task finished, save time: %s", time.Since(startTime))
	}()
	if err := StatsSaveDB(ctx); err != nil {
		log.Errorf("stats save db error: %v", err)
		return
	}
}

func StatsSaveDB(ctx context.Context) error {
	statsTotalCacheLock.RLock()
	totalSnap := statsTotalCache
	statsTotalCacheLock.RUnlock()
	if totalSnap.ID == 0 {
		totalSnap.ID = 1
	}

	statsDailyCacheLock.RLock()
	dailySnap := statsDailyCache
	statsDailyCacheLock.RUnlock()

	statsHourlyCacheLock.RLock()
	hourlyAll := statsHourlyCache
	statsHourlyCacheLock.RUnlock()

	statsChannelCacheNeedUpdateLock.Lock()
	channelIDs := make([]int, 0, len(statsChannelCacheNeedUpdate))
	for id := range statsChannelCacheNeedUpdate {
		channelIDs = append(channelIDs, id)
	}
	statsChannelCacheNeedUpdate = make(map[int]struct{})
	statsChannelCacheNeedUpdateLock.Unlock()

	statsModelCacheNeedUpdateLock.Lock()
	modelIDs := make([]int, 0, len(statsModelCacheNeedUpdate))
	for id := range statsModelCacheNeedUpdate {
		modelIDs = append(modelIDs, id)
	}
	statsModelCacheNeedUpdate = make(map[int]struct{})
	statsModelCacheNeedUpdateLock.Unlock()

	statsAPIKeyCacheNeedUpdateLock.Lock()
	apiKeyIDs := make([]int, 0, len(statsAPIKeyCacheNeedUpdate))
	for id := range statsAPIKeyCacheNeedUpdate {
		apiKeyIDs = append(apiKeyIDs, id)
	}
	statsAPIKeyCacheNeedUpdate = make(map[int]struct{})
	statsAPIKeyCacheNeedUpdateLock.Unlock()

	if err := persistStatsSnapshots(ctx, totalSnap, dailySnap, hourlyAll, channelIDs, modelIDs, apiKeyIDs); err != nil {
		restoreStatsDirty(channelIDs, modelIDs, apiKeyIDs)
		return err
	}
	return persistStatsUsage(ctx)
}

// restoreStatsDirty 在统计持久化失败后恢复本批待写标记。
func restoreStatsDirty(channelIDs, modelIDs, apiKeyIDs []int) {
	statsChannelCacheNeedUpdateLock.Lock()
	for _, id := range channelIDs {
		statsChannelCacheNeedUpdate[id] = struct{}{}
	}
	statsChannelCacheNeedUpdateLock.Unlock()

	statsModelCacheNeedUpdateLock.Lock()
	for _, id := range modelIDs {
		statsModelCacheNeedUpdate[id] = struct{}{}
	}
	statsModelCacheNeedUpdateLock.Unlock()

	statsAPIKeyCacheNeedUpdateLock.Lock()
	for _, id := range apiKeyIDs {
		statsAPIKeyCacheNeedUpdate[id] = struct{}{}
	}
	statsAPIKeyCacheNeedUpdateLock.Unlock()
}

func persistStatsSnapshots(
	ctx context.Context,
	totalSnap model.StatsTotal,
	dailySnap model.StatsDaily,
	hourlyAll [24]model.StatsHourly,
	channelIDs []int,
	modelIDs []int,
	apiKeyIDs []int,
) error {
	dbConn := db.GetDB().WithContext(ctx)

	if result := dbConn.Save(&totalSnap); result.Error != nil {
		return result.Error
	}
	if result := dbConn.Save(&dailySnap); result.Error != nil {
		return result.Error
	}

	todayDate := time.Now().Format("20060102")
	hourlyStats := make([]model.StatsHourly, 0, 24)
	for hour := 0; hour < 24; hour++ {
		if hourlyAll[hour].Date == todayDate {
			hourlyStats = append(hourlyStats, hourlyAll[hour])
		}
	}
	if len(hourlyStats) > 0 {
		if result := dbConn.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "hour"}},
			UpdateAll: true,
		}).Create(&hourlyStats); result.Error != nil {
			return result.Error
		}
	}

	for _, id := range channelIDs {
		statsChannelCacheLock.RLock()
		ch, statsExists := statsChannelCache.Get(id)
		_, channelExists := channelCache.Get(id)
		if !statsExists || !channelExists {
			statsChannelCacheLock.RUnlock()
			continue
		}
		if result := dbConn.Save(&ch); result.Error != nil {
			statsChannelCacheLock.RUnlock()
			return result.Error
		}
		statsChannelCacheLock.RUnlock()
	}

	for _, id := range modelIDs {
		m, ok := statsModelCache.Get(id)
		if !ok {
			continue
		}
		if result := dbConn.Save(&m); result.Error != nil {
			return result.Error
		}
	}

	for _, id := range apiKeyIDs {
		ak, ok := statsAPIKeyCache.Get(id)
		if !ok {
			continue
		}
		if result := dbConn.Save(&ak); result.Error != nil {
			return result.Error
		}
	}

	return nil
}

func statsSaveDBWithDailyOverride(ctx context.Context, dailyOverride model.StatsDaily) error {
	statsTotalCacheLock.RLock()
	totalSnap := statsTotalCache
	statsTotalCacheLock.RUnlock()
	if totalSnap.ID == 0 {
		totalSnap.ID = 1
	}

	statsHourlyCacheLock.RLock()
	hourlyAll := statsHourlyCache
	statsHourlyCacheLock.RUnlock()

	statsChannelCacheNeedUpdateLock.Lock()
	channelIDs := make([]int, 0, len(statsChannelCacheNeedUpdate))
	for id := range statsChannelCacheNeedUpdate {
		channelIDs = append(channelIDs, id)
	}
	statsChannelCacheNeedUpdate = make(map[int]struct{})
	statsChannelCacheNeedUpdateLock.Unlock()

	statsModelCacheNeedUpdateLock.Lock()
	modelIDs := make([]int, 0, len(statsModelCacheNeedUpdate))
	for id := range statsModelCacheNeedUpdate {
		modelIDs = append(modelIDs, id)
	}
	statsModelCacheNeedUpdate = make(map[int]struct{})
	statsModelCacheNeedUpdateLock.Unlock()

	statsAPIKeyCacheNeedUpdateLock.Lock()
	apiKeyIDs := make([]int, 0, len(statsAPIKeyCacheNeedUpdate))
	for id := range statsAPIKeyCacheNeedUpdate {
		apiKeyIDs = append(apiKeyIDs, id)
	}
	statsAPIKeyCacheNeedUpdate = make(map[int]struct{})
	statsAPIKeyCacheNeedUpdateLock.Unlock()

	if err := persistStatsSnapshots(ctx, totalSnap, dailyOverride, hourlyAll, channelIDs, modelIDs, apiKeyIDs); err != nil {

		restoreStatsDirty(channelIDs, modelIDs, apiKeyIDs)
		return err
	}
	return nil

}

func StatsDailyUpdate(ctx context.Context, metrics model.StatsMetrics) error {
	today := time.Now().Format("20060102")

	statsDailyCacheLock.Lock()
	if statsDailyCache.Date == today {
		statsDailyCache.StatsMetrics.Add(metrics)
		statsDailyCacheLock.Unlock()
		return nil
	}

	prevDaily := statsDailyCache
	statsDailyCache = model.StatsDaily{Date: today}
	statsDailyCache.StatsMetrics.Add(metrics)
	statsDailyCacheLock.Unlock()

	return statsSaveDBWithDailyOverride(ctx, prevDaily)
}

func StatsTotalUpdate(metrics model.StatsMetrics) error {
	statsTotalCacheLock.Lock()
	defer statsTotalCacheLock.Unlock()
	if statsTotalCache.ID == 0 {
		statsTotalCache.ID = 1
	}
	statsTotalCache.StatsMetrics.Add(metrics)
	return nil
}

func StatsChannelUpdate(channelID int, metrics model.StatsMetrics) error {

	if _, ok := channelCache.Get(channelID); !ok {
		return nil
	}

	statsChannelCacheLock.Lock()
	defer statsChannelCacheLock.Unlock()

	channelStats, ok := statsChannelCache.Get(channelID)

	if !ok {
		channelStats = model.StatsChannel{
			ChannelID: channelID,
		}
	}

	channelStats.StatsMetrics.Add(metrics)
	statsChannelCache.Set(channelID, channelStats)
	statsChannelCacheNeedUpdateLock.Lock()
	statsChannelCacheNeedUpdate[channelID] = struct{}{}
	statsChannelCacheNeedUpdateLock.Unlock()
	return nil
}

func StatsHourlyUpdate(metrics model.StatsMetrics) error {
	now := time.Now()
	nowHour := now.Hour()
	todayDate := time.Now().Format("20060102")

	statsHourlyCacheLock.Lock()
	defer statsHourlyCacheLock.Unlock()

	if statsHourlyCache[nowHour].Date != todayDate {
		statsHourlyCache[nowHour] = model.StatsHourly{
			Hour: nowHour,
			Date: todayDate,
		}
	}

	statsHourlyCache[nowHour].StatsMetrics.Add(metrics)
	return nil
}

// StatsModelUpdate 累加仍然存在的渠道模型统计并标记为待持久化。
func StatsModelUpdate(stats model.StatsModel) error {
	statsModelCacheNeedUpdateLock.Lock()
	defer statsModelCacheNeedUpdateLock.Unlock()
	if _, ok := channelCache.Get(stats.ChannelID); !ok {
		return nil
	}
	modelCache, ok := statsModelCache.Get(stats.ID)
	if !ok {
		modelCache = model.StatsModel{
			ID:        stats.ID,
			Name:      stats.Name,
			ChannelID: stats.ChannelID,
		}
	}
	modelCache.StatsMetrics.Add(stats.StatsMetrics)
	statsModelCache.Set(stats.ID, modelCache)
	statsModelCacheNeedUpdate[stats.ID] = struct{}{}
	return nil

}

func StatsAPIKeyUpdate(apiKeyID int, metrics model.StatsMetrics) error {
	statsAPIKeyCacheNeedUpdateLock.Lock()
	defer statsAPIKeyCacheNeedUpdateLock.Unlock()
	apiKeyCache, ok := statsAPIKeyCache.Get(apiKeyID)
	if !ok {
		apiKeyCache = model.StatsAPIKey{
			APIKeyID: apiKeyID,
		}
	}
	apiKeyCache.StatsMetrics.Add(metrics)
	statsAPIKeyCache.Set(apiKeyID, apiKeyCache)
	statsAPIKeyCacheNeedUpdate[apiKeyID] = struct{}{}
	return nil
}

func statsChannelCacheClear(id int) {
	statsChannelCacheLock.Lock()
	defer statsChannelCacheLock.Unlock()
	statsChannelCacheClearLocked(id)
}

func statsChannelCacheClearLocked(id int) {
	statsChannelCache.Del(id)
	statsChannelCacheNeedUpdateLock.Lock()
	delete(statsChannelCacheNeedUpdate, id)
	statsChannelCacheNeedUpdateLock.Unlock()
}

func StatsChannelDel(id int) error {
	statsChannelCacheLock.Lock()
	defer statsChannelCacheLock.Unlock()

	statsChannelCacheClearLocked(id)
	return db.GetDB().Where("channel_id = ?", id).Delete(&model.StatsChannel{}).Error
}

func StatsAPIKeyDel(id int) error {
	statsAPIKeyCacheNeedUpdateLock.Lock()
	if _, ok := statsAPIKeyCache.Get(id); !ok {
		statsAPIKeyCacheNeedUpdateLock.Unlock()
		return nil
	}
	statsAPIKeyCache.Del(id)
	delete(statsAPIKeyCacheNeedUpdate, id)
	statsAPIKeyCacheNeedUpdateLock.Unlock()
	return db.GetDB().Delete(&model.StatsAPIKey{}, id).Error
}

func StatsTotalGet() model.StatsTotal {
	statsTotalCacheLock.RLock()
	defer statsTotalCacheLock.RUnlock()
	return statsTotalCache
}

func StatsTodayGet() model.StatsDaily {
	statsDailyCacheLock.RLock()
	defer statsDailyCacheLock.RUnlock()
	return statsDailyCache
}

func StatsChannelGet(id int) model.StatsChannel {

	statsChannelCacheLock.Lock()
	defer statsChannelCacheLock.Unlock()

	stats, ok := statsChannelCache.Get(id)
	if !ok {
		tmp := model.StatsChannel{
			ChannelID: id,
		}
		statsChannelCache.Set(id, tmp)
		statsChannelCacheNeedUpdate[id] = struct{}{}
		return tmp
	}
	return stats
}

func StatsAPIKeyGet(id int) model.StatsAPIKey {
	if stats, ok := statsAPIKeyCache.Get(id); ok {
		return stats
	}
	statsAPIKeyCacheNeedUpdateLock.Lock()
	defer statsAPIKeyCacheNeedUpdateLock.Unlock()
	stats, ok := statsAPIKeyCache.Get(id)
	if !ok {
		tmp := model.StatsAPIKey{
			APIKeyID: id,
		}
		statsAPIKeyCache.Set(id, tmp)
		statsAPIKeyCacheNeedUpdate[id] = struct{}{}
		return tmp
	}
	return stats
}

func StatsAPIKeyList() []model.StatsAPIKey {
	apiKeys := make([]model.StatsAPIKey, 0, statsAPIKeyCache.Len())
	for _, v := range statsAPIKeyCache.GetAll() {
		apiKeys = append(apiKeys, v)
	}
	return apiKeys
}

func StatsHourlyGet() []model.StatsHourly {
	now := time.Now()
	currentHour := now.Hour()
	todayDate := time.Now().Format("20060102")

	statsHourlyCacheLock.RLock()
	defer statsHourlyCacheLock.RUnlock()

	result := make([]model.StatsHourly, 0, currentHour+1)

	for hour := 0; hour <= currentHour; hour++ {
		if statsHourlyCache[hour].Date == todayDate {
			result = append(result, statsHourlyCache[hour])
		} else {
			result = append(result, model.StatsHourly{
				Hour: hour,
				Date: todayDate,
			})
		}
	}

	return result
}

func StatsGetDaily(ctx context.Context) ([]model.StatsDaily, error) {
	var statsDaily []model.StatsDaily
	result := db.GetDB().WithContext(ctx).Find(&statsDaily)
	if result.Error != nil {
		return nil, result.Error
	}
	return statsDaily, nil
}

func statsRefreshCache(ctx context.Context) error {
	dbConn := db.GetDB().WithContext(ctx)
	today := time.Now().Format("20060102")

	var loadedDaily model.StatsDaily
	result := dbConn.Last(&loadedDaily)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to get daily stats: %v", result.Error)
	}
	if result.RowsAffected == 0 || loadedDaily.Date != today {
		loadedDaily = model.StatsDaily{Date: today}
	}

	var loadedTotal model.StatsTotal
	result = dbConn.First(&loadedTotal)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to get total stats: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		loadedTotal = model.StatsTotal{ID: 1}
	} else if loadedTotal.ID == 0 {
		loadedTotal.ID = 1
	}

	var loadedChannels []model.StatsChannel
	result = dbConn.Find(&loadedChannels)
	if result.Error != nil {
		return fmt.Errorf("failed to get channels: %v", result.Error)
	}

	var loadedHourly []model.StatsHourly
	result = dbConn.Find(&loadedHourly)
	if result.Error != nil {
		return fmt.Errorf("failed to get hourly stats: %v", result.Error)
	}

	statsDailyCacheLock.Lock()
	statsDailyCache = loadedDaily
	statsDailyCacheLock.Unlock()

	statsTotalCacheLock.Lock()
	statsTotalCache = loadedTotal
	statsTotalCacheLock.Unlock()

	statsChannelCacheLock.Lock()
	statsChannelCache.Clear()
	statsChannelCacheNeedUpdateLock.Lock()
	statsChannelCacheNeedUpdate = make(map[int]struct{})
	statsChannelCacheNeedUpdateLock.Unlock()
	for _, v := range loadedChannels {
		statsChannelCache.Set(v.ChannelID, v)
	}
	statsChannelCacheLock.Unlock()

	var loadedAPIKeys []model.StatsAPIKey
	result = dbConn.Find(&loadedAPIKeys)
	if result.Error != nil {
		return fmt.Errorf("failed to get api key stats: %v", result.Error)
	}

	statsAPIKeyCache.Clear()
	statsAPIKeyCacheNeedUpdateLock.Lock()
	statsAPIKeyCacheNeedUpdate = make(map[int]struct{})
	statsAPIKeyCacheNeedUpdateLock.Unlock()
	for _, v := range loadedAPIKeys {
		statsAPIKeyCache.Set(v.APIKeyID, v)
	}

	statsHourlyCacheLock.Lock()
	statsHourlyCache = [24]model.StatsHourly{}
	for _, v := range loadedHourly {
		if v.Hour >= 0 && v.Hour < 24 {
			statsHourlyCache[v.Hour] = v
		}
	}
	statsHourlyCacheLock.Unlock()

	return nil
}

// StatsAnalyticsFilter 是分析查询的组合条件。nil 表示不过滤；
// 指向空字符串表示精确筛选 unknown/unassigned。API Key 与渠道的
// ID/name 指针必须同时为 nil 或同时非 nil（由 handler 保证）。
type StatsAnalyticsFilter struct {
	StartDate        *string
	EndDate          *string
	RequestModelName *string
	ActualModelName  *string
	APIKeyID         *int
	APIKeyName       *string
	ChannelID        *int
	ChannelName      *string
	Status           string // "" | "success" | "failed"
}

// StatsAnalyticsMetrics 与分析响应中的原子指标，前端派生请求数、总 token、
// 总成本、成功率和平均耗时。
type StatsAnalyticsMetrics struct {
	InputToken     int64   `json:"input_token"`
	OutputToken    int64   `json:"output_token"`
	InputCost      float64 `json:"input_cost"`
	OutputCost     float64 `json:"output_cost"`
	WaitTime       int64   `json:"wait_time"`
	RequestSuccess int64   `json:"request_success"`
	RequestFailed  int64   `json:"request_failed"`
	CachedToken    int64   `json:"cached_token"`
}

type StatsAnalyticsTrendPoint struct {
	Date string `json:"date"`
	StatsAnalyticsMetrics
}

type StatsAnalyticsModelRow struct {
	Name string `json:"name"`
	StatsAnalyticsMetrics
}

type StatsAnalyticsEntityRow struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	StatsAnalyticsMetrics
}

// StatsAnalyticsResponse 是分析接口的固定响应形状；resolved 日期由 handler
// 按实际采用的服务器日历范围填充，all-time 时为空字符串。
type StatsAnalyticsResponse struct {
	AvailableFrom     string                     `json:"available_from"`
	ResolvedStartDate string                     `json:"resolved_start_date"`
	ResolvedEndDate   string                     `json:"resolved_end_date"`
	Summary           StatsAnalyticsMetrics      `json:"summary"`
	Trend             []StatsAnalyticsTrendPoint `json:"trend"`
	ByModel           []StatsAnalyticsModelRow   `json:"by_model"`
	ByActualModel     []StatsAnalyticsModelRow   `json:"by_actual_model"`
	ByAPIKey          []StatsAnalyticsEntityRow  `json:"by_api_key"`
	ByChannel         []StatsAnalyticsEntityRow  `json:"by_channel"`
}

type StatsAnalyticsEntityRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// StatsAnalyticsDimensions 是筛选下拉的选项来源，来自数据库实际出现的维度值，
// 因而已删除或重命名实体的历史数据仍可筛选；空名称保留为可选择的
// “未知/未分配”项。
type StatsAnalyticsDimensions struct {
	Models       []string                  `json:"models"`
	ActualModels []string                  `json:"actual_models"`
	APIKeys      []StatsAnalyticsEntityRef `json:"api_keys"`
	Channels     []StatsAnalyticsEntityRef `json:"channels"`
}

type statsUsageEntityKey struct {
	ID   int
	Name string
}

// statsUsageFilterScope 为所有分析查询复用同一组 WHERE 条件。
func statsUsageFilterScope(filter StatsAnalyticsFilter) func(*gorm.DB) *gorm.DB {
	return func(q *gorm.DB) *gorm.DB {
		if filter.StartDate != nil {
			q = q.Where("date >= ?", *filter.StartDate)
		}
		if filter.EndDate != nil {
			q = q.Where("date <= ?", *filter.EndDate)
		}
		if filter.RequestModelName != nil {
			q = q.Where("request_model_name = ?", *filter.RequestModelName)
		}
		if filter.ActualModelName != nil {
			q = q.Where("actual_model_name = ?", *filter.ActualModelName)
		}
		if filter.APIKeyID != nil {
			q = q.Where("api_key_id = ?", *filter.APIKeyID)
		}
		if filter.APIKeyName != nil {
			q = q.Where("api_key_name = ?", *filter.APIKeyName)
		}
		if filter.ChannelID != nil {
			q = q.Where("channel_id = ?", *filter.ChannelID)
		}
		if filter.ChannelName != nil {
			q = q.Where("channel_name = ?", *filter.ChannelName)
		}
		switch filter.Status {
		case "success":
			q = q.Where("success = ?", true)
		case "failed":
			q = q.Where("success = ?", false)
		}
		return q
	}
}

// statsUsageKeyMatches 判定 delta 行是否命中 filter，与 statsUsageFilterScope 语义一致。
func statsUsageKeyMatches(key model.StatsUsageKey, filter StatsAnalyticsFilter) bool {
	if filter.StartDate != nil && key.Date < *filter.StartDate {
		return false
	}
	if filter.EndDate != nil && key.Date > *filter.EndDate {
		return false
	}
	if filter.RequestModelName != nil && key.RequestModelName != *filter.RequestModelName {
		return false
	}
	if filter.ActualModelName != nil && key.ActualModelName != *filter.ActualModelName {
		return false
	}
	if filter.APIKeyID != nil && key.APIKeyID != *filter.APIKeyID {
		return false
	}
	if filter.APIKeyName != nil && key.APIKeyName != *filter.APIKeyName {
		return false
	}
	if filter.ChannelID != nil && key.ChannelID != *filter.ChannelID {
		return false
	}
	if filter.ChannelName != nil && key.ChannelName != *filter.ChannelName {
		return false
	}
	switch filter.Status {
	case "success":
		if !key.Success {
			return false
		}
	case "failed":
		if key.Success {
			return false
		}
	}
	return true
}

// statsUsageDeltaFiltered 返回符合 filter 的 delta 行；调用方必须持有
// statsUsageDeltaCacheLock 读锁。
func statsUsageDeltaFiltered(filter StatsAnalyticsFilter) []model.StatsUsage {
	out := make([]model.StatsUsage, 0, len(statsUsageDeltaCache))
	for key, delta := range statsUsageDeltaCache {
		if statsUsageKeyMatches(key, filter) {
			out = append(out, delta)
		}
	}
	return out
}

func statsAnalyticsMetricsOf(u model.StatsUsage) StatsAnalyticsMetrics {
	return StatsAnalyticsMetrics{
		InputToken:     u.InputToken,
		OutputToken:    u.OutputToken,
		InputCost:      u.InputCost,
		OutputCost:     u.OutputCost,
		WaitTime:       u.WaitTime,
		RequestSuccess: u.RequestSuccess,
		RequestFailed:  u.RequestFailed,
		CachedToken:    u.CachedToken,
	}
}

const statsUsageSumSelect = "" +
	"COALESCE(SUM(input_token),0) AS input_token," +
	"COALESCE(SUM(output_token),0) AS output_token," +
	"COALESCE(SUM(input_cost),0) AS input_cost," +
	"COALESCE(SUM(output_cost),0) AS output_cost," +
	"COALESCE(SUM(wait_time),0) AS wait_time," +
	"COALESCE(SUM(request_success),0) AS request_success," +
	"COALESCE(SUM(request_failed),0) AS request_failed," +
	"COALESCE(SUM(cached_token),0) AS cached_token"

// StatsAnalyticsGet 返回组合筛选后的汇总、趋势与各维度拆分；整个查询期间
// 持有 usage 读锁，避免与 flush 交错造成重复或遗漏。
func StatsAnalyticsGet(ctx context.Context, filter StatsAnalyticsFilter) (StatsAnalyticsResponse, error) {
	var result StatsAnalyticsResponse
	statsUsageDeltaCacheLock.RLock()
	defer statsUsageDeltaCacheLock.RUnlock()

	dbConn := db.GetDB().WithContext(ctx)
	scope := statsUsageFilterScope(filter)

	var dbSummary model.StatsUsage
	if err := dbConn.Table("stats_usages").Scopes(scope).Select(statsUsageSumSelect).Scan(&dbSummary).Error; err != nil {
		return result, fmt.Errorf("analytics summary: %w", err)
	}

	var earliestRow struct {
		Date string
	}
	if err := dbConn.Table("stats_usages").Scopes(scope).
		Select("COALESCE(MIN(date),'') AS date").Scan(&earliestRow).Error; err != nil {
		return result, fmt.Errorf("analytics earliest date: %w", err)
	}

	var dbTrend []model.StatsUsage
	if err := dbConn.Table("stats_usages").Scopes(scope).
		Select("date," + statsUsageSumSelect).Group("date").Scan(&dbTrend).Error; err != nil {
		return result, fmt.Errorf("analytics trend: %w", err)
	}

	var dbByModel []model.StatsUsage
	if err := dbConn.Table("stats_usages").Scopes(scope).
		Select("request_model_name," + statsUsageSumSelect).Group("request_model_name").Scan(&dbByModel).Error; err != nil {
		return result, fmt.Errorf("analytics by model: %w", err)
	}

	var dbByActualModel []model.StatsUsage
	if err := dbConn.Table("stats_usages").Scopes(scope).
		Select("actual_model_name," + statsUsageSumSelect).Group("actual_model_name").Scan(&dbByActualModel).Error; err != nil {
		return result, fmt.Errorf("analytics by actual model: %w", err)
	}

	var dbByAPIKey []model.StatsUsage
	if err := dbConn.Table("stats_usages").Scopes(scope).
		Select("api_key_id,api_key_name," + statsUsageSumSelect).Group("api_key_id,api_key_name").Scan(&dbByAPIKey).Error; err != nil {
		return result, fmt.Errorf("analytics by api key: %w", err)
	}

	var dbByChannel []model.StatsUsage
	if err := dbConn.Table("stats_usages").Scopes(scope).
		Select("channel_id,channel_name," + statsUsageSumSelect).Group("channel_id,channel_name").Scan(&dbByChannel).Error; err != nil {
		return result, fmt.Errorf("analytics by channel: %w", err)
	}

	summary := dbSummary
	trendMap := make(map[string]model.StatsUsage, len(dbTrend))
	for _, row := range dbTrend {
		trendMap[row.Date] = row
	}
	byModelMap := make(map[string]model.StatsUsage, len(dbByModel))
	for _, row := range dbByModel {
		byModelMap[row.RequestModelName] = row
	}
	byActualModelMap := make(map[string]model.StatsUsage, len(dbByActualModel))
	for _, row := range dbByActualModel {
		byActualModelMap[row.ActualModelName] = row
	}
	byAPIKeyMap := make(map[statsUsageEntityKey]model.StatsUsage, len(dbByAPIKey))
	for _, row := range dbByAPIKey {
		byAPIKeyMap[statsUsageEntityKey{ID: row.APIKeyID, Name: row.APIKeyName}] = row
	}
	byChannelMap := make(map[statsUsageEntityKey]model.StatsUsage, len(dbByChannel))
	for _, row := range dbByChannel {
		byChannelMap[statsUsageEntityKey{ID: row.ChannelID, Name: row.ChannelName}] = row
	}

	availableFrom := earliestRow.Date
	for _, delta := range statsUsageDeltaFiltered(filter) {
		summary.Add(delta)
		key := delta.Key()

		row, ok := trendMap[key.Date]
		if !ok {
			row = model.StatsUsage{Date: key.Date}
		}
		row.Add(delta)
		trendMap[key.Date] = row

		row, ok = byModelMap[key.RequestModelName]
		if !ok {
			row = model.StatsUsage{RequestModelName: key.RequestModelName}
		}
		row.Add(delta)
		byModelMap[key.RequestModelName] = row

		row, ok = byActualModelMap[key.ActualModelName]
		if !ok {
			row = model.StatsUsage{ActualModelName: key.ActualModelName}
		}
		row.Add(delta)
		byActualModelMap[key.ActualModelName] = row

		row, ok = byAPIKeyMap[statsUsageEntityKey{ID: key.APIKeyID, Name: key.APIKeyName}]
		if !ok {
			row = model.StatsUsage{APIKeyID: key.APIKeyID, APIKeyName: key.APIKeyName}
		}
		row.Add(delta)
		byAPIKeyMap[statsUsageEntityKey{ID: key.APIKeyID, Name: key.APIKeyName}] = row

		row, ok = byChannelMap[statsUsageEntityKey{ID: key.ChannelID, Name: key.ChannelName}]
		if !ok {
			row = model.StatsUsage{ChannelID: key.ChannelID, ChannelName: key.ChannelName}
		}
		row.Add(delta)
		byChannelMap[statsUsageEntityKey{ID: key.ChannelID, Name: key.ChannelName}] = row

		if availableFrom == "" || key.Date < availableFrom {
			availableFrom = key.Date
		}
	}

	result.AvailableFrom = availableFrom
	result.Summary = statsAnalyticsMetricsOf(summary)

	result.Trend = make([]StatsAnalyticsTrendPoint, 0, len(trendMap))
	for date, row := range trendMap {
		result.Trend = append(result.Trend, StatsAnalyticsTrendPoint{
			Date:                  date,
			StatsAnalyticsMetrics: statsAnalyticsMetricsOf(row),
		})
	}
	slices.SortFunc(result.Trend, func(a, b StatsAnalyticsTrendPoint) int {
		return cmp.Compare(a.Date, b.Date)
	})

	result.ByModel = make([]StatsAnalyticsModelRow, 0, len(byModelMap))
	for name, row := range byModelMap {
		result.ByModel = append(result.ByModel, StatsAnalyticsModelRow{
			Name:                  name,
			StatsAnalyticsMetrics: statsAnalyticsMetricsOf(row),
		})
	}
	slices.SortFunc(result.ByModel, func(a, b StatsAnalyticsModelRow) int {
		return cmp.Compare(a.Name, b.Name)
	})

	result.ByActualModel = make([]StatsAnalyticsModelRow, 0, len(byActualModelMap))
	for name, row := range byActualModelMap {
		result.ByActualModel = append(result.ByActualModel, StatsAnalyticsModelRow{
			Name:                  name,
			StatsAnalyticsMetrics: statsAnalyticsMetricsOf(row),
		})
	}
	slices.SortFunc(result.ByActualModel, func(a, b StatsAnalyticsModelRow) int {
		return cmp.Compare(a.Name, b.Name)
	})

	result.ByAPIKey = statsAnalyticsEntityRowsFrom(byAPIKeyMap)
	result.ByChannel = statsAnalyticsEntityRowsFrom(byChannelMap)

	return result, nil
}

func statsAnalyticsEntityRowsFrom(m map[statsUsageEntityKey]model.StatsUsage) []StatsAnalyticsEntityRow {
	rows := make([]StatsAnalyticsEntityRow, 0, len(m))
	for key, row := range m {
		rows = append(rows, StatsAnalyticsEntityRow{
			ID:                    key.ID,
			Name:                  key.Name,
			StatsAnalyticsMetrics: statsAnalyticsMetricsOf(row),
		})
	}
	slices.SortFunc(rows, func(a, b StatsAnalyticsEntityRow) int {
		if a.Name != b.Name {
			return cmp.Compare(a.Name, b.Name)
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return rows
}

// StatsAnalyticsDimensionsGet 返回数据库与 delta 中实际出现的维度选项。
func StatsAnalyticsDimensionsGet(ctx context.Context) (StatsAnalyticsDimensions, error) {
	var dims StatsAnalyticsDimensions
	statsUsageDeltaCacheLock.RLock()
	defer statsUsageDeltaCacheLock.RUnlock()

	dbConn := db.GetDB().WithContext(ctx)

	var dbModels []string
	if err := dbConn.Table("stats_usages").Distinct().Pluck("request_model_name", &dbModels).Error; err != nil {
		return dims, fmt.Errorf("analytics models: %w", err)
	}
	var dbActualModels []string
	if err := dbConn.Table("stats_usages").Distinct().Pluck("actual_model_name", &dbActualModels).Error; err != nil {
		return dims, fmt.Errorf("analytics actual models: %w", err)
	}

	type entityScan struct {
		ID   int    `gorm:"column:api_key_id"`
		Name string `gorm:"column:api_key_name"`
	}
	var dbAPIKeys []entityScan
	if err := dbConn.Table("stats_usages").Distinct().
		Select("api_key_id, api_key_name").Scan(&dbAPIKeys).Error; err != nil {
		return dims, fmt.Errorf("analytics api keys: %w", err)
	}
	type channelScan struct {
		ID   int    `gorm:"column:channel_id"`
		Name string `gorm:"column:channel_name"`
	}
	var dbChannels []channelScan
	if err := dbConn.Table("stats_usages").Distinct().
		Select("channel_id, channel_name").Scan(&dbChannels).Error; err != nil {
		return dims, fmt.Errorf("analytics channels: %w", err)
	}

	modelSet := make(map[string]struct{}, len(dbModels))
	for _, m := range dbModels {
		modelSet[m] = struct{}{}
	}
	actualModelSet := make(map[string]struct{}, len(dbActualModels))
	for _, m := range dbActualModels {
		actualModelSet[m] = struct{}{}
	}
	apiKeySet := make(map[statsUsageEntityKey]struct{}, len(dbAPIKeys))
	for _, row := range dbAPIKeys {
		apiKeySet[statsUsageEntityKey{ID: row.ID, Name: row.Name}] = struct{}{}
	}
	channelSet := make(map[statsUsageEntityKey]struct{}, len(dbChannels))
	for _, row := range dbChannels {
		channelSet[statsUsageEntityKey{ID: row.ID, Name: row.Name}] = struct{}{}
	}

	for key := range statsUsageDeltaCache {
		modelSet[key.RequestModelName] = struct{}{}
		actualModelSet[key.ActualModelName] = struct{}{}
		apiKeySet[statsUsageEntityKey{ID: key.APIKeyID, Name: key.APIKeyName}] = struct{}{}
		channelSet[statsUsageEntityKey{ID: key.ChannelID, Name: key.ChannelName}] = struct{}{}
	}

	dims.Models = make([]string, 0, len(modelSet))
	for m := range modelSet {
		dims.Models = append(dims.Models, m)
	}
	slices.Sort(dims.Models)

	dims.ActualModels = make([]string, 0, len(actualModelSet))
	for m := range actualModelSet {
		dims.ActualModels = append(dims.ActualModels, m)
	}
	slices.Sort(dims.ActualModels)

	dims.APIKeys = statsAnalyticsEntityRefsFrom(apiKeySet)
	dims.Channels = statsAnalyticsEntityRefsFrom(channelSet)

	return dims, nil
}

func statsAnalyticsEntityRefsFrom(m map[statsUsageEntityKey]struct{}) []StatsAnalyticsEntityRef {
	refs := make([]StatsAnalyticsEntityRef, 0, len(m))
	for key := range m {
		refs = append(refs, StatsAnalyticsEntityRef{ID: key.ID, Name: key.Name})
	}
	slices.SortFunc(refs, func(a, b StatsAnalyticsEntityRef) int {
		if a.Name != b.Name {
			return cmp.Compare(a.Name, b.Name)
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return refs
}
