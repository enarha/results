// Package live provides helpers to read Tekton resources directly from the
// Kubernetes API and present them using the Results API types.
package live

import (
	"context"
	"fmt"
	"time"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	pipelineclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"github.com/tektoncd/results/pkg/results/namer"
	"github.com/tektoncd/results/pkg/watcher/convert"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
)

// Client queries live Tekton resources in Kubernetes.
type Client struct {
	pipeline pipelineclient.Interface
}

// New creates a live client backed by the provided Pipeline clientset.
func New(pipeline pipelineclient.Interface) *Client {
	return &Client{pipeline: pipeline}
}

// GetRecord returns a Record representation of a live PipelineRun or TaskRun.
func (c *Client) GetRecord(ctx context.Context, name string) (*pb.Record, error) {
	if c == nil || c.pipeline == nil {
		return nil, status.Error(codes.FailedPrecondition, "live client is not configured")
	}

	parent, res, recName, err := record.ParseName(name)
	if err != nil {
		return nil, err
	}
	targetResult := result.FormatName(parent, res)

	runs, err := c.listRuns(ctx, parent)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list live runs: %v", err)
	}

	for _, run := range runs {
		if namer.ResultName(run.meta) != targetResult {
			continue
		}
		if candidate := namer.RecordName(targetResult, run.meta); candidate == name {
			data, err := convert.ToProto(run.obj)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "convert live object: %v", err)
			}
			return run.toRecord(candidate, data), nil
		}
	}

	return nil, status.Errorf(codes.NotFound, "record %q not found in live data", recName)
}

// GetResult synthesizes a live Result from PipelineRuns/TaskRuns in the
// requested namespace.
func (c *Client) GetResult(ctx context.Context, name string) (*pb.Result, error) {
	if c == nil || c.pipeline == nil {
		return nil, status.Error(codes.FailedPrecondition, "live client is not configured")
	}

	parent, resName, err := result.ParseName(name)
	if err != nil {
		return nil, err
	}
	targetResult := result.FormatName(parent, resName)

	runs, err := c.listRuns(ctx, parent)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list live runs: %v", err)
	}

	var matches []liveRun
	for _, run := range runs {
		if namer.ResultName(run.meta) == targetResult {
			matches = append(matches, run)
		}
	}
	if len(matches) == 0 {
		return nil, status.Errorf(codes.NotFound, "result %q not found in live data", name)
	}

	top := pickTopLevel(matches)
	created := top.meta.GetCreationTimestamp().Time
	updated := top.lastTransition(created)

	res := &pb.Result{
		Name:        targetResult,
		Id:          string(top.meta.GetUID()),
		Uid:         string(top.meta.GetUID()),
		CreatedTime: timestamppb.New(created),
		CreateTime:  timestamppb.New(created),
		UpdatedTime: timestamppb.New(updated),
		UpdateTime:  timestamppb.New(updated),
		Summary:     top.toSummary(targetResult),
	}
	return res, nil
}

type liveRun struct {
	meta   metav1.Object
	obj    runtime.Object
	status apis.ConditionAccessor
}

func (lr liveRun) lastTransition(fallback time.Time) time.Time {
	if lr.status == nil {
		return fallback
	}
	if cond := lr.status.GetCondition(apis.ConditionSucceeded); cond != nil && !cond.LastTransitionTime.Inner.IsZero() {
		return cond.LastTransitionTime.Inner.Time
	}
	return fallback
}

func (lr liveRun) toRecord(name string, data *pb.Any) *pb.Record {
	created := lr.meta.GetCreationTimestamp().Time
	updated := lr.lastTransition(created)

	return &pb.Record{
		Name:        name,
		Id:          string(lr.meta.GetUID()),
		Uid:         string(lr.meta.GetUID()),
		Data:        data,
		CreatedTime: timestamppb.New(created),
		CreateTime:  timestamppb.New(created),
		UpdatedTime: timestamppb.New(updated),
		UpdateTime:  timestamppb.New(updated),
	}
}

func (lr liveRun) toSummary(parent string) *pb.RecordSummary {
	if lr.status == nil {
		return nil
	}

	ready := lr.status.GetCondition(apis.ConditionReady)
	succeeded := lr.status.GetCondition(apis.ConditionSucceeded)

	return &pb.RecordSummary{
		Record:    namer.RecordName(parent, lr.meta),
		Type:      convert.TypeName(lr.obj),
		Status:    convert.Status(lr.status),
		StartTime: conditionTimestamp(ready),
		EndTime:   conditionTimestamp(succeeded),
	}
}

func conditionTimestamp(c *apis.Condition) *timestamppb.Timestamp {
	if c == nil || c.IsFalse() {
		return nil
	}
	return timestamppb.New(c.LastTransitionTime.Inner.Time)
}

func pickTopLevel(runs []liveRun) liveRun {
	var top liveRun
	for _, run := range runs {
		if !namer.IsTopLevel(run.meta) {
			continue
		}
		if top.meta == nil {
			top = run
			continue
		}
		if _, ok := run.obj.(*pipelinev1.PipelineRun); ok {
			// Prefer PipelineRuns when multiple top-level objects are present.
			top = run
			continue
		}
	}
	if top.meta != nil {
		return top
	}
	return runs[0]
}

func (c *Client) listRuns(ctx context.Context, namespace string) ([]liveRun, error) {
	if namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required for live queries")
	}

	var runs []liveRun

	prList, err := c.pipeline.TektonV1().PipelineRuns(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list PipelineRuns: %w", err)
	}
	for i := range prList.Items {
		pr := prList.Items[i]
		runs = append(runs, liveRun{
			meta:   &pr,
			obj:    &pr,
			status: &pr.Status,
		})
	}

	trList, err := c.pipeline.TektonV1().TaskRuns(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list TaskRuns: %w", err)
	}
	for i := range trList.Items {
		tr := trList.Items[i]
		runs = append(runs, liveRun{
			meta:   &tr,
			obj:    &tr,
			status: &tr.Status,
		})
	}

	return runs, nil
}
