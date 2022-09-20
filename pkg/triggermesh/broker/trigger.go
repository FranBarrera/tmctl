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

package broker

import (
	"fmt"
	"os"
	"path"

	"gopkg.in/yaml.v3"

	"github.com/triggermesh/tmcli/pkg/docker"
	"github.com/triggermesh/tmcli/pkg/kubernetes"
	"github.com/triggermesh/tmcli/pkg/manifest"
	"github.com/triggermesh/tmcli/pkg/triggermesh"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ triggermesh.Component = (*Trigger)(nil)

type Trigger struct {
	Name string

	Broker          string
	BrokerConfigDir string

	spec TriggerSpec
}

type TriggerSpec struct {
	Name    string
	Filters []Filter `yaml:"filters,omitempty"`
	Targets []Target `yaml:"targets"`
}

type Filter struct {
	Exact Exact `yaml:"exact"`
}

type Exact struct {
	Type string `yaml:"type"`
}

type Target struct {
	URL             string `yaml:"url"`
	Component       string `yaml:"component,omitempty"` // for local version only
	DeliveryOptions struct {
		Retries       int    `yaml:"retries,omitempty"`
		BackoffDelay  string `yaml:"backoffDelay,omitempty"`
		BackoffPolicy string `yaml:"backoffPolicy,omitempty"`
	} `yaml:"deliveryOptions,omitempty"`
}

func (t *Trigger) AsUnstructured() (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion("eventing.triggermesh.io/v1alpha1")
	u.SetKind("Trigger")
	u.SetName(t.Name)
	u.SetLabels(map[string]string{"context": t.Broker})
	return u, unstructured.SetNestedField(u.Object, t.spec, "spec")
}

func (t *Trigger) AsK8sObject() (*kubernetes.Object, error) {
	return &kubernetes.Object{
		APIVersion: "eventing.triggermesh.io/v1alpha1",
		Kind:       "Trigger",
		Metadata: kubernetes.Metadata{
			Name: t.Name,
			Labels: map[string]string{
				"triggermesh.io/context": t.Broker,
			},
		},
		Spec: map[string]interface{}{
			"filter":  t.spec.Filters,
			"targets": t.spec.Targets,
		},
	}, nil
}

func (t *Trigger) AsContainer() (*docker.Container, error) {
	return nil, nil
}

func (t *Trigger) GetKind() string {
	return "Trigger"
}

func (t *Trigger) GetName() string {
	return t.Name
}

func (t *Trigger) GetImage() string {
	return ""
}

func (t *Trigger) GetSpec() TriggerSpec {
	return t.spec
}

func NewTrigger(name, broker, eventType, configDir string) *Trigger {
	filters := []Filter{{
		Exact: Exact{Type: eventType},
	}}
	if eventType == "" {
		filters = []Filter{}
	}
	return &Trigger{
		Name:            name,
		Broker:          broker,
		BrokerConfigDir: configDir,
		spec: TriggerSpec{
			Name:    name,
			Filters: filters,
		},
	}
}

func (t *Trigger) SetTarget(component, destination string) {
	t.spec.Targets = []Target{
		{
			Component: component,
			URL:       destination,
		},
	}
}

func (t *Trigger) SetFilter(eventType string) {
	t.spec.Filters = []Filter{
		{
			Exact: Exact{eventType},
		},
	}
}

func (t *Trigger) LookupTrigger() error {
	configFile := path.Join(t.BrokerConfigDir, "broker.conf")
	configuration, err := readBrokerConfig(configFile)
	if err != nil {
		return fmt.Errorf("broker config: %w", err)
	}
	for _, trigger := range configuration.Triggers {
		if trigger.Name == t.Name {
			t.spec.Filters = trigger.Filters
			t.spec.Targets = trigger.Targets
			return nil
		}
	}
	return fmt.Errorf("trigger %q not found", t.Name)
}

func (t *Trigger) RemoveTriggerFromConfig() error {
	configFile := path.Join(t.BrokerConfigDir, "broker.conf")
	configuration, err := readBrokerConfig(configFile)
	if err != nil {
		return fmt.Errorf("broker config: %w", err)
	}

	for i, trigger := range configuration.Triggers {
		if trigger.Name == t.Name {
			if len(configuration.Triggers) > i+1 {
				configuration.Triggers = append(configuration.Triggers[:i], configuration.Triggers[i+1:]...)
			} else {
				configuration.Triggers = configuration.Triggers[:i]
			}
			return writeBrokerConfig(configFile, &configuration)
		}
	}
	return nil
}

func (t *Trigger) UpdateBrokerConfig() error {
	configFile := path.Join(t.BrokerConfigDir, "broker.conf")
	configuration, err := readBrokerConfig(configFile)
	if err != nil {
		return fmt.Errorf("broker config: %w", err)
	}

	var exists bool
	for i, trigger := range configuration.Triggers {
		if trigger.Name == t.Name {
			configuration.Triggers[i].Filters = t.spec.Filters
			configuration.Triggers[i].Targets = t.spec.Targets
			exists = true
		}
	}
	if !exists {
		configuration.Triggers = append(configuration.Triggers, t.spec)
	}
	return writeBrokerConfig(configFile, &configuration)
}

func (t *Trigger) UpdateManifest() error {
	m := manifest.New(path.Join(t.BrokerConfigDir, "manifest.yaml"))
	if err := m.Read(); err != nil {
		return fmt.Errorf("manifest read: %w", err)
	}
	o, err := t.AsK8sObject()
	if err != nil {
		return fmt.Errorf("trigger object: %w", err)
	}
	if _, err := m.Add(*o); err != nil {
		return fmt.Errorf("adding trigger: %w", err)
	}
	return m.Write()
}

func readBrokerConfig(path string) (Configuration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Configuration{}, fmt.Errorf("read file: %w", err)
	}
	var config Configuration
	return config, yaml.Unmarshal(data, &config)
}

func writeBrokerConfig(path string, configuration *Configuration) error {
	out, err := yaml.Marshal(configuration)
	if err != nil {
		return fmt.Errorf("marshal broker configuration: %w", err)
	}
	return os.WriteFile(path, out, os.ModePerm)
}
