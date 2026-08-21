package op

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/cache"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/internal/utils/xstrings"
)

var channelCache = cache.New[int, model.Channel](16)
var channelKeyCache = cache.New[int, model.ChannelKey](16)
var channelKeyCacheNeedUpdate = make(map[int]struct{})
var channelKeyCacheNeedUpdateLock sync.Mutex

func ChannelList(ctx context.Context) ([]model.Channel, error) {
	channels := make([]model.Channel, 0, channelCache.Len())
	for _, channel := range channelCache.GetAll() {
		channels = append(channels, channel)
	}
	return channels, nil
}

func ChannelCreate(channel *model.Channel, ctx context.Context) error {
	if channel.Priority < 0 {
		return fmt.Errorf("优先级不能小于 0")
	}
	if channel.RateMultiplier < 0 {
		return fmt.Errorf("倍率必须大于 0")
	}
	if channel.RateMultiplier == 0 {
		channel.RateMultiplier = 1
	}
	tx := db.GetDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	if err := tx.Create(channel).Error; err != nil {
		tx.Rollback()
		return err
	}
	// 渠道 ID 可能复用；删除同 ID 的历史统计，避免新渠道继承已删除渠道的数据。
	if err := tx.Where("channel_id = ?", channel.ID).Delete(&model.StatsChannel{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	statsChannelCacheClear(channel.ID)
	channelCache.Set(channel.ID, *channel)
	for _, k := range channel.Keys {
		if k.ID != 0 {
			channelKeyCache.Set(k.ID, k)
		}
	}
	return nil
}

// ChannelKeyUpdate 原子更新 ChannelKey 的运行状态和费用增量，并标记为需要持久化。
func ChannelKeyUpdate(key model.ChannelKey, costDelta float64) error {
	if key.ID == 0 || key.ChannelID == 0 {
		return fmt.Errorf("invalid channel key")
	}
	channelKeyCacheNeedUpdateLock.Lock()
	defer channelKeyCacheNeedUpdateLock.Unlock()
	current, ok := channelKeyCache.Get(key.ID)
	if !ok {
		return fmt.Errorf("channel key not found")
	}
	current.StatusCode = key.StatusCode
	current.LastUseTimeStamp = key.LastUseTimeStamp
	current.TotalCost += costDelta
	channelKeyCache.Set(key.ID, current)
	channelKeyCacheNeedUpdate[key.ID] = struct{}{}
	return nil
}

func ChannelBaseUrlUpdate(channelID int, baseUrl []model.BaseUrl) error {
	ch, ok := channelCache.Get(channelID)
	if !ok {
		return fmt.Errorf("channel not found")
	}
	// Copy to decouple callers from internal cache storage.
	if baseUrl == nil {
		ch.BaseUrls = nil
	} else {
		cp := make([]model.BaseUrl, len(baseUrl))
		copy(cp, baseUrl)
		ch.BaseUrls = cp
	}
	channelCache.Set(channelID, ch)
	return nil
}

// ChannelKeySaveDB 将运行时更新过的 ChannelKey 缓存写入数据库。
func ChannelKeySaveDB(ctx context.Context) error {
	channelKeyCacheNeedUpdateLock.Lock()
	keyIDs := make([]int, 0, len(channelKeyCacheNeedUpdate))
	for id := range channelKeyCacheNeedUpdate {
		keyIDs = append(keyIDs, id)
	}
	channelKeyCacheNeedUpdate = make(map[int]struct{})
	channelKeyCacheNeedUpdateLock.Unlock()

	if len(keyIDs) == 0 {
		return nil
	}

	dbConn := db.GetDB().WithContext(ctx)
	for _, id := range keyIDs {
		k, ok := channelKeyCache.Get(id)
		if !ok {
			continue
		}
		if err := dbConn.Save(&k).Error; err != nil {
			channelKeyCacheNeedUpdateLock.Lock()
			for _, keyID := range keyIDs {
				channelKeyCacheNeedUpdate[keyID] = struct{}{}
			}
			channelKeyCacheNeedUpdateLock.Unlock()
			return err
		}
	}
	return nil
}

func ChannelUpdate(req *model.ChannelUpdateRequest, ctx context.Context) (*model.Channel, error) {
	oldChannel, ok := channelCache.Get(req.ID)
	if !ok {
		return nil, fmt.Errorf("channel not found")
	}

	tx := db.GetDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var selectFields []string
	updates := model.Channel{ID: req.ID}
	var modelNames []string
	var affectedGroupIDs []int
	var affectedGroupItemIDs []int

	if req.Name != nil {
		selectFields = append(selectFields, "name")
		updates.Name = *req.Name
	}
	if req.Type != nil {
		selectFields = append(selectFields, "type")
		updates.Type = *req.Type
	}
	if req.Enabled != nil {
		selectFields = append(selectFields, "enabled")
		updates.Enabled = *req.Enabled
	}
	if req.BaseUrls != nil {
		selectFields = append(selectFields, "base_urls")
		updates.BaseUrls = *req.BaseUrls
	}
	if req.Model != nil || req.CustomModel != nil {
		autoModels := oldChannel.Model
		customModels := oldChannel.CustomModel
		if req.Model != nil {
			autoModels = *req.Model
		}
		if req.CustomModel != nil {
			customModels = *req.CustomModel
		}

		customModelNames := normalizeChannelModelNames(customModels)
		customModelSet := make(map[string]struct{}, len(customModelNames))
		for _, modelName := range customModelNames {
			customModelSet[modelName] = struct{}{}
		}
		autoModelNames := normalizeChannelModelNames(autoModels)
		filteredAutoModels := make([]string, 0, len(autoModelNames))
		for _, modelName := range autoModelNames {
			if _, custom := customModelSet[modelName]; !custom {
				filteredAutoModels = append(filteredAutoModels, modelName)
			}
		}

		selectFields = append(selectFields, "model")
		updates.Model = strings.Join(filteredAutoModels, ",")
		modelNames = append(filteredAutoModels, customModelNames...)
		if req.CustomModel != nil {
			selectFields = append(selectFields, "custom_model")
			updates.CustomModel = strings.Join(customModelNames, ",")
		}
	}
	if req.Proxy != nil {
		selectFields = append(selectFields, "proxy")
		updates.Proxy = *req.Proxy
	}
	if req.AutoSync != nil {
		selectFields = append(selectFields, "auto_sync")
		updates.AutoSync = *req.AutoSync
	}
	if req.RateMultiplier != nil {
		if *req.RateMultiplier <= 0 {
			tx.Rollback()
			return nil, fmt.Errorf("倍率必须大于 0")
		}
		selectFields = append(selectFields, "rate_multiplier")
		updates.RateMultiplier = *req.RateMultiplier
	}
	if req.RateMultiplierAutoSynced != nil {
		selectFields = append(selectFields, "rate_multiplier_auto_synced")
		updates.RateMultiplierAutoSynced = *req.RateMultiplierAutoSynced
	} else if req.RateMultiplier != nil {
		selectFields = append(selectFields, "rate_multiplier_auto_synced")
		updates.RateMultiplierAutoSynced = false
	}
	if req.Priority != nil {
		if *req.Priority < 0 {
			tx.Rollback()
			return nil, fmt.Errorf("优先级不能小于 0")
		}
		selectFields = append(selectFields, "priority")
		updates.Priority = *req.Priority
	}
	if req.AutoGroup != nil {
		selectFields = append(selectFields, "auto_group")
		updates.AutoGroup = *req.AutoGroup
	}
	if req.CustomHeader != nil {
		selectFields = append(selectFields, "custom_header")
		updates.CustomHeader = *req.CustomHeader
	}
	if req.ChannelProxy != nil {
		selectFields = append(selectFields, "channel_proxy")
		updates.ChannelProxy = req.ChannelProxy
	}
	if req.ParamOverride != nil {
		selectFields = append(selectFields, "param_override")
		updates.ParamOverride = req.ParamOverride
	}
	if req.ParamAppend != nil {
		selectFields = append(selectFields, "param_append")
		updates.ParamAppend = req.ParamAppend
	}
	if req.MatchRegex != nil {
		selectFields = append(selectFields, "match_regex")
		updates.MatchRegex = req.MatchRegex
	}

	// 只有当有字段需要更新时才执行 UPDATE
	if len(selectFields) > 0 {
		if err := tx.Model(&model.Channel{}).Where("id = ?", req.ID).Select(selectFields).Updates(&updates).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to update channel: %w", err)
		}
	}

	// 删除 keys
	if len(req.KeysToDelete) > 0 {
		if err := tx.Where("id IN ? AND channel_id = ?", req.KeysToDelete, req.ID).Delete(&model.ChannelKey{}).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to delete channel keys: %w", err)
		}
	}

	// 更新 keys（逐条，只更新提供的字段）
	if len(req.KeysToUpdate) > 0 {
		for _, ku := range req.KeysToUpdate {
			updates := map[string]interface{}{}
			if ku.Enabled != nil {
				updates["enabled"] = *ku.Enabled
			}
			if ku.ChannelKey != nil {
				updates["channel_key"] = *ku.ChannelKey
			}
			if ku.Remark != nil {
				updates["remark"] = *ku.Remark
			}
			if len(updates) == 0 {
				continue
			}
			if err := tx.Model(&model.ChannelKey{}).
				Where("id = ? AND channel_id = ?", ku.ID, req.ID).
				Updates(updates).Error; err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("failed to update channel key %d: %w", ku.ID, err)
			}
		}
	}

	// 新增 keys
	if len(req.KeysToAdd) > 0 {
		newKeys := make([]model.ChannelKey, 0, len(req.KeysToAdd))
		for _, ka := range req.KeysToAdd {
			newKeys = append(newKeys, model.ChannelKey{
				ChannelID:  req.ID,
				Enabled:    ka.Enabled,
				ChannelKey: ka.ChannelKey,
				Remark:     ka.Remark,
			})
		}
		if err := tx.Create(&newKeys).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create channel keys: %w", err)
		}
	}
	if req.Model != nil || req.CustomModel != nil {
		staleItems := tx.Model(&model.GroupItem{}).Where("channel_id = ?", req.ID)
		if len(modelNames) > 0 {
			staleItems = staleItems.Where("model_name NOT IN ?", modelNames)
		}
		if err := staleItems.Distinct("group_id").Pluck("group_id", &affectedGroupIDs).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to find stale group ids: %w", err)
		}

		staleItems = tx.Model(&model.GroupItem{}).Where("channel_id = ?", req.ID)
		if len(modelNames) > 0 {
			staleItems = staleItems.Where("model_name NOT IN ?", modelNames)
		}
		if err := staleItems.Pluck("id", &affectedGroupItemIDs).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to find stale group items: %w", err)
		}
		if len(affectedGroupItemIDs) > 0 {
			if err := tx.Where("id IN ? AND channel_id = ?", affectedGroupItemIDs, req.ID).Delete(&model.GroupItem{}).Error; err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("failed to delete stale group items: %w", err)
			}
		}
	}

	// key 更新、新增和模型清理都在同一事务中完成，避免缓存暴露半更新状态。
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	for _, groupID := range affectedGroupIDs {
		if err := groupRefreshCacheByID(groupID, ctx); err != nil {
			return nil, fmt.Errorf("failed to refresh group cache for group %d: %w", groupID, err)
		}
	}

	// 刷新缓存并返回最新数据。
	if err := channelRefreshCacheByID(req.ID, ctx); err != nil {
		return nil, err
	}

	channel, _ := channelCache.Get(req.ID)
	return &channel, nil
}

func ChannelEnabled(id int, enabled bool, ctx context.Context) error {
	oldChannel, ok := channelCache.Get(id)
	if !ok {
		return fmt.Errorf("channel not found")
	}
	if err := db.GetDB().WithContext(ctx).Model(&model.Channel{}).Where("id = ?", id).Update("enabled", enabled).Error; err != nil {
		return err
	}
	oldChannel.Enabled = enabled
	channelCache.Set(id, oldChannel)
	return nil
}

func ChannelDel(id int, ctx context.Context) error {
	ch, ok := channelCache.Get(id)
	if !ok {
		return fmt.Errorf("channel not found")
	}

	// 开启事务
	tx := db.GetDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 获取所有受影响的 GroupID，用于刷新缓存
	var affectedGroupIDs []int
	if err := tx.Model(&model.GroupItem{}).
		Where("channel_id = ?", id).
		Pluck("group_id", &affectedGroupIDs).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get affected groups: %w", err)
	}

	// StatsModel 使用自身的主键；只按渠道 ID 清理，不能把 GroupItem.ID 当作统计 ID。
	var affectedStatsModelIDs []int
	if err := tx.Model(&model.StatsModel{}).
		Where("channel_id = ?", id).
		Pluck("id", &affectedStatsModelIDs).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get affected model stats: %w", err)
	}
	if err := tx.Where("channel_id = ?", id).Delete(&model.StatsModel{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete model stats: %w", err)
	}

	// 删除所有引用该渠道的 GroupItem
	if err := tx.Where("channel_id = ?", id).Delete(&model.GroupItem{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete group items: %w", err)
	}

	// 删除渠道 keys
	if err := tx.Where("channel_id = ?", id).Delete(&model.ChannelKey{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete channel keys: %w", err)
	}

	// 删除统计数据
	if err := tx.Where("channel_id = ?", id).Delete(&model.StatsChannel{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete channel stats: %w", err)
	}

	// 删除渠道
	if err := tx.Delete(&model.Channel{}, id).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete channel: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// 先移除渠道缓存，阻止在途请求在删除后重新写入统计。
	channelCache.Del(id)
	channelKeyCacheNeedUpdateLock.Lock()
	for _, k := range ch.Keys {
		if k.ID != 0 {
			channelKeyCache.Del(k.ID)
			delete(channelKeyCacheNeedUpdate, k.ID)
		}
	}
	channelKeyCacheNeedUpdateLock.Unlock()
	StatsChannelDel(id)
	statsModelCacheNeedUpdateLock.Lock()
	for _, modelID := range affectedStatsModelIDs {
		statsModelCache.Del(modelID)
		delete(statsModelCacheNeedUpdate, modelID)
	}
	statsModelCacheNeedUpdateLock.Unlock()

	// 刷新受影响的分组缓存
	for _, groupID := range affectedGroupIDs {
		if err := groupRefreshCacheByID(groupID, ctx); err != nil {
			log.Warnf("failed to refresh group cache for group %d: %v", groupID, err)
		}
	}

	return nil
}

// normalizeChannelModelNames trims model IDs, drops empty values, and keeps first occurrence order.
func normalizeChannelModelNames(values ...string) []string {
	parts := xstrings.SplitTrimCompact(",", values...)
	if len(parts) < 2 {
		return parts
	}
	seen := make(map[string]struct{}, len(parts))
	models := make([]string, 0, len(parts))
	for _, modelName := range parts {
		if _, exists := seen[modelName]; exists {
			continue
		}
		seen[modelName] = struct{}{}
		models = append(models, modelName)
	}
	return models
}

func ChannelLLMList(ctx context.Context) ([]model.LLMChannel, error) {
	models := []model.LLMChannel{}
	for _, channel := range channelCache.GetAll() {
		modelNames := normalizeChannelModelNames(channel.Model, channel.CustomModel)
		for _, modelName := range modelNames {
			if modelName == "" {
				continue
			}
			models = append(models, model.LLMChannel{
				Name:           modelName,
				Enabled:        channel.Enabled,
				ChannelID:      channel.ID,
				ChannelName:    channel.Name,
				RateMultiplier: channel.RateMultiplier,
			})
		}
	}
	return models, nil
}

func ChannelGet(id int, ctx context.Context) (*model.Channel, error) {
	channel, ok := channelCache.Get(id)
	if !ok {
		return nil, fmt.Errorf("channel not found")
	}
	channel.Keys = slices.Clone(channel.Keys)
	for i, key := range channel.Keys {
		if current, ok := channelKeyCache.Get(key.ID); ok {
			channel.Keys[i] = current
		}
	}
	return &channel, nil
}

func channelRefreshCache(ctx context.Context) error {
	channels := []model.Channel{}
	if err := db.GetDB().WithContext(ctx).
		Preload("Keys").
		Preload("Stats").
		Find(&channels).Error; err != nil {
		log.Warnf("failed to get channels: %v", err)
		return err
	}
	channelKeyCache.Clear()
	channelKeyCacheNeedUpdateLock.Lock()
	channelKeyCacheNeedUpdate = make(map[int]struct{})
	channelKeyCacheNeedUpdateLock.Unlock()
	activeChannelIDs := make(map[int]struct{}, len(channels))
	for _, channel := range channels {
		activeChannelIDs[channel.ID] = struct{}{}
	}
	for id := range channelCache.GetAll() {
		if _, ok := activeChannelIDs[id]; !ok {
			channelCache.Del(id)
		}
	}
	for _, channel := range channels {
		channelCache.Set(channel.ID, channel)
		for _, k := range channel.Keys {
			if k.ID != 0 {
				channelKeyCache.Set(k.ID, k)
			}
		}
	}
	return nil
}

func channelRefreshCacheByID(id int, ctx context.Context) error {
	var channel model.Channel
	if err := db.GetDB().WithContext(ctx).
		Preload("Keys").
		Preload("Stats").
		First(&channel, id).Error; err != nil {
		return err
	}
	channelKeyCacheNeedUpdateLock.Lock()
	if old, ok := channelCache.Get(id); ok {
		for _, key := range old.Keys {
			if !slices.ContainsFunc(channel.Keys, func(current model.ChannelKey) bool {
				return current.ID == key.ID
			}) {
				channelKeyCache.Del(key.ID)
				delete(channelKeyCacheNeedUpdate, key.ID)
			}
		}
	}
	for i, key := range channel.Keys {
		if current, ok := channelKeyCache.Get(key.ID); ok {
			key.StatusCode = current.StatusCode
			key.LastUseTimeStamp = current.LastUseTimeStamp
			key.TotalCost = current.TotalCost
			channel.Keys[i] = key
		}
		channelKeyCache.Set(key.ID, channel.Keys[i])
	}
	channelKeyCacheNeedUpdateLock.Unlock()
	channelCache.Set(channel.ID, channel)
	return nil
}
