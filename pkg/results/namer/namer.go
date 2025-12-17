// Package namer contains helpers for generating Tekton Results identifiers
// consistently between the API server and watcher.
package namer

import (
	"strings"

	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// AnnotationPrefix is the base prefix for results-managed annotations.
	AnnotationPrefix = "results.tekton.dev/"

	// AnnotationResult stores the full results.tekton.dev result identifier.
	AnnotationResult = AnnotationPrefix + "result"

	// AnnotationRecord stores the full results.tekton.dev record identifier.
	AnnotationRecord = AnnotationPrefix + "record"
)

// ResultName returns the result name for the provided object.
// The logic matches the watcher: prefer annotations, then trigger labels or
// PipelineRun owners, and finally fall back to the object's UID.
func ResultName(obj metav1.Object) string {
	if v, ok := obj.GetAnnotations()[AnnotationResult]; ok {
		return v
	}

	var part string
	if v, ok := obj.GetLabels()["triggers.tekton.dev/triggers-eventid"]; ok {
		// Trigger event IDs are globally unique, so do not prefix them with the
		// owning PipelineRun UID.
		part = v
	} else if len(obj.GetOwnerReferences()) > 0 {
		for _, owner := range obj.GetOwnerReferences() {
			if strings.EqualFold(owner.Kind, "pipelinerun") {
				part = string(owner.UID)
				break
			}
		}
	}

	if part == "" {
		part = DefaultName(obj)
	}
	return result.FormatName(obj.GetNamespace(), part)
}

// RecordName returns the record name for the provided object scoped to the
// given parent result name.
// For top-level objects, annotations take precedence; otherwise the UID is
// used to avoid conflicts with inherited annotations.
func RecordName(parent string, obj metav1.Object) string {
	if IsTopLevel(obj) {
		if name, ok := obj.GetAnnotations()[AnnotationRecord]; ok {
			return name
		}
	}
	return record.FormatName(parent, DefaultName(obj))
}

// ParentName returns the parent name derived from annotations or the object's
// namespace if none are present.
func ParentName(obj metav1.Object) string {
	if value, found := obj.GetAnnotations()[AnnotationResult]; found {
		if parts := strings.Split(value, "/"); len(parts) != 0 {
			return parts[0]
		}
	}
	return obj.GetNamespace()
}

// DefaultName returns the default identifier for an object.
func DefaultName(obj metav1.Object) string {
	return string(obj.GetUID())
}

// IsTopLevel reports whether the object has no owner references.
func IsTopLevel(obj metav1.Object) bool {
	return len(obj.GetOwnerReferences()) == 0
}
