//go:build !windows

package main

// attachConsole does nothing on platforms whose binaries keep the console they
// were started from.
func attachConsole() {}
