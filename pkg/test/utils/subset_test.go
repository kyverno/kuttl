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
	}, "", nil))

	assert.NotNil(t, IsSubset(map[string]interface{}{
		"hello": "moon",
	}, map[string]interface{}{
		"hello": "world",
		"bye":   "moon",
	}, "", nil))

	assert.Nil(t, IsSubset(map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": "world",
		},
	}, map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": "world",
			"bye":   "moon",
		},
	}, "", nil))

	assert.NotNil(t, IsSubset(map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": "moon",
		},
	}, map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": "world",
			"bye":   "moon",
		},
	}, "", nil))

	assert.NotNil(t, IsSubset(map[string]interface{}{
		"hello": map[string]interface{}{
			"hello": "moon",
		},
	}, map[string]interface{}{
		"hello": "world",
	}, "", nil))

	assert.NotNil(t, IsSubset(map[string]interface{}{
		"hello": "world",
	}, map[string]interface{}{}, "", nil))

	assert.Nil(t, IsSubset(map[string]interface{}{
		"hello": []int{
			1, 2, 3,
		},
	}, map[string]interface{}{
		"hello": []int{
			1, 2, 3,
		},
	}, "", nil))

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
	}, "", nil))

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
	}, "", nil))

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
	}, "", nil))
}
