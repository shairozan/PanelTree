package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/shairozan/PanelTree/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	cmd := cli.Command()
	err := cmd.ExecuteContext(ctx)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
