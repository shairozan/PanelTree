package cli

import (
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/internal/config"
	"github.com/shairozan/PanelTree/model"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io"
	"os"
)

func editCommand(verb string, root *cobra.Command) *cobra.Command {
	var cfg *config.Config
	v := viper.New()
	var revision, page, panel, layer, scope, artifact, changes string
	cmd := &cobra.Command{Use: verb + " [project.yaml]", Short: "Apply revision-aware editorial changes", Args: cobra.ExactArgs(1)}
	if verb == "edit" {
		cmd.Flags().StringVar(&changes, "changes", "", "JSON changeset file (required)")
	} else {
		cmd.Flags().StringVar(&revision, "revision", "", "expected revision from inspect (required)")
		cmd.Flags().StringVar(&page, "page", "", "page ID")
		cmd.Flags().StringVar(&panel, "panel", "", "panel ID")
		cmd.Flags().StringVar(&layer, "layer", "", "layer ID")
		if verb == "lock" {
			cmd.Flags().StringVar(&scope, "scope", "all", "asset, placement, or all")
		}
		if verb == "approve" || verb == "override" {
			cmd.Flags().StringVar(&artifact, "artifact", "", "existing PNG, relative to project root or absolute")
		}
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
		r := app.EditRequest{ProjectFile: args[0], ExpectedRevision: model.Revision(revision)}
		if verb == "edit" {
			if changes == "" {
				return fmt.Errorf("--changes is required")
			}
			f, e := os.Open(changes)
			if e != nil {
				return e
			}
			defer func() { _ = f.Close() }()
			dec := json.NewDecoder(io.LimitReader(f, 8<<20))
			dec.DisallowUnknownFields()
			if e = dec.Decode(&r); e != nil {
				return e
			}
			var extra any
			if e = dec.Decode(&extra); e != io.EOF {
				return fmt.Errorf("expected exactly one JSON changeset")
			}
			r.ProjectFile = args[0]
		} else {
			if revision == "" || page == "" || panel == "" || layer == "" {
				return fmt.Errorf("--revision, --page, --panel and --layer are required")
			}
			r.Operations = []app.Operation{{Target: app.LayerTarget{Page: model.ID(page), Panel: model.ID(panel), Layer: model.ID(layer)}, Action: verb, Scope: model.LockScope(scope), Artifact: artifact}}
		}
		result, e := app.NewService().Edit(cmd.Context(), r)
		if e != nil {
			return e
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	return cmd
}
