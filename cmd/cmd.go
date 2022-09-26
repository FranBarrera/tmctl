/*
Copyright 2022 TriggerMesh Inc.

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
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/triggermesh/tmcli/pkg/docker"
	"github.com/triggermesh/tmcli/pkg/triggermesh/crd"

	configcmd "github.com/triggermesh/tmcli/cmd/config"
	createcmd "github.com/triggermesh/tmcli/cmd/create"
	dumpcmd "github.com/triggermesh/tmcli/cmd/dump"
	startcmd "github.com/triggermesh/tmcli/cmd/start"
	stopcmd "github.com/triggermesh/tmcli/cmd/stop"
	watchcmd "github.com/triggermesh/tmcli/cmd/watch"
)

var (
	defaultConfigPath = "$HOME/.triggermesh/cli"
)

func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "",
		Short: "",
		Long:  ``,

		PersistentPreRunE: func(ccmd *cobra.Command, args []string) error {
			// check docker server
			_, err := docker.NewClient()
			if err != nil {
				return fmt.Errorf("docker client: %w", err)
			}

			if err := initConfig(); err != nil {
				return err
			}
			crdFile, err := crd.Fetch(defaultConfigPath)
			if err != nil {
				return err
			}
			viper.Set("triggermesh.servedCRD", crdFile)
			return nil
		},
		Run: func(ccmd *cobra.Command, args []string) {
			ccmd.HelpFunc()(ccmd, args)
		},
	}
	// persistent flags
	rootCmd.PersistentFlags().String("config", defaultConfigPath, "Config home dir")
	rootCmd.PersistentFlags().String("context", "default", "Context")
	rootCmd.PersistentFlags().String("version", "latest", "TriggerMesh components version")
	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug output")

	// bind config args
	viper.BindPFlag("context", rootCmd.PersistentFlags().Lookup("context"))
	viper.BindPFlag("triggermesh.version", rootCmd.PersistentFlags().Lookup("version"))

	// commands
	rootCmd.AddCommand(createcmd.NewCmd())
	rootCmd.AddCommand(configcmd.NewCmd())
	rootCmd.AddCommand(dumpcmd.NewCmd())
	rootCmd.AddCommand(startcmd.NewCmd())
	rootCmd.AddCommand(stopcmd.NewCmd())
	rootCmd.AddCommand(watchcmd.NewCmd())

	return rootCmd
}

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("User home dir not set: %v\n", err)
		os.Exit(1)
	}
	defaultConfigPath = strings.Replace(defaultConfigPath, "$HOME", home, 1)
}

func initConfig() error {
	viper.SetConfigType("yaml")
	viper.SetConfigName("config")
	viper.AddConfigPath(defaultConfigPath)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(strings.Replace(defaultConfigPath, "$HOME", home, 1), os.ModePerm); err != nil {
				return err
			}
			viper.SetDefault("context", "default")
			viper.SetDefault("triggermesh.crd", "https://github.com/triggermesh/triggermesh/releases/download/${VERSION}/triggermesh-crds.yaml")
			viper.SetDefault("triggermesh.version", "latest")
			return viper.SafeWriteConfig()
		}
		return err
	}
	return nil
}
