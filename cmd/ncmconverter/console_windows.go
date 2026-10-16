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
	// A GUI subsystem build starts with no standard handles, so its streams
	// have to be pointed at the console it just attached to. A stream the
	// caller gave the process, as in `ncmconverter --dry-run > list.txt`, is
	// left alone: its output belongs in the file.
	useConsole(&os.Stdout, "CONOUT$", os.O_WRONLY)
	useConsole(&os.Stderr, "CONOUT$", os.O_WRONLY)
	useConsole(&os.Stdin, "CONIN$", os.O_RDONLY)
}

// useConsole points a standard stream at the console device when the process
// has no handle of its own for it.
func useConsole(stream **os.File, name string, flag int) {
	if hasHandle(*stream) {
		return
	}
	if file, err := os.OpenFile(name, flag, 0); err == nil {
		*stream = file
	}
}

// hasHandle reports whether the process already has a usable handle for the
// stream, as when the caller redirected it to a file or a pipe.
func hasHandle(stream *os.File) bool {
	if stream == nil {
		return false
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getFileType := kernel32.NewProc("GetFileType")
	const fileTypeUnknown = 0
	kind, _, _ := getFileType.Call(stream.Fd())
	return kind != fileTypeUnknown
}
