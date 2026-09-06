package types

import "testing"

func TestIsValidFavoriteResourceType(t *testing.T) {
	tests := []struct {
		resourceType string
		want         bool
	}{
		{ResourceTypeKB, true},
		{ResourceTypeAgent, true},
		// Skills/MCP browser (Skills/MCP 页面) pins sync through the same
		// favorites API as knowledge bases and agents.
		{ResourceTypeSkill, true},
		{ResourceTypeMCP, true},
		{"", false},
		{"document", false},
		{"bogus", false},
		{ResourceTypeAgent + "s", false},
	}
	for _, tt := range tests {
		if got := IsValidFavoriteResourceType(tt.resourceType); got != tt.want {
			t.Errorf("IsValidFavoriteResourceType(%q) = %v, want %v", tt.resourceType, got, tt.want)
		}
	}
}
