package cli

import (
	"encoding/json"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/internal/config"
	"github.com/shairozan/PanelTree/model"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func libraryCommand() *cobra.Command {
	var cfg *config.Config
	v := viper.New()
	var r app.LibraryRequest
	var revision, page, panel, layer string
	c := &cobra.Command{Use: "character-library", Short: "Publish and reuse pinned shared character versions", Args: cobra.NoArgs}
	c.Flags().StringVar(&r.Action, "action", "list", "list, publish, use, delete")
	c.Flags().StringVar(&r.ProjectFile, "project", "", "source/target project path or pg:id")
	c.Flags().StringVar(&r.ID, "id", "", "library character ID")
	c.Flags().StringVar(&r.Version, "version", "", "immutable library version")
	c.Flags().StringVar(&r.Package, "package", "", "project-relative character package to publish")
	c.Flags().StringVar(&r.Set, "set", "", "published reference set")
	c.Flags().StringVar(&r.ReferenceVersion, "reference-version", "", "published reference version")
	c.Flags().StringVar(&revision, "revision", "", "expected target project revision")
	c.Flags().StringVar(&page, "page", "", "target page")
	c.Flags().StringVar(&panel, "panel", "", "target panel")
	c.Flags().StringVar(&layer, "layer", "", "target layer")
	c.PreRunE = config.NewInitializer(&cfg, v, config.InitializerOptions{ConfigFlagName: "config"})
	c.RunE = func(cmd *cobra.Command, _ []string) error {
		s, e := configuredService(cmd.Context(), cfg)
		if e != nil {
			return e
		}
		r.ExpectedRevision = model.Revision(revision)
		r.Target = app.LayerTarget{Page: model.ID(page), Panel: model.ID(panel), Layer: model.ID(layer)}
		out, e := s.Library(cmd.Context(), r)
		if e != nil {
			return e
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
	}
	return c
}
