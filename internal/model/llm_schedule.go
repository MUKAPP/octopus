package model

import (
	"fmt"
	"math"
	"slices"
	"time"
	_ "time/tzdata"
)

// LLMPriceSchedule 的日期和时间均使用指定时区；排除日期整天使用基础价格。
type LLMPriceSchedule struct {
	Timezone      string         `json:"timezone"`
	ExcludedDates []string       `json:"excluded_dates,omitempty"`
	Rules         []LLMTimePrice `json:"rules"`
}

type LLMTimePrice struct {
	Name  string `json:"name"`
	Start string `json:"start"`
	End   string `json:"end"`
	// 0 为周日，空列表表示每天；跨午夜时以开始日决定星期。
	Weekdays   []int   `json:"weekdays,omitempty"`
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
}

func (r LLMTimePrice) price() LLMPrice {
	return LLMPrice{Input: r.Input, Output: r.Output, CacheRead: r.CacheRead, CacheWrite: r.CacheWrite}
}

func priceMinute(value string, allowEndOfDay bool) (int, error) {
	if allowEndOfDay && value == "24:00" {
		return 1440, nil
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil || parsed.Format("15:04") != value {
		return 0, fmt.Errorf("invalid time %q; use HH:MM", value)
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

func (p LLMPrice) Validate() error {
	for _, value := range []float64{p.Input, p.Output, p.CacheRead, p.CacheWrite} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return fmt.Errorf("prices must be finite and non-negative")
		}
	}
	schedule := p.PriceSchedule
	if schedule == nil {
		return nil
	}
	if schedule.Timezone == "" || schedule.Timezone == "Local" {
		return fmt.Errorf("price schedule requires an explicit IANA timezone")
	}
	if _, err := time.LoadLocation(schedule.Timezone); err != nil {
		return fmt.Errorf("invalid price schedule timezone: %w", err)
	}
	if len(schedule.Rules) == 0 {
		return fmt.Errorf("price schedule requires at least one rule")
	}
	for _, date := range schedule.ExcludedDates {
		if parsed, err := time.Parse("2006-01-02", date); err != nil || parsed.Format("2006-01-02") != date {
			return fmt.Errorf("invalid excluded date %q; use YYYY-MM-DD", date)
		}
	}
	// 在一周的分钟格上校验，跨午夜和跨周边界也不能重叠。
	var occupied [7 * 1440]bool
	for i, rule := range schedule.Rules {
		start, err := priceMinute(rule.Start, false)
		if err != nil {
			return fmt.Errorf("price rule %d: %w", i+1, err)
		}
		end, err := priceMinute(rule.End, true)
		if err != nil {
			return fmt.Errorf("price rule %d: %w", i+1, err)
		}
		if start == end {
			return fmt.Errorf("price rule %d: start and end must differ; use 00:00–24:00 for a full day", i+1)
		}
		if err := rule.price().Validate(); err != nil {
			return fmt.Errorf("price rule %d: %w", i+1, err)
		}
		days := rule.Weekdays
		if len(days) == 0 {
			days = []int{0, 1, 2, 3, 4, 5, 6}
		}
		var seen [7]bool
		for _, day := range days {
			if day < 0 || day > 6 || seen[day] {
				return fmt.Errorf("price rule %d: weekdays must be unique values from 0 to 6", i+1)
			}
			seen[day] = true
			duration := end - start
			if duration < 0 {
				duration += 1440
			}
			for minute := 0; minute < duration; minute++ {
				slot := (day*1440 + start + minute) % len(occupied)
				if occupied[slot] {
					return fmt.Errorf("price rule %d overlaps another rule", i+1)
				}
				occupied[slot] = true
			}
		}
	}
	return nil
}

// At 使用请求开始时间选定整次请求的价格；未命中规则时使用基础价格。
func (p LLMPrice) At(at time.Time) LLMPrice {
	schedule := p.PriceSchedule
	p.PriceSchedule = nil
	if schedule == nil {
		return p
	}
	location, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return p
	}
	local := at.In(location)
	if slices.Contains(schedule.ExcludedDates, local.Format("2006-01-02")) {
		return p
	}
	minute := local.Hour()*60 + local.Minute()
	for _, rule := range schedule.Rules {
		start, _ := priceMinute(rule.Start, false)
		end, _ := priceMinute(rule.End, true)
		day := int(local.Weekday())
		matches := minute >= start && minute < end
		if end < start {
			matches = minute >= start || minute < end
			if minute < end {
				day = (day + 6) % 7
			}
		}
		if matches && (len(rule.Weekdays) == 0 || slices.Contains(rule.Weekdays, day)) {
			return rule.price()
		}
	}
	return p
}
