package utilities //nolint:revive // shared utility package used across multiple modules

import (
	"regexp"
	"testing"
)

func TestRandomFunctions(t *testing.T) {
	tests := []struct {
		name            string
		randomFunc      func() string
		expectedPattern string
	}{
		{
			name: "RandomAlphaNumeric",
			randomFunc: func() string {
				return string(RandomAlphaNumeric())
			},
			expectedPattern: `^[a-z0-9]$`,
		},
		{
			name: "RandomLetter",
			randomFunc: func() string {
				return string(RandomLetter())
			},
			expectedPattern: `^[a-z]$`,
		},
		{
			name: "RandomString",
			randomFunc: func() string {
				return RandomString(20)
			},
			expectedPattern: `^[a-z0-9]{20}$`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for i := range 100 {
				s := test.randomFunc()
				pattern := regexp.MustCompile(test.expectedPattern)
				if !pattern.MatchString(s) {
					t.Errorf("Running attempt %d: random string %q does not conform to pattern %v", i+1, s, pattern)
				}
			}
		})
	}
}
