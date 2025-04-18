package twitter

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewNotify_getToken(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		expected string
	}{
		{
			name:     "small number",
			id:       "123",
			expected: "",
		},
		{
			name:     "large number",
			id:       "1234567890123456",
			expected: "3vmjqguc7abepbln",
		},
		{
			name:     "zero",
			id:       "0",
			expected: "",
		},
		{
			name:     "very large number",
			id:       "999999999999999999",
			expected: "2f9lc2ug9mm5ugnaee",
		},
		{
			name:     "number with trailing zeros",
			id:       "1000000000000000",
			expected: "353i5ab8p5fc5vay",
		},
		{
			name:     "number producing trailing zeros in fraction",
			id:       "1234567890000000",
			expected: "3vmjqgtht4eider9",
		},
		{
			name:     "normal twitter id",
			id:       "1908006472073760775",
			expected: "4mi6g4tjjqmsk2b1te",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NewNotify{}
			actual, err := n.getToken(tt.id)
			if err != nil {
				t.Errorf("getToken(%s) error = %v", tt.id, err)
				return
			}
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func Test_toBase36Char(t *testing.T) {
	tests := []struct {
		d        int64
		expected string
	}{
		{0, "0"},
		{9, "9"},
		{10, "a"},
		{35, "z"},
		{36, "10"}, // wraps around
		{-1, "-1"}, // invalid input
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			actual := floatToBase36(float64(tt.d), 15)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
