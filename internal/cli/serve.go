package cli

import (
	"context"
	"fmt"
	"github.com/shairozan/PanelTree/internal/config"
	"github.com/shairozan/PanelTree/internal/web"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func serveCommand(root *cobra.Command) *cobra.Command {
	var cfg *config.Config
	v := viper.New()
	var roots []string
	var listen, state string
	cmd := &cobra.Command{Use: "serve", Short: "Open the local browser workspace", Args: cobra.NoArgs}
	cmd.Flags().StringSliceVar(&roots, "root", nil, "allowed project root (repeatable)")
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:8910", "loopback HTTP address")
	cmd.Flags().StringVar(&state, "state-dir", "", "machine-local registry and preview directory")
	initialize := config.NewInitializer(&cfg, v, config.InitializerOptions{ConfigFlagName: "config"})
	cmd.PreRunE = initialize
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		host, _, e := net.SplitHostPort(listen)
		if e != nil {
			return e
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("serve requires a loopback IP address")
		}
		if len(roots) == 0 {
			roots = cfg.WebRoots
		}
		if len(roots) == 0 {
			return fmt.Errorf("at least one project root is required")
		}
		if state == "" {
			dir, e := os.UserConfigDir()
			if e != nil {
				return e
			}
			state = filepath.Join(dir, "PanelTree", "web")
		}
		service, e := configuredService(cmd.Context(), cfg)
		if e != nil {
			return e
		}
		listener, e := net.Listen("tcp", listen)
		if e != nil {
			return e
		}
		defer func() { _ = listener.Close() }()
		handler, e := web.New(web.Config{Roots: roots, StateDir: state, Authority: listener.Addr().String()}, service)
		if e != nil {
			return e
		}
		defer handler.Close()
		server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
		stopped := make(chan struct{})
		defer close(stopped)
		go func() {
			select {
			case <-cmd.Context().Done():
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = server.Shutdown(ctx)
			case <-stopped:
			}
		}()
		if _, e = fmt.Fprintln(cmd.OutOrStdout(), "http://"+listener.Addr().String()); e != nil {
			return e
		}
		e = server.Serve(listener)
		if e == http.ErrServerClosed {
			return nil
		}
		return e
	}
	return cmd
}
