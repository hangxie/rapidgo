package buildinfo

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModuleVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		info    *debug.BuildInfo
		want    string
	}{
		{name: "tagged install", version: "dev", info: &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, want: "v0.1.0"},
		{name: "local build", version: "dev", info: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, want: "dev"},
		{name: "missing version", version: "dev", info: &debug.BuildInfo{}, want: "dev"},
		{name: "missing metadata", version: "dev", want: "dev"},
		{name: "injected release version", version: "v0.1.0", info: &debug.BuildInfo{Main: debug.Module{Version: "v0.0.9"}}, want: "v0.1.0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, moduleVersion(test.version, test.info))
		})
	}
}
