package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSubset(t *testing.T) {
	assert.Nil(t, IsSubset(map[string]interface{}{
		"hello": "world",
	}, map[string]interface{}{
		"hello": "world",
		"bye":   "moon",
	}, "", DefaultStrategyFactory()))

	assert.NotNil(t, IsSubset(map[string]interface{}{
		"hello": "moon",
	}, map[string]interface{}{
		"hello": "world",
		"bye":   "moon",
	}, "", DefaultStrategyFactory()))

	assert.Nil(t, IsSubset(map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": "world",
		},
	}, map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": "world",
			"bye":   "moon",
		},
	}, "", DefaultStrategyFactory()))

	assert.NotNil(t, IsSubset(map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": "moon",
		},
	}, map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": "world",
			"bye":   "moon",
		},
	}, "", DefaultStrategyFactory()))

	assert.NotNil(t, IsSubset(map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": "moon",
		},
	}, map[string]interface{}{
		"hello": "world",
	}, "", DefaultStrategyFactory()))

	assert.NotNil(t, IsSubset(map[string]interface{}{
		"hello": "world",
	}, map[string]interface{}{}, "", DefaultStrategyFactory()))

	assert.Nil(t, IsSubset(map[string]interface{}{
		"hello": []int{
			1, 2, 3,
		},
	}, map[string]interface{}{
		"hello": []int{
			1, 2, 3,
		},
	}, "", DefaultStrategyFactory()))

	assert.Nil(t, IsSubset(map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": []map[string]interface{}{
				{
					"image": "hello",
				},
			},
		},
	}, map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": []map[string]interface{}{
				{
					"image": "hello",
					"bye":   "moon",
				},
			},
		},
	}, "", DefaultStrategyFactory()))

	assert.NotNil(t, IsSubset(map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": []map[string]interface{}{
				{
					"image": "hello",
				},
			},
		},
	}, map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": []map[string]interface{}{
				{
					"image": "hello",
					"bye":   "moon",
				},
				{
					"bye": "moon",
				},
			},
		},
	}, "", DefaultStrategyFactory()))

	assert.NotNil(t, IsSubset(map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": []map[string]interface{}{
				{
					"image": "hello",
				},
			},
		},
	}, map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": []map[string]interface{}{
				{
					"image": "world",
				},
			},
		},
	}, "", DefaultStrategyFactory()))

	assert.Nil(t, IsSubset(
		map[string]interface{}{
			"hello": map[string]interface{}{
				"world": []map[string]interface{}{
					{
						"image": "earth",
					},
				},
			},
		},
		map[string]interface{}{
			"hello": map[string]interface{}{
				"world": []map[string]interface{}{
					{
						"image": "earth",
						"bye":   "moon",
					},
				},
			},
		},
		"", DefaultStrategyFactory()))

	assert.NotNil(t, IsSubset(
		map[string]interface{}{
			"hello": map[string]interface{}{
				"world": []map[string]interface{}{
					{
						"image": "mars",
					},
				},
			},
		},
		map[string]interface{}{
			"hello": map[string]interface{}{
				"world": []map[string]interface{}{
					{
						"image": "earth",
						"bye":   "moon",
					},
				},
			},
		},
		"", DefaultStrategyFactory()))

	assert.Nil(t, IsSubset(
		map[string]interface{}{
			"hello": map[string]interface{}{
				"world": []map[string]interface{}{
					{
						"image": "earth",
					},
					{
						"image": "mars",
					},
				},
			},
		},
		map[string]interface{}{
			"hello": map[string]interface{}{
				"world": []map[string]interface{}{
					{
						"image": "earth",
						"bye":   "moon",
					},
					{
						"image": "mars",
					},
				},
			},
		},
		"", DefaultStrategyFactory()))
}
