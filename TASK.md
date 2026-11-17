# TASK — Módulo Estoque Inteligente (AutoPecas v1.1)

GOAL: Nova tela "Estoque Inteligente" no AutoPecas (v1.0.12 base): visão geral,
sugestões de compra (demanda/ponto de reposição/qtd sugerida/criticidade),
estoque parado, Curva ABC, comparação de períodos, geração de pedido de compra
integrada ao fluxo real (orders/order_items), configurações, export CSV/PDF e
suíte de testes (22+ cenários). Repo: jonataprezende-commits/autopecas.

## IMPORTANTE (histórico)
- A spec detalhada do usuário (fórmulas exatas, nomes de config) foi perdida na
  compactação — NÃO está no vault nem no repo. O desenho abaixo foi decidido por
  mim com fórmulas explícitas e documentadas, ajustáveis por settings. Foi
  registrado aqui para sobreviver a qualquer compactação futura.

## Design decidido

### Fórmulas (explícitas)
- `demanda_diaria` = soma(qtd vendida em sale_items, vendas status='concluida',
  últimos `ei_demanda_dias` dias) / `ei_demanda_dias`. Sem vendas no período →
  demanda = 0 (NUNCA inventar demanda sem histórico).
- `lead_dias` = `suppliers.prazo_entrega_dias` do fornecedor da peça (migração v8),
  senão config global `ei_lead_dias` (default 7).
- `estoque_seguranca` = demanda_diaria × `ei_seguranca_dias` (default 3).
- `ponto_reposicao` = demanda_diaria × (lead_dias + ei_seguranca_dias).
- `dias_ate_faltar` = estoque_atual / demanda_diaria (null se demanda=0).
- SUGERE quando: (demanda>0 e estoque ≤ ponto_reposicao) OU estoque < min_stock.
- `qtd_sugerida` = max(ponto_reposicao, min_stock) − estoque_atual, ceil, mín 1.
  Sem histórico (demanda=0, só regra mínima): qtd = min_stock − estoque_atual.
- CRITICIDADE (regras explícitas, em ordem):
  - `critica`: estoque ≤ 0 OU (demanda>0 e dias_ate_faltar ≤ lead_dias)
  - `media`: estoque ≤ ponto_reposicao (demanda>0)
  - `baixa`: estoque < min_stock (demanda=0 ou acima do ponto)
- PARADO: estoque > 0 e última saída (stock_moves kind!='entrada' OU venda) há mais
  de `ei_parado_dias` (default 90); ou nunca teve saída e created_at > 90 dias.
  Valor parado = estoque × avg_cost_cents. Ordena por valor desc.
- ABC: receita por peça = soma(sale_items.total_cents) em `ei_abc_dias` (default 90),
  status='concluida'. Ordena desc, % acumulado: A ≤ 80%, B ≤ 95%, C resto.
  Peças sem venda não entram.
- COMPARAR: dois períodos (de/ate). Para cada: receita, custo (avg_cost × qtd),
  margem, nº vendas, itens vendidos. + top 10 peças por crescimento de receita.

### Banco (migração v8)
- `colunaSeFaltar(suppliers, "prazo_entrega_dias", "INTEGER NOT NULL DEFAULT 0")`.
- Settings no firstRun: ei_lead_dias=7, ei_seguranca_dias=3, ei_demanda_dias=90,
  ei_parado_dias=90, ei_abc_dias=90.

### Rotas novas (main.go, padrão "GET /api/...")
- GET /api/estoque-inteligente/resumo
- GET /api/estoque-inteligente/sugestoes
- GET /api/estoque-inteligente/parados
- GET /api/estoque-inteligente/abc
- GET /api/estoque-inteligente/comparar?de1&ate1&de2&ate2
- GET /api/estoque-inteligente/config | POST /api/estoque-inteligente/config
- POST /api/estoque-inteligente/pedido  (gera pedido real, transacional)
- GET /api/estoque-inteligente/{lista}.csv  (sugestoes|parados|abc)
- GET /api/estoque-inteligente/sugestoes.pdf

### Pedido (regras)
- Transacional (a.db.Tx). Agrupa por fornecedor (peça.supplier_id; 0 → supplier NULL).
- Dentro do pedido, peças repetidas SOMAM qtd (nunca duplica item).
- NÃO altera estoque (só entra no recebimento, fluxo existente).
- NÃO usa fichas de falta (faltas ficam intactas; order_items.falta_id = NULL).
- Front avisa se a peça já está em pedido 'aberto' (badge), mas não bloqueia.
- Custo do item = avg_cost_cents da peça (senão cost_cents, senão 0).

## Passos
- [x] Baseline: repo limpo f99158d, build OK, testes >2min (rodar com timeout maior)
- [x] Mapear código: schema (db.go), migrações, pedidos (api_faltas.go), rotas (main.go), testutil
- [x] Ler: handleGetConfig/SaveConfig, api_suppliers (form), pdf.go, app.css, telas.js (tela exemplo), api_reports.go
- [x] db.go: migração v8 (suppliers.prazo_entrega_dias) + versaoEsquema=8 + settings firstRun
- [x] estoque_inteligente.go: cálculos puros (demanda, ponto, criticidade, parado, ABC, comparar)
- [x] api_estoque_inteligente.go: handlers + pedido + CSV/PDF
- [x] main.go: rotas novas
- [x] api_suppliers.go (api_people.go): campo prazo_entrega_dias no GET/POST
- [x] estoque_inteligente_test.go: 22+ cenários
- [x] Front: index.html (menu), telas.js (tela + abas + modal config + gerar pedido), app.css
- [x] gofmt/vet/test (-count=1), build
- [x] CHANGELOG + versao 1.1.0 + docs (manual/AGENTS)
- [ ] Instalador + commit + push + CI verde + Release

## NOW
Todas as funcionalidades implementadas e testadas. Próximo: gerar instalador, commit, push, CI e release v1.1.0.