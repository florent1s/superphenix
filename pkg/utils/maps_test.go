package utils

import (
	"reflect"
	"testing"
)

func TestMapDeepMerge(t *testing.T) {
	tests := []struct {
		name     string
		dst      map[string]interface{}
		src      map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "shallow merge",
			dst: map[string]interface{}{
				"a": 1,
				"b": 2,
			},
			src: map[string]interface{}{
				"b": 3,
				"c": 4,
			},
			expected: map[string]interface{}{
				"a": 1,
				"b": 3,
				"c": 4,
			},
		},
		{
			name: "deep merge",
			dst: map[string]interface{}{
				"nested": map[string]interface{}{
					"a": 1,
					"b": 2,
				},
			},
			src: map[string]interface{}{
				"nested": map[string]interface{}{
					"b": 3,
					"c": 4,
				},
			},
			expected: map[string]interface{}{
				"nested": map[string]interface{}{
					"a": 1,
					"b": 3,
					"c": 4,
				},
			},
		},
		{
			name: "delete key with nil",
			dst: map[string]interface{}{
				"a": 1,
				"b": 2,
			},
			src: map[string]interface{}{
				"b": nil,
			},
			expected: map[string]interface{}{
				"a": 1,
			},
		},
		{
			name: "replace map with scalar",
			dst: map[string]interface{}{
				"nested": map[string]interface{}{
					"a": 1,
				},
			},
			src: map[string]interface{}{
				"nested": 2,
			},
			expected: map[string]interface{}{
				"nested": 2,
			},
		},
		{
			name: "replace scalar with map",
			dst: map[string]interface{}{
				"nested": 1,
			},
			src: map[string]interface{}{
				"nested": map[string]interface{}{
					"a": 2,
				},
			},
			expected: map[string]interface{}{
				"nested": map[string]interface{}{
					"a": 2,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			MapDeepMerge(tt.dst, tt.src)
			if !reflect.DeepEqual(tt.dst, tt.expected) {
				t.Errorf("MapDeepMerge() result = %v, expected %v", tt.dst, tt.expected)
			}
		})
	}
}
