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

package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/triggermesh/tmcli/pkg/docker"
	"github.com/triggermesh/tmcli/pkg/kubernetes"
)

const (
	// container registry to pull adapter images from
	tmContainerRegistry = "gcr.io/triggermesh"
	// port number where adapter service is serving connections
	adapterPort = "8080/tcp"
	// adapter connect retries
	connRetries = 10
)

type adapterLogEntry struct {
	Component string

	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type container struct {
	socket string
	object *unstructured.Unstructured
}

type LocalSetup struct {
	ManifestPath string
	Version      string
	Secrets      []string
}

func NewLocalSetup(manifestFile, version string, secrets []string) *LocalSetup {
	return &LocalSetup{
		ManifestPath: manifestFile,
		Version:      version,
		Secrets:      secrets,
	}
}

func (l *LocalSetup) RunAll(restart bool) error {
	// _, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	// defer func() {
	// 	cancel()
	// 	// TODO: Find the way to clean up containers without time.Sleep()
	// 	time.Sleep(time.Second * 1)
	// }()

	manifest := kubernetes.NewManifest(l.ManifestPath)
	if err := manifest.Read(); err != nil {
		return fmt.Errorf("cannot parse manifest: %w", err)
	}

	components := make([]container, len(manifest.Objects))
	var wg sync.WaitGroup

	wg.Add(len(components))

	for i, object := range manifest.Objects {
		go func(i int, object kubernetes.Object) {
			c, err := RunObject(&object, l.Secrets, l.Version)
			if err != nil {
				panic(fmt.Errorf("cannot create adapter: %v", err))
			}
			components[i] = c
			wg.Done()
		}(i, object)
	}
	wg.Wait()

	// errs := make(chan adapterLogEntry)

	// for _, c := range components {
	// 	logs, err := client.Logs(ctx, c.id)
	// 	if err != nil {
	// 		return fmt.Errorf("cannot open container logs: %w", err)
	// 	}
	// 	go listenLogs(logs, c.object.GetName(), errs, true)
	// 	if true {
	// 		log.Printf("%q is listening on %s", c.object.GetName(), c.socket)
	// 	}
	// }
	// go printLogErrors(ctx, errs)
	return nil
}

func (l *LocalSetup) StopAll() error {
	manifest := kubernetes.NewManifest(l.ManifestPath)
	if err := manifest.Read(); err != nil {
		return fmt.Errorf("cannot parse manifest: %w", err)
	}
	for _, object := range manifest.Objects {
		if err := StopObject(&object); err != nil {
			return err
		}
	}
	return nil
}

func RunObject(object *kubernetes.Object, secrets []string, version string) (container, error) {
	client, err := docker.NewClient()
	if err != nil {
		return container{}, fmt.Errorf("docker client: %w", err)
	}
	k8sObject, err := object.ToUnstructured()
	if err != nil {
		return container{}, fmt.Errorf("cannot translate YAML resource to K8s object: %w", err)
	}
	return initializeAdapter(context.Background(), client, k8sObject, version, secrets)
}

func StopObject(object *kubernetes.Object) error {
	client, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}
	return client.RemoveAdapter(context.Background(), object.Metadata.Name)
}

func GetStatus(object *kubernetes.Object) (string, error) {
	client, err := docker.NewClient()
	if err != nil {
		return "", fmt.Errorf("docker client: %w", err)
	}
	return client.Status(context.Background(), object.Metadata.Name)
}

func listenLogs(output io.ReadCloser, component string, errs chan adapterLogEntry, verbose bool) {
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		var logOutput adapterLogEntry
		if err := json.Unmarshal(scanner.Bytes()[8:], &logOutput); err != nil {
			if verbose {
				log.Printf("%s", scanner.Bytes()[8:])
			}
			continue
		}
		logOutput.Component = component
		if logOutput.Severity != "INFO" && logOutput.Severity != "WARNING" {
			errs <- logOutput
		}
	}
}

func printLogErrors(ctx context.Context, errs chan adapterLogEntry) {
	for {
		select {
		case data := <-errs:
			log.Printf("Adapter %q error: %s", data.Component, data.Message)
		case <-ctx.Done():
			return
		}
	}
}
