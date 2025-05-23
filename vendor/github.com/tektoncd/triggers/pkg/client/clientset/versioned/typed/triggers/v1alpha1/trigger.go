/*
Copyright 2019 The Tekton Authors

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

// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	context "context"

	triggersv1alpha1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"
	scheme "github.com/tektoncd/triggers/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// TriggersGetter has a method to return a TriggerInterface.
// A group's client should implement this interface.
type TriggersGetter interface {
	Triggers(namespace string) TriggerInterface
}

// TriggerInterface has methods to work with Trigger resources.
type TriggerInterface interface {
	Create(ctx context.Context, trigger *triggersv1alpha1.Trigger, opts v1.CreateOptions) (*triggersv1alpha1.Trigger, error)
	Update(ctx context.Context, trigger *triggersv1alpha1.Trigger, opts v1.UpdateOptions) (*triggersv1alpha1.Trigger, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*triggersv1alpha1.Trigger, error)
	List(ctx context.Context, opts v1.ListOptions) (*triggersv1alpha1.TriggerList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *triggersv1alpha1.Trigger, err error)
	TriggerExpansion
}

// triggers implements TriggerInterface
type triggers struct {
	*gentype.ClientWithList[*triggersv1alpha1.Trigger, *triggersv1alpha1.TriggerList]
}

// newTriggers returns a Triggers
func newTriggers(c *TriggersV1alpha1Client, namespace string) *triggers {
	return &triggers{
		gentype.NewClientWithList[*triggersv1alpha1.Trigger, *triggersv1alpha1.TriggerList](
			"triggers",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *triggersv1alpha1.Trigger { return &triggersv1alpha1.Trigger{} },
			func() *triggersv1alpha1.TriggerList { return &triggersv1alpha1.TriggerList{} },
		),
	}
}
