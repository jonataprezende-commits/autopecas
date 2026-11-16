//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// espacoEmDisco devolve livre, total e o texto para a tela (Windows).
func espacoEmDisco(caminho string) map[string]any {
	var livre, total, totalLivres uint64
	p, err := syscall.UTF16PtrFromString(caminho)
	if err != nil {
		return map[string]any{"ok": false, "detalhe": err.Error()}
	}
	r, _, e := procGetDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&livre)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&totalLivres)))
	if r == 0 {
		return map[string]any{"ok": false, "detalhe": e.Error()}
	}
	return map[string]any{
		"ok": true, "livre_bytes": livre, "total_bytes": total,
		"texto": fmtBytes(livre) + " livres de " + fmtBytes(total),
	}
}
