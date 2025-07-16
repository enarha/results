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
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

type applyPatch struct {
	APIVersion string             `json:"apiVersion"`
	Kind       string             `json:"kind"`
	Metadata   applyPatchMetadata `json:"metadata"`
}

type applyPatchMetadata struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Patch creates a server-side apply patch used for adding result / record identifiers as
// well as other internal annotations to an object's annotations field.
func Patch(object metav1.Object, annotations ...Annotation) ([]byte, error) {
	// Get the API version and kind from the object
	var apiVersion, kind string

	// Try to get the kind from the object's GroupVersionKind
	if runtimeObj, ok := object.(runtime.Object); ok {
		if gvk := runtimeObj.GetObjectKind().GroupVersionKind(); !gvk.Empty() {
			kind = gvk.Kind
			apiVersion = gvk.GroupVersion().String()
		}
	}

	// If we couldn't determine the kind or apiVersion, fail
	if kind == "" || apiVersion == "" {
		return nil, fmt.Errorf("could not determine apiVersion and kind from object %s/%s", object.GetNamespace(), object.GetName())
	}

	// Start with a copy of the current annotations
	merged := map[string]string{}
	for k, v := range object.GetAnnotations() {
		merged[k] = v
	}
	// Add/overwrite with new annotations
	for _, annotation := range annotations {
		if len(annotation.Value) != 0 {
			merged[annotation.Name] = annotation.Value
		}
	}
	if isChildAndDone(object) {
		merged[ChildReadyForDeletion] = "true"
	}

	data := applyPatch{
		APIVersion: apiVersion,
		Kind:       kind,
		Metadata: applyPatchMetadata{
			Name:        object.GetName(),
			Namespace:   object.GetNamespace(),
			Annotations: merged,
		},
	}
	return json.Marshal(data)
}

// isChildAndDone returns true if the object in question is a child resource
// (i.e. has owner references) and it's done, therefore eligible to be patched
// with the results.tekton.dev/childReadyForDeletion annotation.
func isChildAndDone(objecct metav1.Object) bool {
	if len(objecct.GetOwnerReferences()) == 0 {
		return false
	}

	doneObj, ok := objecct.(interface{ IsDone() bool })
	if !ok {
		return false
	}
	return doneObj.IsDone()
}

// IsPatched returns true if the object in question contains all relevant
// annotations or false otherwise.
func IsPatched(object metav1.Object, annotations ...Annotation) bool {
	objAnnotations := object.GetAnnotations()
	if isChildAndDone(object) {
		if _, found := objAnnotations[ChildReadyForDeletion]; !found {
			return false
		}
	}

	for _, annotation := range annotations {
		if objAnnotations[annotation.Name] != annotation.Value {
			return false
		}
	}

	return true
}
