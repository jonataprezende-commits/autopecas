# Catálogo do fornecedor — decisão, medição e implementação (1.0.4)

Status: **IMPLEMENTADO na 1.0.4** e ampliado na **1.0.5** (29/10/2026) com critérios
técnicos e o botão de consulta ao TecDoc.

## A pergunta do usuário

> "normalmente temos que procurar em catálogos de papel, o que podemos fazer pra mudar isso?"

## A medição que decidiu o caminho (antes de escrever código)

Script: `qa/spike_catalogo_papel.py` · resultado: `qa/spike-catalogo-resultado.json`
Página sintética de catálogo (20 linhas, 2 colunas, 1600×2100) com perspectiva, sombra e
compressão JPEG, lida com Tesseract 5.3.3 `por` (psm 6, agrupamento por TSV).

| Entrada | Linhas | Códigos exatos | Aplicações |
|---|---|---|---|
| Foto boa (apoiada) `qa/spike-catalogo-boa.jpg` | 14/20 | **10/20** | 13/20 |
| Foto de celular `qa/spike-catalogo-celular.jpg` | 14/20 | **5/20** | 13/20 |
| Texto digital | 20/20 | **20/20** | 20/20 |

Erros canônicos: `Pa-SO11` (PA-5011), `MEDINA` (MEDIDA), `JOGOA-RECAS` (JOGO 4 PEÇAS).

**Decisão: não OCR de catálogo.** Catálogo tem coluna estreita e código alfanumérico;
código errado no balcão vira peça errada na mão do cliente. O catálogo entra como **dado
estruturado** (digitado, CSV do fornecedor, XML); o OCR continua só na nota fiscal, que é
tabela larga e mede 8/8 itens.

## Desenho implementado

- Esquema do banco v2 → **v5** (`db.go`: v3 catálogo, v4 critérios, v5 índice FTS5 trigrama): tabelas `catalog` e `catalog_equivalents`,
  coluna `parts.application`; `buildSearchBlobApp` inclui a aplicação no texto de busca.
- `api_catalogo.go`: `GET/POST /api/catalogo`, `PUT/DELETE /api/catalogo/{id}`,
  `POST /api/catalogo/importar` (CSV `;` `,` ou tab, com aspas, cabeçalho opcional,
  idempotente por `code_norm`), `POST /api/catalogo/{id}/ficha`, `GET /api/busca`.
- O item de catálogo **sempre** volta com a situação do estoque (`tem_estoque`,
  `estoque`, `peca_codigo`), casando código próprio, código de fabricante e equivalentes.
- Busca tolerante ao ano: tenta todos os termos e, se não achar, repete sem o ano e
  ordena por `contaAcertosAno` (entende "08/16" como faixa).
- Tela `Catálogo` (menu, **F5**): busca rápida do balcão, tabela do catálogo, cadastro,
  importação de CSV, ficha de falta. Campo "Aplicação" no cadastro de peça.
- Aplicação vinda do XML deixou de ser descartada (`api_xml.go`).

## Invariantes (cobertos por teste)

1. Catálogo **não cria** peça nem movimento de estoque (`TestCatalogoNaoPoluiEstoque`).
2. Reimportar **atualiza**, não duplica (`TestCatalogoImportarCSV`, C8 no `.exe`).
3. Mesmo código digitado de novo atualiza a linha (`TestCatalogoCodigoRepetidoAtualiza`).
4. Ficha de falta nasce `aberta`, sem `part_id`, com código + aplicação + veículo.
5. Código com `;` dentro de aspas não desalinha colunas do CSV.

## Evidências de teste (1.0.4)

- Go: **56 PASS / 0 SKIP**, `gofmt` vazio, `go vet` limpo, `-race` OK nos testes novos.
- `.exe` instalado sob Wine: `qa/teste_crit_101.py` **47 OK / 0 falhas** (C8 novo, 13 checks).
- Navegador real: `qa/teste_ui_catalogo.py` **10 ok / 0**; `qa/teste_ui_102.py` **8/0**;
  `qa/teste_ui_nota.py` **13/0**; `node qa/teste_parsers.mjs` OK.
- Capturas: `qa/shots/catalogo-1-inicial.png` … `qa/shots/catalogo-6-final.png`.
- Manual: 38 páginas (2 novas, `qa/errata_manual_104.py`).

## Versão 1.0.5 — critérios técnicos e TecDoc

- **`catalog.criteria`** (migração 4): rosca, dentes, altura, perfil, encaixe. Entra no
  `search_blob`, então "rosca 3/4", "120 dentes" e "altura 78mm" acham a peça.
- Importação aceita a coluna como `criterios`, `critérios técnicos`, `especificações`,
  `atributos`, `rosca` ou `dentes`. `juntarCamposExtras` recolhe os `;` não escapados
  (antes, critério com `;` sem aspas desalinhava a linha).
- A ficha de falta gerada do catálogo leva os critérios na observação.
- ~~Botão TecDoc~~: **removido na 1.0.6 a pedido do usuário**. Nenhuma rota, botão ou texto de
  TecDoc permanece no programa. Motivo registrado: a licença do TecDoc Catalogue (inclusive a
  gratuita de 1 ano) autoriza consultar no site/app, mas o FAQ da TecAlliance diz que copiar os
  dados para outro sistema viola direito autoral e licença — integração só com contrato de
  dados B2B (`vendasbrasil@tecalliance.net`).
- **`POST /api/catalogo/semear`** + botão **"Montar com o que já tenho"**: o catálogo nasce das
  peças cadastradas (com aplicação e equivalentes) e das fichas de falta com `[CODIGO]`.
  Idempotente, grava em blocos de 500, não mexe no estoque.

## O que ficou fora de escopo (e por quê)

- Vender direto do catálogo: o carrinho só aceita peça com estoque; o caminho é a ficha
  de falta → pedido → recebimento (que já lança estoque uma única vez).
- Foto de catálogo (ver medição acima).
- Preço de venda sugerido a partir do custo do catálogo: hoje o catálogo guarda custo e a
  tela não sugere margem — candidato para a próxima versão.
