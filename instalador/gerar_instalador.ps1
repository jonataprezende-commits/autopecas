# Gera o instalador do AutoPecas: programa + manual compactados dentro de um
# unico AutoPecas-Setup-<versao>.exe (instala por usuario, sem admin).
#
# Uso, na pasta raiz do repo:  powershell -ExecutionPolicy Bypass -File instalador\gerar_instalador.ps1
# Saida: dist\AutoPecas.exe e dist\AutoPecas-Setup-<versao>.exe

$ErrorActionPreference = "Stop"
$raiz = Split-Path -Parent $PSScriptRoot

$go = (Get-Command go -ErrorAction SilentlyContinue).Source
if (-not $go) { $go = "C:\Program Files\Go\bin\go.exe" }
if (-not (Test-Path -LiteralPath $go)) { throw "Go nao encontrado. Instale com: winget install GoLang.Go" }

$versao = (Select-String -Path (Join-Path $raiz "codigo\main.go") -Pattern 'const appVersion = "([0-9.]+)"').Matches[0].Groups[1].Value
Write-Output "AutoPecas versao $versao"

$dist = Join-Path $raiz "dist"
New-Item -ItemType Directory -Path $dist -Force | Out-Null

Write-Output "1/3 compilando o programa..."
Push-Location (Join-Path $raiz "codigo")
try { & $go build -trimpath -ldflags "-s -w" -o (Join-Path $dist "AutoPecas.exe") . } finally { Pop-Location }
if ($LASTEXITCODE -ne 0) { throw "falha ao compilar o programa" }

Write-Output "2/3 compactando programa + manual..."
& python (Join-Path $raiz "instalador\gerar_payload.py") --programa (Join-Path $dist "AutoPecas.exe") --manual (Join-Path $raiz "manual\Manual-AutoPecas.pdf") --saida (Join-Path $raiz "instalador\payload")
if ($LASTEXITCODE -ne 0) { throw "falha ao gerar o payload" }

Write-Output "3/3 montando o instalador..."
$env:CGO_ENABLED = "0"
Push-Location (Join-Path $raiz "instalador")
try {
    & $go build -trimpath -ldflags "-H windowsgui -s -w" -o (Join-Path $dist "AutoPecas-Setup-$versao.exe") .
} finally { Pop-Location }
if ($LASTEXITCODE -ne 0) { throw "falha ao montar o instalador" }

Get-Item (Join-Path $dist "AutoPecas.exe"), (Join-Path $dist "AutoPecas-Setup-$versao.exe") |
    ForEach-Object { "{0}  {1:N1} MB" -f $_.Name, ($_.Length / 1MB) }
