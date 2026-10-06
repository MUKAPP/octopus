package model

import "fmt"

type APIKey struct {
	ID              int     `json:"id" gorm:"primaryKey"`
	Name            string  `json:"name" gorm:"not null"`
	APIKey          string  `json:"api_key" gorm:"not null"`
	Enabled         bool    `json:"enabled" gorm:"default:true"`
	ExpireAt        int64   `json:"expire_at,omitempty"`
	MaxCost         float64 `json:"max_cost,omitempty"`
	MaxConcurrency  int     `json:"max_concurrency,omitempty" gorm:"not null;default:0"`
	SupportedModels string  `json:"supported_models,omitempty"`
}

func (k *APIKey) Validate() error {
	if k.MaxConcurrency < 0 {
		return fmt.Errorf("max_concurrency must be a non-negative integer")
	}
	return nil
}
