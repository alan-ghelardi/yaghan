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

	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli/prompt"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/node"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/sandbox"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/snapshot"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "yag",
	Short: "The CLI for Yaghan platform",
	Long:  ``,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGKILL, syscall.SIGTERM)
	defer stop()

	prompter := prompt.NewPrompter()

	cmdCtx := cli.NewContext(ctx, prompter)

	rootCmd.SetContext(ctx)
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true

	rootCmd.AddCommand(
		sandbox.New(cmdCtx),
		node.New(cmdCtx),
		snapshot.New(cmdCtx),
	)

	if err := rootCmd.Execute(); err != nil {
		cmdCtx.Fatal(err)
	}
}
