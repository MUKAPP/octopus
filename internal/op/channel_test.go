package op

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
)

func TestChannelUpdateTracksRateMultiplierSource(t *testing.T) {
	if err := db.InitDB("sqlite", filepath.Join(t.TempDir(), "channel.db"), false); err != nil {
		t.Fatalf("初始化测试数据库失败：%v", err)
	}

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
}
