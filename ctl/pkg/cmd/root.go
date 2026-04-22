/*
	Copyright © 2026 Alan Ghelardi

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox"
	"golang.nuinfra.net/ctl/pkg/machinery"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sindri",
	Short: "The CLI for Sindri platform",
	Long:  ``,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGKILL, syscall.SIGTERM)
	defer stop()

	cmdCtx := machinery.NewContext(ctx)

	rootCmd.SetContext(ctx)
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true

	rootCmd.AddCommand(
		sandbox.New(cmdCtx),
	)

	if err := rootCmd.Execute(); err != nil {
		cmdCtx.Fatal(err)
	}
}
