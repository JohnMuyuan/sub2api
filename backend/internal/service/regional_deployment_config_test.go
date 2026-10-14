//go:build unit

package service

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRegionalDeploymentRenderedSnapshots(t *testing.T) {
	root, err := filepath.Abs("../../../deploy/regional")
	require.NoError(t, err)
	out := filepath.Join(t.TempDir(), "rendered")
	// Generate only synthetic keys in temporary storage; never read operator secrets.
	cmd := exec.Command("python3", "-c", "import sys; sys.path.insert(0, sys.argv[1]); import render, test_render; f=test_render.RenderTests(); f.setUp(); render.render(f.spec,f.secrets,sys.argv[2])", root, out)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	for _, instance := range []string{"primary-a-0", "primary-b-0", "region-a-0", "region-a-1", "region-b-0", "region-b-1"} {
		role := config.RuntimeRoleGateway
		if strings.HasPrefix(instance, "primary") {
			role = config.RuntimeRoleFull
		}
		for _, kind := range []string{"auth", "scheduler"} {
			t.Run(instance+"/"+kind, func(t *testing.T) {
				c, err := readRegionalSchedulerConfig(filepath.Join(out, instance, kind+".json"), &config.Config{Runtime: config.RuntimeConfig{Role: role}})
				require.NoError(t, err)
				require.False(t, c.Bootstrap)
				require.Equal(t, kind, string(c.snapshotKind()))
				if role == config.RuntimeRoleGateway {
					require.Contains(t, c.Endpoint, "/internal/regional/"+kind)
					require.Contains(t, c.StateFile, "/"+instance+"/")
				}
			})
		}
	}
}
