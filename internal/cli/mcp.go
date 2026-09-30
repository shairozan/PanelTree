package cli

import (
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shairozan/PanelTree/internal/config"
	server "github.com/shairozan/PanelTree/internal/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io"
)

type writerCloser struct{ io.Writer }

func (writerCloser) Close() error { return nil }

func mcpCommand(root *cobra.Command) *cobra.Command {
	parent := &cobra.Command{Use: "mcp", Short: "Local Model Context Protocol interface"}
	var cfg *config.Config
	v := viper.New()
	cmd := &cobra.Command{Use: "serve", Short: "Serve MCP over stdin/stdout within explicit roots", Args: cobra.NoArgs}
	cmd.Flags().StringSlice("root", nil, "allowed root directory (repeatable; overrides mcp-roots runtime configuration)")
	bindRoot := v.BindPFlag("mcp-roots", cmd.Flags().Lookup("root"))
	bindLog := v.BindPFlag("log-level", root.PersistentFlags().Lookup("log-level"))
	initialize := config.NewInitializer(&cfg, v, config.InitializerOptions{ConfigFlagName: "config"})
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if bindRoot != nil {
			return bindRoot
		}
		if bindLog != nil {
			return bindLog
		}
		return initialize(cmd, args)
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		s, e := server.New(cfg.MCPRoots)
		if e != nil {
			return e
		}
		return s.Run(cmd.Context(), &protocol.IOTransport{Reader: io.NopCloser(cmd.InOrStdin()), Writer: writerCloser{cmd.OutOrStdout()}, MaxLineLength: 8 << 20})
	}
	parent.AddCommand(cmd)
	return parent
}
