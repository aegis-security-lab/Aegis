package control

import (
	"errors"
	"math"
)

func validateModelPricing(pricing ModelPricing) error {
	values := []float64{pricing.Input, pricing.Output, pricing.CacheRead, pricing.CacheWrite}
	for _, value := range values {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return errors.New("模型价格必须是大于或等于 0 的有限数字")
		}
	}
	return nil
}

func calculateModelCost(pricing ModelPricing, input, output, cacheRead, cacheWrite int64) float64 {
	cost := pricing.Input*float64(input) +
		pricing.Output*float64(output) +
		pricing.CacheRead*float64(cacheRead) +
		pricing.CacheWrite*float64(cacheWrite)
	return cost / 1_000_000
}
