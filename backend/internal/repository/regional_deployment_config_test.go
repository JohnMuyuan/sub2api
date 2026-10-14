//go:build unit

package repository

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRegionalDeploymentRenderedConcurrency(t *testing.T) {
	root, err := filepath.Abs("../../../deploy/regional")
	require.NoError(t, err)
	out := filepath.Join(t.TempDir(), "rendered")
	cmd := exec.Command("python3", "-c", "import sys; sys.path.insert(0, sys.argv[1]); import render, test_render; f=test_render.RenderTests(); f.setUp(); render.render(f.spec,f.secrets,sys.argv[2])", root, out)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	for _, instance := range []string{"primary-a-0", "primary-b-0", "region-a-0", "region-a-1", "region-b-0", "region-b-1"} {
		t.Run(instance, func(t *testing.T) {
			c, err := readRegionalConcurrencyConfig(filepath.Join(out, instance, "concurrency.json"))
			require.NoError(t, err)
			require.Equal(t, instance, c.Instance)
			if c.Mode == "primary" {
				require.NoError(t, c.Scope.Validate())
				require.Equal(t, time.Minute, c.Scope.MaximumLifetime)
				require.ElementsMatch(t, []string{"primary-a-0", "primary-b-0"}, c.PrimaryInstances)
			} else {
				require.Len(t, c.Targets, 3)
				for _, target := range c.Targets {
					require.NoError(t, target.Resource.Validate())
					require.Equal(t, 1, target.Capacity)
				}
			}
		})
	}
}
