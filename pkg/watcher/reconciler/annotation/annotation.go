// Copyright 2020 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package annotation

import (
	"context"
	"encoding/json"

	"github.com/tektoncd/results/pkg/watcher/reconciler/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/logging"
)

const (

	// Result identifier.
	Result = "results.tekton.dev/result"

	// Record identifier.
	Record = "results.tekton.dev/record"

	// Log identifier.
	Log = "results.tekton.dev/log"

	// EventList identifier.
	EventList = "results.tekton.dev/eventlist"

	// Stored is an annotation that signals to the controller that a given object
	// has been stored by the Results API.
	Stored = "results.tekton.dev/stored"

	// ResultAnnotations is an annotation that integrators should add to objects in order to store
	// arbitrary keys/values into the Result.Annotations field.
	ResultAnnotations = "results.tekton.dev/resultAnnotations"

	// RecordSummaryAnnotations is an annotation that integrators should add to objects
	// in order to store arbitrary keys/values into the Result.Summary.Annotations field.
	// This allows for additional information to be associated with the summary of a record.
	RecordSummaryAnnotations = "results.tekton.dev/recordSummaryAnnotations"

	// ChildReadyForDeletion is an annotation that signals to the controller that a given child object
	// (e.g. TaskRun owned by a PipelineRun) is done and up to date in the
	// API server and therefore, ready to be garbage collected.
	ChildReadyForDeletion = "results.tekton.dev/childReadyForDeletion"
)

// Annotation is wrapper for Kubernetes resource annotations stored in the metadata.
type Annotation struct {
	Name  string
	Value string
}

type mergePatch struct {
	Metadata metadata `json:"metadata"`
}

type metadata struct {
	Annotations map[string]string `json:"annotations"`
}

// Patch builds and applies a patch with the given annotations to the object using the provided object client.
func Patch(
	ctx context.Context,
	object metav1.Object,
	objectClient client.ObjectClient,
	annotations ...Annotation,
) error {

	logger := logging.FromContext(ctx)

	if IsPatched(object, annotations...) {
		logger.Debugf("Skipping CRD annotation patch: annotations are already set ObjectName: %s", object.GetName())
		return nil
	}

	data := mergePatch{
		Metadata: metadata{
			Annotations: map[string]string{},
		},
	}
	// Start with a copy of the current annotations
	for k, v := range object.GetAnnotations() {
		data.Metadata.Annotations[k] = v
	}
	// Add/overwrite with new annotations
	for _, annotation := range annotations {
		if len(annotation.Value) != 0 {
			data.Metadata.Annotations[annotation.Name] = annotation.Value
		}
	}
	patch, err := json.Marshal(data)
	if err != nil {
		return err
	}

	err = objectClient.Patch(ctx, object.GetName(), types.MergePatchType, patch, metav1.PatchOptions{})

	// After successful patch, update in-memory object
	if err == nil {
		currentAnnotations := object.GetAnnotations()
		if currentAnnotations == nil {
			currentAnnotations = make(map[string]string)
		}
		for _, ann := range annotations {
			currentAnnotations[ann.Name] = ann.Value
		}
		object.SetAnnotations(currentAnnotations)
	}

	return err
}

// IsPatched returns true if the object in question contains all relevant
// annotations or false otherwise.
func IsPatched(object metav1.Object, annotations ...Annotation) bool {
	objAnnotations := object.GetAnnotations()
	for _, annotation := range annotations {
		if objAnnotations[annotation.Name] != annotation.Value {
			return false
		}
	}
	return true
}
