//go:build windows

package main

// Item 8 - caixa de aviso nativa do Windows.
// O AutoPecas e compilado como aplicacao grafica (-H windowsgui), entao nao ha
// console para mostrar erro. Antes usavamos mshta com VBScript, que o Windows
// 11 esta bloqueando. Agora usamos MessageBoxW direto da API do Windows.

import (
	"golang.org/x/sys/windows"
)

func mostrarCaixaErro(msg string) {
	texto, err := windows.UTF16PtrFromString(msg)
	if err != nil {
		return
	}
	titulo, err := windows.UTF16PtrFromString(appName + " " + appVersion)
	if err != nil {
		return
	}
	_, _ = windows.MessageBox(0, texto, titulo, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
}
