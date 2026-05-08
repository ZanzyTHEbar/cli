package login

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/fosrl/cli/internal/api"
	"github.com/fosrl/cli/internal/config"
	"github.com/spf13/cobra"
)

func TestRenderDeviceLoginPromptPlainWritesLineOrientedOutput(t *testing.T) {
	var out bytes.Buffer

	err := renderDeviceLoginPrompt(&out, "CODE", "https://example.test/auth/login/device", webLoginOpts{Plain: true}, false)
	if err != nil {
		t.Fatalf("renderDeviceLoginPrompt returned error: %v", err)
	}

	want := "Device code: CODE\nOpen: https://example.test/auth/login/device\nWaiting for approval...\n"
	if got := out.String(); got != want {
		t.Fatalf("plain prompt mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestRenderDeviceLoginPromptPlainReturnsWriterError(t *testing.T) {
	wantErr := errors.New("write failed")

	err := renderDeviceLoginPrompt(failingWriter{err: wantErr}, "CODE", "https://example.test/auth/login/device", webLoginOpts{Plain: true}, false)
	if !errors.Is(err, wantErr) {
		t.Fatalf("renderDeviceLoginPrompt error = %v, want %v", err, wantErr)
	}
}

func TestRenderDeviceLoginPromptJSONNoBrowserWritesOnlyJSON(t *testing.T) {
	var out bytes.Buffer

	err := renderDeviceLoginPrompt(&out, "CODE", "https://example.test/auth/login/device", webLoginOpts{JSON: true, NoBrowser: true}, false)
	if err != nil {
		t.Fatalf("renderDeviceLoginPrompt returned error: %v", err)
	}

	decoder := json.NewDecoder(&out)
	var prompt deviceLoginPrompt
	if err := decoder.Decode(&prompt); err != nil {
		t.Fatalf("JSON prompt is invalid: %v", err)
	}

	if prompt.DeviceCode != "CODE" {
		t.Fatalf("device code mismatch: %q", prompt.DeviceCode)
	}
	if prompt.DeviceLoginURL != "https://example.test/auth/login/device" {
		t.Fatalf("device login URL mismatch: %q", prompt.DeviceLoginURL)
	}
	if prompt.Mode != "json" {
		t.Fatalf("mode mismatch: %q", prompt.Mode)
	}
	if prompt.BrowserLaunchAttempted {
		t.Fatalf("browser_launch_attempted = true, want false")
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("JSON prompt had extra stdout text or JSON values: %v", err)
	}
}

func TestBrowserDecisionHelpers(t *testing.T) {
	tests := []struct {
		name                string
		opts                webLoginOpts
		wantOpenImmediately bool
		wantPrompt          bool
	}{
		{
			name:                "no-browser plain",
			opts:                webLoginOpts{Plain: true, NoBrowser: true},
			wantOpenImmediately: false,
			wantPrompt:          false,
		},
		{
			name:                "no-browser json",
			opts:                webLoginOpts{JSON: true, NoBrowser: true},
			wantOpenImmediately: false,
			wantPrompt:          false,
		},
		{
			name:                "plain opens immediately",
			opts:                webLoginOpts{Plain: true},
			wantOpenImmediately: true,
			wantPrompt:          false,
		},
		{
			name:                "json opens immediately",
			opts:                webLoginOpts{JSON: true},
			wantOpenImmediately: true,
			wantPrompt:          false,
		},
		{
			name:                "default prompts for enter",
			opts:                webLoginOpts{},
			wantOpenImmediately: false,
			wantPrompt:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldOpenBrowserImmediately(tt.opts); got != tt.wantOpenImmediately {
				t.Fatalf("shouldOpenBrowserImmediately() = %v, want %v", got, tt.wantOpenImmediately)
			}
			if got := shouldPromptForBrowser(tt.opts); got != tt.wantPrompt {
				t.Fatalf("shouldPromptForBrowser() = %v, want %v", got, tt.wantPrompt)
			}
		})
	}
}

func TestBuildDeviceLoginURLEscapesCode(t *testing.T) {
	got := buildDeviceLoginURL("https://example.test/auth/login/device", "A+B C/=")
	want := "https://example.test/auth/login/device?code=A%2BB+C%2F%3D"
	if got != want {
		t.Fatalf("buildDeviceLoginURL() = %q, want %q", got, want)
	}
}

func TestLoginMainJSONNoBrowserKeepsStdoutClean(t *testing.T) {
	env := newLoginMainTestEnv(t, []api.Org{{OrgID: "org-a", Name: "Alpha"}})
	var gotHostname string
	var gotOpts webLoginOpts

	withFakeLoginWithWeb(t, func(hostname string, opts webLoginOpts) (string, error) {
		gotHostname = hostname
		gotOpts = opts
		return "session-token", renderDeviceLoginPrompt(os.Stdout, "CODE", hostname+"/auth/login/device", opts, shouldOpenBrowserImmediately(opts))
	})

	stdout, err := captureStdout(t, func() error {
		return loginMain(env.cmd, &LoginCmdOpts{Hostname: env.serverURL, JSON: true, NoBrowser: true})
	})
	if err != nil {
		t.Fatalf("loginMain returned error: %v", err)
	}
	if gotHostname != env.serverURL {
		t.Fatalf("hostname = %q, want %q", gotHostname, env.serverURL)
	}
	if !gotOpts.JSON || !gotOpts.NoBrowser || gotOpts.Plain {
		t.Fatalf("web login opts = %+v, want JSON no-browser", gotOpts)
	}

	prompt := decodeSingleDevicePrompt(t, stdout)
	if prompt.DeviceCode != "CODE" {
		t.Fatalf("device code = %q, want CODE", prompt.DeviceCode)
	}
	if prompt.BrowserLaunchAttempted {
		t.Fatalf("browser_launch_attempted = true, want false")
	}
	if env.accountStore.ActiveUserID != "user-1" {
		t.Fatalf("active user ID = %q, want user-1", env.accountStore.ActiveUserID)
	}
	if got := env.accountStore.Accounts["user-1"].OrgID; got != "org-a" {
		t.Fatalf("account org ID = %q, want org-a", got)
	}
}

func TestLoginMainPlainNoBrowserUsesEndpointAndLineOutput(t *testing.T) {
	env := newLoginMainTestEnv(t, []api.Org{{OrgID: "org-a", Name: "Alpha"}})
	t.Setenv("PANGOLIN_ENDPOINT", env.serverURL)
	var gotHostname string
	var gotOpts webLoginOpts

	withFakeLoginWithWeb(t, func(hostname string, opts webLoginOpts) (string, error) {
		gotHostname = hostname
		gotOpts = opts
		return "session-token", renderDeviceLoginPrompt(os.Stdout, "CODE", hostname+"/auth/login/device", opts, shouldOpenBrowserImmediately(opts))
	})

	stdout, err := captureStdout(t, func() error {
		return loginMain(env.cmd, &LoginCmdOpts{Plain: true, NoBrowser: true})
	})
	if err != nil {
		t.Fatalf("loginMain returned error: %v", err)
	}
	if gotHostname != env.serverURL {
		t.Fatalf("hostname = %q, want %q", gotHostname, env.serverURL)
	}
	if !gotOpts.Plain || !gotOpts.NoBrowser || gotOpts.JSON {
		t.Fatalf("web login opts = %+v, want plain no-browser", gotOpts)
	}
	for _, want := range []string{"Device code: CODE", "Open: " + env.serverURL + "/auth/login/device", "Waiting for approval..."} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout %q does not contain %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "Press Enter") {
		t.Fatalf("stdout contains Enter prompt: %q", stdout)
	}
}

func TestLoginMainJSONMultipleOrgsRequiresOrgIDWithoutExtraStdout(t *testing.T) {
	env := newLoginMainTestEnv(t, []api.Org{
		{OrgID: "org-a", Name: "Alpha"},
		{OrgID: "org-b", Name: "Beta"},
	})

	withFakeLoginWithWeb(t, func(hostname string, opts webLoginOpts) (string, error) {
		return "session-token", renderDeviceLoginPrompt(os.Stdout, "CODE", hostname+"/auth/login/device", opts, shouldOpenBrowserImmediately(opts))
	})

	stdout, err := captureStdout(t, func() error {
		return loginMain(env.cmd, &LoginCmdOpts{Hostname: env.serverURL, JSON: true, NoBrowser: true})
	})
	if err == nil {
		t.Fatal("loginMain returned nil error for multiple orgs without --org-id")
	}
	for _, want := range []string{"multiple organizations", "--org-id"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err.Error(), want)
		}
	}
	decodeSingleDevicePrompt(t, stdout)
}

func TestResolveOrgForLoginValidOrgIDReturnsOrg(t *testing.T) {
	client, cleanup := newOrgClient(t, []api.Org{{OrgID: "org-a", Name: "Alpha"}})
	defer cleanup()

	orgID, err := resolveOrgForLogin(client, "user-1", "org-a")
	if err != nil {
		t.Fatalf("resolveOrgForLogin returned error: %v", err)
	}
	if orgID != "org-a" {
		t.Fatalf("orgID = %q, want org-a", orgID)
	}
}

func TestResolveOrgForLoginInvalidOrgIDErrorsWithAvailableOrgs(t *testing.T) {
	client, cleanup := newOrgClient(t, []api.Org{
		{OrgID: "org-a", Name: "Alpha"},
		{OrgID: "org-b", Name: "Beta"},
	})
	defer cleanup()

	_, err := resolveOrgForLogin(client, "user-1", "missing")
	if err == nil {
		t.Fatal("resolveOrgForLogin returned nil error for invalid org")
	}
	for _, want := range []string{"missing", "available organizations", "org-a (Alpha)", "org-b (Beta)"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err.Error(), want)
		}
	}
}

func TestResolveOrgForLoginZeroOrgsErrorsClearly(t *testing.T) {
	client, cleanup := newOrgClient(t, nil)
	defer cleanup()

	_, err := resolveOrgForLogin(client, "user-1", "missing")
	if err == nil {
		t.Fatal("resolveOrgForLogin returned nil error for zero orgs")
	}
	for _, want := range []string{"missing", "no organizations"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err.Error(), want)
		}
	}
}

func TestResolveOrgForNonInteractiveLoginOneOrgAutoSelects(t *testing.T) {
	client, cleanup := newOrgClient(t, []api.Org{{OrgID: "only-org", Name: "Only"}})
	defer cleanup()

	orgID, err := resolveOrgForNonInteractiveLogin(client, "user-1", "")
	if err != nil {
		t.Fatalf("resolveOrgForNonInteractiveLogin returned error: %v", err)
	}
	if orgID != "only-org" {
		t.Fatalf("orgID = %q, want only-org", orgID)
	}
}

func TestResolveOrgForNonInteractiveLoginZeroOrgsErrors(t *testing.T) {
	client, cleanup := newOrgClient(t, nil)
	defer cleanup()

	_, err := resolveOrgForNonInteractiveLogin(client, "user-1", "")
	if err == nil {
		t.Fatal("resolveOrgForNonInteractiveLogin returned nil error for zero orgs")
	}
	if !strings.Contains(err.Error(), "no organizations") {
		t.Fatalf("error %q does not contain %q", err.Error(), "no organizations")
	}
}

func TestResolveOrgForNonInteractiveLoginMultipleOrgsErrorsWithOrgIDFlag(t *testing.T) {
	client, cleanup := newOrgClient(t, []api.Org{
		{OrgID: "org-a", Name: "Alpha"},
		{OrgID: "org-b", Name: "Beta"},
	})
	defer cleanup()

	_, err := resolveOrgForNonInteractiveLogin(client, "user-1", "")
	if err == nil {
		t.Fatal("resolveOrgForNonInteractiveLogin returned nil error for multiple orgs")
	}
	for _, want := range []string{"multiple organizations", "--org-id", "org-a (Alpha)", "org-b (Beta)"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err.Error(), want)
		}
	}
}

func TestResolveOrgForNonInteractiveLoginValidStoredOrgRemains(t *testing.T) {
	client, cleanup := newOrgClient(t, []api.Org{
		{OrgID: "stored-org", Name: "Stored"},
		{OrgID: "other-org", Name: "Other"},
	})
	defer cleanup()

	orgID, err := resolveOrgForNonInteractiveLogin(client, "user-1", "stored-org")
	if err != nil {
		t.Fatalf("resolveOrgForNonInteractiveLogin returned error: %v", err)
	}
	if orgID != "stored-org" {
		t.Fatalf("orgID = %q, want stored-org", orgID)
	}
}

func TestResolveOrgForNonInteractiveLoginStaleStoredOrgWithOneAvailableAutoSelects(t *testing.T) {
	client, cleanup := newOrgClient(t, []api.Org{{OrgID: "available-org", Name: "Available"}})
	defer cleanup()

	orgID, err := resolveOrgForNonInteractiveLogin(client, "user-1", "stale-org")
	if err != nil {
		t.Fatalf("resolveOrgForNonInteractiveLogin returned error: %v", err)
	}
	if orgID != "available-org" {
		t.Fatalf("orgID = %q, want available-org", orgID)
	}
}

func TestResolveOrgForNonInteractiveLoginStaleStoredOrgWithMultipleOrgsErrors(t *testing.T) {
	client, cleanup := newOrgClient(t, []api.Org{
		{OrgID: "org-a", Name: "Alpha"},
		{OrgID: "org-b", Name: "Beta"},
	})
	defer cleanup()

	_, err := resolveOrgForNonInteractiveLogin(client, "user-1", "stale-org")
	if err == nil {
		t.Fatal("resolveOrgForNonInteractiveLogin returned nil error for stale stored org with multiple orgs")
	}
	for _, want := range []string{"multiple organizations", "--org-id", "org-a (Alpha)", "org-b (Beta)"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err.Error(), want)
		}
	}
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type loginMainTestEnv struct {
	cmd          *cobra.Command
	accountStore *config.AccountStore
	serverURL    string
}

var loginWithWebFnTestMu sync.Mutex

func newLoginMainTestEnv(t *testing.T, orgs []api.Org) loginMainTestEnv {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/user":
			writeAPIResponse(t, w, api.User{Id: "user-1", UserID: "user-1", Email: "user@example.test"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/user/user-1/orgs":
			writeAPIResponse(t, w, api.ListUserOrgsResponse{Orgs: orgs})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/user/user-1/olm":
			writeAPIResponse(t, w, api.CreateOlmResponse{ID: "olm-record", OlmID: "olm-id", Secret: "secret", Name: "test-device"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/server-info":
			writeAPIResponse(t, w, api.ServerInfo{Version: "test", Build: "oss"})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("SUDO_USER", "")
	configDir := filepath.Join(tmpDir, ".config", "pangolin")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "accounts.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("failed to create accounts file: %v", err)
	}

	accountStore, err := config.LoadAccountStore()
	if err != nil {
		t.Fatalf("LoadAccountStore returned error: %v", err)
	}
	if accountStore.Accounts == nil {
		accountStore.Accounts = map[string]config.Account{}
	}

	apiClient, err := api.NewClient(api.ClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	ctx := api.WithAPIClient(context.Background(), apiClient)
	ctx = config.WithAccountStore(ctx, accountStore)
	cmd := &cobra.Command{Use: "login-test"}
	cmd.SetContext(ctx)

	return loginMainTestEnv{
		cmd:          cmd,
		accountStore: accountStore,
		serverURL:    server.URL,
	}
}

func withFakeLoginWithWeb(t *testing.T, fake func(string, webLoginOpts) (string, error)) {
	t.Helper()

	loginWithWebFnTestMu.Lock()
	original := loginWithWebFn
	loginWithWebFn = fake
	t.Cleanup(func() {
		loginWithWebFn = original
		loginWithWebFnTestMu.Unlock()
	})
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	outCh := make(chan struct {
		stdout string
		err    error
	}, 1)
	go func() {
		out, err := io.ReadAll(reader)
		outCh <- struct {
			stdout string
			err    error
		}{stdout: string(out), err: err}
	}()

	os.Stdout = writer
	restored := false
	defer func() {
		if !restored {
			os.Stdout = original
			_ = writer.Close()
			_ = reader.Close()
		}
	}()

	runErr := fn()
	os.Stdout = original
	restored = true

	if err := writer.Close(); err != nil && runErr == nil {
		runErr = err
	}
	result := <-outCh
	if result.err != nil {
		t.Fatalf("failed to read stdout: %v", result.err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("failed to close stdout reader: %v", err)
	}

	return result.stdout, runErr
}

func decodeSingleDevicePrompt(t *testing.T, stdout string) deviceLoginPrompt {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(stdout))
	var prompt deviceLoginPrompt
	if err := decoder.Decode(&prompt); err != nil {
		t.Fatalf("JSON prompt is invalid: %v\nstdout: %q", err, stdout)
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout contained extra JSON or text after prompt: %v\nstdout: %q", err, stdout)
	}

	return prompt
}

func writeAPIResponse(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"error":   false,
		"data":    data,
	}); err != nil {
		t.Errorf("failed to write API response: %v", err)
	}
}

func newOrgClient(t *testing.T, orgs []api.Org) (*api.Client, func()) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/user/user-1/orgs" {
			t.Errorf("path = %s, want /user/user-1/orgs", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success":true,"error":false,"data":{"orgs":%s}}`, mustMarshalOrgs(t, orgs))
	}))

	client, err := api.NewClient(api.ClientConfig{BaseURL: server.URL})
	if err != nil {
		server.Close()
		t.Fatalf("NewClient returned error: %v", err)
	}

	return client, server.Close
}

func mustMarshalOrgs(t *testing.T, orgs []api.Org) string {
	t.Helper()

	b, err := json.Marshal(orgs)
	if err != nil {
		t.Fatalf("failed to marshal orgs: %v", err)
	}

	return string(b)
}
