package op

import (
	"context"
	"fmt"
	"strings"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/cache"
	"gorm.io/gorm/clause"
)

var llmModelCache = cache.New[string, model.LLMPrice](16)

func normalizeLLMName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func LLMList(ctx context.Context) ([]model.LLMInfo, error) {
	models := make([]model.LLMInfo, 0, llmModelCache.Len())
	for m, cost := range llmModelCache.GetAll() {
		models = append(models, model.LLMInfo{
			Name:     m,
			LLMPrice: cost,
		})
	}
	return models, nil
}

func LLMUpdate(llmInfo model.LLMInfo, ctx context.Context) error {
	llmInfo.Name = normalizeLLMName(llmInfo.Name)
	if llmInfo.Name == "" {
		return fmt.Errorf("model name is required")
	}
	if _, ok := llmModelCache.Get(llmInfo.Name); !ok {
		return fmt.Errorf("model not found")
	}
	if err := db.GetDB().WithContext(ctx).Save(&llmInfo).Error; err != nil {
		return err
	}
	llmModelCache.Set(llmInfo.Name, llmInfo.LLMPrice)
	return nil
}

func LLMDelete(modelName string, ctx context.Context) error {
	modelName = normalizeLLMName(modelName)
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}
	if _, ok := llmModelCache.Get(modelName); !ok {
		return fmt.Errorf("model not found")
	}
	channelModels, err := ChannelLLMList(ctx)
	if err != nil {
		return err
	}
	for _, channelModel := range channelModels {
		if normalizeLLMName(channelModel.Name) == modelName {
			return fmt.Errorf("model is referenced by channel")
		}
	}
	if err := db.GetDB().WithContext(ctx).Delete(&model.LLMInfo{Name: modelName}).Error; err != nil {
		return err
	}
	llmModelCache.Del(modelName)
	return nil
}

func LLMBatchDelete(modelNames []string, ctx context.Context) error {
	if len(modelNames) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(modelNames))
	normalizedNames := make([]string, 0, len(modelNames))
	for _, modelName := range modelNames {
		modelName = normalizeLLMName(modelName)
		if modelName == "" {
			continue
		}
		if _, ok := seen[modelName]; ok {
			continue
		}
		seen[modelName] = struct{}{}
		normalizedNames = append(normalizedNames, modelName)
	}
	if len(normalizedNames) == 0 {
		return nil
	}
	if err := db.GetDB().WithContext(ctx).Where("name IN ?", normalizedNames).Delete(&model.LLMInfo{}).Error; err != nil {
		return err
	}
	llmModelCache.Del(normalizedNames...)
	return nil
}

func LLMCleanupGhosts(ctx context.Context) error {
	channelModels, err := ChannelLLMList(ctx)
	if err != nil {
		return err
	}
	referencedModelNames := make(map[string]struct{}, len(channelModels))
	for _, channelModel := range channelModels {
		if name := normalizeLLMName(channelModel.Name); name != "" {
			referencedModelNames[name] = struct{}{}
		}
	}

	ghostModelNames := make([]string, 0)
	for modelName, modelPrice := range llmModelCache.GetAll() {
		if _, ok := referencedModelNames[normalizeLLMName(modelName)]; ok {
			continue
		}
		// Keep manually assigned or otherwise priced entries even when their
		// model is no longer referenced by a channel.
		if modelPrice.Input != 0 || modelPrice.Output != 0 || modelPrice.CacheRead != 0 || modelPrice.CacheWrite != 0 {
			continue
		}
		ghostModelNames = append(ghostModelNames, modelName)
	}
	if len(ghostModelNames) == 0 {
		return nil
	}
	if err := db.GetDB().WithContext(ctx).Where("name IN ?", ghostModelNames).Delete(&model.LLMInfo{}).Error; err != nil {
		return err
	}
	llmModelCache.Del(ghostModelNames...)
	return nil
}

func LLMCreate(llmInfo model.LLMInfo, ctx context.Context) error {
	llmInfo.Name = normalizeLLMName(llmInfo.Name)
	if llmInfo.Name == "" {
		return fmt.Errorf("model name is required")
	}
	if _, ok := llmModelCache.Get(llmInfo.Name); ok {
		return fmt.Errorf("model already exists")
	}
	if err := db.GetDB().WithContext(ctx).Create(&llmInfo).Error; err != nil {
		return err
	}
	llmModelCache.Set(llmInfo.Name, llmInfo.LLMPrice)
	return nil
}

func LLMBatchCreate(llmInfos []model.LLMInfo, ctx context.Context) error {
	if len(llmInfos) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(llmInfos))
	newLLMInfos := make([]model.LLMInfo, 0, len(llmInfos))
	for _, llmInfo := range llmInfos {
		llmInfo.Name = normalizeLLMName(llmInfo.Name)
		if llmInfo.Name == "" {
			continue
		}
		if _, ok := seen[llmInfo.Name]; ok {
			continue
		}
		if _, ok := llmModelCache.Get(llmInfo.Name); ok {
			continue
		}
		seen[llmInfo.Name] = struct{}{}
		newLLMInfos = append(newLLMInfos, llmInfo)
	}
	if len(newLLMInfos) == 0 {
		return nil
	}
	if err := db.GetDB().WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&newLLMInfos).Error; err != nil {
		return err
	}
	names := make([]string, len(newLLMInfos))
	for i, llmInfo := range newLLMInfos {
		names[i] = llmInfo.Name
	}
	var savedLLMInfos []model.LLMInfo
	if err := db.GetDB().WithContext(ctx).Where("name IN ?", names).Find(&savedLLMInfos).Error; err != nil {
		return err
	}
	for _, llmInfo := range savedLLMInfos {
		llmModelCache.Set(normalizeLLMName(llmInfo.Name), llmInfo.LLMPrice)
	}
	return nil
}

func LLMBatchSave(llmInfos []model.LLMInfo, ctx context.Context) error {
	if len(llmInfos) == 0 {
		return nil
	}
	normalized := make([]model.LLMInfo, 0, len(llmInfos))
	seen := make(map[string]struct{}, len(llmInfos))
	for _, llmInfo := range llmInfos {
		llmInfo.Name = normalizeLLMName(llmInfo.Name)
		if llmInfo.Name == "" {
			continue
		}
		if _, ok := seen[llmInfo.Name]; ok {
			return fmt.Errorf("duplicate model name: %s", llmInfo.Name)
		}
		seen[llmInfo.Name] = struct{}{}
		normalized = append(normalized, llmInfo)
	}
	if len(normalized) == 0 {
		return nil
	}
	if err := db.GetDB().WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&normalized).Error; err != nil {
		return err
	}
	for _, llmInfo := range normalized {
		llmModelCache.Set(llmInfo.Name, llmInfo.LLMPrice)
	}
	return nil
}

func LLMGet(name string) (model.LLMPrice, error) {
	price, ok := llmModelCache.Get(normalizeLLMName(name))
	if !ok {
		return model.LLMPrice{}, fmt.Errorf("model not found")
	}
	return price, nil
}

func llmRefreshCache(ctx context.Context) error {
	models := []model.LLMInfo{}
	if err := db.GetDB().WithContext(ctx).Find(&models).Error; err != nil {
		return err
	}
	for _, llmInfo := range models {
		if name := normalizeLLMName(llmInfo.Name); name != "" {
			llmModelCache.Set(name, llmInfo.LLMPrice)
		}
	}
	return nil
}
