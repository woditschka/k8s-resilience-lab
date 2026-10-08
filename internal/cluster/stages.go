package cluster

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
)

// labsOwnStages is the lab's Stage set: the Stages the KWOK controller applies
// once the Stage CRD is enabled. It is the cluster image's own set at the pinned
// KWOK release, kwokRelease, except pod-ready, which takes KWOK's general form
// reading a Pod's Startup time from podReadyDelayAnnotation and
// podReadyJitterDelayAnnotation, and whose own delay is none, so a Pod carrying
// neither, a provisioning probe among them, is Ready at once.
//
// With the CRD enabled the controller applies the Stages served through the API
// alone and none of the image's configuration file, which was measured on the
// pinned image, so a KWOK upgrade is a review of this set, per
// docs/adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md.
//
//go:embed stages.yaml
var labsOwnStages []byte

// stageResource is the API resource a Stage is served as.
var stageResource = schema.GroupVersionResource{Group: "kwok.x-k8s.io", Version: "v1alpha1", Resource: "stages"}

// stageDecodingBuffer is how far ahead the decoder reads to tell YAML from JSON.
const stageDecodingBuffer = 4096

// ownStages makes the Stages this cluster serves exactly the lab's own set: each
// of the set created or replaced by name, and any other removed, so the outcome
// holds whether or not kwokctl seeds Stages of its own. The Stage resource is
// served a moment after the API server answers, so a list the API refuses is
// retried at the poll interval within the start timeout.
func (c *SimulatedCluster) ownStages(ctx context.Context) error {
	address, err := c.container.APIAddress(ctx)
	if err != nil {
		return err
	}
	client, err := dynamic.NewForConfig(controlPlaneConfig(address))
	if err != nil {
		return fmt.Errorf("build a client for the Stages of %s: %w", c.Name(), err)
	}
	wanted, err := decodeStages(labsOwnStages)
	if err != nil {
		return err
	}

	stages := client.Resource(stageResource)
	served, err := c.awaitStagesServed(ctx, stages)
	if err != nil {
		return err
	}

	return c.reconcileStages(ctx, stages, served, wanted)
}

// awaitStagesServed lists the Stages this cluster serves once the API serves the
// Stage resource at all, within the start timeout.
func (c *SimulatedCluster) awaitStagesServed(ctx context.Context, stages dynamic.ResourceInterface) (map[string]unstructured.Unstructured, error) {
	starting, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()

	for {
		listed, err := stages.List(starting, metav1.ListOptions{})
		if err == nil {
			served := make(map[string]unstructured.Unstructured, len(listed.Items))
			for _, stage := range listed.Items {
				served[stage.GetName()] = stage
			}

			return served, nil
		}

		select {
		case <-starting.Done():
			return nil, fmt.Errorf("the API server of %s served no Stage resource within %s: %w", c.Name(), startTimeout, err)
		case <-time.After(pollInterval):
		}
	}
}

// reconcileStages creates each wanted Stage, replaces one already served under its
// name, and removes every served Stage outside the set.
func (c *SimulatedCluster) reconcileStages(
	ctx context.Context,
	stages dynamic.ResourceInterface,
	served map[string]unstructured.Unstructured,
	wanted []unstructured.Unstructured,
) error {
	for _, stage := range wanted {
		existing, isServed := served[stage.GetName()]
		delete(served, stage.GetName())

		if isServed {
			stage.SetResourceVersion(existing.GetResourceVersion())
			if _, err := stages.Update(ctx, &stage, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("replace the Stage %s of %s: %w", stage.GetName(), c.Name(), err)
			}

			continue
		}
		if _, err := stages.Create(ctx, &stage, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create the Stage %s of %s: %w", stage.GetName(), c.Name(), err)
		}
	}

	for name := range served {
		if err := stages.Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("remove the Stage %s of %s, which is not the lab's own: %w", name, c.Name(), err)
		}
	}

	return nil
}

// decodeStages reads the Stages one YAML document stream holds, in order.
func decodeStages(stream []byte) ([]unstructured.Unstructured, error) {
	decoder := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(stream), stageDecodingBuffer)

	var decoded []unstructured.Unstructured
	for {
		var stage unstructured.Unstructured
		if err := decoder.Decode(&stage.Object); err != nil {
			if errors.Is(err, io.EOF) {
				return decoded, nil
			}

			return nil, fmt.Errorf("decode the lab's own Stages: %w", err)
		}
		decoded = append(decoded, stage)
	}
}
