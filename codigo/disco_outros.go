//go:build !windows

package main

import "syscall"

// espacoEmDisco devolve livre, total e o texto para a tela (Linux/macOS).
func espacoEmDisco(caminho string) map[string]any {
	var st syscall.Statfs_t
	if err := syscall.Statfs(caminho, &st); err != nil {
		return map[string]any{"ok": false, "detalhe": err.Error()}
	}
	livre := uint64(st.Bavail) * uint64(st.Bsize)
	total := uint64(st.Blocks) * uint64(st.Bsize)
	return map[string]any{
		"ok": true, "livre_bytes": livre, "total_bytes": total,
		"texto": fmtBytes(livre) + " livres de " + fmtBytes(total),
	}
}
