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

package test

import (
	"context"
	"fmt"
	"log"
	"net"
	"testing"
	"time"

	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/api/server/logger"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/tektoncd/results/pkg/api/server/test"
	server "github.com/tektoncd/results/pkg/api/server/v1alpha2"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

const (
	port = ":0"
)

// NewResultsClient creates new gRPC Results client for testing purpose
//
//nolint:staticcheck
func NewResultsClient(t *testing.T, config *config.Config, opts ...server.Option) (pb.ResultsClient, pb.LogsClient) {
	t.Helper()
	config.DB_ENABLE_AUTO_MIGRATION = true
	config.LOGS_API = true
	config.LOGS_TYPE = "File"
	srv, err := server.New(config, logger.Get("info"), test.NewDB(t), opts...)
	if err != nil {
		t.Fatalf("Failed to create fake server: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterResultsServer(s, srv)
	pb.RegisterLogsServer(s, srv)
	lis, err := net.Listen("tcp", port)
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Printf("error starting result server: %v\n", err)
		}
	}()
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("did not connect: %v", err)
	}
	dialCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := waitForConnReady(dialCtx, conn); err != nil {
		conn.Close()
		t.Fatalf("did not connect: %v", err)
	}
	t.Cleanup(func() {
		s.Stop()
		lis.Close()
		conn.Close()
	})
	return pb.NewResultsClient(conn), pb.NewLogsClient(conn)
}

func waitForConnReady(ctx context.Context, conn *grpc.ClientConn) error {
	conn.Connect()
	state := conn.GetState()
	for {
		if state == connectivity.Ready {
			return nil
		}
		if state == connectivity.Shutdown {
			if err := ctx.Err(); err != nil {
				return err
			}
			return fmt.Errorf("gRPC connection shut down during dial")
		}
		if !conn.WaitForStateChange(ctx, state) {
			if err := ctx.Err(); err != nil {
				return err
			}
			return fmt.Errorf("gRPC connection state unchanged before timeout")
		}
		state = conn.GetState()
	}
}
