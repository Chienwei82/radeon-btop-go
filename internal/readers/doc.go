// Package readers holds every parse of kernel files.
//
// Readers never panic and never propagate raw I/O errors as failures:
// the fsutil primitives return (value, ok) / nil-on-error, so a file
// vanishing between readdir and open is data ("absent"), not an error.
package readers
