package op

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"gorm.io/gorm"
)

func TestChannelUpdateTracksRateMultiplierSource(t *testing.T) {
	if err := db.InitDB("sqlite", filepath.Join(t.TempDir(), "channel.db"), false); err != nil {
		t.Fatalf("初始化测试数据库失败：%v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("关闭测试数据库失败：%v", err)
		}
	})

	ctx := context.Background()
	channel := model.Channel{
		Name:                     "rate-source-test",
		RateMultiplier:           1.5,
		RateMultiplierAutoSynced: true,
	}
	if err := ChannelCreate(&channel, ctx); err != nil {
		t.Fatalf("创建测试渠道失败：%v", err)
	}
	t.Cleanup(func() {
		channelCache.Del(channel.ID)
	})

	manualRate := 1.25
	updated, err := ChannelUpdate(&model.ChannelUpdateRequest{
		ID:             channel.ID,
		RateMultiplier: &manualRate,
	}, ctx)
	if err != nil {
		t.Fatalf("手动更新倍率失败：%v", err)
	}
	if updated.RateMultiplierAutoSynced {
		t.Fatal("手动更新倍率后应清除自动同步来源标志")
	}

	autoSynced := true
	updated, err = ChannelUpdate(&model.ChannelUpdateRequest{
		ID:                       channel.ID,
		RateMultiplier:           &manualRate,
		RateMultiplierAutoSynced: &autoSynced,
	}, ctx)
	if err != nil {
		t.Fatalf("自动同步倍率失败：%v", err)
	}
	if !updated.RateMultiplierAutoSynced {
		t.Fatal("自动同步成功后应设置倍率来源标志，即使倍率数值未变化")
	}

	priority := 3
	updated, err = ChannelUpdate(&model.ChannelUpdateRequest{ID: channel.ID, Priority: &priority}, ctx)
	if err != nil {
		t.Fatalf("更新渠道优先级失败：%v", err)
	}
	if updated.Priority != priority {
		t.Fatalf("渠道优先级未更新：got %d, want %d", updated.Priority, priority)
	}

	invalidPriority := -1
	if _, err := ChannelUpdate(&model.ChannelUpdateRequest{ID: channel.ID, Priority: &invalidPriority}, ctx); err == nil {
		t.Fatal("更新渠道时应拒绝负数优先级")
	}
}

func TestChannelCreateRejectsNegativePriority(t *testing.T) {
	channel := model.Channel{Name: "invalid-priority", Priority: -1}
	if err := ChannelCreate(&channel, context.Background()); err == nil {
		t.Fatal("创建渠道时应拒绝负数优先级")
	}
}

func TestChannelCreateClearsReusedChannelStats(t *testing.T) {
	if err := db.InitDB("sqlite", filepath.Join(t.TempDir(), "channel-stats.db"), false); err != nil {
		t.Fatalf("初始化测试数据库失败：%v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("关闭测试数据库失败：%v", err)
		}
	})

	ctx := context.Background()
	staleStats := model.StatsChannel{
		ChannelID: 1,
		StatsMetrics: model.StatsMetrics{
			RequestFailed: 585,
			WaitTime:      1234,
		},
	}
	if err := db.GetDB().Connection(func(tx *gorm.DB) error {
		if err := tx.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
			return err
		}
		if err := tx.Create(&staleStats).Error; err != nil {
			return err
		}
		return tx.Exec("PRAGMA foreign_keys = ON").Error
	}); err != nil {
		t.Fatalf("写入遗留统计失败：%v", err)
	}
	if err := statsRefreshCache(ctx); err != nil {
		t.Fatalf("加载遗留统计缓存失败：%v", err)
	}

	channel := model.Channel{ID: 1, Name: "reused-channel", RateMultiplier: 1}
	if err := ChannelCreate(&channel, ctx); err != nil {
		t.Fatalf("创建复用 ID 的渠道失败：%v", err)
	}
	t.Cleanup(func() {
		channelCache.Del(channel.ID)
		statsChannelCacheClear(channel.ID)
	})

	if stats := StatsChannelGet(channel.ID); stats.RequestFailed != 0 || stats.WaitTime != 0 {
		t.Fatalf("新渠道继承了遗留统计：%+v", stats.StatsMetrics)
	}

	var count int64
	if err := db.GetDB().WithContext(ctx).Model(&model.StatsChannel{}).Where("channel_id = ?", channel.ID).Count(&count).Error; err != nil {
		t.Fatalf("查询渠道统计失败：%v", err)
	}
	if count != 0 {
		t.Fatalf("遗留统计未清理：got %d row(s), want 0", count)
	}
}
