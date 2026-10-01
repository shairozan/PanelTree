package cli

import (
	"encoding/json"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/internal/config"
	"github.com/shairozan/PanelTree/render"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func referenceCommand(root *cobra.Command) *cobra.Command {
	var r app.ReferenceRequest
	var g render.Generation
	var cfg *config.Config
	v := viper.New()
	var service *app.Service
	cmd := &cobra.Command{Use: "character-reference [project.yaml]", Short: "Create, inspect, import, request, collect, accept, reject or publish character references", Args: cobra.ExactArgs(1)}
	f := cmd.Flags()
	f.StringVar(&r.Action, "action", "inspect", "reference action")
	f.StringVar(&r.Set, "set", "", "reference set ID")
	f.StringVar(&r.Revision, "revision", "", "expected reference revision")
	f.StringVar(&r.Slot, "slot", "", "body/front, head/front, body/left, head/left, body/right, head/right, body/rear or head/rear")
	f.StringVar(&r.Candidate, "candidate", "", "candidate ID")
	f.StringVar(&r.Path, "path", "", "project-relative imported PNG")
	f.StringVar(&r.License, "license", "", "image/model license")
	f.StringVar(&r.Attribution, "attribution", "", "attribution")
	f.StringVar(&r.Version, "version", "", "immutable publication version")
	f.BoolVar(&r.Reapprove, "reapprove", false, "explicitly approve a stale candidate against current lineage")
	f.StringVar(&r.Key, "key", "", "idempotency key")
	f.StringVar(&r.JobID, "job-id", "", "completed generation job")
	f.IntVar(&r.Width, "width", 512, "generation width")
	f.IntVar(&r.Height, "height", 512, "generation height")
	f.StringVar(&g.Prompt, "prompt", "", "design or correction details")
	f.StringVar(&g.NegativePrompt, "negative-prompt", "", "negative prompt")
	f.Uint64Var(&g.Seed, "seed", 0, "generation seed")
	bindErr := v.BindPFlag("log-level", root.PersistentFlags().Lookup("log-level"))
	initialize := config.NewInitializer(&cfg, v, config.InitializerOptions{ConfigFlagName: "config"})
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if bindErr != nil {
			return bindErr
		}
		if e := initialize(cmd, args); e != nil {
			return e
		}
		var e error
		service, e = app.NewRuntimeService(cfg.ComfyURL, cfg.ComfyProfile)
		return e
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		r.ProjectFile = args[0]
		if r.Action == "request" {
			r.Generation = &g
		}
		out, e := service.Reference(cmd.Context(), r)
		if e != nil {
			return e
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	return cmd
}
