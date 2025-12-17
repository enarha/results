package live

import (
	"context"
	"testing"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	pipelinefake "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/fake"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"github.com/tektoncd/results/pkg/results/namer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetRecord_PipelineRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	pr := &pipelinev1.PipelineRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1",
			Kind:       "PipelineRun",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pr",
			Namespace: "ns",
			UID:       "uid-pr",
		},
	}

	client := pipelinefake.NewSimpleClientset(pr)
	lc := New(client)

	resultName := namer.ResultName(pr)
	recordName := namer.RecordName(resultName, pr)

	rec, err := lc.GetRecord(ctx, recordName)
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if rec.GetName() != recordName {
		t.Fatalf("record name: got %s want %s", rec.GetName(), recordName)
	}
	if rec.GetUid() != string(pr.UID) {
		t.Fatalf("record uid: got %s want %s", rec.GetUid(), pr.UID)
	}
	if rec.GetData() == nil {
		t.Fatal("record data is nil")
	}
}

func TestGetResult_PipelineRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	pr := &pipelinev1.PipelineRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1",
			Kind:       "PipelineRun",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pr",
			Namespace: "ns",
			UID:       "uid-pr",
		},
	}

	client := pipelinefake.NewSimpleClientset(pr)
	lc := New(client)

	resultName := namer.ResultName(pr)

	res, err := lc.GetResult(ctx, resultName)
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if res.GetName() != resultName {
		t.Fatalf("result name: got %s want %s", res.GetName(), resultName)
	}
	if res.GetUid() != string(pr.UID) {
		t.Fatalf("result uid: got %s want %s", res.GetUid(), pr.UID)
	}
	if res.GetSummary() == nil {
		t.Fatal("result summary is nil")
	}

	// Verify the summary record references the expected record name.
	parent, namePart, err := result.ParseName(resultName)
	if err != nil {
		t.Fatalf("ParseName(%s): %v", resultName, err)
	}
	expectedRecord := record.FormatName(result.FormatName(parent, namePart), namer.DefaultName(pr))
	if res.GetSummary().GetRecord() != expectedRecord {
		t.Fatalf("summary record: got %s want %s", res.GetSummary().GetRecord(), expectedRecord)
	}
}
