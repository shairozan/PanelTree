package cli

import (
	"encoding/json"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// projectCommand creates one leaf with its own configuration and shared services.
func projectCommand(verb string, root *cobra.Command) *cobra.Command {
	var cfg *config.Config
	v := viper.New()
	service := app.NewService()
	var inspection app.InspectRequest
	var build app.BuildRequest
	use := verb + " [project.yaml]"
	if verb == "init" {
		use = "init [new-directory]"
	}
	cmd := &cobra.Command{Use: use, Short: map[string]string{"init": "Create a new example book", "validate": "Validate a book, chapter or page", "inspect": "Inspect the validated tree as JSON", "build": "Export one page as a new PNG"}[verb], Args: cobra.ExactArgs(1)}
	if verb == "inspect" || verb == "build" {
		cmd.Flags().IntVar(&inspection.Width, "width", 0, "output pixel width (requires height)")
		cmd.Flags().IntVar(&inspection.Height, "height", 0, "output pixel height (requires width)")
		cmd.Flags().StringVar(&inspection.Fit, "fit", "error", "output aspect policy: error, contain, or cover")
	}
	if verb == "build" {
		cmd.Flags().StringVar(&build.PageID, "page", "", "page ID (required for multi-page input)")
		cmd.Flags().StringVar(&build.Output, "output", "", "new PNG destination; parent directory must exist")
	}
	bindErr := v.BindPFlag("log-level", root.PersistentFlags().Lookup("log-level"))
	initialize := config.NewInitializer(&cfg, v, config.InitializerOptions{ConfigFlagName: "config"})
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if bindErr != nil {
			return bindErr
		}
		return initialize(cmd, args)
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		var result any
		var err error
		switch verb {
		case "init":
			result, err = service.Init(cmd.Context(), app.InitRequest{Directory: args[0]})
		case "validate":
			result, err = service.Validate(cmd.Context(), app.InspectRequest{ProjectFile: args[0]})
		case "inspect":
			inspection.ProjectFile = args[0]
			result, err = service.Inspect(cmd.Context(), inspection)
		case "build":
			build.ProjectFile = args[0]
			build.Width = inspection.Width
			build.Height = inspection.Height
			build.Fit = inspection.Fit
			result, err = service.Build(cmd.Context(), build)
		}
		if err != nil {
			return err
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	return cmd
}
