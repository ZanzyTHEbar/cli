package blueprint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fosrl/cli/internal/api"
	"github.com/fosrl/cli/internal/config"
	"github.com/fosrl/newt/logger"
	"github.com/spf13/cobra"
)

type BlueprintCmdOpts struct {
	Name     string
	Path     string
	APIKey   string
	Endpoint string
	OrgID    string
}

func BlueprintCmd() *cobra.Command {
	opts := BlueprintCmdOpts{}

	cmd := &cobra.Command{
		Use:   "blueprint",
		Short: "Apply a blueprint",
		Long:  "Apply a YAML blueprint to the Pangolin server",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Path == "" {
				return errors.New("--file is required")
			}

			// API key mode requires endpoint and org.
			if opts.APIKey != "" && opts.Endpoint == "" {
				return errors.New("--endpoint is required when using --api-key (use your Integration API URL, e.g. https://<host>/v1)")
			}
			if opts.APIKey != "" && opts.OrgID == "" {
				return errors.New("--org is required when using --api-key")
			}
			if opts.APIKey == "" && opts.OrgID != "" {
				return errors.New("--org is only supported when using --api-key")
			}

			if _, err := os.Stat(opts.Path); err != nil {
				return err
			}

			// Strip file extension and use file basename path as name
			if opts.Name == "" {
				filename := filepath.Base(opts.Path)
				switch ext := strings.ToLower(filepath.Ext(filename)); ext {
				case ".yaml", ".yml":
					opts.Name = strings.TrimSuffix(filename, ext)
				default:
					opts.Name = filename
				}
			}

			if len(opts.Name) < 1 || len(opts.Name) > 255 {
				return errors.New("name must be between 1-255 characters")
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := applyBlueprintMain(cmd, opts); err != nil {
				return err
			}
			logger.Info("Successfully applied blueprint!")
			return nil
		},
	}

	cmd.Flags().StringVarP(&opts.Path, "file", "f", "", "Path to blueprint file (required)")
	cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "Name of blueprint (default: filename, without extension)")
	cmd.Flags().StringVar(&opts.APIKey, "api-key", "", "Integration API key (<id>.<secret>)")
	cmd.Flags().StringVar(&opts.Endpoint, "endpoint", "", "Integration API host URL (required with --api-key, e.g. https://pangolin-api.example.com)")
	cmd.Flags().StringVar(&opts.OrgID, "org", "", "Organization ID (required with --api-key)")
	cmd.MarkFlagRequired("file")

	return cmd
}

func applyBlueprintMain(cmd *cobra.Command, opts BlueprintCmdOpts) error {
	apiClient := api.FromContext(cmd.Context())
	accountStore := config.AccountStoreFromContext(cmd.Context())

	blueprintContents, err := os.ReadFile(opts.Path)
	if err != nil {
		return fmt.Errorf("failed to read blueprint file: %w", err)
	}

	client := apiClient
	orgID := opts.OrgID

	if opts.APIKey != "" {
		client, err = apiClient.WithIntegrationAPIKey(opts.Endpoint, opts.APIKey)
		if err != nil {
			return fmt.Errorf("failed to initialize api key client: %w", err)
		}
	} else {
		account, errAcc := accountStore.ActiveAccount()
		if errAcc != nil {
			return errAcc
		}
		if account.OrgID == "" {
			return errors.New("no organization selected")
		}
		orgID = account.OrgID
	}

	_, err = client.ApplyBlueprint(orgID, opts.Name, string(blueprintContents))
	if err != nil {
		return fmt.Errorf("failed to apply blueprint: %w", err)
	}
	return nil
}
