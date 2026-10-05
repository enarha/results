// Copyright 2026 The Tekton Authors
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

package migrations

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

var fileName = regexp.MustCompile(`^(\d+)_([a-z0-9_]+)\.(up|down)\.sql$`)

// TestEmbeddedMigrations enforces the layout golang-migrate relies on: every
// version has exactly one up and one down file and versions are contiguous
// from 1.
func TestEmbeddedMigrations(t *testing.T) {
	entries, err := files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	type pair struct{ name, up, down string }
	versions := map[uint64]*pair{}
	var highest uint64
	for _, e := range entries {
		m := fileName.FindStringSubmatch(e.Name())
		if m == nil {
			t.Errorf("%s: name must match %s", e.Name(), fileName)
			continue
		}
		v, err := strconv.ParseUint(m[1], 10, 32)
		if err != nil || v == 0 {
			t.Errorf("%s: version must be a positive integer", e.Name())
			continue
		}
		p := versions[v]
		if p == nil {
			p = &pair{name: m[2]}
			versions[v] = p
		}
		if p.name != m[2] {
			t.Errorf("version %d has files with different names: %q and %q", v, p.name, m[2])
		}
		body, err := files.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(body)) == "" {
			t.Errorf("%s: empty migration", e.Name())
		}
		if m[3] == "up" {
			p.up = e.Name()
		} else {
			p.down = e.Name()
		}
		highest = max(highest, v)
	}
	for v := uint64(1); v <= highest; v++ {
		p := versions[v]
		switch {
		case p == nil:
			t.Errorf("version %d: missing migration (versions must be contiguous)", v)
		case p.up == "":
			t.Errorf("version %d: missing up migration", v)
		case p.down == "":
			t.Errorf("version %d: missing down migration", v)
		}
	}
	if len(versions) != int(highest) {
		t.Errorf("duplicate versions with different zero padding")
	}

	latest, err := Latest()
	if err != nil {
		t.Fatal(err)
	}
	if uint64(latest) != highest {
		t.Errorf("Latest() = %d, want %d", latest, highest)
	}
	if RequiredSchemaVersion < 1 || RequiredSchemaVersion > latest {
		t.Errorf("RequiredSchemaVersion = %d, must be between 1 and the latest migration %d", RequiredSchemaVersion, latest)
	}
}

func TestEffective(t *testing.T) {
	for _, tc := range []struct {
		s    Status
		want uint
	}{
		{Status{}, 0},
		{Status{Initialized: true, Version: -1}, 0},
		{Status{Initialized: true, Version: 1}, 1},
		{Status{Initialized: true, Version: 1, Dirty: true}, 0},
		{Status{Initialized: true, Version: 5}, 5},
		{Status{Initialized: true, Version: 5, Dirty: true}, 4},
	} {
		if got := tc.s.Effective(); got != tc.want {
			t.Errorf("%+v.Effective() = %d, want %d", tc.s, got, tc.want)
		}
	}
}

func TestCompatible(t *testing.T) {
	for _, tc := range []struct {
		name     string
		s        Status
		required uint
		wantErr  string
	}{
		{"equal", Status{Initialized: true, Version: 3}, 3, ""},
		{"newer schema", Status{Initialized: true, Version: 7}, 3, ""},
		{"older schema", Status{Initialized: true, Version: 2}, 3, "older than version 3"},
		{"dirty above required", Status{Initialized: true, Version: 4, Dirty: true}, 3, ""},
		{"dirty at required", Status{Initialized: true, Version: 3, Dirty: true}, 3, "is dirty"},
		{"dirty below required", Status{Initialized: true, Version: 2, Dirty: true}, 3, "is dirty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := compatible(tc.s, tc.required)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Errorf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestRequiredVersion(t *testing.T) {
	if got := RequiredVersion(0); got != RequiredSchemaVersion {
		t.Errorf("RequiredVersion(0) = %d, want %d", got, RequiredSchemaVersion)
	}
	if got := RequiredVersion(42); got != 42 {
		t.Errorf("RequiredVersion(42) = %d, want 42", got)
	}
}

func TestNotReachable(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{&net.OpError{Op: "dial", Err: errors.New("connection refused")}, true},
		{fmt.Errorf("wrapped: %w", &net.DNSError{Err: "no such host", IsNotFound: true}), true},
		{io.ErrUnexpectedEOF, true},
		{&pgconn.PgError{Code: pgerrcode.CannotConnectNow}, true},
		{&pgconn.PgError{Code: pgerrcode.TooManyConnections}, true},
		{&pgconn.PgError{Code: pgerrcode.InvalidPassword}, false},
		{&pgconn.PgError{Code: pgerrcode.InvalidCatalogName}, false},
		{errors.New("tls: failed to verify certificate"), false},
	} {
		if got := notReachable(tc.err); got != tc.want {
			t.Errorf("notReachable(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestConnectWaitsForUnreachableDatabase(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().(*net.TCPAddr)
	l.Close() // nothing listens on the port any more

	var attempts int
	o := Options{
		ConnectTimeout: time.Second,
		WaitTimeout:    1500 * time.Millisecond,
		Logf:           func(string, ...any) { attempts++ },
	}
	dsn := fmt.Sprintf("host=127.0.0.1 port=%d user=u password=p dbname=d sslmode=disable", addr.Port)
	start := time.Now()
	_, err = Connect(context.Background(), dsn, o)
	if err == nil || !strings.Contains(err.Error(), "still not reachable after 1.5s") {
		t.Fatalf("Connect() error = %v, want timeout", err)
	}
	if elapsed := time.Since(start); elapsed < o.WaitTimeout {
		t.Errorf("Connect() returned after %s, want at least %s", elapsed, o.WaitTimeout)
	}
	if attempts == 0 {
		t.Error("Connect() did not report any retry")
	}

	o.WaitTimeout = 0
	if _, err := Connect(context.Background(), dsn, o); err == nil || strings.Contains(err.Error(), "still not reachable") {
		t.Errorf("Connect() without wait error = %v, want the connection error", err)
	}
}
