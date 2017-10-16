#!/usr/bin/env bash

# Check for valid formatting. Print unformatted files & exit with error, if there are any.
MALFORMED_FILES=(`find . -name '*.go' -type f -not -path '*/vendor/*' -exec gofmt -l '{}' ';'`)
if [[ -n "$MALFORMED_FILES" ]]; then
	echo "gofmt must be run on file(s):"
  for file in ${MALFORMED_FILES[@]}; do
    echo "  $file"
  done
	exit 1
fi
