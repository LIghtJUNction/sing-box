package main

import "testing"

func TestIsConfigFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		want bool
	}{
		{name: "config.json", want: true},
		{name: "config.jsonc", want: true},
		{name: "config.JSON", want: false},
		{name: "config.yaml", want: false},
		{name: "jsonc", want: false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := isConfigFile(testCase.name); got != testCase.want {
				t.Fatalf("isConfigFile(%q) = %v, want %v", testCase.name, got, testCase.want)
			}
		})
	}
}
