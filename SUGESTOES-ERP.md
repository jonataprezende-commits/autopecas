# AutoPeças — sugestões de evolução do ERP

Baseado no código atual (1.0.1): Go + SQLite (WAL, 1 conexão), UI web embutida, dinheiro em
centavos, offline. As sugestões abaixo são ordenadas por **retorno para uma loja de autopeças**
× **esforço de implementação**, respeitando as decisões já tomadas (sem nuvem, sem dependências,
instalador único). Cada bloco indica a migração de esquema necessária (o versionamento por
`PRAGMA user_version` já está pronto, então cada uma é uma migração numerada nova).

---

## Prioridade 1 — dinheiro e operação do dia a dia

### 1. Caixa / turno de trabalho
**Por quê:** hoje "Fechamento do dia" é só uma consulta (`SUM` das vendas). Não existe abertura de
caixa, sangria, suprimento nem conferência da gaveta — o dono não consegue saber se faltou dinheiro
nem de quem foi o turno.
**Como:** `cash_sessions` (abertura, fundo inicial, fechamento, valor informado por forma de
pagamento, diferença) + `cash_moves` (sangria/suprimento/despesa). Tela "Caixa" com
abrir → vender → fechar, e bloqueio opcional de venda sem caixa aberto.
**Esforço:** médio (2–3 dias). **Migração:** v3.

### 2. Devolução / troca de venda (parcial)
**Por quê:** hoje só existe "Cancelar venda", que desfaz a venda inteira. O caso real é devolver
1 item de 3, ou trocar a peça errada.
**Como:** `sale_returns` + `sale_return_items`; devolve estoque (com `stock_moves` de entrada,
motivo "devolução"), gera crédito ao cliente ou estorno no recebível, e marca o item devolvido para
não devolver duas vezes (mesmo padrão do `received_at` do item 4).
**Esforço:** médio (2–3 dias). **Migração:** v4.

### 3. Custo médio ponderado (o lucro hoje mente)
**Por quê:** em `handleReceiveOrder` o custo da peça é **sobrescrito** pelo custo da última compra
(`UPDATE parts SET cost_cents=?`). Se a peça foi comprada a R$ 10 e depois a R$ 15, todo o lucro
retroativo muda, e o relatório de margem passa a mentir.
**Como:** coluna `avg_cost_cents` em `parts`, recalculada a cada entrada
(`(saldo·médio + qtd·custo) / (saldo+qtd)`); `stock_moves.unit_cost_cents` já guarda tudo, então dá
para recalcular o histórico com uma migração de dados. Usar `avg_cost_cents` nos relatórios de lucro
e no valor de estoque; manter `cost_cents` como "último custo" (útil para comprar).
**Esforço:** baixo-médio (1–2 dias). **Migração:** v5 + backfill.

### 4. Orçamento → venda
**Por quê:** o cliente pede preço e vai pensar. Hoje não há onde registrar isso sem sujar as vendas.
**Como:** tabela `quotes`/`quote_items` (ou um `sales.status='orcamento'` — **cuidado**: existem
várias consultas somando vendas por status; uma tabela separada é mais segura), validade,
impressão em A4/PDF reaproveitando o `pdf.go`, botão "Converter em venda" e "Enviar por WhatsApp".
**Esforço:** médio (2 dias). **Migração:** v6.

### 5. PIX copia-e-cola no cupom (offline)
**Por quê:** a maioria das vendas é PIX; digitar a chave na mão é fonte de erro.
**Como:** gerar o BR Code EMV (payload PIX + CRC16) a partir da chave PIX salva em `settings`, e
imprimir o QR no cupom. É cálculo puro — **não precisa de internet nem de API de banco**, cabe em
~150 linhas de Go e um QR gerado localmente. Mercado Pago/banco não é necessário.
**Esforço:** baixo (1 dia). **Sem migração** (só 1 chave nova em `settings`).

---

## Prioridade 2 — o que diferencia um sistema de autopeças

### 6. Aplicação por veículo ("cabe no meu carro?")
**Por quê:** é a pergunta nº 1 do balcão e hoje o sistema só tem `equivalents` (códigos
equivalentes) e texto livre. Um sistema de estoque genérico não tem isso — é o maior diferencial.
**Como:** `part_applications` (part_id, marca, modelo, ano_de, ano_fim, motor/observação) + tela de
busca "veículo → peças que servem" e filtro na busca da bancada. Importação do XML do fornecedor já
traz aplicação em muitos catálogos (vale mapear no `api_xml.go`).
**Esforço:** médio-alto (3–5 dias, mais a carga de dados). **Migração:** v7.

### 7. Código de barras + etiquetas
**Por quê:** quase toda peça tem código de barras do fabricante; digitar código é o gargalo do
balcão. Hoje `parts` não tem coluna de código de barras (só `code` e `mfr_code`).
**Como:** coluna `barcode` com índice (aceitar EAN-13 e o código interno), leitura por scanner
(scanner = teclado + Enter, já funciona no campo de busca) e impressão de etiqueta de gôndola
(Code128 desenhado em Go, direto no PDF que já existe).
**Esforço:** baixo-médio (2 dias). **Migração:** v8.

### 8. Tabelas de preço e margem mínima
**Por quê:** hoje há **um** preço por peça. Oficina/revenda compra com desconto; e nada impede vender
abaixo do custo por erro de digitação.
**Como:** `price_cents` (varejo) + `price_wholesale_cents`/`price_shop_cents` opcionais, seletor no
carrinho, e um alerta quando o desconto derruba a margem abaixo do mínimo configurado.
**Esforço:** baixo (1–2 dias). **Migração:** v9.

### 9. Aviso de "peça chegou" por WhatsApp
**Por quê:** as fichas de falta guardam telefone e cliente; quando o pedido é recebido, ninguém avisa
o cliente e a peça fica parada.
**Como:** na tela de pedido recebido e na ficha, botão "Avisar cliente" que abre
`https://wa.me/55<telefone>?text=...` (sem API, sem mensalidade). Depois de 1.0.1 o recebimento já
sabe exatamente quais fichas fecharam — o gancho está pronto.
**Esforço:** baixo (meio dia). **Sem migração.**

---

## Prioridade 3 — confiabilidade e gestão

### 10. Usuários e permissões
**Por quê:** não existe login. Com `-host` aberto na rede, qualquer pessoa com a senha única vê
**custo e lucro** de tudo e pode cancelar vendas. Um vendedor não deveria ver margem.
**Como:** `users` (nome, hash scrypt/PBKDF2 — só stdlib), sessão em cookie, papéis
(dono/gerente/vendedor), e registro de "quem fez o quê" nas operações sensíveis (cancelamento,
desconto acima do limite, baixa de recebível). Observação: `golang.org/x/crypto` seria uma dependência
nova — precisa da sua autorização, ou usar `crypto/pbkdf2` do stdlib (Go 1.24 já tem).
**Esforço:** médio-alto (3–4 dias). **Migração:** v10.

### 11. Não travar o caixa quando o gerente abre um relatório
**Por quê:** o banco usa **uma conexão** (`SetMaxOpenConns(1)`). Em rede, um relatório pesado (ou o
`VACUUM INTO` do backup) faz o segundo computador esperar até o `busy_timeout` de 8 s. Hoje isso
passa despercebido com 1 máquina; com 2–3 terminais vira reclamação.
**Como:** abrir um **segundo `*sql.DB` somente leitura** (`?mode=ro`) com `MaxOpenConns(N)` para
dashboards/relatórios/consultas; WAL já permite leitores simultâneos. O handle de escrita continua
com 1 conexão (evita `SQLITE_BUSY` em transação). É a mudança de maior impacto por linha escrita.
**Esforço:** baixo (meio dia + testes de concorrência). **Sem migração.**

### 12. Inventário / contagem
**Por quê:** estoque é `REAL` (float) e vai sendo ajustado por venda, recebimento e movimento
manual. Sem contagem periódica a divergência aparece só no prejuízo.
**Como:** tela de contagem (por categoria/família), relatório de divergência, ajuste gerando
`stock_moves` "inventário"; e um relatório de conferência `SUM(stock_moves.qty_delta) = parts.stock`
para achar inconsistência de graça.
**Esforço:** baixo-médio (2 dias). **Migração:** v11 (só se quiser guardar as contagens).

### 13. Relatórios que faltam (todos só leitura, baratos)
- **Curva ABC** (quais 20% das peças fazem 80% do faturamento).
- **Giro e cobertura** de estoque (quantos dias de venda ainda tem).
- **Sem venda há X dias** (capital parado — ótimo para promoção).
- **Comparativo mês a mês** e **margem por categoria/marca**.
- **DRE simplificado** do mês (vendas − custo − despesas de caixa).
- **Clientes inativos** e **fichas antigas por fornecedor** (cobrar quem não entregou).
**Esforço:** baixo por relatório (2–4 h cada, reaproveita `buildReport` + CSV + PDF).

### 14. Reposição automática por giro
**Por quê:** hoje o alerta é `stock <= min_stock`, com mínimo digitado à mão (e frequentemente
esquecido). A sugestão de compra poderia sair da velocidade de venda.
**Como:** ponto de pedido = média de venda dos últimos 90 dias × prazo de entrega + estoque de
segurança; tela "Sugestão de compra" já agrupada por fornecedor, gerando pedido com 1 clique
(reaproveita `handleGenerateOrders`).
**Esforço:** médio (2 dias).

---

## Prioridade 4 — avaliar custo/benefício

### 15. Documento fiscal (NFC-e / NF-e)
Em quase todos os estados a venda no varejo exige **NFC-e**; hoje o cupom é não fiscal. É o maior
buraco funcional, mas também o maior projeto: certificado A1, assinatura XML, SEFAZ por UF,
contingência, cancelamento, manifestação do destinatário. Recomendação: **não** tentar fazer sozinho
no curto prazo — avaliar um provedor de API fiscal (pago, mas pronto) ou uma biblioteca dedicada, e
tratar como projeto separado com prazo e orçamento próprios. Já existe `api_xml.go` para importar NF
de compra, então metade da infraestrutura XML está lá.

### 16. Backup verificado + cópia em nuvem
O backup já vai para uma segunda pasta (pendrive/Google Drive). Próximo passo: **testar** o backup
depois de gerado (abrir em banco temporário e rodar `PRAGMA quick_check` — a função
`validarArquivoBackup` do item 3 já faz exatamente isso) e mostrar "backup verificado ✔" na tela.
**Esforço:** baixo (2 h).

### 17. Rotina de qualidade
A suíte de testes da 1.0.1 (24 testes, `go test -race` verde) deveria entrar num script de build
(`build.ps1`) que roda `go vet` + `go test` + gera exe/payload/instalador, para nunca mais publicar
com regressão. **Esforço:** baixo.

---

## O que eu **não** faria

- **Trocar SQLite por servidor de banco.** Com WAL + um handle de escrita e um de leitura, 2–3
  terminais vão bem; a instalação de 1 clique é um ativo do produto.
- **Reescrever o front em framework** (React/Vue). A UI atual é rápida, sem build e sem CDN; o custo
  de reescrita não se paga.
- **Depender de nuvem** para vender ou emitir cupom. A promessa "100% offline" é o que fecha a venda
  numa loja com internet ruim.
- **Login por usuário antes** de resolver caixa e custo médio: são menos trabalho e doem mais no
  dia a dia.

---

## Sequência sugerida (versão 1.1)

1. Custo médio ponderado (item 3) + relatório de conferência de estoque (item 12) — 2 dias, corrige
   números que hoje enganam.
2. Caixa/turno (item 1) — 3 dias, é o que o dono cobra primeiro.
3. Devolução parcial (item 2) — 3 dias.
4. Barcode + etiqueta (item 7) e WhatsApp de ficha (item 9) — 2,5 dias, ganho imediato no balcão.
5. Handle de leitura separado (item 11) — meio dia, antes de aparecer reclamação com 2 terminais.
6. Orçamento (item 4) e PIX no cupom (item 5) — 3 dias.

Total ≈ 2 semanas de trabalho para uma 1.1 redonda; **aplicação por veículo (item 6)** entra como 1.2
porque depende de carga de dados e é o maior diferencial comercial.
