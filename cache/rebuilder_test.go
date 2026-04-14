package cache

import (
	"os"
	"testing"
)

func TestRebuild(t *testing.T) {
	// Implement me!
	t.Skip("skipping unimplemented test.")
}

func TestNormalizeDockerPath(t *testing.T) {
	tests := []struct {
		name      string
		stageType string
		input     string
		expected  string
	}{
		{
			name:      "non-docker env returns input unchanged",
			stageType: "",
			input:     "/tmp/harness/uuid-abc123/.gradle",
			expected:  "/tmp/harness/uuid-abc123/.gradle",
		},
		{
			name:      "docker env with unix harness path",
			stageType: "DOCKER",
			input:     "/tmp/harness/uuid-abc123/.gradle",
			expected:  "docker/.gradle",
		},
		{
			name:      "docker env with nested unix path",
			stageType: "DOCKER",
			input:     "/tmp/harness/NsisF6Y9QfaOtdukTItZfA/.gradle/caches/modules-2",
			expected:  "docker/.gradle/caches/modules-2",
		},
		{
			name:      "docker env with windows harness path",
			stageType: "DOCKER",
			input:     "C:/tmp/harness/uuid-abc123/.gradle",
			expected:  "docker/.gradle",
		},
		{
			name:      "docker env with non-matching prefix returns unchanged",
			stageType: "DOCKER",
			input:     "/home/user/.gradle",
			expected:  "/home/user/.gradle",
		},
		{
			name:      "docker env with no UUID segment returns unchanged",
			stageType: "DOCKER",
			input:     "/tmp/harness/nouuidhere",
			expected:  "/tmp/harness/nouuidhere",
		},
		{
			name:      "docker env with relative path returns unchanged",
			stageType: "DOCKER",
			input:     ".gradle",
			expected:  ".gradle",
		},
		{
			name:      "docker env with only UUID and single file",
			stageType: "DOCKER",
			input:     "/tmp/harness/some-uuid/file.txt",
			expected:  "docker/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.stageType != "" {
				os.Setenv("DRONE_STAGE_TYPE", tt.stageType)
				defer os.Unsetenv("DRONE_STAGE_TYPE")
			} else {
				os.Unsetenv("DRONE_STAGE_TYPE")
			}

			got := normalizeDockerPath(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeDockerPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
