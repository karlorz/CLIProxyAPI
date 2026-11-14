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
		buildSkip := requireChild(t, requireChild(t, build, "jobs"), "skip-bot-release-pr")
		if got := strings.TrimSpace(requireScalar(t, buildSkip, "if")); got != "github.head_ref == 'bot/release-pr'" {
			t.Fatalf("pr-test-build skip-bot-release-pr if = %q, want success job for bot/release-pr", got)
		}

		guard := loadWorkflow(t, "pr-path-guard.yml")
		guardJob := requireChild(t, requireChild(t, guard, "jobs"), "ensure-no-translator-changes")
		if got := strings.TrimSpace(requireScalar(t, guardJob, "if")); got != "github.head_ref != 'bot/release-pr'" {
			t.Fatalf("translator-path-guard if = %q, want skip bot/release-pr", got)
		}
		guardSkip := requireChild(t, requireChild(t, guard, "jobs"), "skip-bot-release-pr")
		if got := strings.TrimSpace(requireScalar(t, guardSkip, "if")); got != "github.head_ref == 'bot/release-pr'" {
			t.Fatalf("translator-path-guard skip-bot-release-pr if = %q, want success job for bot/release-pr", got)
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

	t.Run("release_pr_auto_merges_with_merge_commit", func(t *testing.T) {
		release := loadWorkflow(t, "release-pr.yml")
		permissions := requireChild(t, release, "permissions")
		if got := requireScalar(t, permissions, "actions"); got != "write" {
			t.Fatalf("release-pr permissions.actions = %q, want write", got)
		}

		_, upsert := findStep(t, release, "draft-release-pr", func(step *yaml.Node) bool {
			return scalarValue(step, "id") == "upsert_pr"
		})
		upsertRun := requireScalar(t, upsert, "run")
		if strings.Contains(upsertRun, "--draft") {
			t.Fatal("release PR create must not use --draft")
		}
		if strings.Contains(upsertRun, "--undo") {
			t.Fatal("release PR must stay ready, not return to draft")
		}

		_, mergeStep := findStep(t, release, "draft-release-pr", func(step *yaml.Node) bool {
			return scalarValue(step, "id") == "merge_release_pr"
		})
		mergeEnv := requireChild(t, mergeStep, "env")
		if got := requireScalar(t, mergeEnv, "FORK_SYNC_TOKEN"); got != "${{ secrets.FORK_SYNC_TOKEN }}" {
			t.Fatalf("merge FORK_SYNC_TOKEN env = %q", got)
		}
		mergeRun := requireScalar(t, mergeStep, "run")
		for _, want := range []string{
			"gh pr ready",
			"gh pr merge",
			"--merge",
			"gh workflow run fork-tag.yml",
			`if [ -z "${FORK_SYNC_TOKEN}" ]`,
		} {
			if !strings.Contains(mergeRun, want) {
				t.Fatalf("merge step missing %q:\n%s", want, mergeRun)
			}
		}
		if strings.Contains(mergeRun, "--squash") || strings.Contains(mergeRun, "--rebase") {
			t.Fatal("release PR must not squash or rebase")
		}
	})

	t.Run("release_create_retries_and_checksums_need_prepare", func(t *testing.T) {
		release := loadWorkflow(t, "release.yaml")
		_, create := findStep(t, release, "prepare-release", func(step *yaml.Node) bool {
			return scalarValue(step, "name") == "Create release"
		})
		createRun := requireScalar(t, create, "run")
		for _, want := range []string{
			"create_or_edit_release",
			"retrying in",
			"gh release create",
			"gh release edit",
		} {
			if !strings.Contains(createRun, want) {
				t.Fatalf("create release missing %q:\n%s", want, createRun)
			}
		}

		checksums := requireChild(t, requireChild(t, release, "jobs"), "publish-checksums")
		if got := strings.Join(strings.Fields(requireScalar(t, checksums, "if")), " "); !strings.Contains(got, "needs.prepare-release.result == 'success'") {
			t.Fatalf("publish-checksums if = %q, want prepare-release success", got)
		}
		needs := requireChild(t, checksums, "needs")
		if needs.Kind != yaml.SequenceNode {
			t.Fatalf("publish-checksums.needs kind = %d, want sequence", needs.Kind)
		}
		foundPrepare := false
		for _, item := range needs.Content {
			if item.Value == "prepare-release" {
				foundPrepare = true
				break
			}
		}
		if !foundPrepare {
			t.Fatal("publish-checksums.needs must include prepare-release")
		}
	})
}
