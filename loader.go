// Copyright 2017 Nordstrom, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package swaggrpc

// Helper function & types to load an in-memory proto file.

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
)

// Wrapper around bytes.Reader that implements io.ReadCloser.
type bytesReaderCloser struct {
	*bytes.Reader
}

// Closer implementation. No-op.
func (*bytesReaderCloser) Close() error {
	return nil
}

// Filename used for the in-memory proto file when parsing from memory.
const dummyFilename = "__dummy"

// Loads an in-memory proto definition into a single file descriptor. Returns any error encountered.
// Note that this will open "import"-ed files using os.Open (the default behavior of protoparse),
// which could introduce security issues if run on arbitrary input.
func loadProtoFromBytes(contents []byte) (*desc.FileDescriptor, error) {
	// Generate a fake wrapper for the dummy filename we'll provide.
	accessor := func(filename string) (io.ReadCloser, error) {
		if filename == dummyFilename {
			return &bytesReaderCloser{Reader: bytes.NewReader(contents)}, nil
		}

		// Fallback to the default implementation.
		return os.Open(filename)
	}

	parser := protoparse.Parser{Accessor: accessor}

	descs, err := parser.ParseFiles(dummyFilename)
	if err != nil {
		return nil, err
	}
	if len(descs) != 1 {
		// This should never happen by contract.
		return nil, fmt.Errorf("expected a single descriptor, got %d", len(descs))
	}
	return descs[0], nil
}
