# AutoPecas — notas para agentes

Sistema de gestão para loja de autopeças. Programa local (offline), banco
SQLite, interface web embutida no executável. Um único binário para Windows.

## Estrutura

- `codigo/` — todo o programa Go (pacote `main`).
  - `main.go` — flags, servidor HTTP, rotas, `appVersion`.
  - `db.go` — schema, `OpenDB`, migrações (`PRAGMA user_version`), helpers.
  - `api_*.go` — handlers HTTP por área (peças, vendas, estoque, xml, etc.).
  - `seguranca.go` — middleware anti-CSRF/Host/Origin + senha.
  - `sessao.go` — login, cookies e limite de tentativas.
  - `web/` — front embutido (`embed.FS`): HTML/CSS/JS vanilla.
  - `*_test.go` — testes de integração com banco real + `httptest`.
- `entrega/` — pacote de entrega (exemplos de XML, instalador).
- `testes/`, `manual/`, `instalador/` — QA em Python, documentação, build.

## Comandos

Rodar a partir de `codigo/`:

```
go build ./...
go vet ./...
go test ./...            # suíte completa (~3 min)
go test -run TestNome ./...
gofmt -w <arquivo>.go
```

Instalador (na raiz do repo): `powershell -ExecutionPolicy Bypass -File instalador\gerar_instalador.ps1`
→ `dist\AutoPecas.exe` + `dist\AutoPecas-Setup-<versão>.exe` (versão lida de `codigo/main.go`).
Teste sem sujar o PC: rodar o setup com `LOCALAPPDATA`/`APPDATA`/`USERPROFILE` apontando para uma pasta temporária e `-silencioso`/`/S`.

## Convenções e decisões

- **Nunca perder dados**: migrações são idempotentes e numeradas em
  `aplicarMigracao`; incremente `versaoEsquema` e adicione um `case`.
- **Colunas reais** (não invente outros nomes): `parts.code_norm`,
  `parts.search_blob`, `parts.created_at`, `parts.avg_cost_cents`.
- **Campos anuláveis** vão para JSON como número/null (`idOuNulo`), nunca como
  `{"Int64":0,"Valid":false}`.
- **Segurança já existe**: `X-AutoPecas: 1` em métodos mutantes, validação de
  Host/Origin/Sec-Fetch-Site, cookies `HttpOnly`+`SameSite=Strict`, limite de
  senha. Não recriar — estender se preciso.
- **Custo**: `cost_cents` = último preço pago; `avg_cost_cents` = custo médio
  ponderado (usado na valorização de estoque e relatórios). Entradas de compra
  atualizam a média via `calcularCustoMedio`; saídas/ajustes não.
- **Dependência única**: `modernc.org/sqlite` (pure-Go, sem CGO). Evite novas
  dependências.
- Testes podem usar `novaAppTeste(t)` (banco temporário + servidor real).

## Estado (v1.0.12)

- Custo médio ponderado implementado e coberto por testes.
- CI em `.github/workflows/ci.yml` (build + vet + testes).
- Front: overlay de carregando `#overlay-carregando`; use `comCarregando(() => fetch(...))`
  em qualquer chamada nova que possa demorar (o `api()` já faz isso).
- **Release v1.0.12 publicada** (tag `v1.0.12`, commit `23f6ef4`) com
  `AutoPecas-Setup-1.0.12.exe` (19,8 MB). Instalador gerado por
  `instalador/gerar_instalador.ps1` — `dist/` fica fora do git.
- Backlog em `CHANGELOG.md` (caixa/turno, devolução parcial, NFC-e etc.).
