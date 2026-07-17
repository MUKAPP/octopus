package op

import (
	"context"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

func TestGroupGetEnabledMapUsesChannelRateMultiplier(t *testing.T) {
	const groupID = 91001
	const channelID = 92001
	group := model.Group{
		ID:    groupID,
		Name:  "rate-test-group",
		Items: []model.GroupItem{{ID: 1, GroupID: groupID, ChannelID: channelID, ModelName: "gpt-test"}},
	}
	groupMap.Set(group.Name, group)
	channelCache.Set(channelID, model.Channel{ID: channelID, Enabled: true, RateMultiplier: 0.6})
	t.Cleanup(func() {
		groupMap.Del(group.Name)
		channelCache.Del(channelID)
	})

	got, err := GroupGetEnabledMap(group.Name, context.Background())
	if err != nil {
		t.Fatalf("获取分组失败：%v", err)
	}
	if len(got.Items) != 1 || got.Items[0].RateMultiplier != 0.6 {
		t.Fatalf("分组项应使用渠道倍率，实际为：%+v", got.Items)
	}
}
