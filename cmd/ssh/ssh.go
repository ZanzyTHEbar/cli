package ssh

import (
	"errors"
	"os"

	"github.com/fosrl/cli/internal/logger"
	"github.com/spf13/cobra"
)

var (
	errHostnameRequired = errors.New("--hostname is required")
	errIdentityRequired = errors.New("identity file (-i) is required for the built-in SSH client")
)

func SSHCmd() *cobra.Command {
	opts := struct {
		User     string
		Hostname string
		Identity string
		Exec     bool
	}{}

	cmd := &cobra.Command{
		Use:   "ssh",
		Short: "Run an interactive SSH session",
		Long:  `Run an SSH client in the terminal. By default uses the built-in Go SSH client (no system ssh required). Use --exec to run the system ssh binary instead.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Hostname == "" {
				return errHostnameRequired
			}
			if !opts.Exec && opts.Identity == "" {
				return errIdentityRequired
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			runOpts := RunOpts{
				User:        opts.User,
				Hostname:    opts.Hostname,
				Identity:    opts.Identity,
				PassThrough: args,
			}
			var exitCode int
			var err error
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
	cmd.Flags().StringVarP(&opts.Identity, "identity", "i", "", "Path to private key file (required for built-in client)")
	cmd.Flags().BoolVar(&opts.Exec, "exec", false, "Use system ssh binary instead of the built-in client")

	// Allow arbitrary args after flags (e.g. after --) to pass through to ssh
	cmd.Args = cobra.ArbitraryArgs

	return cmd
}
