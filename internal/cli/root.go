// Package cli constructs PanelTree's command tree without registration side effects.
package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shairozan/PanelTree/internal/config"
)

// Command returns a fresh root command and its invocation-local configuration.
func Command() *cobra.Command {
	var cfg *config.Config
	v := viper.New()
	cmd := &cobra.Command{
		Use:           "paneltree",
		Short:         "Compose sequential art from declarative projects",
		Long:          "PanelTree is a composition-first toolchain for books, pages, panels, and independently editable layers.",
		Version:       "dev",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	bindErr := attributes(cmd, v)
	initialize := config.NewInitializer(&cfg, v, config.InitializerOptions{ConfigFlagName: "config"})
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if bindErr != nil {
			return bindErr
		}
		return initialize(cmd, args)
	}
	return cmd
}

func attributes(cmd *cobra.Command, v *viper.Viper) error {
	cmd.PersistentFlags().String("config", "", "runtime YAML configuration file")
	cmd.PersistentFlags().String("log-level", "info", "log level: debug, info, warn, or error")
	return v.BindPFlag("log-level", cmd.PersistentFlags().Lookup("log-level"))
}
