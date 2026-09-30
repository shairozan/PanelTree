package cli

import (
	"encoding/json"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/internal/config"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func jobCommands(root *cobra.Command) []*cobra.Command {
	asset := &cobra.Command{Use: "asset", Short: "Request and select revision-safe draft assets"}
	for _, verb := range []string{"request", "select"} {
		asset.AddCommand(jobCommand(verb, root))
	}
	jobs := &cobra.Command{Use: "jobs", Short: "Inspect, cancel and execute durable local jobs"}
	for _, verb := range []string{"list", "status", "cancel", "run"} {
		jobs.AddCommand(jobCommand(verb, root))
	}
	return []*cobra.Command{asset, jobs, jobCommand("renderers", root)}
}
func jobCommand(verb string, root *cobra.Command) *cobra.Command {
	var cfg *config.Config
	v := viper.New()
	service := app.NewService()
	var request app.AssetRequest
	var generation render.Generation
	var id, revision, page, panel, layer string
	workers := 2
	cmd := &cobra.Command{Use: verb + " [project.yaml]", Short: map[string]string{"request": "Queue a frozen leaf rendering request", "select": "Select a successful candidate at its original revision", "list": "List durable jobs", "status": "Read job progress and diagnostics", "cancel": "Cancel a queued or running job", "run": "Drain queued jobs with bounded workers", "renderers": "Discover available renderer capabilities"}[verb], Args: cobra.ExactArgs(1)}
	if verb == "renderers" {
		cmd.Use = "renderers"
		cmd.Args = cobra.NoArgs
	}
	if verb == "request" || verb == "select" {
		cmd.Flags().StringVar(&revision, "revision", "", "expected project revision")
	}
	if verb == "request" {
		cmd.Flags().StringVar(&generation.Prompt, "prompt", "", "semantic prompt for a generated draft")
		cmd.Flags().StringVar(&generation.NegativePrompt, "negative-prompt", "", "negative prompt (profile must support it)")
		cmd.Flags().Uint64Var(&generation.Seed, "seed", 0, "explicit generation seed")
		cmd.Flags().StringVar(&generation.Output, "output-kind", "rgb", "generation capability (rgb only)")
		cmd.Flags().StringVar(&request.IdempotencyKey, "key", "", "idempotency key (required)")
		cmd.Flags().StringVar(&request.Renderer, "renderer", "builtin", "renderer name")
		cmd.Flags().StringVar(&page, "page", "", "page ID")
		cmd.Flags().StringVar(&panel, "panel", "", "panel ID")
		cmd.Flags().StringVar(&layer, "layer", "", "leaf layer ID")
		cmd.Flags().IntVar(&request.Width, "width", 0, "page output width (requires height)")
		cmd.Flags().IntVar(&request.Height, "height", 0, "page output height (requires width)")
		cmd.Flags().StringVar(&request.Fit, "fit", "error", "page output aspect policy")
	}
	if verb == "status" || verb == "cancel" || verb == "select" {
		cmd.Flags().StringVar(&id, "id", "", "job ID")
	}
	if verb == "run" {
		cmd.Flags().IntVar(&workers, "workers", 2, "concurrent workers (1 through 8)")
	}
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
		var result any
		var e error
		ctx := cmd.Context()
		switch verb {
		case "renderers":
			result = service.Renderers()
		case "request":
			if request.Renderer == "comfyui" || cmd.Flags().Changed("prompt") || cmd.Flags().Changed("negative-prompt") || cmd.Flags().Changed("seed") || cmd.Flags().Changed("output-kind") {
				request.Generation = &generation
			}
			request.ProjectFile = args[0]
			request.ExpectedRevision = model.Revision(revision)
			request.Target = app.LayerTarget{Page: model.ID(page), Panel: model.ID(panel), Layer: model.ID(layer)}
			result, e = service.RequestAsset(ctx, request)
		case "select":
			result, e = service.SelectCandidate(ctx, app.SelectCandidateRequest{ProjectFile: args[0], JobID: id, ExpectedRevision: model.Revision(revision)})
		case "list":
			result, e = service.Jobs(ctx, args[0])
		case "status":
			result, e = service.Job(ctx, app.JobRequest{ProjectFile: args[0], ID: id})
		case "cancel":
			result, e = service.CancelJob(ctx, app.JobRequest{ProjectFile: args[0], ID: id})
		case "run":
			result, e = service.RunJobs(ctx, app.RunJobsRequest{ProjectFile: args[0], Workers: workers})
		}
		if e != nil {
			return e
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	return cmd
}
