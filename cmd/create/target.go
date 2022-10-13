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

package create

import (
	"context"
	"fmt"
	"log"
	"path"
	"strings"

	"github.com/spf13/cobra"

	"github.com/triggermesh/tmcli/pkg/output"
	"github.com/triggermesh/tmcli/pkg/triggermesh"
	"github.com/triggermesh/tmcli/pkg/triggermesh/components/target"
	"github.com/triggermesh/tmcli/pkg/triggermesh/crd"
)

func (o *CreateOptions) NewTargetCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "target <kind> <args>",
		Short:              "TriggerMesh target",
		DisableFlagParsing: true,
		SilenceErrors:      true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			o.initializeOptions(cmd)
			if len(args) == 0 {
				sources, err := crd.ListTargets(o.CRD)
				if err != nil {
					return fmt.Errorf("list sources: %w", err)
				}
				fmt.Printf("Available targets:\n---\n%s\n", strings.Join(sources, "\n"))
				return nil
			}
			kind, args, err := parse(args)
			if err != nil {
				return err
			}
			name, args := parameterFromArgs("name", args)
			eventSourcesFilter, args := parameterFromArgs("source", args)
			eventTypesFilter, args := parameterFromArgs("eventTypes", args)
			var typeFilter, sourceFilter []string
			if eventTypesFilter != "" {
				typeFilter = strings.Split(eventTypesFilter, ",")
			}
			if eventSourcesFilter != "" {
				sourceFilter = strings.Split(eventSourcesFilter, ",")
			}
			return o.target(name, kind, args, sourceFilter, typeFilter)
		},
	}
}

func (o *CreateOptions) target(name, kind string, args []string, eventSourcesFilter, eventTypesFilter []string) error {
	ctx := context.Background()
	configDir := path.Join(o.ConfigBase, o.Context)
	manifest := path.Join(configDir, manifestFile)

	for _, source := range eventSourcesFilter {
		et, err := o.producersEventTypes(source)
		if err != nil {
			return fmt.Errorf("%q event types: %w", source, err)
		}
		eventTypesFilter = append(eventTypesFilter, et...)
	}

	t := target.New(name, o.CRD, kind, o.Context, o.Version, args)

	secretEnv, secretsChanged, err := triggermesh.ProcessSecrets(ctx, t, manifest)
	if err != nil {
		return fmt.Errorf("spec processing: %w", err)
	}

	log.Println("Updating manifest")
	restart, err := triggermesh.WriteObject(ctx, t, manifest)
	if err != nil {
		return err
	}
	log.Println("Starting container")
	container, err := triggermesh.Start(ctx, t, (restart || secretsChanged), secretEnv)
	if err != nil {
		return err
	}

	if len(eventTypesFilter) != 0 {
		log.Println("Creating trigger")
		if err := o.createTrigger("", container.Name, container.HostPort(), eventTypesFilter...); err != nil {
			return err
		}
	}

	output.PrintStatus("consumer", t, eventSourcesFilter, eventTypesFilter)
	return nil
}
