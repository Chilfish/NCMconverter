//go:build windows

package main

import (
	"os"
	"syscall"
)

// attachConsole attaches the process to the console it was started from, so
// that a windowsgui build still prints when run from a terminal. It does
// nothing when there is no console to attach to, as when the app is started
// from Explorer or as a service.
//
// It uses only the standard library's syscall, so the binary keeps building
// without cgo.
func attachConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	attach := kernel32.NewProc("AttachConsole")
	const attachParentProcess = ^uintptr(0) // DWORD(-1)
	if ok, _, _ := attach.Call(attachParentProcess); ok == 0 {
		return
	}
	if out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = out, out
	}
	if in, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
		os.Stdin = in
	}
}
