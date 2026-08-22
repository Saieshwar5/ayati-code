package vmagent

import "errors"

var errPathTraversal = errors.New("tar entry escapes workspace root")
