package login

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/fosrl/cli/internal/api"
	"github.com/fosrl/cli/internal/config"
	"github.com/fosrl/cli/internal/logger"
	"github.com/fosrl/cli/internal/utils"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

type webLoginOpts struct {
	Plain     bool
	NoBrowser bool
	JSON      bool
}

type deviceLoginPrompt struct {
	DeviceCode             string `json:"device_code"`
	DeviceLoginURL         string `json:"device_login_url"`
	Mode                   string `json:"mode,omitempty"`
	BrowserLaunchAttempted bool   `json:"browser_launch_attempted"`
}

func shouldOpenBrowserImmediately(opts webLoginOpts) bool {
	return !opts.NoBrowser && (opts.Plain || opts.JSON)
}

func shouldPromptForBrowser(opts webLoginOpts) bool {
	return !opts.NoBrowser && !opts.Plain && !opts.JSON
}

func renderDeviceLoginPrompt(w io.Writer, code, baseLoginURL string, opts webLoginOpts, browserLaunchAttempted bool) error {
	if opts.JSON {
		prompt := deviceLoginPrompt{
			DeviceCode:             code,
			DeviceLoginURL:         baseLoginURL,
			Mode:                   "json",
			BrowserLaunchAttempted: browserLaunchAttempted,
		}

		if err := json.NewEncoder(w).Encode(prompt); err != nil {
			return fmt.Errorf("failed to write device login prompt: %w", err)
		}

		return nil
	}

	if opts.Plain {
		if _, err := fmt.Fprintf(w, "Device code: %s\n", code); err != nil {
			return fmt.Errorf("failed to write device login prompt: %w", err)
		}
		if _, err := fmt.Fprintf(w, "Open: %s\n", baseLoginURL); err != nil {
			return fmt.Errorf("failed to write device login prompt: %w", err)
		}
		if _, err := fmt.Fprintln(w, "Waiting for approval..."); err != nil {
			return fmt.Errorf("failed to write device login prompt: %w", err)
		}
	}

	return nil
}

func buildDeviceLoginURL(baseLoginURL, code string) string {
	return fmt.Sprintf("%s?code=%s", baseLoginURL, url.QueryEscape(code))
}

var loginWithWebFn = loginWithWeb

type HostingOption string

const (
	HostingOptionCloud      HostingOption = "cloud"
	HostingOptionSelfHosted HostingOption = "self-hosted"
)

// getDeviceName returns a human-readable device name
func getDeviceName() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "Unknown Device"
	}
	return hostname
}

func loginWithWeb(hostname string, opts webLoginOpts) (string, error) {
	// Build base URL for login (use hostname as-is, StartDeviceWebAuth will add /api/v1)
	baseURL := hostname

	// Create a temporary API client for login (without auth)
	loginClient, err := api.NewClient(api.ClientConfig{
		BaseURL:           baseURL,
		AgentName:         "pangolin-cli",
		SessionCookieName: "p_session_token",
		CSRFToken:         "x-csrf-protection",
	})
	if err != nil {
		return "", fmt.Errorf("failed to create API client: %w", err)
	}

	// Get device name
	deviceName := getDeviceName()

	// Request device code
	startReq := api.DeviceWebAuthStartRequest{
		ApplicationName: "Pangolin CLI",
		DeviceName:      deviceName,
	}

	startResp, err := api.StartDeviceWebAuth(loginClient, startReq)
	if err != nil {
		return "", fmt.Errorf("failed to start device web auth: %w", err)
	}

	code := startResp.Code
	// Calculate expiry time from relative seconds
	expiresAt := time.Now().Add(time.Duration(startResp.ExpiresInSeconds) * time.Second)

	// Build the base login URL (without query parameter) for display
	baseLoginURL := fmt.Sprintf("%s/auth/login/device", strings.TrimSuffix(hostname, "/"))
	// Build the login URL with code as query parameter for browser
	loginURL := buildDeviceLoginURL(baseLoginURL, code)
	browserLaunchAttempted := shouldOpenBrowserImmediately(opts)

	if opts.JSON || opts.Plain {
		if err := renderDeviceLoginPrompt(os.Stdout, code, baseLoginURL, opts, browserLaunchAttempted); err != nil {
			return "", err
		}
	} else {
		// Display code and instructions (similar to GH CLI format)
		logger.Info("First copy your one-time code: %s", code)
		if opts.NoBrowser {
			logger.Info("Open %s in your browser to continue.", baseLoginURL)
		} else {
			logger.Info("Press Enter to open %s in your browser...", baseLoginURL)
		}
	}

	if shouldOpenBrowserImmediately(opts) {
		if err := browser.OpenURL(loginURL); err != nil {
			if opts.JSON {
				fmt.Fprintf(os.Stderr, "Failed to open browser automatically\nPlease manually visit: %s\n", baseLoginURL)
			} else {
				logger.Warning("Failed to open browser automatically")
				logger.Info("Please manually visit: %s", baseLoginURL)
			}
		}
	} else if shouldPromptForBrowser(opts) {
		// Wait for Enter in a goroutine (non-blocking) and open browser when pressed
		go func() {
			reader := bufio.NewReader(os.Stdin)
			_, err := reader.ReadString('\n')
			if err == nil {
				// User pressed Enter, open browser
				if err := browser.OpenURL(loginURL); err != nil {
					// Don't fail if browser can't be opened, just warn
					logger.Warning("Failed to open browser automatically")
					logger.Info("Please manually visit: %s", baseLoginURL)
				}
			}
		}()
	}

	// Poll for verification (starts immediately, doesn't wait for Enter)
	pollInterval := 1 * time.Second
	startTime := time.Now()
	maxPollDuration := 5 * time.Minute

	var token string

	for {
		// print
		if !opts.JSON {
			logger.Debug("Polling for device web auth verification...")
		}
		// Check if code has expired
		if time.Now().After(expiresAt) {
			logger.Error("Device web auth code has expired")
			return "", fmt.Errorf("code expired. Please try again")
		}

		// Check if we've exceeded max polling duration
		if time.Since(startTime) > maxPollDuration {
			logger.Error("Polling timed out after %v", maxPollDuration)
			return "", fmt.Errorf("polling timeout. Please try again")
		}

		// Poll for verification status
		pollResp, message, err := api.PollDeviceWebAuth(loginClient, code)
		// print debug info
		if !opts.JSON {
			logger.Debug("Polling response: %+v, message: %s, err: %v", pollResp, message, err)
		}
		if err != nil {
			logger.Error("Error polling device web auth: %v", err)
			return "", fmt.Errorf("failed to poll device web auth: %w", err)
		}

		// Check verification status
		if pollResp.Verified {
			token = pollResp.Token
			if token == "" {
				logger.Error("Verification succeeded but no token received")
				return "", fmt.Errorf("verification succeeded but no token received")
			}
			return token, nil
		}

		// Check for expired or not found messages
		if message == "Code expired" || message == "Code not found" {
			logger.Error("Device web auth code has expired or not found")
			return "", fmt.Errorf("code expired or not found. Please try again")
		}

		// Wait before next poll
		time.Sleep(pollInterval)
	}
}

type LoginCmdOpts struct {
	Hostname  string
	Plain     bool
	NoBrowser bool
	JSON      bool
	OrgID     string
}

func LoginCmd() *cobra.Command {
	opts := LoginCmdOpts{}

	cmd := &cobra.Command{
		Use:   "login [hostname]",
		Short: "Login to Pangolin",
		Long:  "Interactive login to select your hosting option and configure access. Use --plain or --json with --no-browser for headless and container environments.",
		Example: `  # Container-friendly login without browser or TUI
  pangolin login https://vpn.example.com --plain --no-browser

  # Same flow through the auth subcommand
  pangolin auth login https://vpn.example.com --plain --no-browser

  # Emit the initial device prompt as JSON
  pangolin login https://vpn.example.com --json --no-browser --org-id <org-id>

  # Use PANGOLIN_ENDPOINT in plain/JSON modes
  PANGOLIN_ENDPOINT=https://vpn.example.com pangolin login --plain --no-browser`,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
				return err
			}

			if len(args) > 0 {
				opts.Hostname = args[0]
			}

			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			if err := loginMain(cmd, &opts); err != nil {
				os.Exit(1)
			}
		},
	}

	cmd.Flags().BoolVar(&opts.Plain, "plain", false, "Use line-oriented non-TUI login instructions")
	cmd.Flags().BoolVar(&opts.NoBrowser, "no-browser", false, "Do not launch a browser or prompt to open one")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Print the initial device login prompt as JSON")
	cmd.Flags().StringVar(&opts.OrgID, "org-id", "", "Select an organization by ID without prompting")

	return cmd
}

func resolveOrgForLogin(client *api.Client, userID, orgID string) (string, error) {
	orgsResp, err := client.ListUserOrgs(userID)
	if err != nil {
		return "", fmt.Errorf("failed to list organizations: %w", err)
	}

	for _, org := range orgsResp.Orgs {
		if org.OrgID == orgID {
			return orgID, nil
		}
	}

	if len(orgsResp.Orgs) == 0 {
		return "", fmt.Errorf("organization %q not found; authenticated user has no organizations", orgID)
	}

	return "", fmt.Errorf("organization %q not found for authenticated user; available organizations: %s", orgID, formatOrgChoices(orgsResp.Orgs))
}

func resolveOrgForNonInteractiveLogin(client *api.Client, userID, currentOrgID string) (string, error) {
	orgsResp, err := client.ListUserOrgs(userID)
	if err != nil {
		return "", fmt.Errorf("failed to list organizations: %w", err)
	}

	if currentOrgID != "" {
		for _, org := range orgsResp.Orgs {
			if org.OrgID == currentOrgID {
				return currentOrgID, nil
			}
		}
	}

	switch len(orgsResp.Orgs) {
	case 0:
		return "", fmt.Errorf("no organizations found for authenticated user")
	case 1:
		return orgsResp.Orgs[0].OrgID, nil
	default:
		return "", fmt.Errorf("multiple organizations found; rerun with --org-id <id>. Available organizations: %s", formatOrgChoices(orgsResp.Orgs))
	}
}

func formatOrgChoices(orgs []api.Org) string {
	choices := make([]string, 0, len(orgs))
	for _, org := range orgs {
		if org.Name == "" {
			choices = append(choices, org.OrgID)
			continue
		}

		choices = append(choices, fmt.Sprintf("%s (%s)", org.OrgID, org.Name))
	}

	return strings.Join(choices, ", ")
}

func loginMain(cmd *cobra.Command, opts *LoginCmdOpts) error {
	apiClient := api.FromContext(cmd.Context())
	accountStore := config.AccountStoreFromContext(cmd.Context())

	hostname := opts.Hostname
	explicitPlain := opts.Plain || opts.JSON

	// If hostname was provided, skip hosting option selection
	if hostname == "" {
		if explicitPlain {
			hostname = os.Getenv("PANGOLIN_ENDPOINT")
			if hostname == "" {
				hostname = "app.pangolin.net"
			}
		} else {
			var hostingOption HostingOption

			// First question: select hosting option
			form := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[HostingOption]().
						Title("Select your hosting option").
						Options(
							huh.NewOption("Pangolin Cloud (app.pangolin.net)", HostingOptionCloud),
							huh.NewOption("Self-hosted or Dedicated instance", HostingOptionSelfHosted),
						).
						Value(&hostingOption),
				),
			)

			if err := form.Run(); err != nil {
				logger.Error("Error: %v", err)
				return err
			}

			// If self-hosted, prompt for hostname
			if hostingOption == HostingOptionSelfHosted {
				hostnameForm := huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Enter hostname URL").
							Placeholder("https://your-instance.example.com").
							Value(&hostname),
					),
				)

				if err := hostnameForm.Run(); err != nil {
					logger.Error("Error: %v", err)
					return err
				}
			} else {
				// For cloud, set the default hostname
				hostname = "app.pangolin.net"
			}
		}
	}

	// Normalize hostname (preserve protocol, remove trailing slash)
	hostname = strings.TrimSuffix(hostname, "/")

	// If no protocol specified, default to https
	if !strings.HasPrefix(hostname, "http://") && !strings.HasPrefix(hostname, "https://") {
		hostname = "https://" + hostname
	}

	// Perform web login
	sessionToken, err := loginWithWebFn(hostname, webLoginOpts{
		Plain:     opts.Plain,
		NoBrowser: opts.NoBrowser,
		JSON:      opts.JSON,
	})
	if err != nil {
		logger.Error("%v", err)
		return err
	}

	if sessionToken == "" {
		err := errors.New("login appeared successful but no session token was received")
		logger.Error("Error: %v", err)
		return err
	}

	// Update the global API client (always initialized)
	// Update base URL and token (hostname already includes protocol)
	apiBaseURL := hostname + "/api/v1"
	apiClient.SetBaseURL(apiBaseURL)
	apiClient.SetToken(sessionToken)

	if !opts.JSON {
		logger.Success("Device authorized")
		fmt.Println()
	}

	// Get user information
	var user *api.User
	user, err = apiClient.GetUser()
	if err != nil {
		logger.Error("Failed to get user information: %v", err)
		return err
	}

	var newAccount config.Account

	// Re-use the current account entry in case it exists
	// This preserves OLM credentials across logout/login cycles
	if account, exists := accountStore.Accounts[user.UserID]; exists {
		newAccount = account
	}

	userID := user.UserID

	newAccount.UserID = userID
	newAccount.Email = user.Email
	newAccount.Host = hostname
	newAccount.SessionToken = sessionToken

	// Update account with username and name from user data
	if user.Username != nil {
		newAccount.Username = user.Username
	}
	if user.Name != nil {
		newAccount.Name = user.Name
	}

	// Ensure new user has an organization selected.
	orgIDFlag := strings.TrimSpace(opts.OrgID)
	if orgIDFlag != "" {
		orgID, err := resolveOrgForLogin(apiClient, userID, orgIDFlag)
		if err != nil {
			logger.Error("Failed to select organization: %v", err)
			return err
		}

		newAccount.OrgID = orgID
	} else if explicitPlain {
		orgID, err := resolveOrgForNonInteractiveLogin(apiClient, userID, newAccount.OrgID)
		if err != nil {
			logger.Error("Failed to select organization: %v", err)
			return err
		}

		newAccount.OrgID = orgID
	} else if newAccount.OrgID == "" {
		orgID, err := utils.SelectOrgForm(apiClient, userID)
		if err != nil {
			logger.Error("Failed to select organization: %v", err)
			return err
		}

		newAccount.OrgID = orgID
	}

	// Ensure OLM credentials exist
	if newAccount.OlmCredentials == nil {
		newOlmCreds, err := apiClient.CreateOlm(userID, getDeviceName())
		if err != nil {
			logger.Error("Failed to obtain olm credentials: %v", err)
			return err
		}

		newAccount.OlmCredentials = &config.OlmCredentials{
			ID:     newOlmCreds.OlmID,
			Secret: newOlmCreds.Secret,
		}
	} else {
		// logger.Info("Olm credentials already exist for this account, skipping generation")
	}

	accountStore.Accounts[user.UserID] = newAccount
	accountStore.ActiveUserID = userID

	err = accountStore.Save()
	if err != nil {
		logger.Error("Failed to save account store: %s", err)
		if !opts.JSON {
			logger.Warning("You may not be able to login properly until this is saved.")
		}
		return err
	}

	// Fetch server info after successful authentication
	apiServerInfo, err := apiClient.GetServerInfo()
	if err != nil {
		// Log warning but don't fail login if server info fetch fails
		if !opts.JSON {
			logger.Debug("Failed to fetch server info: %v", err)
		}
	} else if apiServerInfo != nil {
		// Convert api.ServerInfo to config.ServerInfo
		serverInfo := &config.ServerInfo{
			Version:                apiServerInfo.Version,
			SupporterStatusValid:   apiServerInfo.SupporterStatusValid,
			Build:                  apiServerInfo.Build,
			EnterpriseLicenseValid: apiServerInfo.EnterpriseLicenseValid,
			EnterpriseLicenseType:  apiServerInfo.EnterpriseLicenseType,
		}
		// Update account with server info
		account := accountStore.Accounts[user.UserID]
		account.ServerInfo = serverInfo
		accountStore.Accounts[user.UserID] = account
		if err := accountStore.Save(); err != nil {
			if !opts.JSON {
				logger.Debug("Failed to save server info: %v", err)
			}
		}
	}

	// Print logged in message after all setup is complete
	if user != nil {
		displayName := utils.UserDisplayName(user)
		if displayName != "" && !opts.JSON {
			logger.Success("Logged in as %s", displayName)
		}
	}

	return nil
}
