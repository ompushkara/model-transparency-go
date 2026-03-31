// Copyright 2025 The Sigstore Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"bytes"
	"os"
	"testing"

	"github.com/sigstore/model-signing/cmd/model-signing/cli/options"
)

func TestPrintSignVerifyResult_stdout(t *testing.T) {
	saved := ro
	t.Cleanup(func() { ro = saved })

	tests := []struct {
		name     string
		opts     options.RootOptions
		verified bool
		message  string
		want     string
	}{
		{
			name:     "text",
			opts:     options.RootOptions{LogLevel: "info", Output: options.ResultOutputText},
			verified: true,
			message:  "ok",
			want:     "ok\n",
		},
		{
			name:     "json",
			opts:     options.RootOptions{LogLevel: "info", Output: options.ResultOutputJSON},
			verified: false,
			message:  "nope",
			want:     `{"verified":false,"message":"nope"}` + "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ro = &tt.opts
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			oldOut := os.Stdout
			os.Stdout = w
			errCh := make(chan error, 1)
			go func() {
				errCh <- printSignVerifyResult(tt.verified, tt.message)
				_ = w.Close()
			}()
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r)
			_ = r.Close()
			os.Stdout = oldOut
			if err := <-errCh; err != nil {
				t.Fatalf("printSignVerifyResult: %v", err)
			}
			if got := buf.String(); got != tt.want {
				t.Fatalf("stdout = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrintSignVerifyResult_silentNoWrite(t *testing.T) {
	saved := ro
	t.Cleanup(func() { ro = saved })

	ro = &options.RootOptions{LogLevel: "silent", Output: options.ResultOutputJSON}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut := os.Stdout
	os.Stdout = w
	errCh := make(chan error, 1)
	go func() {
		errCh <- printSignVerifyResult(true, "x")
		_ = w.Close()
	}()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	_ = r.Close()
	os.Stdout = oldOut
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no stdout, got %q", buf.String())
	}
}
