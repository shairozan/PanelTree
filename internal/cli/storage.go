package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func configuredService(ctx context.Context, c *config.Config) (*app.Service, error) {
	s, e := app.NewRuntimeService(c.ComfyURL, c.ComfyProfile, c.Generation)
	if e != nil {
		return nil, e
	}
	if e = s.ConfigureStorage(ctx, c.Storage); e != nil {
		return nil, e
	}
	return s, nil
}
func storageCommand(root *cobra.Command) *cobra.Command {
	parent := &cobra.Command{Use: "storage", Short: "Manage alternate PostgreSQL storage"}
	for _, action := range []string{"migrate", "list", "import", "export"} {
		var cfg *config.Config
		v := viper.New()
		var r app.StorageRequest
		r.Action = action
		c := &cobra.Command{Use: action + " [project]", Args: cobra.MaximumNArgs(1)}
		c.Flags().StringVar(&r.ID, "id", "", "new project ID for import")
		c.Flags().StringVar(&r.Path, "output", "", "new portable project directory")
		c.PreRunE = config.NewInitializer(&cfg, v, config.InitializerOptions{ConfigFlagName: "config"})
		c.RunE = func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				r.ProjectFile = args[0]
			}
			if (action == "import" || action == "export") && r.ProjectFile == "" {
				return fmt.Errorf("project argument required")
			}
			s, e := configuredService(cmd.Context(), cfg)
			if e != nil {
				return e
			}
			out, e := s.Storage(cmd.Context(), r)
			if e != nil {
				return e
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		}
		parent.AddCommand(c)
	}
	return parent
}
