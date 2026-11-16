/* AutoPeças — interface (parte 2: estoque, fichas, pedidos, clientes, fornecedores,
   contas a receber, fechamento, relatórios, backup) */

/* ================= ESTOQUE ================= */
ROTAS.estoque = async function () {
  $("#conteudo").innerHTML = `
  <div class="topo">
    <div><h1>Estoque</h1><p class="sub">Entradas (compras), saídas, ajustes e baixas com motivo</p></div>
    <div class="acoes"><button class="btn" onclick="irPara('inicio')">Ver alertas</button></div>
  </div>
  <div class="bancada">
    <div>
      <div class="painel">
        <div class="painel-topo"><h3>Movimentar peça</h3><span class="muted small">Selecione a peça na busca abaixo</span></div>
        <div class="busca-grande">
          <input type="search" id="busca-estoque" placeholder="Buscar peça para movimentar…" autocomplete="off">
        </div>
        <div id="lista-estoque" class="resultados" style="max-height:220px">
          <div class="vazio">Busque a peça pelo código ou descrição.</div>
        </div>
      </div>
      <div class="painel">
        <div class="painel-topo"><h3>Últimas movimentações</h3>
          <div class="acoes"><input type="date" id="filtro-mov-data" style="width:auto">
          <button class="btn pequeno" onclick="carregarMovimentos()">Filtrar</button></div>
        </div>
        <div id="movimentos"><div class="vazio">Carregando…</div></div>
      </div>
    </div>
    <div>
      <div class="painel">
        <h3>⚠ Abaixo do mínimo</h3>
        <div id="lista-minimos" style="margin-top:10px"><div class="vazio">Carregando…</div></div>
      </div>
    </div>
  </div>`;

  campoBuscaPeca("busca-estoque", "lista-estoque", (p) => abrirMovimento(p));
  $("#filtro-mov-data").onchange = carregarMovimentos;
  await carregarMovimentos();
  await carregarMinimos();
};

async function carregarMovimentos() {
  const data = $("#filtro-mov-data") ? $("#filtro-mov-data").value : "";
  try {
    const r = await api("/estoque/movimentos?limite=200" + (data ? "&data=" + data : ""));
    const alvo = $("#movimentos");
    if (!alvo) return;
    if (!r.itens.length) { alvo.innerHTML = '<div class="vazio">Nenhuma movimentação registrada.</div>'; return; }
    alvo.innerHTML = `<div class="tabela-wrap" style="max-height:420px"><table><thead><tr>
      <th>Data</th><th>Peça</th><th>Tipo</th><th class="dir">Qtd</th><th class="dir">Saldo</th><th>Motivo</th></tr></thead><tbody>
      ${r.itens.map(m => `<tr>
        <td class="small">${dataHoraBR(m.data)}</td>
        <td><strong>${esc(m.descricao)}</strong><div class="muted small mono">${esc(m.codigo)}</div></td>
        <td><span class="tag ${m.tipo === "entrada" ? "verde" : m.tipo === "saida" ? "azul" : m.tipo === "baixa" ? "vermelho" : "amarelo"}">${esc(m.tipo)}</span></td>
        <td class="dir">${m.quantidade > 0 ? "+" : ""}${fmtQtd(m.quantidade)}</td>
        <td class="dir">${fmtQtd(m.saldo)}</td>
        <td class="small">${esc(m.motivo)}</td></tr>`).join("")}
      </tbody></table></div>`;
  } catch (e) { toast(e.message, "erro"); }
}
window.carregarMovimentos = carregarMovimentos;

async function carregarMinimos() {
  try {
    const r = await api("/pecas?abaixo_minimo=1&limite=100");
    const alvo = $("#lista-minimos");
    if (!alvo) return;
    if (!r.itens.length) { alvo.innerHTML = '<div class="vazio">Nenhuma peça abaixo do mínimo </div>'; return; }
    alvo.innerHTML = r.itens.map(p => `<div class="alerta-linha">
      <div><strong>${esc(p.descricao)}</strong><div class="muted small mono">${esc(p.codigo)} · mín ${fmtQtd(p.minimo)}</div></div>
      <div style="text-align:right"><span class="tag vermelho">${fmtQtd(p.estoque)}</span><br>
      <button class="btn pequeno" style="margin-top:4px" onclick='abrirMovimento(${JSON.stringify(p).replace(/'/g, "&#39;")})'>Entrada</button></div>
    </div>`).join("");
  } catch (e) {}
}

function abrirMovimento(p) {
  abrirModal("Movimentar estoque — " + p.descricao, `
    <div class="ok-box">Peça: <strong>${esc(p.descricao)}</strong> · <span class="mono">${esc(p.codigo)}</span>
      · estoque atual: <strong>${fmtQtd(p.estoque)}</strong> · mínimo ${fmtQtd(p.minimo)}</div>
    <div class="grade g2">
      <div class="campo"><label>Tipo de movimento</label>
        <select id="m-tipo">
          <option value="entrada">Entrada (compra / recebimento)</option>
          <option value="saida">Saída (venda / uso)</option>
          <option value="ajuste">Ajuste (corrigir saldo)</option>
          <option value="baixa">Baixa (perda, avaria, garantia)</option>
        </select></div>
      <div class="campo"><label id="m-qtd-rotulo">Quantidade</label>
        <input type="number" id="m-qtd" value="1" step="0.01" min="0"></div>
    </div>
    <div class="grade g2">
      <div class="campo"><label>Custo unitário (opcional, R$)</label><input type="text" id="m-custo" placeholder="Manter custo atual" inputmode="decimal"></div>
      <div class="campo"><label>Motivo</label><input type="text" id="m-motivo" placeholder="Ex.: compra fornecedor X / peça danificada"></div>
    </div>
    <div class="dica" id="m-dica">No ajuste, informe o saldo correto (a quantidade passa a ser o novo saldo).</div>
    `,
    `<button class="btn" onclick="fecharModal()">Cancelar</button>
     <button class="btn primario" onclick="salvarMovimento(${p.id})">Confirmar movimento</button>`);
  $("#m-tipo").onchange = () => {
    const t = $("#m-tipo").value;
    $("#m-qtd-rotulo").textContent = t === "ajuste" ? "Novo saldo" : "Quantidade";
    $("#m-motivo").placeholder = t === "entrada" ? "Ex.: NF 1234 fornecedor X" :
      t === "saida" ? "Ex.: venda balcão" : t === "baixa" ? "Ex.: peça danificada" : "Ex.: correção de inventário";
  };
  setTimeout(() => $("#m-qtd").select(), 80);
}
window.abrirMovimento = abrirMovimento;
window.movEstoqueRapido = function (id) {
  api("/pecas/" + id).then(p => abrirMovimento(p)).catch(e => toast(e.message, "erro"));
};

async function salvarMovimento(pecaId) {
  const corpo = {
    peca_id: pecaId,
    tipo: $("#m-tipo").value,
    quantidade: parseQtd($("#m-qtd").value),
    custo_centavos: parseDinheiro($("#m-custo").value),
    motivo: $("#m-motivo").value.trim()
  };
  try {
    const r = await api("/estoque/movimento", { corpo });
    toast("Estoque atualizado. Novo saldo: " + fmtQtd(r.peca.estoque), "ok");
    fecharModal();
    if (rotaAtual() === "estoque") { carregarMovimentos(); carregarMinimos(); }
    if (rotaAtual() === "pecas") carregarPecas();
    atualizarAlertas();
  } catch (e) { toast(e.message, "erro"); }
}
window.salvarMovimento = salvarMovimento;

/* ================= FICHAS DE FALTA ================= */
let FILTRO_FICHAS = "ativas";

ROTAS.fichas = async function () {
  $("#conteudo").innerHTML = `
  <div class="topo">
    <div><h1>Fichas de falta</h1><p class="sub">O caderninho digital: peças que o cliente pediu e não temos em estoque</p></div>
    <div class="acoes">
      <button class="btn primario" onclick="novaFicha()">+ Nova ficha (F4)</button>
      <button class="btn" onclick="gerarPedidos()">Gerar pedido por fornecedor</button>
    </div>
  </div>
  <div class="pill-tabs">
    <button data-f="ativas" class="ativo">Ativas (abertas + pedidas)</button>
    <button data-f="aberta">Só abertas</button>
    <button data-f="pedida">Só pedidas</button>
    <button data-f="recebida">Recebidas</button>
    <button data-f="antigas">Abertas há 7+ dias</button>
    <button data-f="todas">Todas</button>
  </div>
  <div class="painel"><div id="lista-fichas"><div class="vazio">Carregando…</div></div></div>`;

  $$(".pill-tabs button").forEach(b => {
    b.onclick = () => {
      $$(".pill-tabs button").forEach(x => x.classList.remove("ativo"));
      b.classList.add("ativo");
      FILTRO_FICHAS = b.dataset.f;
      carregarFichas();
    };
  });
  await carregarFichas();
};

async function carregarFichas() {
  let url = "/fichas?status=";
  if (FILTRO_FICHAS === "antigas") url = "/fichas?status=ativas&dias=7";
  else url += FILTRO_FICHAS;
  try {
    const r = await api(url);
    const alvo = $("#lista-fichas");
    if (!alvo) return;
    if (!r.itens.length) {
      alvo.innerHTML = '<div class="vazio">Nenhuma ficha aqui. Registre a próxima peça pedida em “Nova ficha (F4)”.</div>';
      return;
    }
    alvo.innerHTML = `<div class="tabela-wrap"><table><thead><tr>
      <th>#</th><th>Data</th><th>Peça pedida</th><th class="dir">Qtd</th><th>Cliente</th><th>Veículo</th>
      <th>Fornecedor</th><th>Status</th><th class="dir">Dias</th><th></th></tr></thead><tbody>
      ${r.itens.map(f => `<tr>
        <td>#${f.id}</td>
        <td class="small">${dataBR(f.criada_em)}</td>
        <td><strong>${esc(f.descricao)}</strong>${f.peca_codigo ? `<div class="muted small mono">${esc(f.peca_codigo)}</div>` : '<div class="muted small">peça não cadastrada</div>'}</td>
        <td class="dir">${fmtQtd(f.quantidade)}</td>
        <td>${esc(f.cliente_nome || "—")}${f.cliente_telefone ? `<div class="muted small">${esc(f.cliente_telefone)}</div>` : ""}</td>
        <td class="small">${esc(f.veiculo || "—")}</td>
        <td class="small">${esc(f.fornecedor_nome || "—")}</td>
        <td>${tagStatusFicha(f.status)}</td>
        <td class="dir ${f.dias_aberta >= 7 && f.status !== "recebida" ? "erro" : ""}">${f.dias_aberta}</td>
        <td class="dir" style="white-space:nowrap">
          ${f.status !== "recebida" ? `<button class="btn pequeno verde" onclick="receberFicha(${f.id},${f.peca_id || 0},${f.quantidade})">Receber</button>` : ""}
          ${f.status === "aberta" ? `<button class="btn pequeno" onclick="marcarPedida(${f.id})">Pedida</button>` : ""}
          <button class="btn pequeno" onclick="editarFicha(${f.id})">Editar</button>
        </td>
      </tr>`).join("")}
      </tbody></table></div>
      <p class="dica">${r.itens.length} ficha(s). “Receber” dá entrada no estoque e fecha a ficha.</p>`;
  } catch (e) { toast(e.message, "erro"); }
}
window.carregarFichas = carregarFichas;

function tagStatusFicha(s) {
  const map = { aberta: ["amarelo", "aberta"], pedida: ["azul", "pedida"], recebida: ["verde", "recebida"], cancelada: ["cinza", "cancelada"] };
  const m = map[s] || ["cinza", s];
  return `<span class="tag ${m[0]}">${m[1]}</span>`;
}

async function novaFicha(id) {
  let f = { id: 0, peca_id: null, descricao: "", quantidade: 1, cliente_id: null, cliente_nome: "",
    cliente_telefone: "", veiculo: "", fornecedor_id: null, custo_centavos: 0, observacoes: "" };
  if (id) {
    try {
      const r = await api("/fichas?status=todas");
      const achou = r.itens.find(x => x.id === id);
      if (achou) f = { ...achou, custo_centavos: achou.custo_centavos, cliente_id: achou.cliente_id };
    } catch (e) {}
  }
  const forn = await api("/fornecedores");
  const cli = await api("/clientes?q=");
  window.__fichaPeca = f.peca_id ? { id: f.peca_id, descricao: f.descricao } : null;

  abrirModal(id ? "Editar ficha #" + id : "Nova ficha de falta", `
    <div class="campo">
      <label>1. Peça (busque no cadastro — se ainda não existir, digite a descrição livre)</label>
      <input type="search" id="f-busca" placeholder="Digite o código ou descrição…" autocomplete="off">
      <div id="f-resultado" class="resultados" style="max-height:170px;margin-top:6px">
        <div class="vazio">Busque a peça. Se não encontrar, continue e escreva a descrição abaixo.</div>
      </div>
      <div id="f-peca-escolhida" style="margin-top:8px"></div>
    </div>
    <div class="campo"><label>Descrição da peça pedida *</label>
      <input type="text" id="f-desc" value="${esc(f.descricao)}" placeholder="Ex.: Bobina de ignição Corsa 1.4"></div>
    <div class="grade g3">
      <div class="campo"><label>Quantidade</label><input type="number" id="f-qtd" value="${f.quantidade}" min="0.01" step="1"></div>
      <div class="campo"><label>Custo estimado (R$)</label><input type="text" id="f-custo" value="${(f.custo_centavos / 100).toFixed(2).replace(".", ",")}" inputmode="decimal"></div>
      <div class="campo"><label>Fornecedor</label><select id="f-forn"><option value="">— escolher depois —</option>
        ${forn.itens.map(x => `<option value="${x.id}" ${f.fornecedor_id === x.id ? "selected" : ""}>${esc(x.nome)}</option>`).join("")}</select></div>
    </div>
    <div class="grade g3">
      <div class="campo"><label>Cliente</label>
        <input type="text" id="f-cliente" value="${esc(f.cliente_nome || "")}" list="f-clientes" placeholder="Nome do cliente">
        <datalist id="f-clientes">${cli.itens.map(c => `<option value="${esc(c.nome)}" data-id="${c.id}"></option>`).join("")}</datalist></div>
      <div class="campo"><label>Telefone</label><input type="tel" id="f-fone" value="${esc(f.cliente_telefone || "")}" placeholder="(21) 99999-0000"></div>
      <div class="campo"><label>Veículo</label><input type="text" id="f-veiculo" value="${esc(f.veiculo || "")}" placeholder="Modelo / placa / ano"></div>
    </div>
    <div class="campo"><label>Observações</label><input type="text" id="f-obs" value="${esc(f.observacoes || "")}" placeholder="Ex.: cliente precisa até sexta"></div>
    `,
    `<button class="btn" onclick="fecharModal()">Cancelar</button>
     <button class="btn primario" onclick="salvarFicha(${id || 0})">Salvar ficha</button>`, "min(820px,96vw)");

  const inp = $("#f-busca");
  const buscar = debounce(async () => {
    const q = inp.value.trim();
    if (!q) return;
    const r = await api("/pecas?q=" + encodeURIComponent(q) + "&limite=20");
    $("#f-resultado").innerHTML = r.itens.length
      ? r.itens.map(p => `<div class="res-item" data-id="${p.id}" data-desc="${esc(p.descricao)}">
          <div><div class="d1">${esc(p.descricao)}</div><div class="d2 mono">${esc(p.codigo)} · estoque ${fmtQtd(p.estoque)}</div></div>
          <div class="preco">${fmtMoney(p.preco_centavos)}</div></div>`).join("")
      : '<div class="vazio">Nenhuma peça cadastrada com esse texto. A ficha pode ser salva só com a descrição livre.</div>';
    $$("#f-resultado .res-item").forEach(el => el.onclick = () => {
      window.__fichaPeca = { id: Number(el.dataset.id), descricao: el.dataset.desc };
      $("#f-desc").value = el.dataset.desc;
      $("#f-resultado").innerHTML = "";
      inp.value = "";
      api("/pecas/" + el.dataset.id).then(p => { $("#f-custo").value = (p.custo_centavos / 100).toFixed(2).replace(".", ","); }).catch(() => {});
      mostrarPecaEscolhida();
    });
  }, 200);
  inp.oninput = buscar;
  mostrarPecaEscolhida();
  setTimeout(() => inp.focus(), 80);
}
window.novaFicha = novaFicha;
function mostrarPecaEscolhida() {
  const alvo = $("#f-peca-escolhida");
  if (!alvo) return;
  const p = window.__fichaPeca;
  alvo.innerHTML = p
    ? `<span class="chip">Peça vinculada: ${esc(p.descricao)} <button onclick="window.__fichaPeca=null;mostrarPecaEscolhida()">✕</button></span>`
    : '<span class="muted small">Sem peça vinculada — a ficha fica com descrição livre (vincule depois para dar entrada no estoque).</span>';
}
window.mostrarPecaEscolhida = mostrarPecaEscolhida;

function editarFicha(id) { novaFicha(id); }
window.editarFicha = editarFicha;

async function salvarFicha(id) {
  const nomeCli = $("#f-cliente").value.trim();
  const opt = $$("#f-clientes option").find(o => o.value === nomeCli);
  const corpo = {
    peca_id: window.__fichaPeca ? window.__fichaPeca.id : null,
    descricao: $("#f-desc").value.trim(),
    quantidade: parseQtd($("#f-qtd").value) || 1,
    cliente_id: opt ? Number(opt.dataset.id) : null,
    cliente_nome: nomeCli,
    cliente_telefone: $("#f-fone").value.trim(),
    veiculo: $("#f-veiculo").value.trim(),
    fornecedor_id: $("#f-forn").value ? Number($("#f-forn").value) : null,
    custo_centavos: parseDinheiro($("#f-custo").value),
    observacoes: $("#f-obs").value.trim()
  };
  if (!corpo.descricao) { toast("Informe a peça ou a descrição.", "aviso"); return; }
  try {
    await api(id ? "/fichas/" + id : "/fichas", { corpo, method: id ? "PUT" : "POST" });
    toast("Ficha salva.", "ok");
    fecharModal();
    carregarFichas();
    atualizarAlertas();
  } catch (e) { toast(e.message, "erro"); }
}
window.salvarFicha = salvarFicha;

function marcarPedida(id) {
  api("/fichas/" + id + "/status", { corpo: { status: "pedida" } })
    .then(() => { toast("Ficha marcada como pedida.", "ok"); carregarFichas(); atualizarAlertas(); })
    .catch(e => toast(e.message, "erro"));
}
window.marcarPedida = marcarPedida;

function receberFicha(id, pecaId, qtd) {
  if (pecaId) {
    confirmar(`Receber a peça desta ficha? Serão lançados ${fmtQtd(qtd)} no estoque e a ficha será fechada.`, async () => {
      await api("/fichas/" + id + "/status", { corpo: { status: "recebida", quantidade: qtd } });
      toast("Ficha recebida e estoque atualizado.", "ok");
      carregarFichas(); atualizarAlertas();
    }, "Receber peça");
    return;
  }
  // ficha sem peça cadastrada: escolher peça
  abrirModal("Vincular peça e receber a ficha", `
    <div class="aviso-box">Esta ficha foi criada só com descrição. Busque a peça já cadastrada para dar entrada no estoque.</div>
    <div class="campo"><label>Buscar peça</label><input type="search" id="rf-busca" placeholder="Código ou descrição…"></div>
    <div id="rf-lista" class="resultados" style="max-height:260px"></div>`,
    `<button class="btn" onclick="fecharModal()">Fechar</button>`);
  campoBuscaPeca("rf-busca", "rf-lista", async (p) => {
    try {
      await api("/fichas/" + id + "/status", { corpo: { status: "recebida", peca_id: p.id, quantidade: qtd } });
      toast("Ficha recebida e estoque atualizado.", "ok");
      fecharModal(); carregarFichas(); atualizarAlertas();
    } catch (e) { toast(e.message, "erro"); }
  });
}
window.receberFicha = receberFicha;

async function gerarPedidos() {
  let fichas = [];
  try {
    const r = await api("/fichas?status=aberta");
    fichas = r.itens;
  } catch (e) { toast(e.message, "erro"); return; }
  if (!fichas.length) {
    toast("Não há fichas com status 'aberta' para gerar pedido.", "aviso");
    return;
  }
  const grupos = {};
  fichas.forEach(f => {
    const k = f.fornecedor_id || 0;
    grupos[k] = grupos[k] || { nome: f.fornecedor_nome || "(sem fornecedor definido)", itens: [] };
    grupos[k].itens.push(f);
  });
  let html = `<div class="ok-box">Confira o que será pedido. As fichas passam para o status <strong>pedida</strong>.</div>`;
  Object.values(grupos).forEach(g => {
    const total = g.itens.reduce((s, f) => s + f.custo_centavos * f.quantidade, 0);
    html += `<h3 style="margin:14px 0 6px">🏭 ${esc(g.nome)} — ${g.itens.length} item(ns) · ${fmtMoney(total)}</h3>
      <table><thead><tr><th>Ficha</th><th>Peça</th><th class="dir">Qtd</th><th class="dir">Custo est.</th><th class="dir">Subtotal</th></tr></thead><tbody>
      ${g.itens.map(f => `<tr><td>#${f.id}</td><td>${esc(f.descricao)}<div class="muted small">${esc(f.cliente_nome || "")}</div></td>
        <td class="dir">${fmtQtd(f.quantidade)}</td><td class="dir">${fmtMoney(f.custo_centavos)}</td>
        <td class="dir">${fmtMoney(f.custo_centavos * f.quantidade)}</td></tr>`).join("")}
      </tbody></table>`;
  });
  abrirModal("Gerar pedido de compra", html,
    `<button class="btn" onclick="fecharModal()">Cancelar</button>
     <button class="btn primario" id="btn-gerar">Gerar ${Object.keys(grupos).length} pedido(s)</button>`, "min(860px,96vw)");
  $("#btn-gerar").onclick = async () => {
    try {
      const r = await api("/pedidos/gerar", { corpo: { ficha_ids: fichas.map(f => f.id) } });
      fecharModal();
      toast("Pedido(s) gerado(s): " + r.pedidos.length + ". Veja em Pedidos.", "ok");
      irPara("pedidos");
      atualizarAlertas();
    } catch (e) { toast(e.message, "erro"); }
  };
}
window.gerarPedidos = gerarPedidos;

/* ================= PEDIDOS ================= */
ROTAS.pedidos = async function () {
  $("#conteudo").innerHTML = `
  <div class="topo">
    <div><h1>Pedidos de compra</h1><p class="sub">Fichas de falta agrupadas por fornecedor. Ao receber, o estoque entra e as fichas fecham.</p></div>
    <div class="acoes"><button class="btn" onclick="irPara('fichas')">Ir para fichas</button></div>
  </div>
  <div class="painel"><div id="lista-pedidos"><div class="vazio">Carregando…</div></div></div>`;
  try {
    const r = await api("/pedidos?status=todos");
    const alvo = $("#lista-pedidos");
    if (!r.itens.length) { alvo.innerHTML = '<div class="vazio">Nenhum pedido ainda. Gere um em Fichas de Falta → “Gerar pedido por fornecedor”.</div>'; return; }
    alvo.innerHTML = `<div class="tabela-wrap"><table><thead><tr>
      <th>Pedido</th><th>Data</th><th>Fornecedor</th><th class="dir">Itens</th><th class="dir">Total estimado</th><th>Status</th><th></th>
      </tr></thead><tbody>
      ${r.itens.map(o => `<tr>
        <td><strong>#${o.id}</strong></td>
        <td class="small">${dataBR(o.criado)}</td>
        <td>${esc(o.fornecedor || "(sem fornecedor)")}</td>
        <td class="dir">${o.itens}</td>
        <td class="dir">${fmtMoney(o.total_centavos)}</td>
        <td>${o.status === "aberto" ? '<span class="tag azul">aberto</span>' :
              o.status === "recebido" ? '<span class="tag verde">recebido</span>' :
              '<span class="tag cinza">' + esc(o.status) + "</span>"}</td>
        <td class="dir"><button class="btn pequeno" onclick="verPedido(${o.id})">Abrir</button></td>
      </tr>`).join("")}</tbody></table></div>`;
  } catch (e) { toast(e.message, "erro"); }
};

async function verPedido(id) {
  try {
    const p = await api("/pedidos/" + id);
    const itens = p.itens.map(i => `<tr>
      <td>${esc(i.descricao)}<div class="muted small mono">${esc(i.codigo || "")}</div></td>
      <td class="dir">${fmtQtd(i.quantidade)}</td>
      <td class="dir">${fmtMoney(i.custo_centavos)}</td>
      <td class="dir">${fmtMoney(i.custo_centavos * i.quantidade)}</td>
      <td>${tagStatusFicha(i.ficha_status || "—")}</td></tr>`).join("");
    abrirModal("Pedido #" + p.id, `
      <div class="grade g3" style="margin-bottom:12px">
        <div><label>Fornecedor</label><div>${esc(p.fornecedor || "(sem fornecedor)")}</div></div>
        <div><label>Data</label><div>${dataHoraBR(p.criado)}</div></div>
        <div><label>Status</label><div>${esc(p.status)}</div></div>
      </div>
      <table><thead><tr><th>Peça</th><th class="dir">Qtd</th><th class="dir">Custo</th><th class="dir">Subtotal</th><th>Ficha</th></tr></thead>
      <tbody>${itens}</tbody></table>
      <div class="kv" style="margin-top:12px"><strong>Total estimado</strong><strong>${fmtMoney(p.total_centavos)}</strong></div>`,
      `${p.status === "aberto" ? `<button class="btn vermelho" onclick="cancelarPedido(${p.id})">Cancelar pedido</button>
        <button class="btn verde" onclick="receberPedido(${p.id})">✓ Receber pedido (entra no estoque)</button>` : ""}
       <button class="btn" onclick="fecharModal()">Fechar</button>`, "min(820px,96vw)");
  } catch (e) { toast(e.message, "erro"); }
}
window.verPedido = verPedido;

function receberPedido(id) {
  confirmar("Receber o pedido #" + id + "? Todas as peças entram no estoque e as fichas de falta são fechadas.", async () => {
    const r = await api("/pedidos/" + id + "/receber", { corpo: {} });
    toast("Recebido: " + r.recebidos + " item(ns). Estoque atualizado.", "ok");
    fecharModal();
    navegar();
    atualizarAlertas();
  }, "Receber pedido");
}
window.receberPedido = receberPedido;

function cancelarPedido(id) {
  confirmar("Cancelar o pedido #" + id + "? As fichas voltam para 'aberta'.", async () => {
    await api("/pedidos/" + id + "/cancelar", { method: "POST" });
    toast("Pedido cancelado. Fichas voltaram para aberta.", "ok");
    fecharModal(); navegar(); atualizarAlertas();
  }, "Cancelar pedido");
}
window.cancelarPedido = cancelarPedido;

/* ================= CLIENTES ================= */
ROTAS.clientes = async function () {
  $("#conteudo").innerHTML = `
  <div class="topo">
    <div><h1>Clientes e veículos</h1><p class="sub">Histórico de compras, veículos e fichas de falta de cada cliente</p></div>
    <div class="acoes"><button class="btn primario" onclick="editarCliente(null)">+ Novo cliente</button></div>
  </div>
  <div class="painel">
    <div class="campo"><input type="search" id="busca-clientes" placeholder="Buscar por nome, telefone, documento, placa ou modelo do veículo…" autocomplete="off"></div>
    <div id="lista-clientes-view"><div class="vazio">Carregando…</div></div>
  </div>`;
  $("#busca-clientes").addEventListener("input", debounce(carregarClientes, 200));
  await carregarClientes();
};

async function carregarClientes() {
  const q = $("#busca-clientes") ? $("#busca-clientes").value.trim() : "";
  const r = await api("/clientes?q=" + encodeURIComponent(q));
  const alvo = $("#lista-clientes-view");
  if (!alvo) return;
  if (!r.itens.length) { alvo.innerHTML = '<div class="vazio">Nenhum cliente encontrado.</div>'; return; }
  alvo.innerHTML = `<div class="tabela-wrap"><table><thead><tr><th>Nome</th><th>Telefone</th><th>Documento</th>
    <th class="dir">Veículos</th><th></th></tr></thead><tbody>
    ${r.itens.map(c => `<tr class="clicavel" onclick="verCliente(${c.id})">
      <td><strong>${esc(c.nome)}</strong><div class="muted small">${esc(c.endereco || "")}</div></td>
      <td>${esc(c.telefone || "—")}</td><td>${esc(c.documento || "—")}</td>
      <td class="dir">${c.qtd_veiculos}</td>
      <td class="dir" onclick="event.stopPropagation()">
        <button class="btn pequeno" onclick="verCliente(${c.id})">Abrir</button></td>
    </tr>`).join("")}</tbody></table></div>`;
}
window.carregarClientes = carregarClientes;

async function verCliente(id) {
  const c = await api("/clientes/" + id);
  const veic = c.veiculos.length ? c.veiculos.map(v => `<tr>
      <td class="mono">${esc(v.placa || "—")}</td><td>${esc(v.modelo)}</td><td>${esc(v.ano)}</td>
      <td class="dir"><button class="btn pequeno vermelho" onclick="excluirVeiculo(${v.id},${id})">✕</button></td></tr>`).join("")
    : `<tr><td colspan="4" class="muted">Nenhum veículo cadastrado.</td></tr>`;
  const compras = c.compras.length ? c.compras.map(s => `<tr class="clicavel" onclick="fecharModal();verVenda(${s.id})">
      <td class="small">${dataHoraBR(s.data)}</td><td>${esc(s.veiculo_modelo || "")} ${esc(s.veiculo_placa || "")}</td>
      <td>${nomePagamento(s.forma_pagamento)}</td><td class="dir">${fmtMoney(s.total_centavos)}</td>
      <td>${s.status === "cancelada" ? '<span class="tag vermelho">cancelada</span>' : '<span class="tag verde">ok</span>'}</td></tr>`).join("")
    : `<tr><td colspan="5" class="muted">Nenhuma compra registrada.</td></tr>`;
  const fichas = c.fichas.length ? c.fichas.map(f => `<tr>
      <td>#${f.id}</td><td class="small">${dataBR(f.data)}</td><td>${esc(f.descricao)}</td>
      <td class="dir">${fmtQtd(f.quantidade)}</td><td>${tagStatusFicha(f.status)}</td></tr>`).join("")
    : `<tr><td colspan="5" class="muted">Nenhuma ficha de falta.</td></tr>`;

  abrirModal("Cliente — " + c.nome, `
    <div class="grade g4" style="margin-bottom:14px">
      <div><label>Telefone</label><div>${esc(c.telefone || "—")}</div></div>
      <div><label>Documento</label><div>${esc(c.documento || "—")}</div></div>
      <div><label>Endereço</label><div>${esc(c.endereco || "—")}</div></div>
      <div><label>Cliente desde</label><div>${dataBR(c.criado)}</div></div>
    </div>
    ${c.observacoes ? `<div class="aviso-box">${esc(c.observacoes)}</div>` : ""}

    <h3 style="margin:16px 0 8px">Veículos</h3>
    <table><thead><tr><th>Placa</th><th>Modelo</th><th>Ano</th><th></th></tr></thead><tbody>${veic}</tbody></table>
    <div class="grade g3" style="margin-top:10px">
      <input type="text" id="v-placa" placeholder="Placa">
      <input type="text" id="v-modelo" placeholder="Modelo">
      <div style="display:flex;gap:6px"><input type="text" id="v-ano" placeholder="Ano" style="width:80px">
        <button class="btn" onclick="salvarVeiculo(${id})">Adicionar</button></div>
    </div>

    <h3 style="margin:20px 0 8px">Compras</h3>
    <table><thead><tr><th>Data</th><th>Veículo</th><th>Pagamento</th><th class="dir">Total</th><th></th></tr></thead><tbody>${compras}</tbody></table>

    <h3 style="margin:20px 0 8px">Fichas de falta</h3>
    <table><thead><tr><th>#</th><th>Data</th><th>Peça</th><th class="dir">Qtd</th><th>Status</th></tr></thead><tbody>${fichas}</tbody></table>
    `,
    `<button class="btn" onclick="fecharModal()">Fechar</button>
     <button class="btn primario" onclick="editarCliente(${id})">Editar cadastro</button>`, "min(880px,96vw)");
}
window.verCliente = verCliente;

async function editarCliente(id) {
  let c = { id: 0, nome: "", telefone: "", documento: "", endereco: "", observacoes: "" };
  if (id) {
    const r = await api("/clientes/" + id);
    c = { id, nome: r.nome, telefone: r.telefone, documento: r.documento, endereco: r.endereco, observacoes: r.observacoes };
  }
  abrirModal(id ? "Editar cliente" : "Novo cliente", `
    <div class="campo"><label>Nome *</label><input type="text" id="c-nome" value="${esc(c.nome)}"></div>
    <div class="grade g3">
      <div class="campo"><label>Telefone</label><input type="tel" id="c-fone" value="${esc(c.telefone)}"></div>
      <div class="campo"><label>CPF/CNPJ</label><input type="text" id="c-doc" value="${esc(c.documento)}"></div>
      <div class="campo"><label>Endereço</label><input type="text" id="c-end" value="${esc(c.endereco)}"></div>
    </div>
    <div class="campo"><label>Observações</label><textarea id="c-obs" rows="3">${esc(c.observacoes)}</textarea></div>
    `,
    `<button class="btn" onclick="fecharModal()">Cancelar</button>
     ${id ? `<button class="btn vermelho" onclick="excluirCliente(${id})">Excluir</button>` : ""}
     <button class="btn primario" onclick="salvarCliente(${id})">Salvar</button>`);
}
window.editarCliente = editarCliente;

async function salvarCliente(id) {
  const corpo = {
    nome: $("#c-nome").value.trim(), telefone: $("#c-fone").value.trim(),
    documento: $("#c-doc").value.trim(), endereco: $("#c-end").value.trim(),
    observacoes: $("#c-obs").value.trim()
  };
  if (!corpo.nome) { toast("Informe o nome do cliente.", "aviso"); return; }
  try {
    await api(id ? "/clientes/" + id : "/clientes", { corpo, method: id ? "PUT" : "POST" });
    toast("Cliente salvo.", "ok");
    fecharModal();
    if (rotaAtual() === "clientes") carregarClientes();
  } catch (e) { toast(e.message, "erro"); }
}
window.salvarCliente = salvarCliente;

function excluirCliente(id) {
  confirmar("Excluir este cliente? O histórico de vendas continua guardado.", async () => {
    await api("/clientes/" + id, { method: "DELETE" });
    toast("Cliente excluído.", "ok");
    fecharModal();
    if (rotaAtual() === "clientes") carregarClientes();
  }, "Excluir");
}
window.excluirCliente = excluirCliente;

async function salvarVeiculo(clienteId) {
  const corpo = { placa: $("#v-placa").value.trim(), modelo: $("#v-modelo").value.trim(), ano: $("#v-ano").value.trim() };
  if (!corpo.placa && !corpo.modelo) { toast("Informe a placa ou o modelo.", "aviso"); return; }
  try {
    await api("/clientes/" + clienteId + "/veiculos", { corpo });
    toast("Veículo adicionado.", "ok");
    verCliente(clienteId);
  } catch (e) { toast(e.message, "erro"); }
}
window.salvarVeiculo = salvarVeiculo;

function excluirVeiculo(id, clienteId) {
  api("/veiculos/" + id, { method: "DELETE" })
    .then(() => { toast("Veículo removido.", "ok"); verCliente(clienteId); })
    .catch(e => toast(e.message, "erro"));
}
window.excluirVeiculo = excluirVeiculo;

/* ================= FORNECEDORES ================= */
ROTAS.fornecedores = async function () {
  $("#conteudo").innerHTML = `
  <div class="topo">
    <div><h1>Fornecedores</h1><p class="sub">Usados nas fichas de falta, pedidos de compra e importação de XML</p></div>
    <div class="acoes"><button class="btn primario" onclick="editarFornecedor(null)">+ Novo fornecedor</button></div>
  </div>
  <div class="painel">
    <div class="campo"><input type="search" id="busca-forn" placeholder="Buscar fornecedor…" autocomplete="off"></div>
    <div id="lista-forn"><div class="vazio">Carregando…</div></div>
  </div>`;
  $("#busca-forn").addEventListener("input", debounce(carregarFornecedores, 200));
  await carregarFornecedores();
};

async function carregarFornecedores() {
  const q = $("#busca-forn") ? $("#busca-forn").value.trim() : "";
  const r = await api("/fornecedores?q=" + encodeURIComponent(q));
  const alvo = $("#lista-forn");
  if (!alvo) return;
  if (!r.itens.length) { alvo.innerHTML = '<div class="vazio">Nenhum fornecedor cadastrado. Clique em “Novo fornecedor”.</div>'; return; }
  alvo.innerHTML = `<div class="tabela-wrap"><table><thead><tr><th>Fornecedor</th><th>Contato</th><th>Telefone</th>
    <th>E-mail</th><th class="dir">Fichas abertas</th><th class="dir">Peças</th><th></th></tr></thead><tbody>
    ${r.itens.map(f => `<tr>
      <td><strong>${esc(f.nome)}</strong><div class="muted small">${esc(f.observacoes || "")}</div></td>
      <td>${esc(f.contato || "—")}</td><td>${esc(f.telefone || "—")}</td><td>${esc(f.email || "—")}</td>
      <td class="dir">${f.fichas_abertas}</td><td class="dir">${f.pecas}</td>
      <td class="dir"><button class="btn pequeno" onclick="editarFornecedor(${f.id})">Editar</button></td>
    </tr>`).join("")}</tbody></table></div>`;
}
window.carregarFornecedores = carregarFornecedores;

async function editarFornecedor(id) {
  let f = { id: 0, nome: "", contato: "", telefone: "", email: "", observacoes: "" };
  if (id) {
    const r = await api("/fornecedores");
    const achou = r.itens.find(x => x.id === id);
    if (achou) f = achou;
  }
  abrirModal(id ? "Editar fornecedor" : "Novo fornecedor", `
    <div class="campo"><label>Nome / razão social *</label><input type="text" id="fo-nome" value="${esc(f.nome)}"></div>
    <div class="grade g3">
      <div class="campo"><label>Pessoa de contato</label><input type="text" id="fo-contato" value="${esc(f.contato)}"></div>
      <div class="campo"><label>Telefone / WhatsApp</label><input type="tel" id="fo-fone" value="${esc(f.telefone)}"></div>
      <div class="campo"><label>E-mail</label><input type="text" id="fo-email" value="${esc(f.email)}"></div>
    </div>
    <div class="campo"><label>Observações</label><textarea id="fo-obs" rows="3">${esc(f.observacoes)}</textarea></div>
    `,
    `<button class="btn" onclick="fecharModal()">Cancelar</button>
     ${id ? `<button class="btn vermelho" onclick="excluirFornecedor(${id})">Excluir</button>` : ""}
     <button class="btn primario" onclick="salvarFornecedor(${id})">Salvar</button>`);
}
window.editarFornecedor = editarFornecedor;

async function salvarFornecedor(id) {
  const corpo = {
    nome: $("#fo-nome").value.trim(), contato: $("#fo-contato").value.trim(),
    telefone: $("#fo-fone").value.trim(), email: $("#fo-email").value.trim(),
    observacoes: $("#fo-obs").value.trim()
  };
  if (!corpo.nome) { toast("Informe o nome do fornecedor.", "aviso"); return; }
  try {
    await api(id ? "/fornecedores/" + id : "/fornecedores", { corpo, method: id ? "PUT" : "POST" });
    toast("Fornecedor salvo.", "ok");
    fecharModal();
    if (rotaAtual() === "fornecedores") carregarFornecedores();
  } catch (e) { toast(e.message, "erro"); }
}
window.salvarFornecedor = salvarFornecedor;

function excluirFornecedor(id) {
  confirmar("Excluir este fornecedor? As peças e fichas ligadas a ele ficam sem fornecedor.", async () => {
    await api("/fornecedores/" + id, { method: "DELETE" });
    toast("Fornecedor excluído.", "ok");
    fecharModal();
    carregarFornecedores();
  }, "Excluir");
}
window.excluirFornecedor = excluirFornecedor;

/* ================= CONTAS A RECEBER ================= */
ROTAS.receber = async function () {
  $("#conteudo").innerHTML = `
  <div class="topo">
    <div><h1>Contas a receber</h1><p class="sub">Vendas faturadas (a prazo) com alerta de vencimento</p></div>
  </div>
  <div class="pill-tabs">
    <button data-s="aberto" class="ativo">Em aberto</button>
    <button data-s="vencidos">Vencidas</button>
    <button data-s="pago">Pagas</button>
    <button data-s="todos">Todas</button>
  </div>
  <div class="painel"><div id="lista-receber"><div class="vazio">Carregando…</div></div></div>`;
  $$(".pill-tabs button").forEach(b => {
    b.onclick = () => {
      $$(".pill-tabs button").forEach(x => x.classList.remove("ativo"));
      b.classList.add("ativo");
      carregarReceber(b.dataset.s);
    };
  });
  await carregarReceber("aberto");
};

async function carregarReceber(filtro) {
  let status = filtro;
  if (filtro === "vencidos") status = "aberto";
  const r = await api("/recebiveis?status=" + status);
  let itens = r.itens;
  if (filtro === "vencidos") itens = itens.filter(i => i.vencido);
  const alvo = $("#lista-receber");
  if (!alvo) return;
  let total = 0;
  itens.forEach(i => { if (i.status === "aberto") total += i.valor_centavos; });
  if (!itens.length) { alvo.innerHTML = '<div class="vazio">Nenhuma conta aqui.</div>'; return; }
  alvo.innerHTML = `
    <div class="tabela-wrap"><table><thead><tr><th>Cliente</th><th>Venda</th><th class="dir">Valor</th>
      <th>Vencimento</th><th>Situação</th><th></th></tr></thead><tbody>
      ${itens.map(i => `<tr>
        <td><strong>${esc(i.cliente || "—")}</strong></td>
        <td>#${i.venda_id}</td>
        <td class="dir num">${fmtMoney(i.valor_centavos)}</td>
        <td>${dataBR(i.vencimento)}</td>
        <td>${i.status === "pago" ? '<span class="tag verde">pago</span>' :
              i.status === "estornado" ? '<span class="tag vermelho">estornada (venda cancelada)</span>' :
              i.status === "cancelado" ? '<span class="tag vermelho">cancelada</span>' :
              i.vencido ? `<span class="tag vermelho">vencido há ${i.dias_atraso} dia(s)</span>` :
              '<span class="tag azul">em aberto</span>'}</td>
        <td class="dir">${i.status === "aberto" ? `<button class="btn pequeno verde" onclick="baixarReceber(${i.id})">Receber</button>` : ""}</td>
      </tr>`).join("")}</tbody></table></div>
    <p class="dica">Total em aberto: <strong>${fmtMoney(total)}</strong></p>`;
}
window.carregarReceber = carregarReceber;

function baixarReceber(id) {
  confirmar("Confirmar o recebimento desta conta?", async () => {
    await api("/recebiveis/" + id + "/baixar", { method: "POST" });
    toast("Conta baixada como paga.", "ok");
    carregarReceber("aberto"); atualizarAlertas();
  }, "Confirmar recebimento");
}
window.baixarReceber = baixarReceber;


/* ================= CONTAS A PAGAR ================= */
ROTAS.pagar = async function () {
  $("#conteudo").innerHTML = `
  <div class="topo">
    <div><h1>Contas a pagar</h1><p class="sub">Boletos dos fornecedores — lançados sozinhos quando você importa a nota</p></div>
    <div class="acoes">
      <button class="btn" onclick="abrirHistoricoNotas()">Notas importadas</button>
      <button class="btn primario" onclick="abrirNovaConta()">+ Nova conta</button>
    </div>
  </div>
  <div class="pill-tabs">
    <button data-s="aberto" class="ativo">Em aberto</button>
    <button data-s="vencidos">Vencidas</button>
    <button data-s="pago">Pagas</button>
    <button data-s="todos">Todas</button>
  </div>
  <div class="painel"><div id="lista-pagar"><div class="vazio">Carregando…</div></div></div>`;
  $$(".pill-tabs button").forEach(b => {
    b.onclick = () => {
      $$(".pill-tabs button").forEach(x => x.classList.remove("ativo"));
      b.classList.add("ativo");
      carregarPagar(b.dataset.s);
    };
  });
  await carregarPagar("aberto");
};

async function carregarPagar(filtro) {
  const r = await api("/pagaveis?status=" + (filtro || "aberto"));
  const alvo = $("#lista-pagar");
  if (!alvo) return;
  let itens = r.itens || [];
  const hojeStr = r.hoje || hoje();
  if (filtro === "vencidos") itens = itens.filter(i => i.status === "aberto" && i.vencimento < hojeStr);
  const totalLista = itens.reduce((s, i) => s + (i.status === "aberto" ? i.valor_centavos : 0), 0);
  if (!itens.length) {
    alvo.innerHTML = '<div class="vazio">Nenhuma conta aqui. Elas aparecem sozinhas quando você importa o XML da nota.</div>';
    return;
  }
  alvo.innerHTML = `
    <div class="tabela-wrap"><table><thead><tr><th>Fornecedor</th><th>Nota</th>
      <th class="dir">Valor</th><th>Vencimento</th><th>Situação</th><th></th></tr></thead><tbody>
      ${itens.map(i => {
        const vencido = i.status === "aberto" && i.vencimento < hojeStr;
        return `<tr>
        <td><strong>${esc(i.fornecedor || "—")}</strong>
          ${i.cnpj ? `<div class="muted small mono">${esc(i.cnpj.replace(/(\d{2})(\d{3})(\d{3})(\d{4})(\d{2})/, "$1.$2.$3/$4-$5"))}</div>` : ""}</td>
        <td class="mono small">${i.documento ? "#" + esc(i.documento) : (i.cnpj ? esc(i.cnpj) : "—")}${i.parcela ? `<div class="muted small">parc. ${esc(i.parcela)}</div>` : ""}</td>
        <td class="dir num">${fmtMoney(i.valor_centavos)}</td>
        <td>${dataBR(i.vencimento)}</td>
        <td>${i.status === "pago" ? '<span class="tag verde">pago</span>' :
              i.status === "cancelado" ? '<span class="tag vermelho">cancelada</span>' :
              vencido ? '<span class="tag vermelho">vencida</span>' :
              '<span class="tag azul">em aberto</span>'}</td>
        <td class="dir" style="white-space:nowrap">
          ${i.status === "aberto" ? `<button class="btn pequeno verde" onclick="pagarConta(${i.id},${i.valor_centavos})">Pagar</button>
            <button class="btn pequeno" onclick="editarConta(${i.id})">Editar</button>
            <button class="btn pequeno" onclick="cancelarConta(${i.id})">Cancelar</button>` : ""}
        </td></tr>`;
      }).join("")}</tbody></table></div>
    <p class="dica">Nesta lista: <strong>${fmtMoney(totalLista)}</strong> em aberto.
      Total geral em aberto: <strong>${fmtMoney(r.total_aberto_centavos || 0)}</strong>
      ${r.qtd_vencidas ? ` — <span style="color:#c0392b">${r.qtd_vencidas} vencida(s), ${fmtMoney(r.total_vencido_centavos)}</span>` : ""}</p>`;
}
window.carregarPagar = carregarPagar;

function pagarConta(id, valor) {
  abrirModal("Pagar a conta", `
    <p class="dica">Informe quanto foi realmente pago. Se houver juros ou desconto, é só mudar o valor.</p>
    <div class="campo"><label>Valor pago (R$)</label>
      <input type="text" id="pagar-valor" value="${(valor / 100).toFixed(2).replace(".", ",")}" inputmode="decimal"></div>
    <div class="campo"><label>Data do pagamento</label><input type="date" id="pagar-data" value="${hoje()}"></div>`,
    `<button class="btn" onclick="fecharModal()">Cancelar</button>
     <button class="btn verde" onclick="confirmarPagamento(${id})">Confirmar pagamento</button>`);
}
window.pagarConta = pagarConta;

async function confirmarPagamento(id) {
  try {
    const valor = parseDinheiro($("#pagar-valor").value);
    const data = $("#pagar-data").value;
    await api("/pagaveis/" + id + "/baixar", { corpo: { valor_pago_centavos: valor, pago_em: data } });
    fecharModal();
    toast("Conta baixada como paga.", "ok");
    carregarPagar("aberto");
    atualizarAlertas();
  } catch (e) { toast(e.message, "erro"); }
}
window.confirmarPagamento = confirmarPagamento;

async function cancelarConta(id) {
  confirmar("Cancelar esta conta? Ela sai do total a pagar, mas fica no histórico.", async () => {
    await api("/pagaveis/" + id + "/cancelar", { method: "POST" });
    toast("Conta cancelada.", "ok");
    carregarPagar("aberto");
    atualizarAlertas();
  }, "Cancelar conta");
}
window.cancelarConta = cancelarConta;

function abrirNovaConta() {
  abrirModal("Nova conta a pagar", `
    <p class="dica">Para a despesa que não vem de nota (aluguel, luz, salário, frete avulso…).</p>
    <div class="grade g2">
      <div class="campo"><label>Fornecedor / descrição</label>
        <input type="text" id="conta-fornecedor" placeholder="Ex.: ALUGUEL DO PONTO"></div>
      <div class="campo"><label>CNPJ / documento (opcional)</label>
        <input type="text" id="conta-doc" placeholder="00.000.000/0000-00"></div>
      <div class="campo"><label>Valor (R$)</label>
        <input type="text" id="conta-valor" placeholder="0,00" inputmode="decimal"></div>
      <div class="campo"><label>Vencimento</label><input type="date" id="conta-venc" value="${hoje()}"></div>
    </div>
    <div class="campo"><label>Observação (opcional)</label>
      <input type="text" id="conta-obs" placeholder="Ex.: referente a outubro"></div>`,
    `<button class="btn" onclick="fecharModal()">Cancelar</button>
     <button class="btn primario" id="btn-salvar-conta" onclick="salvarNovaConta()">Salvar conta</button>`);
}
window.abrirNovaConta = abrirNovaConta;

async function salvarNovaConta() {
  const btn = $("#btn-salvar-conta");
  try {
    const valor = parseDinheiro($("#conta-valor").value);
    const r = await api("/pagaveis", { corpo: {
      fornecedor: $("#conta-fornecedor").value.trim(),
      documento: $("#conta-doc").value.trim(),
      valor_centavos: valor,
      vencimento: $("#conta-venc").value,
      observacao: $("#conta-obs").value.trim()
    }});
    fecharModal();
    toast("Conta lançada.", "ok");
    carregarPagar("aberto");
    atualizarAlertas();
    return r;
  } catch (e) { toast(e.message, "erro"); if (btn) btn.disabled = false; }
}
window.salvarNovaConta = salvarNovaConta;

async function editarConta(id) {
  try {
    const r = await api("/pagaveis?status=todos");
    const c = (r.itens || []).find(x => x.id === id);
    if (!c) { toast("Conta não encontrada.", "aviso"); return; }
    abrirModal("Editar conta a pagar", `
      <div class="grade g2">
        <div class="campo"><label>Fornecedor</label><input type="text" id="conta-fornecedor" value="${esc(c.fornecedor)}"></div>
        <div class="campo"><label>Documento</label><input type="text" id="conta-doc" value="${esc(c.cnpj || "")}"></div>
        <div class="campo"><label>Valor (R$)</label><input type="text" id="conta-valor" value="${(c.valor_centavos / 100).toFixed(2).replace(".", ",")}" inputmode="decimal"></div>
        <div class="campo"><label>Vencimento</label><input type="date" id="conta-venc" value="${esc(c.vencimento)}"></div>
      </div>
      <div class="campo"><label>Observação</label><input type="text" id="conta-obs" value="${esc(c.observacao || "")}"></div>`,
      `<button class="btn" onclick="fecharModal()">Cancelar</button>
       <button class="btn primario" id="btn-salvar-conta" onclick="salvarEdicaoConta(${id})">Salvar</button>`);
  } catch (e) { toast(e.message, "erro"); }
}
window.editarConta = editarConta;

async function salvarEdicaoConta(id) {
  try {
    await api("/pagaveis/" + id, { method: "PUT", corpo: {
      fornecedor: $("#conta-fornecedor").value.trim(),
      documento: $("#conta-doc").value.trim(),
      valor_centavos: parseDinheiro($("#conta-valor").value),
      vencimento: $("#conta-venc").value,
      observacao: $("#conta-obs").value.trim()
    }});
    fecharModal();
    toast("Conta atualizada.", "ok");
    carregarPagar("aberto");
    atualizarAlertas();
  } catch (e) { toast(e.message, "erro"); }
}
window.salvarEdicaoConta = salvarEdicaoConta;

async function abrirHistoricoNotas() {
  try {
    const r = await api("/notas-entrada");
    const itens = r.itens || [];
    abrirModal("Notas que já entraram no estoque", `
      ${!itens.length ? '<div class="vazio">Nenhuma nota importada ainda.</div>' : `
      <div class="tabela-wrap" style="max-height:55vh"><table><thead><tr><th>Nota</th><th>Fornecedor</th>
        <th>Emitida</th><th class="dir">Total</th><th class="dir">Contas</th><th></th></tr></thead><tbody>
        ${itens.map(n => `<tr>
          <td class="mono small">#${esc(n.numero || "—")}${n.serie ? "/" + esc(n.serie) : ""}</td>
          <td><strong>${esc(n.fornecedor || "—")}</strong><div class="muted small">${n.itens} item(ns)</div></td>
          <td>${n.emitida_em ? dataBR(n.emitida_em) : "—"}</td>
          <td class="dir num">${fmtMoney(n.total_centavos)}</td>
          <td class="dir">${n.contas || 0} boleto(s)<div class="muted small">${fmtMoney(n.contas_centavos || 0)}</div></td>
          <td class="dir">${n.chave ? `<span class="mono small" title="${esc(n.chave)}">${esc(n.chave.slice(0, 8))}…</span>` : ""}</td>
        </tr>`).join("")}</tbody></table></div>
      <p class="dica">A nota que já está aqui não lança a despesa de novo se você importar o mesmo arquivo.</p>`}`,
      `<button class="btn" onclick="fecharModal()">Fechar</button>`, "min(900px,96vw)");
  } catch (e) { toast(e.message, "erro"); }
}
window.abrirHistoricoNotas = abrirHistoricoNotas;

/* ================= FECHAMENTO DO DIA ================= */
ROTAS.fechamento = async function () {
  $("#conteudo").innerHTML = `
  <div class="topo">
    <div><h1>Fechamento do dia</h1><p class="sub">Total vendido, por forma de pagamento e itens vendidos</p></div>
    <div class="acoes">
      <input type="date" id="fech-data" value="${hoje()}" style="width:auto">
      <button class="btn" onclick="carregarFechamento()">Atualizar</button>
      <button class="btn" onclick="imprimirFechamento()">Imprimir</button>
      <button class="btn primario" onclick="fecharCaixaComBackup()">Fechar o caixa (com backup)</button>
    </div>
  </div>
  <div id="fech-corpo"><div class="vazio">Carregando…</div></div>`;
  $("#fech-data").onchange = carregarFechamento;
  await carregarFechamento();
};

let ULTIMO_FECHAMENTO = null;
async function carregarFechamento() {
  const data = $("#fech-data") ? $("#fech-data").value : hoje();
  const d = await api("/fechamento?data=" + data);
  ULTIMO_FECHAMENTO = d;
  const alvo = $("#fech-corpo");
  if (!alvo) return;
  const formas = d.por_forma.map(f => `<div class="kv"><span>${nomePagamento(f.forma)} <span class="muted small">(${f.qtd} venda(s))</span></span>
      <strong>${fmtMoney(f.total_centavos)}</strong></div>`).join("") || '<div class="vazio">Nenhuma venda neste dia.</div>';
  alvo.innerHTML = `
    <div class="cards">
      <div class="card ok"><div class="rotulo">Total vendido</div><div class="valor">${fmtMoney(d.total_centavos)}</div>
        <div class="obs">${d.qtd_vendas} venda(s)</div></div>
      <div class="card info"><div class="rotulo">Lucro estimado</div><div class="valor">${fmtMoney(d.lucro_centavos)}</div>
        <div class="obs">custo ${fmtMoney(d.custo_centavos)}</div></div>
      <div class="card"><div class="rotulo">Ticket médio</div>
        <div class="valor">${d.qtd_vendas ? fmtMoney(Math.round(d.total_centavos / d.qtd_vendas)) : fmtMoney(0)}</div>
        <div class="obs">por venda</div></div>
      <div class="card"><div class="rotulo">Itens vendidos</div>
        <div class="valor">${d.itens.reduce((s, i) => s + i.quantidade, 0)}</div>
        <div class="obs">${d.itens.length} peça(s) diferentes</div></div>
    </div>
    <div class="grade g2">
      <div class="painel"><h3>Por forma de pagamento</h3><div style="margin-top:10px">${formas}</div></div>
      <div class="painel"><h3>Itens vendidos</h3>
        ${d.itens.length ? `<div class="tabela-wrap" style="max-height:300px"><table><thead><tr><th>Peça</th>
          <th class="dir">Qtd</th><th class="dir">Total</th></tr></thead><tbody>
          ${d.itens.map(i => `<tr><td>${esc(i.descricao)}<div class="muted small mono">${esc(i.codigo)}</div></td>
            <td class="dir">${fmtQtd(i.quantidade)}</td><td class="dir">${fmtMoney(i.total_centavos)}</td></tr>`).join("")}
          </tbody></table></div>` : '<div class="vazio">Sem itens.</div>'}
      </div>
    </div>`;
}
window.carregarFechamento = carregarFechamento;

function imprimirFechamento() {
  const d = ULTIMO_FECHAMENTO;
  if (!d) return;
  const html = `<div class="relatorio-print">
    <h3>${esc(ESTADO.config.loja_nome || "")} — Fechamento do dia ${dataBR(d.data)}</h3>
    <p>Total vendido: <strong>${fmtMoney(d.total_centavos)}</strong> em ${d.qtd_vendas} venda(s) · Lucro estimado ${fmtMoney(d.lucro_centavos)}</p>
    <table><thead><tr><th>Forma de pagamento</th><th>Vendas</th><th>Total</th></tr></thead><tbody>
    ${d.por_forma.map(f => `<tr><td>${nomePagamento(f.forma)}</td><td>${f.qtd}</td><td>${fmtMoney(f.total_centavos)}</td></tr>`).join("")}
    </tbody></table>
    <h4>Itens vendidos</h4>
    <table><thead><tr><th>Código</th><th>Peça</th><th>Qtd</th><th>Total</th></tr></thead><tbody>
    ${d.itens.map(i => `<tr><td>${esc(i.codigo)}</td><td>${esc(i.descricao)}</td><td>${fmtQtd(i.quantidade)}</td><td>${fmtMoney(i.total_centavos)}</td></tr>`).join("")}
    </tbody></table></div>`;
  imprimirHTML(html);
}
window.imprimirFechamento = imprimirFechamento;

/* ================= RELATÓRIOS ================= */
const TIPOS_RELATORIO = [
  ["vendas", "Vendas no período (diário/mensal)"],
  ["lucro", "Lucro estimado por dia"],
  ["mais_vendidos", "Produtos mais vendidos"],
  ["estoque_minimo", "Peças abaixo do estoque mínimo"],
  ["pedidos_abertos", "Pedidos de compra abertos"],
  ["fichas_abertas", "Fichas de falta em aberto"],
  ["recebiveis", "Contas a receber (vencidas e em aberto)"],
  ["pagaveis", "Contas a pagar (boletos de fornecedor)"],
  ["estoque_valor", "Saldo do estoque em dinheiro"],
  ["clientes", "Clientes e histórico de compras"],
];

ROTAS.relatorios = async function () {
  $("#conteudo").innerHTML = `
  <div class="topo">
    <div><h1>Relatórios</h1><p class="sub">Escolha o relatório, o período e exporte em CSV (Excel) ou PDF</p></div>
  </div>
  <div class="painel">
    <div class="grade g4">
      <div class="campo"><label>Relatório</label><select id="rel-tipo">
        ${TIPOS_RELATORIO.map(([v, t]) => `<option value="${v}">${t}</option>`).join("")}</select></div>
      <div class="campo"><label>De</label><input type="date" id="rel-de" value="${diasAtras(30)}"></div>
      <div class="campo"><label>Até</label><input type="date" id="rel-ate" value="${hoje()}"></div>
      <div class="campo" style="display:flex;align-items:flex-end;gap:8px">
        <button class="btn primario" onclick="gerarRelatorio()">Gerar</button>
        <button class="btn" onclick="exportarRelatorio('csv')">CSV</button>
        <button class="btn" onclick="exportarRelatorio('pdf')">PDF</button>
      </div>
    </div>
    <div id="rel-corpo"><div class="vazio">Escolha um relatório e clique em Gerar.</div></div>
  </div>`;
  $("#rel-tipo").onchange = gerarRelatorio;
  gerarRelatorio();
};

function querystringRel() {
  const p = new URLSearchParams({
    de: $("#rel-de").value, ate: $("#rel-ate").value
  });
  return p.toString();
}

async function gerarRelatorio() {
  const tipo = $("#rel-tipo").value;
  const alvo = $("#rel-corpo");
  alvo.innerHTML = '<div class="vazio">Gerando…</div>';
  try {
    const r = await api("/relatorios/" + tipo + "?" + querystringRel());
    const linhas = r.linhas.map(l => "<tr>" + l.map((c, i) =>
      `<td class="${r.colunas[i] && r.colunas[i].align === "R" ? "dir num" : ""}">${esc(c)}</td>`).join("") + "</tr>").join("");
    alvo.innerHTML = `
      <div class="painel-topo" style="margin-top:14px">
        <div><h3 style="margin:0">${esc(r.titulo)}</h3><div class="muted small">${esc(r.subtitulo)}</div></div>
        <span class="muted small">${r.linhas.length} linha(s)</span>
      </div>
      ${r.linhas.length ? `<div class="tabela-wrap" style="max-height:52vh"><table>
        <thead><tr>${r.colunas.map(c => `<th class="${c.align === "R" ? "dir" : ""}">${esc(c.titulo)}</th>`).join("")}</tr></thead>
        <tbody>${linhas}</tbody></table></div>` : '<div class="vazio">Nenhum registro no período.</div>'}
      <div style="margin-top:12px">${r.resumo.map(k => `<div class="kv"><span>${esc(k.label)}</span><strong>${esc(k.valor)}</strong></div>`).join("")}</div>`;
  } catch (e) {
    alvo.innerHTML = '<div class="erro-box">' + esc(e.message) + "</div>";
  }
}
window.gerarRelatorio = gerarRelatorio;

function exportarRelatorio(formato) {
  const tipo = $("#rel-tipo").value;
  window.location = `/api/relatorios/${tipo}/${formato}?${querystringRel()}`;
}
window.exportarRelatorio = exportarRelatorio;

/* ================= BACKUP E AJUSTES ================= */
ROTAS.backup = async function () {
  const cfg = await api("/config");
  ESTADO.config = cfg;
  const b = await api("/backup");
  let plano = null;
  try { plano = await api("/saude-backup"); } catch (e) { plano = null; }
  $("#loja-nome").textContent = cfg.loja_nome || "Minha loja";
  const lista = b.itens.length ? `<div class="tabela-wrap" style="max-height:300px"><table><thead><tr>
      <th>Arquivo</th><th>Data</th><th class="dir">Tamanho</th><th></th></tr></thead><tbody>
      ${b.itens.map(x => `<tr>
        <td class="mono small">${esc(x.arquivo)} ${x.automatico ? '<span class="tag azul">automático</span>' : ""}</td>
        <td>${esc(x.data)}</td>
        <td class="dir">${(x.tamanho_bytes / 1024).toFixed(0)} KB</td>
        <td class="dir" style="white-space:nowrap">
          <a class="btn pequeno" href="/api/backup/baixar?arquivo=${encodeURIComponent(x.arquivo)}">Baixar</a>
          <button class="btn pequeno" onclick="restaurarBackup('${esc(x.arquivo)}')">Restaurar</button>
        </td></tr>`).join("")}</tbody></table></div>`
    : '<div class="vazio">Nenhum backup ainda. Clique em “Fazer backup agora”.</div>';

  $("#conteudo").innerHTML = `
  <div class="topo">
    <div><h1>Backup e ajustes</h1><p class="sub">Backup de hora em hora, ao fechar o caixa e cópia na nuvem</p></div>
  </div>
  ${htmlPlanoB(plano)}

  <div class="painel">
    <div class="painel-topo">
      <h3>Backup</h3>
      <div class="acoes">
        <button class="btn primario" onclick="fazerBackupRapido().then(()=>navegar())">Fazer backup agora</button>
      </div>
    </div>
    <div class="kv"><span>Último backup</span><strong>${cfg.ultimo_backup ? dataHoraBR(cfg.ultimo_backup) : "nunca"}</strong></div>
    <div class="kv"><span>Backup automático</span><span>${cfg.backup_auto === "1" ? "ligado (uma cópia a cada " + (plano ? plano.intervalo_minutos : 60) + " minutos)" : "desligado"}</span></div>
    <div class="kv"><span>Pasta onde os backups são salvos</span><span class="mono small">${esc(b.pasta)}</span></div>
    <div class="kv"><span>Backups diários guardados</span><span>${cfg.backup_manter} arquivos</span></div>
    <div class="kv"><span>Segunda pasta de backup (pendrive ou nuvem)</span>
      <span class="mono small">${cfg.backup_pasta_extra ? esc(cfg.backup_pasta_extra) : "não configurada"}</span></div>
    ${cfg.backup_pasta_extra_erro ? `<div class="erro-box">${esc(cfg.backup_pasta_extra_erro)}<br>
      Corrija o caminho em “Segunda pasta de backup” abaixo ou deixe o campo vazio.</div>` : ""}
    <div class="aviso-box" style="margin-top:12px">O backup é <strong>um arquivo único (.db)</strong>. Copie para um pen drive,
      para o Google Drive da loja ou outro computador. Para restaurar, use o botão em um backup da lista
      (ou envie o arquivo de outro computador abaixo).</div>
    ${lista}
  </div>

  <div class="painel">
    <h3>Restaurar backup de outro computador</h3>
    <p class="dica">Escolha o arquivo .db do backup. O sistema guarda um backup de segurança antes de restaurar.</p>
    <div style="display:flex;gap:10px;flex-wrap:wrap">
      <input type="file" id="arquivo-restore" accept=".db" style="max-width:420px">
      <button class="btn vermelho" onclick="restaurarArquivo()">Restaurar este arquivo</button>
    </div>
  </div>

  <div class="painel">
    <h3>Dados da loja e preferências</h3>
    <div class="grade g2" style="margin-top:12px">
      <div class="campo"><label>Nome da loja (aparece no cupom e relatórios)</label>
        <input type="text" id="cfg-nome" value="${esc(cfg.loja_nome)}"></div>
      <div class="campo"><label>CNPJ</label><input type="text" id="cfg-cnpj" value="${esc(cfg.loja_cnpj)}"></div>
      <div class="campo"><label>Telefone</label><input type="text" id="cfg-fone" value="${esc(cfg.loja_telefone)}"></div>
      <div class="campo"><label>Endereço</label><input type="text" id="cfg-end" value="${esc(cfg.loja_endereco)}"></div>
      <div class="campo"><label>Prazo padrão de faturado (dias)</label>
        <input type="number" id="cfg-prazo" value="${esc(cfg.prazo_faturado_dias)}" min="0"></div>
      <div class="campo"><label>Backups diários a manter</label>
        <input type="number" id="cfg-manter" value="${esc(cfg.backup_manter)}" min="1"></div>
      <div class="campo"><label>Teto de espaço da pasta de backups (MB)</label>
        <input type="number" id="cfg-limite-mb" value="${esc(cfg.backup_limite_mb || "300")}" min="20" max="20000">
        <div class="dica">Se a pasta passar disso, os backups mais antigos são apagados
          automaticamente. 0 = sem teto.</div></div>
      <div class="campo"><label>Backup de hora em hora: intervalo (minutos)</label>
        <input type="number" id="cfg-intervalo" value="${esc(cfg.backup_intervalo_minutos || "60")}" min="15" max="1440">
        <div class="dica">60 = uma cópia por hora. Mínimo 15, máximo 1440 (um dia).</div></div>
      <div class="campo"><label>Cópias de hora em hora a manter</label>
        <input type="number" id="cfg-manter-horarios" value="${esc(cfg.backup_manter_horarios || "24")}" min="1" max="500">
        <div class="dica">1 = guarda só a última. Recomendado 3 (a de agora e as duas anteriores).</div></div>
      <div class="campo"><label>Cópias de fechamento de caixa a manter</label>
        <input type="number" id="cfg-manter-fechamentos" value="${esc(cfg.backup_manter_fechamentos || "60")}" min="1" max="500"></div>
      <div class="campo"><label>Cópias manuais a manter</label>
        <input type="number" id="cfg-manter-manuais" value="${esc(cfg.backup_manter_manuais || "60")}" min="1" max="500"></div>
      <div class="campo"><label>Cópias de segurança (antes de restaurar) a manter</label>
        <input type="number" id="cfg-manter-seguranca" value="${esc(cfg.backup_manter_seguranca || "10")}" min="1" max="100"></div>
      <div class="campo"><label>Segunda pasta de backup — pendrive ou nuvem (opcional)</label>
        <div style="display:flex;gap:8px;flex-wrap:wrap">
          <input type="text" id="cfg-pasta-extra" value="${esc(cfg.backup_pasta_extra || "")}"
            placeholder="ex.: C:\Users\Voce\Dropbox\Backup AutoPecas" style="flex:1 1 320px;min-width:220px">
          <button class="btn" type="button" onclick="escolherPastaNuvem()">Escolher a pasta…</button>
          <button class="btn" type="button" onclick="testarPastaNuvem()">Testar</button>
        </div>
        <div class="dica">Depois de cada backup o sistema copia o mesmo arquivo para esta pasta.
          Use a pasta do Dropbox, Google Drive ou OneDrive que já está no seu computador: o AutoPeças
          grava nela e o aplicativo da nuvem faz o envio. O programa não usa internet.</div></div>
    </div>
    <div class="campo">
      <label><input type="checkbox" id="cfg-auto" ${cfg.backup_auto === "1" ? "checked" : ""}> Fazer backups automáticos (de hora em hora e ao abrir o programa)</label>
      <label><input type="checkbox" id="cfg-neg" ${cfg.permitir_estoque_negativo === "1" ? "checked" : ""}> Permitir vender peça sem estoque (saldo negativo)</label>
    </div>
    <div class="painel" style="background:#f7f9fc;margin:6px 0 14px">
      <div class="painel-topo" style="margin-bottom:4px"><h3 style="font-size:14px">Quanto espaço os backups ocupam</h3></div>
      <p class="dica" style="margin-top:0">Cada backup é o banco inteiro. Escolha um perfil — ele já apaga o que passou do limite:</p>
      <div class="acoes" style="margin-top:6px">
        <button class="btn" onclick="presetRetencao('minimo')">Economia máxima (1 de cada)</button>
        <button class="btn" onclick="presetRetencao('recomendado')">Econômico e seguro (3 horários + 7 dias)</button>
        <button class="btn" onclick="presetRetencao('padrao')">Padrão (24 horários + 30 dias)</button>
      </div>
      <div class="dica" style="margin-top:6px">“Economia máxima” mantém só a última cópia de cada tipo.
        O risco é pequeno (todo backup é conferido ao ser criado), mas com 3 cópias de hora em hora
        você ainda tem as duas horas anteriores se precisar voltar atrás.</div>
    </div>
    <button class="btn primario" onclick="salvarConfig()">Salvar ajustes</button>
    <button class="btn vermelho" style="margin-left:8px" onclick="encerrarSistema()">Encerrar o sistema</button>
    <div class="dica" style="margin-top:10px">Pasta de dados: <span class="mono">${esc(cfg.pasta_dados)}</span></div>
  </div>`;

  $("#arquivo-restore").onchange = () => {};
};

async function salvarConfig() {
  const corpo = {
    loja_nome: $("#cfg-nome").value.trim(),
    loja_cnpj: $("#cfg-cnpj").value.trim(),
    loja_telefone: $("#cfg-fone").value.trim(),
    loja_endereco: $("#cfg-end").value.trim(),
    prazo_faturado_dias: $("#cfg-prazo").value,
    backup_manter: $("#cfg-manter").value,
    backup_intervalo_minutos: ($("#cfg-intervalo") || {}).value || "60",
    backup_manter_horarios: ($("#cfg-manter-horarios") || {}).value || "24",
    backup_manter_fechamentos: ($("#cfg-manter-fechamentos") || {}).value || "60",
    backup_manter_manuais: ($("#cfg-manter-manuais") || {}).value || "60",
    backup_manter_seguranca: ($("#cfg-manter-seguranca") || {}).value || "10",
    backup_limite_mb: ($("#cfg-limite-mb") || {}).value || "300",
    backup_pasta_extra: $("#cfg-pasta-extra") ? $("#cfg-pasta-extra").value.trim() : "",
    backup_auto: $("#cfg-auto").checked ? "1" : "0",
    permitir_estoque_negativo: $("#cfg-neg").checked ? "1" : "0"
  };
  // 1.0.10: campo numerico vazio/lixo tem que avisar qual campo e' (antes ia
  // assim para o servidor e o ajuste ficava quebrado).
  const numeros = [
    ["prazo_faturado_dias", "Prazo do faturado (dias)", 1, 3650],
    ["backup_manter", "Cópias diárias a manter", 1, 999],
    ["backup_intervalo_minutos", "Intervalo do backup (minutos)", 15, 1440],
    ["backup_manter_horarios", "Cópias de hora em hora a manter", 1, 999],
    ["backup_manter_fechamentos", "Cópias de fechamento a manter", 1, 999],
    ["backup_manter_manuais", "Cópias manuais a manter", 1, 999],
    ["backup_manter_seguranca", "Cópias de segurança a manter", 1, 999],
    ["backup_limite_mb", "Teto de espaço dos backups (MB)", 0, 100000]
  ];
  for (const [chave, rotulo, minimo, maximo] of numeros) {
    const bruto = String(corpo[chave] == null ? "" : corpo[chave]).trim();
    const n = Number(bruto);
    if (bruto === "" || !Number.isFinite(n) || !Number.isInteger(n)) {
      toast(`${rotulo}: informe um número inteiro.`, "erro");
      return;
    }
    if (n < minimo || n > maximo) {
      toast(`${rotulo}: use um valor entre ${minimo} e ${maximo}.`, "erro");
      return;
    }
    corpo[chave] = String(n);
  }
  try {
    await api("/config", { corpo });
    toast("Ajustes salvos.", "ok");
    const cfg = await api("/config");
    ESTADO.config = cfg;
    $("#loja-nome").textContent = cfg.loja_nome;
  } catch (e) { toast(e.message, "erro"); }
}
window.salvarConfig = salvarConfig;

function restaurarBackup(arquivo) {
  confirmar(`Restaurar o backup "${arquivo}"? Todos os dados atuais serão substituídos pelos do arquivo.`,
    async () => {
      await api("/restaurar?arquivo=" + encodeURIComponent(arquivo), { method: "POST" });
      toast("Backup restaurado com sucesso.", "ok");
      const cfg = await api("/config");
      ESTADO.config = cfg;
      navegar(); atualizarAlertas();
    }, "Restaurar agora");
}
window.restaurarBackup = restaurarBackup;

function restaurarArquivo() {
  const inp = $("#arquivo-restore");
  if (!inp.files || !inp.files[0]) { toast("Escolha o arquivo .db do backup.", "aviso"); return; }
  const f = inp.files[0];
  confirmar(`Restaurar do arquivo "${f.name}"? Os dados atuais serão substituídos.`, async () => {
    const fd = new FormData();
    fd.append("arquivo", f);
    const resp = await fetch("/api/restaurar", { method: "POST", body: fd, headers: cabecalhosSeguros() });
    const r = await resp.json();
    if (!resp.ok) throw new Error(r.erro || "erro ao restaurar");
    toast("Backup restaurado com sucesso.", "ok");
    const cfg = await api("/config");
    ESTADO.config = cfg;
    navegar(); atualizarAlertas();
  }, "Restaurar este arquivo");
}
window.restaurarArquivo = restaurarArquivo;

function encerrarSistema() {
  confirmar("Encerrar o sistema? Feche esta aba do navegador depois. Para usar de novo, abra o atalho AutoPeças.",
    async () => {
      try { await api("/encerrar", { method: "POST" }); } catch (e) {}
      document.body.innerHTML = '<div style="display:grid;place-items:center;height:100vh;font-family:Segoe UI,Arial,sans-serif;text-align:center">' +
        '<div><h2>AutoPeças encerrado</h2><p>Você já pode fechar esta janela do navegador.<br>' +
        'Para abrir o sistema novamente, use o atalho do AutoPeças na área de trabalho.</p></div></div>';
    }, "Encerrar");
}
window.encerrarSistema = encerrarSistema;

/* ---------------- inicio ---------------- */
document.addEventListener("DOMContentLoaded", iniciar);

/* ================= CATÁLOGO DO FORNECEDOR ================= */
/* A peça que a loja NÃO tem: antes era catálogo de papel, agora é busca. */

ROTAS.catalogo = async function () {
  $("#conteudo").innerHTML = `
  <div class="topo">
    <div><h1>Catálogo do fornecedor</h1>
      <p class="sub">A peça que você não tem no estoque — busque pelo carro, pelo código, pela descrição ou por um equivalente</p></div>
    <div class="acoes">
      <button class="btn" onclick="semearCatalogo()">Montar com o que já tenho</button>
      <button class="btn" onclick="importarCatalogo()">Importar lista do fornecedor</button>
      <button class="btn primario" onclick="editarCatalogo(null)">+ Peça no catálogo</button>
    </div>
  </div>

  <div class="painel">
    <div class="painel-topo"><h3>Busca rápida do balcão</h3>
      <span class="muted small">Uma busca só: “eu tenho?” e “existe? qual o código?”</span></div>
    <div class="busca-grande">
      <input type="search" id="busca-rapida" placeholder="Ex.: gol 2012 filtro de oleo, W712/90, PSL560…" autocomplete="off">
    </div>
    <div id="resultado-rapida"><div class="vazio">Digite o carro, o código ou a descrição da peça.</div></div>
  </div>

  <div class="painel">
    <div class="painel-topo"><h3>Peças no catálogo</h3>
      <div class="acoes"><input type="search" id="busca-catalogo" placeholder="Filtrar o catálogo…" autocomplete="off" style="width:260px"></div>
    </div>
    <div id="lista-catalogo"><div class="vazio">Carregando…</div></div>
  </div>`;

  const inp = $("#busca-rapida");
  inp.addEventListener("input", debounce(buscaRapida, 200));
  $("#busca-catalogo").addEventListener("input", debounce(carregarCatalogo, 200));
  setTimeout(() => inp.focus(), 120);
  await carregarCatalogo();
};

async function buscaRapida() {
  const alvo = $("#resultado-rapida");
  if (!alvo) return;
  const q = $("#busca-rapida").value.trim();
  if (!q) { alvo.innerHTML = '<div class="vazio">Digite o carro, o código ou a descrição da peça.</div>'; return; }
  try {
    const r = await api("/busca?q=" + encodeURIComponent(q));
    if (!r.itens.length) {
      alvo.innerHTML = `<div class="vazio">Nada encontrado para <strong>${esc(q)}</strong>. Confira se a peça está no catálogo
        (botão “+ Peça no catálogo”) ou registre uma ficha de falta.</div>`;
      return;
    }
    alvo.innerHTML = `<div class="tabela-wrap"><table><thead><tr>
      <th>Situação</th><th>Código</th><th>Peça</th><th>Aplicação</th><th class="dir">Preço</th><th></th>
    </tr></thead><tbody>
    ${r.itens.map((it, i) => `<tr>
      <td>${it.origem === "estoque"
          ? `<span class="tag verde">TEM no estoque: ${fmtQtd(it.estoque)}</span>`
          : (it.tem_estoque
              ? `<span class="tag verde">no estoque: ${fmtQtd(it.estoque)}</span>`
              : '<span class="tag amarelo">não tenho</span>')}</td>
      <td class="mono">${esc(it.codigo)}</td>
      <td><strong>${esc(it.descricao)}</strong>
        ${it.equivalentes && it.equivalentes.length ? `<div class="muted small">equiv.: ${it.equivalentes.slice(0, 4).map(esc).join(", ")}</div>` : ""}</td>
      <td class="small">${esc(it.aplicacao || "—")}</td>
      <td class="dir num">${it.preco_centavos ? fmtMoney(it.preco_centavos) : "—"}</td>
      <td class="dir" style="white-space:nowrap">
        ${it.origem === "catalogo" && !it.tem_estoque
          ? `<button class="btn pequeno" onclick="fichaDoCatalogo(${it.id})">Ficha de falta</button>` : ""}
        ${it.origem === "estoque" ? `<button class="btn pequeno" onclick="editarPeca(${it.id})">Abrir peça</button>` : ""}
      </td>
    </tr>`).join("")}
    </tbody></table></div>
    <p class="dica">${r.itens.filter(x => x.origem === "estoque" || x.tem_estoque).length} na prateleira · 
      ${r.itens.filter(x => x.origem === "catalogo" && !x.tem_estoque).length} só no catálogo (dá para encomendar).</p>`;
  } catch (e) { alvo.innerHTML = '<div class="erro-box">' + esc(e.message) + "</div>"; }
}
window.buscaRapida = buscaRapida;

function tagEstoqueCatalogo(it) {
  if (it.tem_estoque) return `<span class="tag verde">tem: ${fmtQtd(it.estoque)}</span>`;
  return '<span class="tag amarelo">não tenho</span>';
}

async function carregarCatalogo() {
  const alvo = $("#lista-catalogo");
  if (!alvo) return;
  const q = $("#busca-catalogo") ? $("#busca-catalogo").value.trim() : "";
  try {
    const r = await api("/catalogo?q=" + encodeURIComponent(q) + "&limite=300");
    if (!r.itens.length) {
      alvo.innerHTML = '<div class="vazio">' + (q
        ? "Nada no catálogo para esta busca."
        : "O catálogo está vazio. Use “Importar lista do fornecedor” (CSV) ou cadastre as peças que você mais encomenda.") + "</div>";
      return;
    }
    alvo.innerHTML = `<div class="tabela-wrap"><table><thead><tr>
      <th>Código</th><th>Peça</th><th>Aplicação</th><th>Medida / critérios</th><th>Marca</th>
      <th class="dir">Custo</th><th>Estoque</th><th></th></tr></thead><tbody>
      ${r.itens.map(it => `<tr>
        <td class="mono">${esc(it.codigo)}</td>
        <td><strong>${esc(it.descricao)}</strong>
          ${it.equivalentes.length ? `<div class="muted small">equiv.: ${it.equivalentes.slice(0, 5).map(esc).join(", ")}</div>` : ""}</td>
        <td class="small">${esc(it.aplicacao || "—")}</td>
        <td class="small">${esc(it.medida || "—")}
          ${it.criterios ? `<div class="muted small">${esc(it.criterios)}</div>` : ""}</td>
        <td class="small">${esc(it.marca || "—")}</td>
        <td class="dir num">${it.custo_centavos ? fmtMoney(it.custo_centavos) : "—"}</td>
        <td>${tagEstoqueCatalogo(it)}${it.peca_codigo ? `<div class="muted small mono">${esc(it.peca_codigo)}</div>` : ""}</td>
        <td class="dir" style="white-space:nowrap">
          <button class="btn pequeno" onclick="fichaDoCatalogo(${it.id})">Ficha</button>
          <button class="btn pequeno" onclick="editarCatalogo(${it.id})">Editar</button>
        </td>
      </tr>`).join("")}
      </tbody></table></div>
      <p class="dica">${r.itens.length} peça(s) no catálogo. O catálogo não mexe no estoque: quem dá entrada é o XML, a nota por foto ou o cadastro de peça.</p>`;
  } catch (e) { alvo.innerHTML = '<div class="erro-box">' + esc(e.message) + "</div>"; }
}
window.carregarCatalogo = carregarCatalogo;

async function editarCatalogo(id) {
  let it = { id: 0, codigo: "", descricao: "", aplicacao: "", medida: "", criterios: "", marca: "", categoria: "",
    custo_centavos: 0, observacoes: "", equivalentes: [] };
  if (id) {
    try {
      const r = await api("/catalogo?limite=500");
      const achado = r.itens.find(x => x.id === id);
      if (achado) it = achado;
    } catch (e) { toast(e.message, "erro"); return; }
  }
  abrirModal(id ? "Editar peça do catálogo" : "Peça no catálogo", `
    <div class="grade g3">
      <div class="campo"><label>Código *</label><input type="text" id="c-codigo" value="${esc(it.codigo)}" placeholder="Ex.: W712/90"></div>
      <div class="campo"><label>Marca</label><input type="text" id="c-marca" value="${esc(it.marca)}" placeholder="Ex.: MANN"></div>
      <div class="campo"><label>Medida / especificação</label><input type="text" id="c-medida" value="${esc(it.medida)}" placeholder="Ex.: ROSCA 3/4-16"></div>
    </div>
    <div class="campo"><label>Descrição</label><input type="text" id="c-desc" value="${esc(it.descricao)}" placeholder="Ex.: FILTRO DE ÓLEO"></div>
    <div class="campo"><label>Aplicação — para quais veículos serve (é o que a busca usa)</label>
      <input type="text" id="c-aplic" value="${esc(it.aplicacao)}" placeholder="Ex.: VW GOL 1.0/1.6 08/16; VOYAGE 09/16"></div>
    <div class="campo"><label>Critérios técnicos — rosca, dentes, altura, encaixe (a busca também usa)</label>
      <input type="text" id="c-criterios" value="${esc(it.criterios || "")}" placeholder="Ex.: rosca 3/4-16; altura 78mm; válvula anti-retorno"></div>
    <div class="grade g3">
      <div class="campo"><label>Categoria</label><input type="text" id="c-cat" value="${esc(it.categoria)}" placeholder="Ex.: Filtros"></div>
      <div class="campo"><label>Custo (R$)</label><input type="text" id="c-custo" value="${it.custo_centavos ? (it.custo_centavos / 100).toFixed(2).replace(".", ",") : ""}" inputmode="decimal"></div>
    </div>
    <div class="campo">
      <label>Códigos equivalentes de outras marcas</label>
      <div style="display:flex;gap:8px">
        <input type="text" id="c-novo-equiv" placeholder="Digite um código e tecle Enter">
        <button class="btn" onclick="adicionarEquivCatalogo()">Adicionar</button>
      </div>
      <div class="grid-equivalentes" id="c-equivs"></div>
    </div>
    <div class="campo"><label>Observação</label><input type="text" id="c-obs" value="${esc(it.observacoes || "")}" placeholder="Ex.: fornecedor só vende caixa com 10"></div>
  `,
    `<button class="btn" onclick="fecharModal()">Cancelar</button>
     ${id ? `<button class="btn vermelho" onclick="excluirCatalogo(${id})">Excluir do catálogo</button>` : ""}
     <button class="btn primario" onclick="salvarCatalogoItem(${id})">Salvar</button>`);

  window.__equivsCat = (it.equivalentes || []).slice();
  renderEquivsCatalogo();
  $("#c-novo-equiv").addEventListener("keydown", ev => {
    if (ev.key === "Enter") { ev.preventDefault(); adicionarEquivCatalogo(); }
  });
}
window.editarCatalogo = editarCatalogo;

function renderEquivsCatalogo() {
  const div = $("#c-equivs");
  if (!div) return;
  if (!window.__equivsCat.length) { div.innerHTML = '<span class="muted small">Nenhum equivalente cadastrado.</span>'; return; }
  div.innerHTML = window.__equivsCat.map((e, i) =>
    `<span class="chip">${esc(e)}<button onclick="removerEquivCatalogo(${i})">✕</button></span>`).join("");
}
function adicionarEquivCatalogo() {
  const inp = $("#c-novo-equiv");
  const v = inp.value.trim();
  if (!v) return;
  if (!window.__equivsCat.some(e => e.toUpperCase() === v.toUpperCase())) window.__equivsCat.push(v.toUpperCase());
  inp.value = "";
  renderEquivsCatalogo();
  inp.focus();
}
function removerEquivCatalogo(i) { window.__equivsCat.splice(i, 1); renderEquivsCatalogo(); }
window.adicionarEquivCatalogo = adicionarEquivCatalogo;
window.removerEquivCatalogo = removerEquivCatalogo;

async function salvarCatalogoItem(id) {
  const corpo = {
    codigo: $("#c-codigo").value.trim(),
    descricao: $("#c-desc").value.trim(),
    aplicacao: $("#c-aplic").value.trim(),
    medida: $("#c-medida").value.trim(),
    criterios: $("#c-criterios").value.trim(),
    marca: $("#c-marca").value.trim(),
    categoria: $("#c-cat").value.trim(),
    custo_centavos: parseDinheiro($("#c-custo").value),
    observacoes: $("#c-obs").value.trim(),
    equivalentes: window.__equivsCat
  };
  if (!corpo.codigo) { toast("Informe o código da peça.", "aviso"); return; }
  if (!corpo.descricao && !corpo.aplicacao) { toast("Informe a descrição ou a aplicação.", "aviso"); return; }
  try {
    await api(id ? "/catalogo/" + id : "/catalogo", { corpo, method: id ? "PUT" : "POST" });
    toast("Catálogo salvo: " + corpo.codigo, "ok");
    fecharModal();
    carregarCatalogo();
    if ($("#busca-rapida") && $("#busca-rapida").value.trim()) buscaRapida();
  } catch (e) { toast(e.message, "erro"); }
}
window.salvarCatalogoItem = salvarCatalogoItem;

function excluirCatalogo(id) {
  confirmar("Tirar esta peça do catálogo? O estoque não é afetado.", async () => {
    await api("/catalogo/" + id, { method: "DELETE" });
    toast("Peça removida do catálogo.", "ok");
    fecharModal();
    carregarCatalogo();
  }, "Excluir");
}
window.excluirCatalogo = excluirCatalogo;

/* Ficha de falta direto do catálogo: achou, não tem, o cliente quer. */
async function fichaDoCatalogo(id) {
  let it = null;
  try {
    const r = await api("/catalogo?limite=500");
    it = r.itens.find(x => x.id === id) || null;
  } catch (e) { toast(e.message, "erro"); return; }
  if (!it) { toast("Não achei este item no catálogo.", "erro"); return; }
  abrirModal("Ficha de falta — " + it.codigo, `
    <div class="ok-box" style="margin-bottom:12px">
      <strong>${esc(it.descricao || it.codigo)}</strong><br>
      <span class="small">${esc(it.aplicacao || "sem aplicação informada")}</span>
    </div>
    <div class="grade g3">
      <div class="campo"><label>Quantidade</label><input type="number" id="f-qtd" value="1" min="1" step="1"></div>
      <div class="campo"><label>Cliente</label><input type="text" id="f-cliente" placeholder="Nome do cliente"></div>
      <div class="campo"><label>Telefone</label><input type="text" id="f-fone" placeholder="(00) 00000-0000"></div>
    </div>
    <div class="campo"><label>Veículo do cliente</label><input type="text" id="f-veiculo" value="${esc(it.aplicacao || "")}" placeholder="Ex.: Gol 2013 placa ABC1234"></div>
    <div class="campo"><label>Observação</label><input type="text" id="f-obs" placeholder="Ex.: cliente precisa até sexta"></div>
    <div class="dica">A ficha entra como <strong>aberta</strong> na tela Fichas de Falta, com o código e a aplicação já preenchidos.
      Nada é lançado no estoque.</div>
  `,
    `<button class="btn" onclick="fecharModal()">Cancelar</button>
     <button class="btn primario" onclick="salvarFichaDoCatalogo(${id})">Criar ficha</button>`);
}
window.fichaDoCatalogo = fichaDoCatalogo;

async function salvarFichaDoCatalogo(id) {
  const corpo = {
    quantidade: parseQtd($("#f-qtd").value) || 1,
    cliente_nome: $("#f-cliente").value.trim(),
    cliente_telefone: $("#f-fone").value.trim(),
    veiculo: $("#f-veiculo").value.trim(),
    observacoes: $("#f-obs").value.trim()
  };
  try {
    const r = await api("/catalogo/" + id + "/ficha", { corpo });
    toast("Ficha #" + r.ficha_id + " criada.", "ok");
    fecharModal();
    if (rotaAtual() === "fichas") carregarFichas();
    atualizarAlertas();
  } catch (e) { toast(e.message, "erro"); }
}
window.salvarFichaDoCatalogo = salvarFichaDoCatalogo;

/* Montar o catálogo com o que a loja já tem dentro do sistema (sem planilha) */
function semearCatalogo() {
  abrirModal("Montar o catálogo com o que já tenho", `
    <div class="ok-box" style="margin-bottom:12px">
      O catálogo começa a se montar <strong>sozinho</strong>, sem planilha nenhuma:
    </div>
    <ul style="margin:0 0 12px 18px;line-height:1.7">
      <li><strong>Peças do estoque</strong> — cada peça cadastrada vira item de catálogo,
        com a aplicação e os códigos equivalentes que ela já tem.</li>
      <li><strong>Fichas de falta</strong> — toda ficha feita pelo botão “Ficha” (com o código
        entre colchetes) volta para o catálogo, com o veículo que o cliente informou.</li>
    </ul>
    <div class="dica">Daqui para frente o catálogo cresce a cada consulta que você registra.
      Rodar de novo <strong>atualiza</strong> os itens, não duplica. Nada é lançado no estoque.</div>
    <div id="res-semear"></div>
  `,
    `<button class="btn" onclick="fecharModal()">Fechar</button>
     <button class="btn primario" onclick="executarSemear()">Montar agora</button>`);
}
window.semearCatalogo = semearCatalogo;

async function executarSemear() {
  const alvo = $("#res-semear");
  alvo.innerHTML = '<div class="vazio">Montando o catálogo…</div>';
  try {
    const r = await api("/catalogo/semear", { corpo: { pecas: true, fichas: true } });
    alvo.innerHTML = `<div class="ok-box"><strong>${r.novos}</strong> item(ns) novo(s), 
      <strong>${r.atualizados}</strong> atualizado(s)${r.ignorados ? `, ${r.ignorados} ignorado(s)` : ""}.
      O catálogo está com <strong>${r.total_no_catalogo}</strong> peça(s).</div>
      ${r.problemas && r.problemas.length ? `<div class="lista-ignorados"><strong>Não entraram</strong><ul>` +
        r.problemas.map(p => `<li>${esc(p)}</li>`).join("") + `</ul></div>` : ""}`;
    carregarCatalogo();
  } catch (e) { alvo.innerHTML = '<div class="erro-box">' + esc(e.message) + "</div>"; }
}
window.executarSemear = executarSemear;

/* Importar a lista do fornecedor (CSV com ; ou ,) */
function importarCatalogo() {
  abrirModal("Importar lista do fornecedor", `
    <div class="campo">
      <label>Arquivo CSV ou TXT (planilha salva como CSV)</label>
      <input type="file" id="arq-catalogo" accept=".csv,.txt">
      <div class="dica">Primeira linha com os títulos: <span class="mono">codigo;descricao;aplicacao;medida;marca;custo;equivalentes;criterios</span><br>
        Aceita também sem título, nessa mesma ordem. A coluna de critérios (rosca, dentes, altura) também pode vir
        como “critérios técnicos”, “especificações” ou “atributos”. Reimportar o mesmo arquivo
        <strong>atualiza</strong> as peças, não duplica.</div>
    </div>
    <div id="res-import-catalogo"></div>
  `,
    `<button class="btn" onclick="fecharModal()">Fechar</button>
     <button class="btn primario" onclick="enviarCatalogo()">Importar</button>`);
}
window.importarCatalogo = importarCatalogo;

async function enviarCatalogo() {
  const inp = $("#arq-catalogo");
  const alvo = $("#res-import-catalogo");
  if (!inp.files.length) { toast("Escolha o arquivo da lista.", "aviso"); return; }
  const fd = new FormData();
  fd.append("arquivo", inp.files[0]);
  alvo.innerHTML = '<div class="vazio">Importando…</div>';
  try {
    const resp = await fetch("/api/catalogo/importar", { method: "POST", body: fd, headers: cabecalhosSeguros() });
    const r = await resp.json();
    if (!resp.ok) throw new Error(r.erro || "não consegui importar");
    let html = `<div class="ok-box"><strong>${r.importados}</strong> peça(s) nova(s), 
      <strong>${r.atualizados}</strong> atualizada(s), <strong>${r.ignorados}</strong> ignorada(s).</div>`;
    if (r.problemas && r.problemas.length) {
      html += `<div class="lista-ignorados"><strong>Linhas com problema</strong><ul>` +
        r.problemas.map(p => `<li>${esc(p)}</li>`).join("") + `</ul></div>`;
    }
    alvo.innerHTML = html;
    carregarCatalogo();
  } catch (e) { alvo.innerHTML = '<div class="erro-box">' + esc(e.message) + "</div>"; }
}
window.enviarCatalogo = enviarCatalogo;

/* ================= PLANO B (1.0.7) ================= */

// htmlPlanoB desenha o painel de saude do backup. Recebe a resposta de
// /api/saude-backup (ou null quando o programa nao respondeu).
function htmlPlanoB(p) {
  if (!p) return `<div class="painel"><h3>Plano B</h3>
    <div class="erro-box" style="margin-top:8px">Não consegui ler o estado dos backups agora.
    Recarregue a página; se continuar, faça um backup em “Fazer backup agora”.</div></div>`;

  const cores = { verde: "#1d7a46", amarelo: "#8a6100", vermelho: "#b3261e" };
  const fundos = { verde: "#e7f6ed", amarelo: "#fff6e0", vermelho: "#fdecea" };
  const titulos = {
    verde: "Protegido — o backup está em dia",
    amarelo: "Atenção — o último backup tem mais de 1 hora",
    vermelho: "PERIGO — sem backup recente"
  };
  const quando = p.minutos_desde_ultimo < 0 ? "nenhum backup ainda"
    : p.minutos_desde_ultimo < 1 ? "agora mesmo"
    : p.minutos_desde_ultimo < 60 ? "há " + p.minutos_desde_ultimo + " minuto(s)"
    : p.minutos_desde_ultimo < 48 * 60 ? "há " + Math.round(p.minutos_desde_ultimo / 60) + " hora(s)"
    : "há " + Math.round(p.minutos_desde_ultimo / 1440) + " dia(s)";

  const nuvem = p.nuvem || {};
  const nuvemTexto = !nuvem.pasta
    ? '<span class="tag amarelo">não configurada</span> — só neste computador'
    : (nuvem.ok ? '<span class="tag verde">copiando</span> ' + esc(nuvem.pasta)
                : '<span class="tag vermelho">com problema</span> ' + esc(nuvem.pasta));

  const tipos = (p.categorias || []).map(c => `<tr>
      <td>${esc(c.rotulo)}</td>
      <td class="dir">${c.quantidade}</td>
      <td class="dir muted">de ${c.limite}</td>
    </tr>`).join("");

  return `<div class="painel" id="painel-plano-b">
    <div class="painel-topo">
      <h3>Plano B — proteção dos seus dados</h3>
      <div class="acoes">
        <button class="btn primario" onclick="fazerBackupRapido().then(()=>navegar())">Fazer backup agora</button>
        <button class="btn" onclick="testarRestauracao()">Testar a restauração</button>
        <button class="btn" onclick="gerarKitEmergencia()">Gerar kit de emergência</button>
        <button class="btn" onclick="apagarBackupsAntigos()" title="Aplica agora os limites configurados">Apagar os antigos agora</button>
      </div>
    </div>

    <div class="plano-status" style="background:${fundos[p.nivel]};border-left:5px solid ${cores[p.nivel]}">
      <div>
        <strong style="color:${cores[p.nivel]};font-size:15px">${titulos[p.nivel]}</strong>
        <div class="small" style="margin-top:2px">Último backup: <strong>${esc(quando)}</strong>
          ${p.ultimo_backup ? `<span class="muted">(${dataHoraBR(p.ultimo_backup)})</span>` : ""}
          · ${p.total_arquivos} arquivo(s) · ${(p.total_bytes / 1048576).toFixed(1)} MB
          ${p.limite_mb && Number(p.limite_mb) > 0 ? ` <span class="muted">(teto ${p.limite_mb} MB)</span>` : ""}</div>
      </div>
      <div class="small" style="text-align:right">
        Cópia automática a cada <strong>${p.intervalo_minutos} min</strong><br>
        ${p.disco && p.disco.ok ? esc(p.disco.texto) + " no disco" : ""}
      </div>
    </div>

    <div class="grade g2" style="margin-top:12px">
      <div>
        <div class="kv"><span>Última cópia de hora em hora</span><strong>${p.ultimo_horario ? dataHoraBR(p.ultimo_horario.replace("T", " ").slice(0, 19)) : "—"}</strong></div>
        <div class="kv"><span>Último fechamento de caixa</span><strong>${esc(p.ultimo_fechamento || "—")}</strong></div>
        <div class="kv"><span>Cópia na nuvem / pendrive</span><span class="small">${nuvemTexto}</span></div>
        <div class="kv"><span>Pasta dos backups</span>
          <span class="mono small">${esc(p.pasta)}
            <button class="btn pequeno" style="margin-left:6px" onclick="abrirPastaSistema('backup')">Abrir</button></span></div>
        ${nuvem.erro ? `<div class="erro-box" style="margin-top:10px">${esc(nuvem.erro)}<br>
          <span class="small">O backup neste computador continua funcionando. Corrija o caminho
          em “Segunda pasta de backup” abaixo.</span></div>` : ""}
        ${p.ultimo_erro ? `<div class="erro-box" style="margin-top:10px">Falha registrada: ${esc(p.ultimo_erro)}</div>` : ""}
      </div>
      <div>
        <div class="tabela-wrap" style="max-height:none">
          <table><thead><tr><th>Tipo de cópia</th><th class="dir">Guardadas</th><th class="dir">Limite</th></tr></thead>
          <tbody>${tipos}</tbody></table>
        </div>
        <div class="dica" style="margin-top:8px">Se o computador quebrar: use o
          <strong>kit de emergência</strong> (programa + backup + instrução em um zip) em qualquer
          outro computador. Se a luz acabar, o banco não corrompe — mas o que não foi lançado se perde.</div>
      </div>
    </div>
  </div>`;
}
window.htmlPlanoB = htmlPlanoB;

/* Apaga os backups antigos na hora, aplicando os limites salvos.
   Serve para quem quer disco livre sem esperar o proximo backup. */
async function apagarBackupsAntigos() {
  confirmar("Apagar agora os backups que passaram do limite? Os mais novos de cada tipo ficam.",
    async () => {
      try {
        const r = await api("/backup/limpar", { method: "POST" });
        if (r.removidos > 0) {
          toast(`${r.removidos} arquivo(s) apagado(s) — ${(r.liberados_bytes / 1048576).toFixed(1)} MB liberados. Restam ${r.total_texto}.`, "ok");
        } else {
          toast(`Nada para apagar: a pasta está com ${r.total_texto}.`, "ok");
        }
        navegar();
      } catch (e) { toast(e.message, "erro"); }
    }, "Apagar antigos");
}
window.apagarBackupsAntigos = apagarBackupsAntigos;

/* Presets de retencao: um clique para "disco no minimo" ou "voltar ao padrao".
   Economia maxima = 1 copia de cada tipo (a mais nova). */
async function presetRetencao(qual) {
  const valores = {
    minimo:    { backup_manter: "1", backup_manter_horarios: "1", backup_manter_fechamentos: "1", backup_manter_manuais: "1", backup_manter_seguranca: "1" },
    recomendado: { backup_manter: "7", backup_manter_horarios: "3", backup_manter_fechamentos: "30", backup_manter_manuais: "10", backup_manter_seguranca: "3" },
    padrao:    { backup_manter: "30", backup_manter_horarios: "24", backup_manter_fechamentos: "60", backup_manter_manuais: "60", backup_manter_seguranca: "10" }
  }[qual];
  if (!valores) return;
  try {
    await api("/config", { corpo: valores });
    const r = await api("/backup/limpar", { method: "POST" });
    toast(`Limites aplicados${r.removidos ? " — " + r.removidos + " arquivo(s) apagado(s), " + (r.liberados_bytes / 1048576).toFixed(1) + " MB livres" : ""}.`,
      "ok");
    navegar();
  } catch (e) { toast(e.message, "erro"); }
}
window.presetRetencao = presetRetencao;

async function testarRestauracao() {
  try {
    const r = await api("/backup/testar", { method: "POST" });
    abrirModal("Teste de restauração — deu certo",
      `<div class="ok-box">O backup <strong>${esc(r.arquivo)}</strong> abre e está íntegro.
        Nada foi alterado no sistema.</div>
       <div class="kv"><span>Peças cadastradas</span><strong>${r.pecas}</strong></div>
       <div class="kv"><span>Vendas concluídas</span><strong>${r.vendas}</strong></div>
       <div class="kv"><span>Clientes</span><strong>${r.clientes}</strong></div>
       <div class="kv"><span>Movimentações de estoque</span><strong>${r.movimentos}</strong></div>
       <div class="kv"><span>Última venda registrada</span><strong>${r.ultima_venda ? dataHoraBR(r.ultima_venda) : "—"}</strong></div>
       <div class="kv"><span>Tamanho do arquivo</span><strong>${(r.tamanho_bytes / 1048576).toFixed(2)} MB</strong></div>
       <p class="dica">É assim que o sistema fica se você precisar restaurar este arquivo.</p>`,
      `<button class="btn primario" onclick="fecharModal()">Entendi</button>`, "min(560px,96vw)");
  } catch (e) { toast(e.message, "erro"); }
}
window.testarRestauracao = testarRestauracao;

async function gerarKitEmergencia() {
  try {
    toast("Montando o kit…", "");
    const r = await api("/kit-emergencia", { method: "POST" });
    let html = `<div class="ok-box">Kit pronto: <strong>${esc(r.nome)}</strong>
      (${(r.tamanho_bytes / 1048576).toFixed(1)} MB)</div>
      <p>Dentro do zip: o programa, o backup mais recente e um <strong>LEIA-ME</strong> com o passo a passo
      para voltar a funcionar em outro computador.</p>`;
    if (r.aviso) html += `<div class="aviso-box">${esc(r.aviso)}</div>`;
    abrirModal("Kit de emergência", html,
      `<a class="btn" href="/api/kit-emergencia/baixar">Baixar o kit</a>
       <button class="btn" onclick="abrirPastaSistema('kit')">Abrir a pasta</button>
       <button class="btn primario" onclick="fecharModal()">Fechar</button>`, "min(560px,96vw)");
  } catch (e) { toast(e.message, "erro"); }
}
window.gerarKitEmergencia = gerarKitEmergencia;

async function abrirPastaSistema(qual) {
  try { await api("/abrir-pasta", { corpo: { qual: qual } }); }
  catch (e) { toast(e.message, "aviso"); }
}
window.abrirPastaSistema = abrirPastaSistema;

// escolherPastaNuvem lista as pastas de nuvem encontradas no computador.
async function escolherPastaNuvem() {
  let r;
  try { r = await api("/nuvem"); } catch (e) { return toast(e.message, "erro"); }
  const cands = r.candidatas || [];
  let html = "";
  if (cands.length) {
    html += `<p class="dica" style="margin-top:0">Encontrei estas pastas de nuvem neste computador:</p>`;
    html += cands.map(c => `<div class="endereco-celular" style="margin-bottom:6px">
        <span style="min-width:150px"><strong>${esc(c.servico)}</strong></span>
        <code style="flex:1 1 240px">${esc(c.pasta)}</code>
        <button class="btn pequeno" onclick="usarPastaNuvem('${esc(c.pasta)}')">Usar</button>
      </div>`).join("");
  } else {
    html += `<div class="aviso-box">Não encontrei Dropbox, Google Drive nem OneDrive instalados neste computador.
      Se você usa um deles, instale o aplicativo (ele cria a pasta) e volte aqui — ou digite o caminho à mão.</div>`;
  }
  html += `<div class="campo" style="margin-top:12px"><label>Ou digite/escolha outra pasta</label>
      <div style="display:flex;gap:8px;flex-wrap:wrap">
        <input type="text" id="nuvem-manual" placeholder="C:\\Users\\Voce\\Dropbox\\Backup AutoPecas" style="flex:1 1 300px">
        <button class="btn" onclick="usarPastaNuvem($('#nuvem-manual').value)">Usar esta</button>
      </div>
      <div class="dica">A pasta pode ser de um pendrive também. O sistema grava nela sempre que fizer backup.</div></div>`;
  abrirModal("Escolher a pasta do backup", html,
    `<button class="btn" onclick="fecharModal()">Fechar</button>`, "min(640px,96vw)");
}
window.escolherPastaNuvem = escolherPastaNuvem;

async function usarPastaNuvem(pasta) {
  pasta = (pasta || "").trim();
  if (!pasta) return toast("Informe o caminho da pasta.", "aviso");
  try {
    const r = await api("/nuvem/testar", { corpo: { pasta: pasta } });
    if ($("#cfg-pasta-extra")) $("#cfg-pasta-extra").value = r.pasta || pasta;
    fecharModal();
    await salvarConfig();
    toast("Pasta da nuvem definida e testada: " + (r.pasta || pasta), "ok");
    navegar();
  } catch (e) { toast(e.message, "erro"); }
}
window.usarPastaNuvem = usarPastaNuvem;

async function testarPastaNuvem() {
  const pasta = ($("#cfg-pasta-extra") || {}).value || "";
  try {
    const r = await api("/nuvem/testar", { corpo: { pasta: pasta } });
    toast(r.vazio ? "Nenhuma pasta escolhida (o backup fica só neste computador)." : "Pasta OK: " + r.detalhe, "ok");
  } catch (e) { toast(e.message, "erro"); }
}
window.testarPastaNuvem = testarPastaNuvem;

// fecharCaixaComBackup: encerra o dia e ja guarda o backup do fechamento.
async function fecharCaixaComBackup() {
  const data = ($("#fech-data") || {}).value || hoje();
  confirmar(`Fechar o caixa do dia ${dataBR(data)} e guardar o backup deste momento?`,
    async () => {
      const r = await api("/fechamento/fechar", { corpo: { data: data } });
      let html = `<div class="ok-box">Caixa de <strong>${dataBR(r.data)}</strong> fechado.
        Backup guardado: <strong>${esc(r.arquivo)}</strong> (${(r.tamanho_bytes / 1048576).toFixed(1)} MB).</div>`;
      if (r.aviso_nuvem) html += `<div class="aviso-box">${esc(r.aviso_nuvem)}</div>`;
      abrirModal("Caixa fechado", html,
        `<button class="btn" onclick="fecharModal();imprimirFechamento()">Imprimir o resumo</button>
         <button class="btn primario" onclick="fecharModal()">Fechar</button>`, "min(540px,96vw)");
      toast("Caixa fechado e backup guardado.", "ok");
    }, "Fechar o caixa");
}
window.fecharCaixaComBackup = fecharCaixaComBackup;
