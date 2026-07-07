package v2

import (
	"fmt"
	"os"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

const (
	owner = "coder"
	repo  = "coder"
)

// newExecutor returns the appropriate CommandExecutor based on the
// dry-run setting.
//
//nolint:revive // dryRun selects the dry-run executor.
func newExecutor(dryRun bool) CommandExecutor {
	if dryRun {
		return newDryRunExecutor(os.Stderr)
	}
	return realExecutor{}
}

// Commands returns the three release subcommands: rc, branch, and
// release. Each is run from a developer's command line and detects the
// branch it is invoked from; there is no --ref flag.
func Commands() []*serpent.Command {
	return []*serpent.Command{
		releaseCommand("rc", "Tag a release candidate from the current branch (main or release/X.Y).", "rc"),
		releaseCommand("branch", "Cut a new release branch from main and tag its first release candidate.", "create-release-branch"),
		releaseCommand("release", "Tag a stable release or patch from the current release/X.Y branch.", "release"),
	}
}

// releaseCommand builds a subcommand for a fixed release type.
func releaseCommand(use, short, releaseType string) *serpent.Command {
	var (
		commitSHA string
		dryRun    bool
	)
	return &serpent.Command{
		Use:   use,
		Short: short,
		Options: serpent.OptionSet{
			{
				Name:        "commit",
				Flag:        "commit",
				Description: "Tag an earlier commit instead of the branch tip. Useful for tagging an older commit on main for an rc.",
				Value:       serpent.StringOf(&commitSHA),
			},
			{
				Name:        "dry-run",
				Flag:        "dry-run",
				Description: "Print mutating commands (tag, push, workflow trigger) instead of executing them.",
				Value:       serpent.BoolOf(&dryRun),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			if releaseType == "release" && commitSHA != "" {
				return xerrors.New("--commit is not supported for release; releases always tag the branch tip")
			}
			return runReleaseType(inv, newExecutor(dryRun), releaseType, commitSHA)
		},
	}
}

// runReleaseType executes the full release flow for the given type from
// the current branch: calculate the next version, create and push the
// tag (and the release branch when cutting one), generate release notes,
// and trigger the release.yaml workflow, mirroring the legacy tool.
func runReleaseType(inv *serpent.Invocation, exec CommandExecutor, releaseType, commitSHA string) error {
	w := inv.Stderr

	branch, err := currentBranch(exec)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "Releasing %q from branch %q\n", releaseType, branch)

	result, err := calculateNextVersion(exec, releaseType, branch, commitSHA)
	if err != nil {
		return err
	}

	var (
		versionTag  string
		previousTag string
		targetRef   string
		stable      bool
		branchName  string
	)
	switch v := result.(type) {
	case CreateBranchRequest:
		versionTag, previousTag, targetRef, stable, branchName = v.Version, v.PreviousVersion, v.TargetRef, v.Stable, v.BranchName
	case ReleaseRequest:
		versionTag, previousTag, targetRef, stable = v.Version, v.PreviousVersion, v.TargetRef, v.Stable
	default:
		return xerrors.Errorf("unexpected result type %T", result)
	}

	_, _ = fmt.Fprintf(w, "Version:  %s\n", versionTag)
	_, _ = fmt.Fprintf(w, "Previous: %s\n", previousTag)
	_, _ = fmt.Fprintf(w, "Commit:   %s\n", targetRef)
	if branchName != "" {
		_, _ = fmt.Fprintf(w, "Branch:   %s\n", branchName)
	}

	// Create and push the tag, then the release branch when cutting one.
	if err := createAndPushTag(exec, versionTag, targetRef); err != nil {
		return err
	}
	if branchName != "" {
		if err := createAndPushBranch(exec, branchName, targetRef); err != nil {
			return err
		}
	}

	// Generate release notes for the new tag.
	newVer, err := parseVersion(versionTag)
	if err != nil {
		return xerrors.Errorf("parse new version %q: %w", versionTag, err)
	}
	prevVer, err := parseVersion(previousTag)
	if err != nil {
		return xerrors.Errorf("parse previous version %q: %w", previousTag, err)
	}
	notes, err := generateReleaseNotes(exec, newVer, prevVer)
	if err != nil {
		return xerrors.Errorf("generate release notes: %w", err)
	}
	_, _ = fmt.Fprint(inv.Stdout, notes)

	// Trigger the release.yaml workflow at the new tag, matching the
	// inputs used by the legacy interactive tool.
	channel := channelFor(newVer, stable)
	if err := triggerReleaseWorkflow(exec, versionTag, channel, notes); err != nil {
		return xerrors.Errorf("trigger release workflow: %w", err)
	}
	_, _ = fmt.Fprintf(w, "Triggered release.yaml (channel=%s) for %s\n", channel, versionTag)
	return nil
}

// currentBranch returns the branch the command is run from. In detached
// HEAD state it returns "main" when HEAD is an ancestor of origin/main,
// matching how release managers check out a specific commit on main
// before tagging an rc.
func currentBranch(exec CommandExecutor) (string, error) {
	branch, err := gitOutput(exec, "branch", "--show-current")
	if err != nil {
		return "", xerrors.Errorf("detect current branch: %w", err)
	}
	if branch != "" {
		return branch, nil
	}
	if err := gitRun(exec, "merge-base", "--is-ancestor", "HEAD", "origin/main"); err == nil {
		return "main", nil
	}
	return "", xerrors.New("detached HEAD is not an ancestor of origin/main; check out main or a release/X.Y branch")
}

// channelFor maps a version and its stable flag to the release.yaml
// release_channel input.
func channelFor(v version, stable bool) string {
	switch {
	case v.IsRC():
		return "rc"
	case stable:
		return "stable"
	default:
		return "mainline"
	}
}

// triggerReleaseWorkflow dispatches the release.yaml GitHub Actions
// workflow at the given tag. It is a mutating operation, so it is
// printed instead of executed in dry-run mode.
func triggerReleaseWorkflow(exec CommandExecutor, tag, channel, notes string) error {
	return exec.RunMutation("gh", "workflow", "run", "release.yaml",
		"--repo", fmt.Sprintf("%s/%s", owner, repo),
		"--ref", tag,
		"-f", "dry_run=false",
		"-f", "release_channel="+channel,
		"-f", "release_notes="+notes,
	)
}
