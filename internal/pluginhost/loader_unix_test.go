//go:build cgo && (linux || darwin || freebsd)

package pluginhost

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func dylibExtension() string {
	switch runtime.GOOS {
	case "darwin":
		return ".dylib"
	default:
		return ".so"
	}
}

func buildTestFixture(t *testing.T, dir string, targetPath string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-buildmode=c-shared", "-o", targetPath, ".")
	cmd.Dir = dir
	cmd.WaitDelay = 5 * time.Second
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build fixture from %s: %v, output: %s", dir, err, string(out))
	}
}

const (
	envSubprocessCase = "TEST_LOADER_UNIX_SUBPROCESS_CASE"
	envPluginPath     = "TEST_LOADER_UNIX_PLUGIN_PATH"
	envInitMode       = "TEST_LIFECYCLE_INIT_MODE"
	envMarkerFile     = "TEST_LIFECYCLE_MARKER_FILE"
)

func runSubprocess(t *testing.T, testCase, pluginPath, initMode, markerFile string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.v", "-test.run=^TestDynamicLibraryUnixSubprocessHelper$")
	cmd.WaitDelay = 5 * time.Second
	cmd.Env = append(os.Environ(),
		envSubprocessCase+"="+testCase,
		envPluginPath+"="+pluginPath,
		envInitMode+"="+initMode,
		envMarkerFile+"="+markerFile,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess %s failed: %v\noutput:\n%s", testCase, err, string(out))
	}
	if !cmd.ProcessState.Success() {
		t.Fatalf("subprocess %s process state not successful: %v\noutput:\n%s", testCase, cmd.ProcessState, string(out))
	}
}

func TestDynamicLibraryUnixSubprocessHelper(t *testing.T) {
	testCase := os.Getenv(envSubprocessCase)
	if testCase == "" {
		t.Skip("skipping subprocess helper when not executed in child mode")
		return
	}

	pluginPath := os.Getenv(envPluginPath)
	markerFile := os.Getenv(envMarkerFile)
	loader := defaultPluginLoader()
	host := NewForTest(loader)

	switch testCase {
	case "success_lifecycle":
		file := pluginFile{ID: "test-plugin", Path: pluginPath}
		client, errOpen := loader.Open(file, host)
		if errOpen != nil {
			t.Fatalf("loader.Open() error = %v", errOpen)
		}
		if client == nil {
			t.Fatalf("loader.Open() returned nil client")
		}
		t.Cleanup(client.Shutdown)

		dynClient, okDyn := client.(*dynamicLibraryClient)
		if !okDyn {
			t.Fatalf("client is %T, want *dynamicLibraryClient", client)
		}

		resp, errCall := client.Call(context.Background(), "test.ping", []byte(`{}`))
		if errCall != nil {
			t.Fatalf("client.Call() before shutdown error = %v", errCall)
		}
		if !strings.Contains(string(resp), `"ok":true`) {
			t.Fatalf("unexpected call response: %s", string(resp))
		}

		// First shutdown releases resources
		client.Shutdown()

		// Verify client fields cleared
		if dynClient.hostAPI != nil {
			t.Fatalf("hostAPI was not cleared on Shutdown()")
		}
		if dynClient.hostCtx != nil {
			t.Fatalf("hostCtx was not cleared on Shutdown()")
		}
		if dynClient.handle != nil {
			t.Fatalf("handle was not cleared on Shutdown()")
		}
		if dynClient.api.call != nil || dynClient.api.free_buffer != nil || dynClient.api.shutdown != nil {
			t.Fatalf("plugin API function table was not invalidated on Shutdown()")
		}

		// Verify callback entry removed from sync.Map
		callbackCount := 0
		hostCallbackEntries.Range(func(_, _ any) bool {
			callbackCount++
			return true
		})
		if callbackCount != 0 {
			t.Fatalf("hostCallbackEntries count = %d, want 0", callbackCount)
		}

		// Call after shutdown fails closed
		_, errCallAfter := client.Call(context.Background(), "test.ping", []byte(`{}`))
		if errCallAfter == nil || !strings.Contains(errCallAfter.Error(), "closed") {
			t.Fatalf("client.Call() after shutdown expected closed error, got: %v", errCallAfter)
		}

		// Shutdown idempotence: second Shutdown must be safe and not re-invoke plugin shutdown
		client.Shutdown()

		if markerFile != "" {
			markerBytes, errMarker := os.ReadFile(markerFile)
			if errMarker != nil {
				t.Fatalf("read marker file: %v", errMarker)
			}
			if got := strings.TrimSpace(string(markerBytes)); got != "1" {
				t.Fatalf("plugin shutdown calls = %q, want exactly 1", got)
			}
		}

	case "missing_init":
		file := pluginFile{ID: "test-missing-init", Path: pluginPath}
		client, errOpen := loader.Open(file, host)
		if errOpen == nil {
			if client != nil {
				t.Cleanup(client.Shutdown)
			}
			t.Fatalf("expected Open() to fail for missing init symbol, got client")
		}
		if !strings.Contains(errOpen.Error(), "missing cliproxy_plugin_init") {
			t.Fatalf("unexpected error message: %v", errOpen)
		}

	case "init_error":
		file := pluginFile{ID: "test-init-error", Path: pluginPath}
		client, errOpen := loader.Open(file, host)
		if errOpen == nil {
			if client != nil {
				t.Cleanup(client.Shutdown)
			}
			t.Fatalf("expected Open() to fail when init returns error, got client")
		}
		if !strings.Contains(errOpen.Error(), "cliproxy_plugin_init returned 1") {
			t.Fatalf("unexpected error message: %v", errOpen)
		}

	case "wrong_abi":
		file := pluginFile{ID: "test-wrong-abi", Path: pluginPath}
		client, errOpen := loader.Open(file, host)
		if errOpen == nil {
			if client != nil {
				t.Cleanup(client.Shutdown)
			}
			t.Fatalf("expected Open() to fail for wrong ABI, got client")
		}
		if !strings.Contains(errOpen.Error(), "plugin ABI version 99999 is not supported") {
			t.Fatalf("unexpected error message: %v", errOpen)
		}

	case "incomplete_api":
		file := pluginFile{ID: "test-incomplete-api", Path: pluginPath}
		client, errOpen := loader.Open(file, host)
		if errOpen == nil {
			if client != nil {
				t.Cleanup(client.Shutdown)
			}
			t.Fatalf("expected Open() to fail for incomplete function table, got client")
		}
		if !strings.Contains(errOpen.Error(), "plugin function table is incomplete") {
			t.Fatalf("unexpected error message: %v", errOpen)
		}

	default:
		t.Fatalf("unknown subprocess test case: %s", testCase)
	}

	// Shared runtime.GC/concurrency tail for all child cases
	done := make(chan struct{})
	go func() {
		for i := 0; i < 5; i++ {
			runtime.GC()
			runtime.Gosched()
		}
		close(done)
	}()
	<-done
}

func TestDynamicLibraryUnixLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	normalPath := filepath.Join(tempDir, "lifecycle-normal"+dylibExtension())
	missingPath := filepath.Join(tempDir, "lifecycle-missing"+dylibExtension())

	buildTestFixture(t, filepath.Join("testdata", "go-shared-lifecycle"), normalPath)
	buildTestFixture(t, filepath.Join("testdata", "go-shared-lifecycle", "missing_init"), missingPath)

	tests := []struct {
		name       string
		caseName   string
		pluginPath string
		initMode   string
		useMarker  bool
	}{
		{
			name:       "SuccessLifecycle",
			caseName:   "success_lifecycle",
			pluginPath: normalPath,
			initMode:   "default",
			useMarker:  true,
		},
		{
			name:       "MissingInit",
			caseName:   "missing_init",
			pluginPath: missingPath,
			initMode:   "default",
		},
		{
			name:       "InitReturnsError",
			caseName:   "init_error",
			pluginPath: normalPath,
			initMode:   "error",
		},
		{
			name:       "WrongABIVersion",
			caseName:   "wrong_abi",
			pluginPath: normalPath,
			initMode:   "wrong_abi",
		},
		{
			name:       "IncompleteAPI",
			caseName:   "incomplete_api",
			pluginPath: normalPath,
			initMode:   "incomplete_api",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			markerFile := ""
			if tc.useMarker {
				markerFile = filepath.Join(t.TempDir(), "shutdown_marker.txt")
			}
			runSubprocess(t, tc.caseName, tc.pluginPath, tc.initMode, markerFile)
		})
	}
}
