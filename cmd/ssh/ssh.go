package ssh

import (
	"errors"
	"os"

	"github.com/fosrl/cli/internal/api"
	"github.com/fosrl/cli/internal/config"
	"github.com/fosrl/cli/internal/logger"
	"github.com/spf13/cobra"
)

var (
	errHostnameRequired   = errors.New("--hostname is required")
	errResourceIDRequired = errors.New("--resource-id is required to sign the SSH key")
	errOrgRequired        = errors.New("--org is required, or select an organization (pangolin select org)")
)

func SSHCmd() *cobra.Command {
	opts := struct {
		User       string
		Hostname   string
		OrgID      string
		ResourceID int
		Exec       bool
	}{}

	cmd := &cobra.Command{
		Use:   "ssh",
		Short: "Run an interactive SSH session",
		Long:  `Run an SSH client in the terminal. Generates a key pair and signs it just-in-time, then connects to the target resource.`,
		PreRunE: func(c *cobra.Command, args []string) error {
			if opts.Hostname == "" {
				return errHostnameRequired
			}
			if opts.ResourceID == 0 {
				return errResourceIDRequired
			}
			return nil
		},
		Run: func(c *cobra.Command, args []string) {
			apiClient := api.FromContext(c.Context())
			accountStore := config.AccountStoreFromContext(c.Context())

			orgID, err := ResolveOrgID(accountStore, opts.OrgID)
			if err != nil {
				logger.Error("%v", err)
				os.Exit(1)
			}

			privPEM, _, cert, _, err := GenerateAndSignKey(apiClient, orgID, opts.ResourceID)
			if err != nil {
				logger.Error("%v", err)
				os.Exit(1)
			}

			runOpts := RunOpts{
				User:          opts.User,
				Hostname:      opts.Hostname,
				PrivateKeyPEM: privPEM,
				Certificate:   cert,
				PassThrough:   args,
			}

			var exitCode int
			if opts.Exec {
				exitCode, err = RunExec(runOpts)
			} else {
				exitCode, err = RunNative(runOpts)
			}
			if err != nil {
				logger.Error("%v", err)
				os.Exit(1)
			}
			os.Exit(exitCode)
		},
	}

	cmd.Flags().StringVarP(&opts.User, "user", "u", "", "SSH login user (maps to ssh -l)")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "Target host (required)")
	cmd.Flags().StringVar(&opts.OrgID, "org", "", "Organization ID (default: selected organization)")
	cmd.Flags().IntVar(&opts.ResourceID, "resource-id", 0, "Resource ID for key signing (required)")
	cmd.Flags().BoolVar(&opts.Exec, "exec", false, "Use system ssh binary instead of the built-in client")

	cmd.Args = cobra.ArbitraryArgs

	cmd.AddCommand(SignCmd())

	return cmd
}
