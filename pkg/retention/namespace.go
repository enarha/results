/*
Copyright 2024 The Tekton Authors

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

package retention

import (
	"context"
	"errors"
	"fmt"
	"sort"

	dberrors "github.com/tektoncd/results/pkg/api/server/db/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// namespaceListPageSize is the page size used when listing the cluster namespaces.
const namespaceListPageSize = 500

// kubeNamespaceLister lists the cluster namespaces through the Kubernetes API.
type kubeNamespaceLister struct {
	client kubernetes.Interface
}

// List returns the names of all the namespaces of the cluster.
func (l *kubeNamespaceLister) List(ctx context.Context) ([]string, error) {
	var names []string
	opts := metav1.ListOptions{Limit: namespaceListPageSize}
	for {
		list, err := l.client.CoreV1().Namespaces().List(ctx, opts)
		if err != nil {
			return nil, err
		}
		for i := range list.Items {
			names = append(names, list.Items[i].Name)
		}
		if list.Continue == "" {
			return names, nil
		}
		opts.Continue = list.Continue
	}
}

// cleanupDeletedNamespaces removes the data of the namespaces that no longer
// exist in the cluster.
func (a *Agent) cleanupDeletedNamespaces() {
	cfg := a.NamespaceCleanup

	liveNamespaces, err := a.namespaces.List(a.ctx)
	if err != nil {
		a.Logger.Errorf("namespace cleanup skipped, failed to list the cluster namespaces: %s", err.Error())
		return
	}
	// An empty list is never trusted. Deleting the data of every namespace
	// because of an unexpected API server answer is not recoverable.
	if len(liveNamespaces) == 0 {
		a.Logger.Error("namespace cleanup skipped, the cluster reported no namespaces")
		return
	}

	candidates, err := a.inactiveNamespaces(cfg.GracePeriodDuration.Seconds())
	if err != nil {
		a.Logger.Errorf("namespace cleanup skipped, failed to list the namespaces stored in the database: %s", err.Error())
		return
	}

	keep := make(map[string]struct{}, len(liveNamespaces)+len(cfg.ExcludeNamespaces))
	for _, ns := range liveNamespaces {
		keep[ns] = struct{}{}
	}
	for _, ns := range cfg.ExcludeNamespaces {
		keep[ns] = struct{}{}
	}

	var deleted []string
	for _, ns := range candidates {
		if _, ok := keep[ns]; !ok {
			deleted = append(deleted, ns)
		}
	}
	sort.Strings(deleted)

	if len(deleted) == 0 {
		a.Logger.Info("namespace cleanup found no data belonging to deleted namespaces")
		return
	}

	if len(deleted) > cfg.MaxNamespacesPerRun {
		a.Logger.Errorf("namespace cleanup skipped, %d namespaces are eligible for cleanup but maxNamespacesPerRun is %d: %v",
			len(deleted), cfg.MaxNamespacesPerRun, deleted)
		return
	}

	if cfg.DryRun {
		a.Logger.Infof("namespace cleanup is in dry run mode, the data of the following deleted namespaces would be removed: %v", deleted)
		return
	}

	for _, ns := range deleted {
		rows, err := a.deleteNamespace(ns)
		if err != nil {
			a.Logger.Errorf("namespace cleanup failed to delete the data of the deleted namespace %s: %s", ns, err.Error())
			continue
		}
		a.Logger.Infof("namespace cleanup deleted %d results of the deleted namespace %s", rows, ns)
	}
}

// inactiveNamespaces returns the namespaces stored in the database whose data
// hasn't been updated for at least gracePeriodSeconds.
func (a *Agent) inactiveNamespaces(gracePeriodSeconds float64) ([]string, error) {
	query := fmt.Sprintf(`
        SELECT parent FROM results
        GROUP BY parent
        HAVING MAX(updated_time) < NOW() - INTERVAL '%f seconds'
    `, gracePeriodSeconds)

	var namespaces []string
	if err := dberrors.Wrap(a.db.Raw(query).Scan(&namespaces).Error); err != nil {
		return nil, err
	}
	return namespaces, nil
}

// deleteNamespace removes the results of a single namespace. The associated
// records are removed by the database through the foreign key constraint.
func (a *Agent) deleteNamespace(namespace string) (int64, error) {
	if namespace == "" {
		return 0, errors.New("the namespace mustn't be empty")
	}
	result := a.db.Exec("DELETE FROM results WHERE parent = ?", namespace)
	if err := dberrors.Wrap(result.Error); err != nil {
		return 0, err
	}
	return result.RowsAffected, nil
}
