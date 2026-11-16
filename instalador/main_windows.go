//go:build windows

// Instalador do AutoPecas - assistente unico para Windows.
// Instala por usuario (nao pede senha de administrador), cria atalhos e
// oferece abrir o programa no final.
package main

import (
	"bytes"
	"compress/flate"
	"embed"
	"fmt"
	"io"
	"os"

	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

//go:embed payload/programa.deflate payload/manual.deflate
var payloadFS embed.FS

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	pRegisterClassExW  = user32.NewProc("RegisterClassExW")
	pCreateWindowExW   = user32.NewProc("CreateWindowExW")
	pDefWindowProcW    = user32.NewProc("DefWindowProcW")
	pShowWindow        = user32.NewProc("ShowWindow")
	pUpdateWindow      = user32.NewProc("UpdateWindow")
	pGetMessageW       = user32.NewProc("GetMessageW")
	pTranslateMessage  = user32.NewProc("TranslateMessage")
	pDispatchMessageW  = user32.NewProc("DispatchMessageW")
	pPostQuitMessage   = user32.NewProc("PostQuitMessage")
	pMessageBoxW       = user32.NewProc("MessageBoxW")
	pSetWindowTextW    = user32.NewProc("SetWindowTextW")
	pGetDpiForSystem   = user32.NewProc("GetDpiForSystem")
	pEnableWindow      = user32.NewProc("EnableWindow")
	pSendMessageW      = user32.NewProc("SendMessageW")
	pDestroyWindow     = user32.NewProc("DestroyWindow")
	pCreateFontW       = gdi32.NewProc("CreateFontW")
	pGetModuleHandleW  = kernel32.NewProc("GetModuleHandleW")
	pShellExecuteW     = shell32.NewProc("ShellExecuteW")
)

const (
	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	wsChild            = 0x40000000
	wsTabStop          = 0x00010000
	wsBorder           = 0x00800000
	bsPushButton       = 0x00000000
	bsDefPushButton    = 0x00000001
	wsCaption          = 0x00C00000
	wsSysMenu          = 0x00080000
	wsMinimizeBox      = 0x00020000
	ssLeft             = 0x00000000
	swShow             = 5
	mbYesNo            = 0x00000004
	mbIconInfo         = 0x00000040
	mbIconQuestion     = 0x00000020
	mbIconError        = 0x00000010
	mbIconWarning      = 0x00000030
	idcArrow           = 32512
	colorWindow        = 5
	colorBtnFace       = 15
	defaultCharset     = 1
	fwNormal           = 400
	fwBold             = 700
	swShownormal       = 1
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     syscall.Handle
	hIcon         syscall.Handle
	hCursor       syscall.Handle
	hbrBackground syscall.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       syscall.Handle
}

type msgStruct struct {
	hwnd    syscall.Handle
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

type point struct{ x, y int32 }

var (
	hMain    syscall.Handle
	hFont    uintptr
	hFontBig uintptr
	hInst    uintptr
	pastaDst string
	instalado bool
)

// alturaFonte converte altura negativa (tamanho em pixels) para uintptr
func alturaFonte(px int32) uintptr { return uintptr(uint32(px)) }

func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func msgBox(titulo, texto string, flags uint32) int {
	r, _, _ := pMessageBoxW.Call(0, uintptr(unsafe.Pointer(u16(texto))), uintptr(unsafe.Pointer(u16(titulo))), uintptr(flags))
	return int(r)
}

func criarLabel(parent syscall.Handle, texto string, x, y, w, h int, negrito bool, grande bool) syscall.Handle {
	font := hFont
	if negrito || grande {
		font = hFontBig
	}
	hw, _, _ := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(u16("STATIC"))), uintptr(unsafe.Pointer(u16(texto))),
		uintptr(wsChild|wsVisible|ssLeft),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		uintptr(parent), 0, uintptr(hInst), 0)
	if font != 0 {
		pSendMessageW.Call(hw, 0x0030 /*WM_SETFONT*/, uintptr(font), 1)
	}
	return syscall.Handle(hw)
}

func criarBotao(parent syscall.Handle, texto string, x, y, w, h, id int, padrao bool) syscall.Handle {
	estilo := uintptr(wsChild | wsVisible | wsTabStop | bsPushButton)
	if padrao {
		estilo |= bsDefPushButton
	}
	hw, _, _ := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(u16("BUTTON"))), uintptr(unsafe.Pointer(u16(texto))),
		estilo,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		uintptr(parent), uintptr(id), uintptr(hInst), 0)
	pSendMessageW.Call(hw, 0x0030, hFont, 1)
	return syscall.Handle(hw)
}

// pastaDestino: instala por usuario (sem senha de administrador)
func pastaDestino() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.Getenv("USERPROFILE")
	}
	return filepath.Join(base, "Programs", "AutoPecas")
}

func descomprimir(dados []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(dados))
	defer r.Close()
	var out bytes.Buffer
	if _, err := io.Copy(&out, r); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func carregarPayload(nome string) ([]byte, error) {
	dados, err := payloadFS.ReadFile(nome)
	if err != nil {
		return nil, err
	}
	return descomprimir(dados)
}

func escreverArquivo(caminho string, dados []byte) error {
	if err := os.MkdirAll(filepath.Dir(caminho), 0o755); err != nil {
		return err
	}
	return os.WriteFile(caminho, dados, 0o755)
}

// ---------- criacao de atalhos (.lnk) via COM, sem depender de PowerShell ----------

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidShellLink   = guid{0x00021401, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIShellLinkW   = guid{0x000214F9, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIPersistFile  = guid{0x0000010B, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	ole32            = syscall.NewLazyDLL("ole32.dll")
	pCoInitialize    = ole32.NewProc("CoInitialize")
	pCoUninitialize  = ole32.NewProc("CoUninitialize")
	pCoCreateInstance = ole32.NewProc("CoCreateInstance")
)

// comCall chama o metodo de indice idx da interface COM apontada por p
func comCall(p unsafe.Pointer, idx int, args ...uintptr) uintptr {
	vtable := *(*uintptr)(p)
	fn := *(*uintptr)(unsafe.Pointer(vtable + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	chamada := append([]uintptr{uintptr(p)}, args...)
	r, _, _ := syscall.SyscallN(fn, chamada...)
	return r
}

func comRelease(p unsafe.Pointer) {
	if p != nil {
		comCall(p, 2) // IUnknown::Release
	}
}

func criarAtalho(lnk, alvo, pastaTrabalho, descricao, argumentos string) error {
	pCoInitialize.Call(0)
	defer pCoUninitialize.Call()

	var psl unsafe.Pointer
	hr, _, _ := pCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidShellLink)), 0,
		1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(&iidIShellLinkW)),
		uintptr(unsafe.Pointer(&psl)))
	if int32(hr) != 0 || psl == nil {
		return fmt.Errorf("nao consegui criar o atalho (COM 0x%X)", uint32(hr))
	}
	defer comRelease(psl)

	// IShellLinkW: SetPath=20, SetWorkingDirectory=9, SetDescription=7, SetIconLocation=17
	comCall(psl, 20, uintptr(unsafe.Pointer(u16(alvo))))
	comCall(psl, 9, uintptr(unsafe.Pointer(u16(pastaTrabalho))))
	comCall(psl, 7, uintptr(unsafe.Pointer(u16(descricao))))
	comCall(psl, 17, uintptr(unsafe.Pointer(u16(alvo))), 0)
	if argumentos != "" {
		// SetArguments = 11 (usado pelo atalho "Encerrar AutoPecas")
		comCall(psl, 11, uintptr(unsafe.Pointer(u16(argumentos))))
	}

	// IPersistFile::Save
	var ppf unsafe.Pointer
	rqi := comCall(psl, 0 /*QueryInterface*/, uintptr(unsafe.Pointer(&iidIPersistFile)), uintptr(unsafe.Pointer(&ppf)))
	if int32(rqi) != 0 {
		return fmt.Errorf("nao consegui gravar o atalho (QI 0x%X)", uint32(rqi))
	}
	if ppf == nil {
		return fmt.Errorf("nao consegui gravar o atalho")
	}
	defer comRelease(ppf)
	_ = os.MkdirAll(filepath.Dir(lnk), 0o755)
	r := comCall(ppf, 6 /*Save*/, uintptr(unsafe.Pointer(u16(lnk))), 1)
	if int32(r) != 0 {
		return fmt.Errorf("falha ao salvar o atalho (0x%X)", uint32(r))
	}
	return nil
}

func instalar() error {
	dst := pastaDestino()
	pastaDst = dst
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("nao consegui criar a pasta %s: %v", dst, err)
	}

	exe, err := carregarPayload("payload/programa.deflate")
	if err != nil {
		return fmt.Errorf("arquivo do programa corrompido: %v", err)
	}
	pdf, err := carregarPayload("payload/manual.deflate")
	if err != nil {
		return fmt.Errorf("manual corrompido: %v", err)
	}

	caminhoExe := filepath.Join(dst, "AutoPecas.exe")
	if err := escreverArquivo(caminhoExe, exe); err != nil {
		return fmt.Errorf("nao consegui gravar o programa (feche o AutoPecas se estiver aberto): %v", err)
	}
	if err := escreverArquivo(filepath.Join(dst, "Manual-AutoPecas.pdf"), pdf); err != nil {
		return fmt.Errorf("nao consegui gravar o manual: %v", err)
	}
	_ = escreverArquivo(filepath.Join(dst, "LEIA-ME.txt"), []byte(leiaMe))
	desinstalar := fmt.Sprintf(scriptDesinstalar, dst, dst, dst, dst)
	_ = escreverArquivo(filepath.Join(dst, "Desinstalar.cmd"), []byte(desinstalar))

	// atalhos
	desktop := filepath.Join(os.Getenv("USERPROFILE"), "Desktop")
	if _, err := os.Stat(desktop); err != nil {
		desktop = filepath.Join(os.Getenv("USERPROFILE"), "OneDrive", "Desktop")
	}
	_ = criarAtalho(filepath.Join(desktop, "AutoPecas.lnk"), caminhoExe, dst, "AutoPecas - sistema de gestao da loja", "")
	menuIniciar := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs")
	_ = criarAtalho(filepath.Join(menuIniciar, "AutoPecas.lnk"), caminhoExe, dst, "AutoPecas - sistema de gestao da loja", "")
	_ = criarAtalho(filepath.Join(menuIniciar, "AutoPecas - Manual.lnk"), filepath.Join(dst, "Manual-AutoPecas.pdf"), dst, "Manual do AutoPecas", "")
	// item 11: fecha o sistema aberto (equivalente a opcao -encerrar)
	_ = criarAtalho(filepath.Join(menuIniciar, "Encerrar AutoPecas.lnk"), caminhoExe, dst, "Fechar o AutoPecas que estiver aberto", "-encerrar")
	_ = criarAtalho(filepath.Join(menuIniciar, "Desinstalar AutoPecas.lnk"), filepath.Join(dst, "Desinstalar.cmd"), dst, "Remover o AutoPecas", "")

	instalado = true
	return nil
}

const leiaMe = "AutoPecas 1.0.11 - sistema de gestao para loja de autopecas\r\n" +
	"=========================================================\r\n\r\n" +
	"COMO ABRIR\r\n" +
	"1. Clique duas vezes no atalho 'AutoPecas' da Area de Trabalho.\r\n" +
	"2. Uma janela preta aparece (e o motor do sistema). Deixe-a aberta.\r\n" +
	"3. O sistema abre sozinho no navegador, no endereco http://127.0.0.1:8787\r\n\r\n" +
	"ONDE FICAM OS DADOS\r\n" +
	"%%APPDATA%%\\AutoPecas\\autopecas.db (banco de dados)\r\n" +
	"%%APPDATA%%\\AutoPecas\\backups (backups)\r\n\r\n" +
	"BACKUP\r\n" +
	"No sistema, abra o menu 'Backup e Ajustes' e clique em 'Fazer backup agora'.\r\n" +
	"O backup e um arquivo unico (.db) que pode ser copiado para um pen drive.\r\n\r\n" +
	"MANUAL\r\n" +
	"O manual completo esta no arquivo Manual-AutoPecas.pdf nesta pasta.\r\n\r\n" +
	"Funciona 100%% offline. Nao precisa de internet e nao tem mensalidade.\r\n"

const scriptDesinstalar = "@echo off\r\n" +
	"chcp 65001 >nul\r\n" +
	"echo Removendo o AutoPecas...\r\n" +
	"echo.\r\n" +
	"echo ATENCAO: os seus DADOS (pecas, vendas, clientes) NAO serao apagados.\r\n" +
	"echo Eles ficam em %%APPDATA%%\\AutoPecas e podem ser restaurados depois.\r\n" +
	"echo.\r\n" +
	"pause\r\n" +
	"taskkill /IM AutoPecas.exe /F >nul 2>&1\r\n" +
	"del /q \"%s\\AutoPecas.exe\" >nul 2>&1\r\n" +
	"del /q \"%s\\Manual-AutoPecas.pdf\" >nul 2>&1\r\n" +
	"del /q \"%s\\LEIA-ME.txt\" >nul 2>&1\r\n" +
	"del /q \"%%USERPROFILE%%\\Desktop\\AutoPecas.lnk\" >nul 2>&1\r\n" +
	"del /q \"%%APPDATA%%\\Microsoft\\Windows\\Start Menu\\Programs\\AutoPecas.lnk\" >nul 2>&1\r\n" +
	"del /q \"%%APPDATA%%\\Microsoft\\Windows\\Start Menu\\Programs\\AutoPecas - Manual.lnk\" >nul 2>&1\r\n" +
	"del /q \"%%APPDATA%%\\Microsoft\\Windows\\Start Menu\\Programs\\Encerrar AutoPecas.lnk\" >nul 2>&1\r\n" +
	"echo.\r\n" +
	"echo AutoPecas removido. Seus dados continuam em %%APPDATA%%\\AutoPecas\r\n" +
	"pause\r\n" +
	"del /q \"%s\\Desinstalar.cmd\" >nul 2>&1\r\n"

func abrirPrograma() {
	exe := filepath.Join(pastaDst, "AutoPecas.exe")
	r, _, _ := pShellExecuteW.Call(0,
		uintptr(unsafe.Pointer(u16("open"))),
		uintptr(unsafe.Pointer(u16(exe))),
		0, uintptr(unsafe.Pointer(u16(pastaDst))), swShownormal)
	if r <= 32 {
		msgBox("AutoPecas", "Nao consegui abrir o programa.\r\n\r\nAbra manualmente pelo atalho da Area de Trabalho.", mbIconWarning)
	}
}

func abrirManual() {
	pdf := filepath.Join(pastaDst, "Manual-AutoPecas.pdf")
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(u16("open"))), uintptr(unsafe.Pointer(u16(pdf))), 0, 0, swShownormal)
}

const (
	idInstalar = 101
	idSair     = 102
	WM_COMMAND = 0x0111
	WM_DESTROY = 0x0002
	WM_CLOSE   = 0x0010
)

func wndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		switch int(wParam & 0xffff) {
		case idInstalar:
			pEnableWindow.Call(uintptr(hwnd), 0)
			pSetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(u16("Instalando o AutoPecas..."))))
			if err := instalar(); err != nil {
				msgBox("Erro na instalacao", err.Error(), mbIconError)
				pEnableWindow.Call(uintptr(hwnd), 1)
				pSetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(u16("Instalacao do AutoPecas 1.0.11"))))
				return 0
			}
			r := msgBox("Instalacao concluida",
				"O AutoPecas foi instalado com sucesso!\r\n\r\n"+
					"Foi criado o atalho \"AutoPecas\" na Area de Trabalho.\r\n\r\n"+
					"Deseja abrir o programa agora?",
				mbYesNo|mbIconInfo)
			if r == 6 { // IDYES
				abrirPrograma()
			}
			r2 := msgBox("Manual do usuario",
				"O manual em PDF (pagina por pagina) foi instalado junto com o programa.\r\n\r\n"+
					"Deseja abrir o manual agora?", mbYesNo|mbIconQuestion)
			if r2 == 6 {
				abrirManual()
			}
			pDestroyWindow.Call(uintptr(hwnd))
			return 0
		case idSair:
			pDestroyWindow.Call(uintptr(hwnd))
			return 0
		}
	case WM_CLOSE:
		if !instalado {
			r := msgBox("AutoPecas", "Cancelar a instalacao?", mbYesNo|mbIconQuestion)
			if r != 6 {
				return 0
			}
		}
		pDestroyWindow.Call(uintptr(hwnd))
		return 0
	case WM_DESTROY:
		pPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

// instalacao silenciosa: AutoPecas-Setup.exe /S  (sem janela, bom para rede)
func silencioso() bool {
	args := strings.ToLower(strings.Join(os.Args[1:], " "))
	return strings.Contains(args, "/s") || strings.Contains(args, "-silencioso") || strings.Contains(args, "/silencioso")
}

func main() {
	hInst, _, _ = pGetModuleHandleW.Call(0)
	if silencioso() {
		if err := instalar(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	// fonte padrao usada nos textos e botoes
	hFont, _, _ = pCreateFontW.Call(
		alturaFonte(-15), 0, 0, 0, fwNormal, 0, 0, 0, defaultCharset, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(u16("Segoe UI"))))
	hFontBig, _, _ = pCreateFontW.Call(
		alturaFonte(-19), 0, 0, 0, fwBold, 0, 0, 0, defaultCharset, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(u16("Segoe UI"))))

	className := u16("AutoPecasSetup")
	wc := wndClassEx{
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     syscall.Handle(hInst),
		hbrBackground: syscall.Handle(colorBtnFace + 1),
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	estilo := uintptr(wsOverlappedWindow)
	hwnd, _, _ := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(u16("Instalacao do AutoPecas 1.0.11"))),
		estilo,
		170, 110, 600, 340,
		0, 0, uintptr(hInst), 0)
	hMain = syscall.Handle(hwnd)

	// icone da janela / cursor padrao
	user32.NewProc("LoadCursorW").Call(0, idcArrow)

	criarLabel(hMain, "AutoPecas 1.0.11", 28, 22, 480, 30, true, true)
	criarLabel(hMain, "Sistema de gestao para loja de autopecas - 100% offline", 30, 56, 500, 22, false, false)
	criarLabel(hMain, "Este assistente vai instalar o programa neste computador:", 30, 92, 500, 20, false, false)
	criarLabel(hMain, pastaDestino(), 30, 114, 500, 20, false, false)
	criarLabel(hMain, "O atalho \"AutoPecas\" sera criado na Area de Trabalho.", 30, 138, 500, 20, false, false)
	criarLabel(hMain, "Nao pede senha de administrador e nao baixa nada da internet.", 30, 160, 500, 20, false, false)
	criarLabel(hMain, "Seus dados ficam no computador e o backup e feito em 1 clique.", 30, 182, 500, 20, false, false)

	criarBotao(hMain, "Instalar agora", 300, 240, 160, 40, idInstalar, true)
	criarBotao(hMain, "Sair", 470, 240, 100, 40, idSair, false)

	pShowWindow.Call(hwnd, swShow)
	pUpdateWindow.Call(hwnd)

	var msg msgStruct
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
