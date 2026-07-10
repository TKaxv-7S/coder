package agentscripts_test

import (
	"context"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentscripts"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestExecuteBasic(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	fLogger := newFakeScriptLogger()
	runner := setup(t, func(uuid2 uuid.UUID) agentscripts.ScriptLogger {
		return fLogger
	})
	defer runner.Close()
	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
	err := runner.Init([]codersdk.WorkspaceAgentScript{{
		LogSourceID: uuid.New(),
		Script:      "echo hello",
	}}, aAPI.ScriptCompleted)
	require.NoError(t, err)
	require.NoError(t, runner.Execute(context.Background(), agentscripts.ExecuteAllScripts))
	log := testutil.TryReceive(ctx, t, fLogger.logs)
	require.Equal(t, "hello", log.Output)
}

func TestEnv(t *testing.T) {
	t.Parallel()
	fLogger := newFakeScriptLogger()
	runner := setup(t, func(uuid2 uuid.UUID) agentscripts.ScriptLogger {
		return fLogger
	})
	defer runner.Close()
	id := uuid.New()
	script := "echo $CODER_SCRIPT_DATA_DIR\necho $CODER_SCRIPT_BIN_DIR\n"
	if runtime.GOOS == "windows" {
		script = `
			cmd.exe /c echo %CODER_SCRIPT_DATA_DIR%
			cmd.exe /c echo %CODER_SCRIPT_BIN_DIR%
		`
	}
	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
	err := runner.Init([]codersdk.WorkspaceAgentScript{{
		LogSourceID: id,
		Script:      script,
	}}, aAPI.ScriptCompleted)
	require.NoError(t, err)

	ctx := testutil.Context(t, testutil.WaitLong)

	done := testutil.Go(t, func() {
		err := runner.Execute(ctx, agentscripts.ExecuteAllScripts)
		assert.NoError(t, err)
	})
	defer func() {
		select {
		case <-ctx.Done():
		case <-done:
		}
	}()

	var log []agentsdk.Log
	for {
		select {
		case <-ctx.Done():
			require.Fail(t, "timed out waiting for logs")
		case l := <-fLogger.logs:
			t.Logf("log: %s", l.Output)
			log = append(log, l)
		}
		if len(log) >= 2 {
			break
		}
	}
	require.Contains(t, log[0].Output, filepath.Join(runner.DataDir(), id.String()))
	require.Contains(t, log[1].Output, runner.ScriptBinDir())
}

func TestTimeout(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "darwin" {
		t.Skip("this test is flaky on macOS, see https://github.com/coder/internal/issues/329")
	}
	runner := setup(t, nil)
	defer runner.Close()
	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
	err := runner.Init([]codersdk.WorkspaceAgentScript{{
		LogSourceID: uuid.New(),
		Script:      "sleep infinity",
		Timeout:     100 * time.Millisecond,
	}}, aAPI.ScriptCompleted)
	require.NoError(t, err)
	require.ErrorIs(t, runner.Execute(context.Background(), agentscripts.ExecuteAllScripts), agentscripts.ErrTimeout)
}

func TestScriptReportsTiming(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	fLogger := newFakeScriptLogger()
	runner := setup(t, func(uuid2 uuid.UUID) agentscripts.ScriptLogger {
		return fLogger
	})

	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
	err := runner.Init([]codersdk.WorkspaceAgentScript{{
		DisplayName: "say-hello",
		LogSourceID: uuid.New(),
		Script:      "echo hello",
	}}, aAPI.ScriptCompleted)
	require.NoError(t, err)
	require.NoError(t, runner.Execute(ctx, agentscripts.ExecuteAllScripts))
	runner.Close()

	log := testutil.TryReceive(ctx, t, fLogger.logs)
	require.Equal(t, "hello", log.Output)

	timings := aAPI.GetTimings()
	require.Equal(t, 1, len(timings))

	timing := timings[0]
	require.Equal(t, int32(0), timing.ExitCode)
	if assert.True(t, timing.Start.IsValid(), "start time should be valid") {
		require.NotZero(t, timing.Start.AsTime(), "start time should not be zero")
	}
	if assert.True(t, timing.End.IsValid(), "end time should be valid") {
		require.NotZero(t, timing.End.AsTime(), "end time should not be zero")
	}
	require.GreaterOrEqual(t, timing.End.AsTime(), timing.Start.AsTime())
}

// TestCronClose exists because cron.Run() can happen after cron.Close().
// If this happens, there used to be a deadlock.
func TestCronClose(t *testing.T) {
	t.Parallel()
	runner := agentscripts.New(agentscripts.Options{})
	runner.StartCron()
	require.NoError(t, runner.Close(), "close runner")
}

func TestExecuteOptions(t *testing.T) {
	t.Parallel()

	startScript := codersdk.WorkspaceAgentScript{
		ID:          uuid.New(),
		LogSourceID: uuid.New(),
		Script:      "echo start",
		RunOnStart:  true,
	}
	stopScript := codersdk.WorkspaceAgentScript{
		ID:          uuid.New(),
		LogSourceID: uuid.New(),
		Script:      "echo stop",
		RunOnStop:   true,
	}
	regularScript := codersdk.WorkspaceAgentScript{
		ID:          uuid.New(),
		LogSourceID: uuid.New(),
		Script:      "echo regular",
	}

	scripts := []codersdk.WorkspaceAgentScript{
		startScript,
		stopScript,
		regularScript,
	}

	scriptByID := func(t *testing.T, id uuid.UUID) codersdk.WorkspaceAgentScript {
		for _, script := range scripts {
			if script.ID == id {
				return script
			}
		}
		t.Fatal("script not found")
		return codersdk.WorkspaceAgentScript{}
	}

	wantOutput := map[uuid.UUID]string{
		startScript.ID:   "start",
		stopScript.ID:    "stop",
		regularScript.ID: "regular",
	}

	testCases := []struct {
		name    string
		option  agentscripts.ExecuteOption
		wantRun []uuid.UUID
	}{
		{
			name:    "ExecuteAllScripts",
			option:  agentscripts.ExecuteAllScripts,
			wantRun: []uuid.UUID{startScript.ID, stopScript.ID, regularScript.ID},
		},
		{
			name:    "ExecuteStartScripts",
			option:  agentscripts.ExecuteStartScripts,
			wantRun: []uuid.UUID{startScript.ID},
		},
		{
			name:    "ExecuteStopScripts",
			option:  agentscripts.ExecuteStopScripts,
			wantRun: []uuid.UUID{stopScript.ID},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			executedScripts := make(map[uuid.UUID]bool)
			fLogger := &executeOptionTestLogger{
				tb:              t,
				executedScripts: executedScripts,
				wantOutput:      wantOutput,
			}

			runner := setup(t, func(uuid.UUID) agentscripts.ScriptLogger {
				return fLogger
			})
			defer runner.Close()

			aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
			err := runner.Init(
				scripts,
				aAPI.ScriptCompleted,
			)
			require.NoError(t, err)

			err = runner.Execute(ctx, tc.option)
			require.NoError(t, err)

			gotRun := map[uuid.UUID]bool{}
			for _, id := range tc.wantRun {
				gotRun[id] = true
				require.True(t, executedScripts[id],
					"script %s should have run when using filter %s", scriptByID(t, id).Script, tc.name)
			}

			for _, script := range scripts {
				if _, ok := gotRun[script.ID]; ok {
					continue
				}
				require.False(t, executedScripts[script.ID],
					"script %s should not have run when using filter %s", script.Script, tc.name)
			}
		})
	}
}

type executeOptionTestLogger struct {
	tb              testing.TB
	executedScripts map[uuid.UUID]bool
	wantOutput      map[uuid.UUID]string
	mu              sync.Mutex
}

func (l *executeOptionTestLogger) Send(_ context.Context, logs ...agentsdk.Log) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, log := range logs {
		l.tb.Log(log.Output)
		for id, output := range l.wantOutput {
			if log.Output == output {
				l.executedScripts[id] = true
				break
			}
		}
	}
	return nil
}

func (*executeOptionTestLogger) Flush(context.Context) error {
	return nil
}

func setup(t *testing.T, getScriptLogger func(logSourceID uuid.UUID) agentscripts.ScriptLogger) *agentscripts.Runner {
	t.Helper()
	if getScriptLogger == nil {
		// noop
		getScriptLogger = func(uuid.UUID) agentscripts.ScriptLogger {
			return noopScriptLogger{}
		}
	}
	fs := afero.NewMemMapFs()
	logger := testutil.Logger(t)
	s, err := agentssh.NewServer(context.Background(), logger, prometheus.NewRegistry(), fs, agentexec.DefaultExecer, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = s.Close()
	})
	return agentscripts.New(agentscripts.Options{
		LogDir:          t.TempDir(),
		DataDirBase:     t.TempDir(),
		Logger:          logger,
		SSHServer:       s,
		Filesystem:      fs,
		GetScriptLogger: getScriptLogger,
	})
}

type noopScriptLogger struct{}

func (noopScriptLogger) Send(context.Context, ...agentsdk.Log) error {
	return nil
}

func (noopScriptLogger) Flush(context.Context) error {
	return nil
}

type fakeScriptLogger struct {
	logs chan agentsdk.Log
}

func (f *fakeScriptLogger) Send(ctx context.Context, logs ...agentsdk.Log) error {
	for _, log := range logs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case f.logs <- log:
			// OK!
		}
	}
	return nil
}

func (*fakeScriptLogger) Flush(context.Context) error {
	return nil
}

func newFakeScriptLogger() *fakeScriptLogger {
	return &fakeScriptLogger{make(chan agentsdk.Log, 100)}
}

func setupWithUnitManager(t *testing.T, getScriptLogger func(uuid.UUID) agentscripts.ScriptLogger) (*agentscripts.Runner, *unit.Manager) {
	t.Helper()
	logger := testutil.Logger(t)
	fs := afero.NewOsFs()
	s, err := agentssh.NewServer(context.Background(), logger, prometheus.NewRegistry(), fs, agentexec.DefaultExecer, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = s.Close()
	})
	m := unit.NewManager()
	r := agentscripts.New(agentscripts.Options{
		LogDir:          t.TempDir(),
		DataDirBase:     t.TempDir(),
		Logger:          logger,
		SSHServer:       s,
		Filesystem:      fs,
		GetScriptLogger: getScriptLogger,
		UnitManager:     m,
	})
	return r, m
}

func TestScriptUnitsRegistered(t *testing.T) {
	t.Parallel()

	runner, mgr := setupWithUnitManager(t, func(_ uuid.UUID) agentscripts.ScriptLogger {
		return noopScriptLogger{}
	})
	defer runner.Close()

	scripts := []codersdk.WorkspaceAgentScript{
		{
			ID:              uuid.New(),
			LogSourceID:     uuid.New(),
			DisplayName:     "Install Tools",
			ResourceAddress: "coder_script.install_tools",
			Script:          "echo install",
			RunOnStart:      true,
		},
		{
			ID:              uuid.New(),
			LogSourceID:     uuid.New(),
			DisplayName:     "Configure Environment",
			ResourceAddress: "module.dev.coder_script.configure",
			Script:          "echo configure",
			RunOnStart:      true,
		},
		{
			ID:          uuid.New(),
			LogSourceID: uuid.New(),
			DisplayName: "", // No display name or resource address; should be skipped.
			Script:      "echo nameless",
			RunOnStart:  true,
		},
		{
			ID:          uuid.New(),
			LogSourceID: uuid.New(),
			DisplayName: "Fallback Only", // No resource address; falls back to display name.
			Script:      "echo fallback",
			RunOnStart:  true,
		},
	}

	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
	err := runner.Init(scripts, aAPI.ScriptCompleted)
	require.NoError(t, err)

	// Three scripts should be registered (two with resource address, one with display name only).
	units := mgr.ListUnits()
	require.Len(t, units, 3)

	unitsByName := make(map[unit.ID]unit.Unit)
	for _, u := range units {
		unitsByName[u.ID()] = u
	}

	// ResourceAddress takes precedence over DisplayName.
	for _, name := range []string{"coder_script.install_tools", "module.dev.coder_script.configure", "Fallback Only"} {
		u, ok := unitsByName[unit.ID(name)]
		require.True(t, ok, "expected unit %q to be registered", name)
		require.Equal(t, unit.StatusPending, u.Status(), "unit %q should be pending", name)
	}
}

func TestScriptUnitsLifecycle(t *testing.T) {
	t.Parallel()

	runner, mgr := setupWithUnitManager(t, func(_ uuid.UUID) agentscripts.ScriptLogger {
		return noopScriptLogger{}
	})
	defer runner.Close()

	scriptName := "coder_script.my_script"
	scripts := []codersdk.WorkspaceAgentScript{
		{
			ID:              uuid.New(),
			LogSourceID:     uuid.New(),
			DisplayName:     "My Script",
			ResourceAddress: scriptName,
			Script:          "echo lifecycle",
			RunOnStart:      true,
		},
	}

	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
	err := runner.Init(scripts, aAPI.ScriptCompleted)
	require.NoError(t, err)

	// Before execution: pending.
	u, err := mgr.Unit(unit.ID(scriptName))
	require.NoError(t, err)
	require.Equal(t, unit.StatusPending, u.Status())

	// Execute the script.
	err = runner.Execute(context.Background(), agentscripts.ExecuteStartScripts)
	require.NoError(t, err)

	// After execution: completed.
	u, err = mgr.Unit(unit.ID(scriptName))
	require.NoError(t, err)
	require.Equal(t, unit.StatusComplete, u.Status())
}

func TestScriptUnitDependenciesAddedFromManifest(t *testing.T) {
	t.Parallel()

	runner, mgr := setupWithUnitManager(t, func(_ uuid.UUID) agentscripts.ScriptLogger {
		return noopScriptLogger{}
	})
	defer runner.Close()

	scripts := []codersdk.WorkspaceAgentScript{
		{
			ID:              uuid.New(),
			LogSourceID:     uuid.New(),
			ResourceAddress: "coder_script.clone",
			Script:          "echo clone",
			RunOnStart:      true,
		},
		{
			ID:              uuid.New(),
			LogSourceID:     uuid.New(),
			ResourceAddress: "coder_script.install",
			Script:          "echo install",
			RunOnStart:      true,
			Dependencies: []codersdk.WorkspaceAgentScriptDependency{
				{ResourceAddress: "coder_script.clone", RequiredStatus: string(unit.StatusComplete)},
			},
		},
	}

	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
	require.NoError(t, runner.Init(scripts, aAPI.ScriptCompleted))

	deps, err := mgr.GetAllDependencies(unit.ID("coder_script.install"))
	require.NoError(t, err)
	require.Len(t, deps, 1)
	require.Equal(t, unit.ID("coder_script.clone"), deps[0].DependsOn)
	require.Equal(t, unit.StatusComplete, deps[0].RequiredStatus)

	// The dependency script itself declares no ordering.
	cloneDeps, err := mgr.GetAllDependencies(unit.ID("coder_script.clone"))
	require.NoError(t, err)
	require.Empty(t, cloneDeps)
}

func TestScriptUnitDependencyGating(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	runner, mgr := setupWithUnitManager(t, func(_ uuid.UUID) agentscripts.ScriptLogger {
		return noopScriptLogger{}
	})
	defer runner.Close()

	// gate is an external unit the script depends on. Registering it lets the
	// test control when the dependency becomes satisfied.
	const gate unit.ID = "external.gate"
	require.NoError(t, mgr.Register(gate))

	const scriptName = "coder_script.install"
	scripts := []codersdk.WorkspaceAgentScript{
		{
			ID:              uuid.New(),
			LogSourceID:     uuid.New(),
			ResourceAddress: scriptName,
			Script:          "echo gated",
			RunOnStart:      true,
			Dependencies: []codersdk.WorkspaceAgentScriptDependency{
				{ResourceAddress: string(gate), RequiredStatus: string(unit.StatusComplete)},
			},
		},
	}

	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
	require.NoError(t, runner.Init(scripts, aAPI.ScriptCompleted))

	done := testutil.Go(t, func() {
		assert.NoError(t, runner.Execute(ctx, agentscripts.ExecuteStartScripts))
	})

	// The script must not run while its dependency is unsatisfied. Its unit
	// stays pending because run() blocks before marking it started.
	select {
	case <-done:
		t.Fatal("script executed before its dependency was satisfied")
	case <-time.After(testutil.IntervalMedium):
	}
	u, err := mgr.Unit(unit.ID(scriptName))
	require.NoError(t, err)
	require.Equal(t, unit.StatusPending, u.Status())

	// Satisfy the dependency; the script should now run to completion.
	require.NoError(t, mgr.UpdateStatus(gate, unit.StatusComplete))

	_, ok := <-done
	require.False(t, ok, "execute goroutine should have finished")

	u, err = mgr.Unit(unit.ID(scriptName))
	require.NoError(t, err)
	require.Equal(t, unit.StatusComplete, u.Status())
}

func TestScriptUnitDependencyTimeoutFailOpen(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	runner, mgr := setupWithUnitManager(t, func(_ uuid.UUID) agentscripts.ScriptLogger {
		return noopScriptLogger{}
	})
	defer runner.Close()
	// Keep the wait short so the fail-open path is exercised quickly. The
	// dependency target is never registered, so it can never be satisfied.
	runner.DependencyWaitTimeout = testutil.IntervalMedium

	const scriptName = "coder_script.install"
	scripts := []codersdk.WorkspaceAgentScript{
		{
			ID:              uuid.New(),
			LogSourceID:     uuid.New(),
			ResourceAddress: scriptName,
			Script:          "echo failopen",
			RunOnStart:      true,
			Dependencies: []codersdk.WorkspaceAgentScriptDependency{
				{ResourceAddress: "external.never", RequiredStatus: string(unit.StatusComplete)},
			},
		},
	}

	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
	require.NoError(t, runner.Init(scripts, aAPI.ScriptCompleted))

	// Despite the never-satisfied dependency, the script runs after the
	// timeout elapses (fail-open) and completes without error.
	require.NoError(t, runner.Execute(ctx, agentscripts.ExecuteStartScripts))

	u, err := mgr.Unit(unit.ID(scriptName))
	require.NoError(t, err)
	require.Equal(t, unit.StatusComplete, u.Status())
}
