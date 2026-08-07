package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type StatsMetrics struct {
	InputToken     int64   `json:"input_token" gorm:"bigint"`
	OutputToken    int64   `json:"output_token" gorm:"bigint"`
	InputCost      float64 `json:"input_cost" gorm:"type:real"`
	OutputCost     float64 `json:"output_cost" gorm:"type:real"`
	WaitTime       int64   `json:"wait_time" gorm:"bigint"`
	RequestSuccess int64   `json:"request_success" gorm:"bigint"`
	RequestFailed  int64   `json:"request_failed" gorm:"bigint"`
}

type StatsTotal struct {
	ID int `gorm:"primaryKey"`
	StatsMetrics
}

type StatsHourly struct {
	Hour int    `json:"hour" gorm:"primaryKey"`
	Date string `json:"date" gorm:"not null"` // 记录最后更新日期，格式：20060102
	StatsMetrics
}

type StatsDaily struct {
	Date string `json:"date" gorm:"primaryKey"`
	StatsMetrics
}

type StatsModel struct {
	ID        int    `json:"id" gorm:"primaryKey"`
	Name      string `json:"name" gorm:"not null"`
	ChannelID int    `json:"channel_id" gorm:"not null"`
	StatsMetrics
}

type StatsChannel struct {
	ChannelID int `json:"channel_id" gorm:"primaryKey"`
	StatsMetrics
}

type StatsAPIKey struct {
	APIKeyID int `json:"api_key_id" gorm:"primaryKey"`
	StatsMetrics
}

// Add aggregates another StatsMetrics into the current one.
func (s *StatsMetrics) Add(delta StatsMetrics) {
	s.InputToken += delta.InputToken
	s.OutputToken += delta.OutputToken
	s.InputCost += delta.InputCost
	s.OutputCost += delta.OutputCost
	s.WaitTime += delta.WaitTime
	s.RequestSuccess += delta.RequestSuccess
	s.RequestFailed += delta.RequestFailed
}

// StatsUsageKey 标识一行多维日聚合的唯一维度组合。名称必须进入键，
// 避免渠道或 API Key 删除后复用数值 ID 时把新旧实体合并。
type StatsUsageKey struct {
	Date             string `json:"date"`
	RequestModelName string `json:"request_model_name"`
	ActualModelName  string `json:"actual_model_name"`
	APIKeyID         int    `json:"api_key_id"`
	APIKeyName       string `json:"api_key_name"`
	ChannelID        int    `json:"channel_id"`
	ChannelName      string `json:"channel_name"`
	Success          bool   `json:"success"`
}

// StatsUsage 是数据分析页的多维日聚合行，以 KeyHash（SHA-256 hex，64 字符）为主键，
// 长期保留且不受日志保留期与清理影响。
type StatsUsage struct {
	KeyHash          string `json:"key_hash" gorm:"primaryKey;size:64"`
	Date             string `json:"date" gorm:"not null;index"`
	RequestModelName string `json:"request_model_name" gorm:"index"`
	ActualModelName  string `json:"actual_model_name" gorm:"index"`
	APIKeyID         int    `json:"api_key_id" gorm:"index"`
	APIKeyName       string `json:"api_key_name"`
	ChannelID        int    `json:"channel_id" gorm:"index"`
	ChannelName      string `json:"channel_name"`
	Success          bool   `json:"success" gorm:"index"`
	CachedToken      int64  `json:"cached_token" gorm:"bigint"`
	StatsMetrics
}

// Key 返回身份字段构成的组合键。
func (s StatsUsage) Key() StatsUsageKey {
	return StatsUsageKey{
		Date:             s.Date,
		RequestModelName: s.RequestModelName,
		ActualModelName:  s.ActualModelName,
		APIKeyID:         s.APIKeyID,
		APIKeyName:       s.APIKeyName,
		ChannelID:        s.ChannelID,
		ChannelName:      s.ChannelName,
		Success:          s.Success,
	}
}

// Hash 对字段顺序固定的键做 JSON 序列化后取 SHA-256 hex。marshal 错误原样返回，
// 调用方不能退化为空 key。
func (k StatsUsageKey) Hash() (string, error) {
	b, err := json.Marshal(k)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Add 把 delta 的指标累加到当前行；身份字段不参与累加。
func (s *StatsUsage) Add(delta StatsUsage) {
	s.StatsMetrics.Add(delta.StatsMetrics)
	s.CachedToken += delta.CachedToken
}
