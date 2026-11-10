package workflows

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestForkMaintenancePipeline(t *testing.T) {
	t.Run("empty_release_pr_skips_pr_ci", func(t *testing.T) {
		build := loadWorkflow(t, "pr-test-build.yml")
		buildJob := requireChild(t, requireChild(t, build, "jobs"), "build")
		if got := strings.TrimSpace(requireScalar(t, buildJob, "if")); got != "github.head_ref != 'bot/release-pr'" {
			t.Fatalf("pr-test-build build if = %q, want skip bot/release-pr", got)
		}

		guard := loadWorkflow(t, "pr-path-guard.yml")
		guardJob := requireChild(t, requireChild(t, guard, "jobs"), "ensure-no-translator-changes")
		if got := strings.TrimSpace(requireScalar(t, guardJob, "if")); got != "github.head_ref != 'bot/release-pr'" {
			t.Fatalf("translator-path-guard if = %q, want skip bot/release-pr", got)
		}
	})

	t.Run("upstream_sync_dry_run_and_optional_token", func(t *testing.T) {
		upstream := loadWorkflow(t, "upstream-sync-pr.yml")

		env := requireChild(t, upstream, "env")
		if got := requireScalar(t, env, "DRY_RUN"); got != "${{ github.event_name == 'workflow_dispatch' && inputs.dry_run }}" {
			t.Fatalf("DRY_RUN = %q", got)
		}

		triggers := requireChild(t, upstream, "on")
		dispatch := requireChild(t, triggers, "workflow_dispatch")
		inputs := requireChild(t, dispatch, "inputs")
		dryRun := requireChild(t, inputs, "dry_run")
		if got := requireScalar(t, dryRun, "type"); got != "boolean" {
			t.Fatalf("dry_run.type = %q, want boolean", got)
		}

		_, dryRunStep := findStep(t, upstream, "sync-pr", func(step *yaml.Node) bool {
			return scalarValue(step, "name") == "Dry-run compare only"
		})
		if got := strings.TrimSpace(requireScalar(t, dryRunStep, "if")); got != "env.DRY_RUN == 'true'" {
			t.Fatalf("dry-run step if = %q", got)
		}

		_, syncStep := findStep(t, upstream, "sync-pr", func(step *yaml.Node) bool {
			return scalarValue(step, "id") == "sync_upstream"
		})
		if got := strings.TrimSpace(requireScalar(t, syncStep, "if")); got != "steps.compare.outputs.has_changes == 'true' && env.DRY_RUN != 'true'" {
			t.Fatalf("sync_upstream if = %q", got)
		}
		syncEnv := requireChild(t, syncStep, "env")
		if got := requireScalar(t, syncEnv, "GH_TOKEN"); got != "${{ secrets.FORK_SYNC_TOKEN || github.token }}" {
			t.Fatalf("sync_upstream GH_TOKEN = %q", got)
		}

		_, closeStep := findStep(t, upstream, "sync-pr", func(step *yaml.Node) bool {
			return scalarValue(step, "name") == "Close obsolete conflict review PR"
		})
		if !strings.Contains(requireScalar(t, closeStep, "if"), "env.DRY_RUN != 'true'") {
			t.Fatalf("close obsolete PR must skip dry-run: %q", scalarValue(closeStep, "if"))
		}

		_, reviewStep := findStep(t, upstream, "sync-pr", func(step *yaml.Node) bool {
			return scalarValue(step, "id") == "review_pr"
		})
		if !strings.Contains(requireScalar(t, reviewStep, "if"), "env.DRY_RUN != 'true'") {
			t.Fatalf("review_pr must skip dry-run: %q", scalarValue(reviewStep, "if"))
		}
		reviewEnv := requireChild(t, reviewStep, "env")
		if got := requireScalar(t, reviewEnv, "GH_TOKEN"); got != "${{ secrets.FORK_SYNC_TOKEN || github.token }}" {
			t.Fatalf("review_pr GH_TOKEN = %q", got)
		}
	})
}
