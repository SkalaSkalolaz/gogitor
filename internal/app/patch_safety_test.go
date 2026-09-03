package app

import (
	"testing"

	"gogitor/internal/workspace"
)

func TestShouldAuditPatch(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		protocol workspace.PatchProtocol
		deep     bool
		want     bool
	}{
		{
			name:     "always",
			mode:     "always",
			protocol: workspace.PatchProtocolSearchReplace,
			want:     true,
		},
		{
			name:     "off",
			mode:     "off",
			protocol: workspace.PatchProtocolReplaceOnly,
			want:     false,
		},
		{
			name:     "auto deep",
			mode:     "auto",
			protocol: workspace.PatchProtocolSearchReplace,
			deep:     true,
			want:     true,
		},
		{
			name:     "auto replace only",
			mode:     "auto",
			protocol: workspace.PatchProtocolReplaceOnly,
			deep:     false,
			want:     true,
		},
		{
			name:     "auto ordinary shallow",
			mode:     "auto",
			protocol: workspace.PatchProtocolSearchReplace,
			deep:     false,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldAuditPatch(
				tt.mode,
				tt.protocol,
				tt.deep,
			)

			if got != tt.want {
				t.Fatalf(
					"shouldAuditPatch() = %v, want %v",
					got,
					tt.want,
				)
			}
		})
	}
}
