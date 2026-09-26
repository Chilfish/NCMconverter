package ncm

import "errors"

// ErrExtNcm reports that a file does not use the .ncm extension.
var ErrExtNcm = errors.New("file should have ext .ncm")

// ErrMagicHeader reports that the leading bytes of a file are not the magic
// header of an NCM container.
var ErrMagicHeader = errors.New("magic header does not match")
