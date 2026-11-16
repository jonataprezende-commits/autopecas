/* AutoPeças — interface (parte 1: base, início, bancada, peças, XML) */
"use strict";

const ROTAS = {};
const ESTADO = { config: {}, dashboard: {}, ultimoBackup: "" };

/* ---------------- utilidades ---------------- */
function fmtMoney(c) {
  c = Number(c || 0);
  const neg = c < 0;
  c = Math.abs(Math.round(c));
  const s = (c / 100).toFixed(2).replace(".", ",");
  const partes = s.split(",");
  partes[0] = partes[0].replace(/\B(?=(\d{3})+(?!\d))/g, ".");
  return (neg ? "-R$ " : "R$ ") + partes.join(",");
}
function fmtMoneyCurto(c) { return fmtMoney(c); }
function fmtQtd(q) {
  q = Number(q || 0);
  return Number.isInteger(q) ? String(q) : q.toFixed(2).replace(".", ",");
}
// "1.234" / "10.000" / "1.000.000" = separador de milhar (grupos de 3 digitos).
// Sem esta regra, "1.234" era lido como 1,234 (R$ 1,23) e "10.000" como R$ 10,00.
const RE_MILHAR = /^[0-9]{1,3}(\.[0-9]{3})+$/;

// normalizaNumero coloca o numero no formato interno (ponto decimal)
function normalizaNumero(s) {
  s = String(s === null || s === undefined ? "" : s).replace(/[R$\s]/g, "").trim();
  if (!s) return "";
  if (s.includes(",")) return s.replace(/\./g, "").replace(",", ".");  // 1.234,56 -> 1234.56
  if (RE_MILHAR.test(s)) return s.replace(/\./g, "");                  // 1.234 -> 1234
  return s;
}

function parseDinheiro(txt) {
  let s = normalizaNumero(txt);
  if (!s) return 0;
  let neg = false;
  if (s.startsWith("-")) { neg = true; s = s.slice(1); }
  const v = parseFloat(s);
  if (isNaN(v)) return 0;
  return Math.round((neg ? -v : v) * 100);
}
function parseQtd(txt) {
  const s = normalizaNumero(txt);
  const v = parseFloat(s);
  return isNaN(v) ? 0 : v;
}
function dataBR(s) {
  if (!s) return "";
  const d = String(s).slice(0, 10);
  if (d.length !== 10) return s;
  return d.slice(8, 10) + "/" + d.slice(5, 7) + "/" + d.slice(0, 4);
}
function dataHoraBR(s) {
  if (!s) return "";
  return dataBR(s) + (String(s).length > 10 ? " " + String(s).slice(11, 16) : "");
}
function hoje() {
  const d = new Date();
  const p = (n) => String(n).padStart(2, "0");
  return d.getFullYear() + "-" + p(d.getMonth() + 1) + "-" + p(d.getDate());
}
function diasAtras(n) {
  const d = new Date();
  d.setDate(d.getDate() - n);
  const p = (x) => String(x).padStart(2, "0");
  return d.getFullYear() + "-" + p(d.getMonth() + 1) + "-" + p(d.getDate());
}
function esc(s) {
  return String(s === null || s === undefined ? "" : s)
    .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;").replace(/'/g, "&#39;");
}
function $(sel, raiz) { return (raiz || document).querySelector(sel); }
function $$(sel, raiz) { return Array.from((raiz || document).querySelectorAll(sel)); }
function debounce(fn, ms) {
  let t;
  return function (...a) { clearTimeout(t); t = setTimeout(() => fn.apply(this, a), ms || 200); };
}

// Cabecalho obrigatorio nas chamadas que gravam dados (proteção contra outros
// sites tentando usar a API local). Precisa ir em TODAS as chamadas, inclusive
// nos envios de arquivo (multipart).
const CABECALHO_SEGURANCA = { "X-AutoPecas": "1" };
function cabecalhosSeguros(extra) { return { ...CABECALHO_SEGURANCA, ...(extra || {}) }; }

// ---- 1.0.12: aviso de "carregando" nas operacoes demoradas ----
// Contador de pedidos em andamento. O aviso so aparece se a resposta demorar
// mais que 250ms (chamadas rapidinhas nao piscam a tela) e some quando o
// ultimo pedido termina. Vale para TODA chamada que passa por api() e para os
// envios de arquivo (XML, foto), que sao os mais lentos.
let _pedidosCarregando = 0;
let _timerCarregando = null;
function _mostrarCarregando() {
  _pedidosCarregando++;
  if (_pedidosCarregando !== 1) return;
  _timerCarregando = setTimeout(function () {
    var ov = document.getElementById("overlay-carregando");
    if (ov && _pedidosCarregando > 0) ov.classList.remove("hidden");
  }, 250);
}
function _esconderCarregando() {
  _pedidosCarregando = Math.max(0, _pedidosCarregando - 1);
  if (_pedidosCarregando === 0) {
    clearTimeout(_timerCarregando);
    var ov = document.getElementById("overlay-carregando");
    if (ov) ov.classList.add("hidden");
  }
}
// comCarregando(fn) executa fn mostrando o aviso enquanto ela demorar.
async function comCarregando(fn) {
  _mostrarCarregando();
  try { return await fn(); }
  finally { _esconderCarregando(); }
}

async function api(caminho, opcoes) {
  opcoes = Object.assign({}, opcoes || {});
  // 1.0.11: rede de seguranca contra um erro que ja aconteceu de verdade — quem
  // chamava api("/rota", {campo: 1}) esquecendo o {corpo: ...} mandava um GET e a
  // rota (que so aceita POST) respondia "404 page not found". Agora os campos
  // soltos viram corpo da requisicao, e a chamada sai como POST.
  const conhecidas = ["corpo", "method", "headers"];
  const soltos = Object.keys(opcoes).filter(k => conhecidas.indexOf(k) < 0);
  if (soltos.length) {
    const juntos = Object.assign({}, opcoes.corpo || {});
    soltos.forEach(k => { juntos[k] = opcoes[k]; delete opcoes[k]; });
    opcoes.corpo = juntos;
  }
  const conf = { headers: cabecalhosSeguros(), ...opcoes };
  if (conf.corpo !== undefined) {
    conf.method = conf.method || "POST";
    conf.headers["Content-Type"] = "application/json";
    conf.body = JSON.stringify(conf.corpo);
    delete conf.corpo;
  }
  const resp = await comCarregando(() => fetch("/api" + caminho, conf));
  let dados = null;
  const texto = await resp.text();
  try { dados = texto ? JSON.parse(texto) : null; } catch (e) { dados = { erro: texto }; }
  if (!resp.ok) {
    if (resp.status === 401 && caminho.indexOf("/sessao") !== 0 && caminho.indexOf("/entrar") !== 0) {
      pedirSenha((dados && dados.erro) || "informe a senha do sistema");
    }
    throw new Error((dados && dados.erro) || "Erro " + resp.status);
  }
  return dados;
}

/* ---------------- 1.0.7: senha e acesso pelo celular ---------------- */
// SESSAO guarda a resposta de /api/sessao. No modo local (sem senha) o sistema
// abre direto, como sempre; quando o acesso pelo celular esta ligado, aparece a
// tela de senha antes de qualquer outra coisa.
let SESSAO = { autenticado: true, precisa_senha: false, modo: "local" };
let SISTEMA_ABERTO = false;

function pedirSenha(mensagem) {
  const tela = $("#tela-login");
  if (!tela) return;
  const erro = $("#login-erro");
  if (erro) {
    if (mensagem) { erro.textContent = mensagem; erro.classList.remove("hidden"); }
    else { erro.textContent = ""; erro.classList.add("hidden"); }
  }
  tela.classList.remove("hidden");
  const app = $("#app"); if (app) app.style.display = "none";
  setTimeout(() => { const i = $("#login-senha"); if (i) i.focus(); }, 120);
}

function esconderSenha() {
  const tela = $("#tela-login"); if (tela) tela.classList.add("hidden");
  const app = $("#app"); if (app) app.style.display = "";
  const erro = $("#login-erro"); if (erro) erro.classList.add("hidden");
  const campo = $("#login-senha"); if (campo) campo.value = "";
  atualizarBotoesSessao();
}

function atualizarBotoesSessao() {
  // "Sair" so aparece para quem entrou por senha (celular ou outro computador).
  // No proprio PC da loja nao faz sentido: daria a impressao de que o sistema fechou.
  const mostrar = !!SESSAO.precisa_senha && !!SESSAO.autenticado && !SESSAO.local;
  ["#btn-sair", "#btn-sair-topo"].forEach(sel => {
    const b = $(sel); if (b) b.style.display = mostrar ? (sel === "#btn-sair" ? "block" : "flex") : "none";
  });
}

async function entrarNoSistema(senha) {
  try {
    const r = await api("/entrar", { corpo: { senha: senha } });
    SESSAO.autenticado = true;
    SESSAO.modo = r.modo || SESSAO.modo;
    SESSAO.precisa_senha = r.senha_necessaria !== false;
    SESSAO.local = !!r.local;
    esconderSenha();
    await abrirSistema();
    toast("Bem-vindo!", "ok");
  } catch (e) {
    const erro = $("#login-erro");
    if (erro) { erro.textContent = e.message; erro.classList.remove("hidden"); }
    const campo = $("#login-senha");
    if (campo) { campo.select(); campo.focus(); }
  }
}
window.entrarNoSistema = entrarNoSistema;

async function sairDoSistema() {
  try { await api("/sair", { method: "POST" }); } catch (e) { /* segue em frente */ }
  SESSAO.autenticado = false;
  SISTEMA_ABERTO = false;
  const c = $("#conteudo"); if (c) c.innerHTML = '<div class="loading">Entre com a senha para continuar…</div>';
  const topo = $("#topo-titulo"); if (topo) topo.textContent = "AutoPeças";
  pedirSenha("Você saiu do sistema.");
}
window.sairDoSistema = sairDoSistema;

function fecharMenuCelular() { document.body.classList.remove("menu-aberto"); }
window.fecharMenuCelular = fecharMenuCelular;

function ligarMenuCelular() {
  const btn = $("#btn-menu");
  if (btn) btn.onclick = () => document.body.classList.toggle("menu-aberto");
  const fundo = $("#menu-fundo");
  if (fundo) fundo.onclick = fecharMenuCelular;
  $$("#menu a").forEach(a => a.addEventListener("click", fecharMenuCelular));
  window.addEventListener("hashchange", fecharMenuCelular);
  window.addEventListener("resize", () => { if (window.innerWidth > 900) fecharMenuCelular(); });
  const sairTopo = $("#btn-sair-topo");
  if (sairTopo) sairTopo.onclick = () => sairDoSistema();
}

function toast(msg, tipo) {
  const el = document.createElement("div");
  el.className = "toast " + (tipo || "");
  el.textContent = msg;
  $("#toasts").appendChild(el);
  setTimeout(() => { el.style.opacity = "0"; el.style.transition = "opacity .4s"; }, 3200);
  setTimeout(() => el.remove(), 3700);
}

function abrirModal(titulo, corpoHTML, rodapeHTML, largura) {
  $("#modal-titulo").textContent = titulo;
  $("#modal-corpo").innerHTML = corpoHTML;
  $("#modal-rodape").innerHTML = rodapeHTML || "";
  $(".modal").style.width = largura || "min(780px,96vw)";
  $("#modal").classList.remove("hidden");
  const primeiro = $("#modal-corpo input:not([type=hidden]), #modal-corpo select, #modal-corpo textarea");
  if (primeiro) setTimeout(() => primeiro.focus(), 60);
}
function fecharModal() { $("#modal").classList.add("hidden"); }
window.fecharModal = fecharModal;

function confirmar(msg, aoConfirmar, textoBotao) {
  abrirModal("Confirmação",
    `<p style="font-size:15px;margin:4px 0 2px">${esc(msg)}</p>`,
    `<button class="btn" onclick="fecharModal()">Cancelar</button>
     <button class="btn vermelho" id="btn-confirmar">${esc(textoBotao || "Confirmar")}</button>`);
  $("#btn-confirmar").onclick = async () => {
    fecharModal();
    try { await aoConfirmar(); } catch (e) { toast(e.message, "erro"); }
  };
}

/* ---------------- busca de peças reutilizável ---------------- */
function campoBuscaPeca(idInput, idLista, aoEscolher, opcoes) {
  opcoes = opcoes || {};
  const inp = document.getElementById(idInput);
  const lst = document.getElementById(idLista);
  if (!inp || !lst) return;
  const buscar = debounce(async () => {
    const q = inp.value.trim();
    if (q.length < 1) { lst.innerHTML = '<div class="vazio">Digite para buscar por descrição, código, equivalente ou categoria…</div>'; return; }
    try {
      const r = await api("/pecas?q=" + encodeURIComponent(q) + "&limite=40");
      if (!r.itens.length) { lst.innerHTML = '<div class="vazio">Nenhuma peça encontrada.</div>'; return; }
      lst.innerHTML = r.itens.map(p => `
        <div class="res-item" data-id="${p.id}">
          <div>
            <div class="d1">${esc(p.descricao)}</div>
            <div class="d2">
              <span class="mono">${esc(p.codigo)}</span>
              ${p.codigo_fabricante ? " · fab. " + esc(p.codigo_fabricante) : ""}
              ${p.marca ? " · " + esc(p.marca) : ""}
              · estoque ${fmtQtd(p.estoque)}
              ${p.local ? " · " + esc(p.local) : ""}
            </div>
          </div>
          <div style="text-align:right">
            <div class="preco">${fmtMoney(p.preco_centavos)}</div>
            ${p.abaixo_minimo ? '<span class="tag vermelho">abaixo do mínimo</span>' : ""}
          </div>
        </div>`).join("");
      $$(".res-item", lst).forEach(el => {
        el.onclick = () => {
          const p = r.itens.find(x => String(x.id) === el.dataset.id);
          if (aoEscolher) aoEscolher(p);
        };
      });
      if (opcoes.autoPrimeiro && r.itens.length === 1) aoEscolher(r.itens[0]);
    } catch (e) { lst.innerHTML = '<div class="vazio">' + esc(e.message) + "</div>"; }
  }, 170);
  inp.oninput = buscar;
  inp.onfocus = () => { if (inp.value.trim()) buscar(); };
  if (opcoes.limpar) inp.value = "";
}

/* ---------------- roteador ---------------- */
function rotaAtual() {
  const h = location.hash.replace(/^#\/?/, "") || "inicio";
  return h.split("?")[0] || "inicio";
}
async function navegar() {
  const r = rotaAtual();
  $$("#menu a").forEach(a => a.classList.toggle("ativo", a.dataset.rota === r));
  const topo = $("#topo-titulo");
  if (topo) {
    const link = document.querySelector('#menu a[data-rota="' + r + '"]');
    topo.textContent = link ? link.textContent.replace(/\s*F\d+\s*$/, "").trim() : "AutoPeças";
  }
  const fn = ROTAS[r] || ROTAS.inicio;
  try { await fn(); } catch (e) {
    $("#conteudo").innerHTML = '<div class="erro-box">Erro ao carregar a tela: ' + esc(e.message) + "</div>";
  }
  window.scrollTo(0, 0);
}
window.navegar = navegar;
window.irPara = (r) => { location.hash = "#/" + r; };

/* ---------------- shell ---------------- */
async function iniciar() {
  ligarMenuCelular();
  try {
    SESSAO = await api("/sessao");
  } catch (e) {
    // nao consegui perguntar (programa fechando, por exemplo): pede a senha
    SESSAO = { autenticado: false, precisa_senha: true, modo: "rede" };
  }
  if (!SESSAO.autenticado) { pedirSenha(); return; }
  atualizarBotoesSessao();
  await abrirSistema();
}

async function abrirSistema() {
  if (SISTEMA_ABERTO) { await navegar(); return; }
  SISTEMA_ABERTO = true;
  try {
    const cfg = await api("/config");
    ESTADO.config = cfg;
    $("#loja-nome").textContent = cfg.loja_nome || "Minha loja";
    $("#versao").textContent = "v" + (cfg.versao || "1.0.0") + " · offline";
    const lojaLogin = $("#login-loja"); if (lojaLogin) lojaLogin.textContent = cfg.loja_nome || "Minha loja";
    if (cfg.senha_definida === "true") SESSAO.precisa_senha = true;
  } catch (e) {
    toast("Não consegui falar com o sistema: " + e.message, "erro");
  }
  await atualizarAlertas();
  window.addEventListener("hashchange", navegar);
  document.addEventListener("keydown", atalhos);
  await navegar();
  // backup automático do dia é feito pelo programa ao iniciar; aqui só informamos
  if (ESTADO.config.ultimo_backup) {
    const d = ESTADO.config.ultimo_backup.slice(0, 10);
    if (d !== hoje()) {
      setTimeout(() => toast("O backup de hoje ainda não foi feito. Abra Backup e Ajustes → Fazer backup agora.", "aviso"), 1500);
    }
  }
}

async function atualizarAlertas() {
  try {
    const d = await api("/dashboard");
    ESTADO.dashboard = d;
    const partes = [];
    if (d.qtd_abaixo_minimo > 0) {
      partes.push(`<a href="#/inicio" style="display:flex;justify-content:space-between;color:#ffd9a8;margin-bottom:6px">
        <span>⚠ Estoque baixo</span><span class="badge-num">${d.qtd_abaixo_minimo}</span></a>`);
    }
    if (d.fichas_abertas > 0) {
      partes.push(`<a href="#/fichas" style="display:flex;justify-content:space-between;color:#cfe0f5;margin-bottom:6px">
        <span>Fichas abertas</span><span class="badge-num">${d.fichas_abertas}</span></a>`);
    }
    if (d.recebiveis_vencidos_qtd > 0) {
      partes.push(`<a href="#/receber" style="display:flex;justify-content:space-between;color:#ffc9c4;margin-bottom:6px">
        <span>💳 Contas vencidas</span><span class="badge-num">${d.recebiveis_vencidos_qtd}</span></a>`);
    }
    if (d.pagaveis_vencidos_qtd > 0) {
      partes.push(`<a href="#/pagar" style="display:flex;justify-content:space-between;color:#ffc9c4;margin-bottom:6px">
        <span>🧾 A pagar vencido</span><span class="badge-num">${d.pagaveis_vencidos_qtd}</span></a>`);
    }
    $("#alertas-side").innerHTML = partes.join("");
  } catch (e) { /* silencioso */ }
}

/* ---------------- atalhos de teclado ---------------- */
function atalhos(ev) {
  const digitando = /input|textarea|select/i.test(document.activeElement.tagName);
  if (ev.key === "Escape") { fecharModal(); return; }
  if (ev.key === "F2") { ev.preventDefault(); irPara("bancada"); setTimeout(() => $("#busca-bancada") && $("#busca-bancada").focus(), 350); return; }
  if (ev.key === "F3") { ev.preventDefault(); irPara("pecas"); setTimeout(() => $("#busca-pecas") && $("#busca-pecas").focus(), 350); return; }
  if (ev.key === "F4") { ev.preventDefault(); irPara("fichas"); setTimeout(() => { if (window.novaFicha) window.novaFicha(); }, 400); return; }
  if (ev.key === "F5") { ev.preventDefault(); irPara("catalogo"); setTimeout(() => $("#busca-rapida") && $("#busca-rapida").focus(), 350); return; }
  if (ev.key === "F9") { ev.preventDefault(); if (window.finalizarVenda) window.finalizarVenda(); return; }
  if (ev.key === "F8") { ev.preventDefault(); irPara("fechamento"); return; }
  if (ev.ctrlKey && (ev.key === "b" || ev.key === "B")) { ev.preventDefault(); irPara("backup"); return; }
  if (ev.key === "/" && !digitando) { ev.preventDefault(); irPara("pecas"); setTimeout(() => $("#busca-pecas") && $("#busca-pecas").focus(), 350); }
}

/* ================= INÍCIO ================= */
ROTAS.inicio = async function () {
  const d = await api("/dashboard");
  ESTADO.dashboard = d;
  const cfg = ESTADO.config;
  const backupHoje = (d.ultimo_backup || "").slice(0, 10) === hoje();

  let html = `<div class="topo">
    <div><h1>Início</h1><p class="sub">Resumo do movimento — ${dataBR(d.hoje)}</p></div>
    <div class="acoes">
      <button class="btn primario" onclick="irPara('bancada')">Nova venda (F2)</button>
      <button class="btn" onclick="irPara('fichas')">Nova ficha (F4)</button>
      <button class="btn" onclick="irPara('xml')">Importar XML</button>
    </div>
  </div>`;

  if (d.qtd_abaixo_minimo > 0) {
    html += `<div class="aviso-box" style="display:flex;justify-content:space-between;align-items:center;gap:12px">
      <div><strong>⚠ ${d.qtd_abaixo_minimo} peça(s) abaixo do estoque mínimo.</strong> Veja a lista abaixo para repor.</div>
      <button class="btn pequeno" onclick="irPara('estoque')">Movimentar estoque</button>
    </div>`;
  }
  if (d.recebiveis_vencidos_qtd > 0) {
    html += `<div class="erro-box" style="display:flex;justify-content:space-between;align-items:center;gap:12px">
      <div><strong>💳 ${d.recebiveis_vencidos_qtd} conta(s) a receber vencida(s)</strong> — total ${fmtMoney(d.recebiveis_vencidos_centavos)}.</div>
      <button class="btn pequeno" onclick="irPara('receber')">Ver contas</button>
    </div>`;
  }
  if (d.pagaveis_vencidos_qtd > 0) {
    html += `<div class="erro-box" style="display:flex;justify-content:space-between;align-items:center;gap:12px">
      <div><strong>🧾 ${d.pagaveis_vencidos_qtd} boleto(s) de fornecedor vencido(s)</strong> — total ${fmtMoney(d.pagaveis_vencidos_centavos)}.</div>
      <button class="btn pequeno" onclick="irPara('pagar')">Ver contas a pagar</button>
    </div>`;
  }
  if (!backupHoje) {
    html += `<div class="aviso-box" style="display:flex;justify-content:space-between;align-items:center;gap:12px">
      <div><strong>Backup de hoje pendente.</strong> Recomendado fazer um backup por dia (arquivo único).</div>
      <button class="btn pequeno" onclick="fazerBackupRapido()">Fazer backup agora</button>
    </div>`;
  }

  html += `<div class="cards">
    <div class="card ok"><div class="rotulo">Vendido hoje</div><div class="valor">${fmtMoney(d.vendas_hoje_centavos)}</div><div class="obs">${d.qtd_vendas_hoje} venda(s)</div></div>
    <div class="card info"><div class="rotulo">Vendido no mês</div><div class="valor">${fmtMoney(d.vendas_mes_centavos)}</div><div class="obs">lucro estimado ${fmtMoney(d.lucro_mes_centavos)}</div></div>
    <div class="card ${d.qtd_abaixo_minimo ? "alerta" : "ok"}"><div class="rotulo">Abaixo do mínimo</div><div class="valor">${d.qtd_abaixo_minimo}</div><div class="obs">peças para repor</div></div>
    <div class="card ${d.fichas_abertas ? "aviso" : "ok"}"><div class="rotulo">Fichas de falta</div><div class="valor">${d.fichas_abertas}</div><div class="obs">${d.fichas_antigas} há 7 dias ou mais</div></div>
    <div class="card ${d.recebiveis_vencidos_qtd ? "alerta" : "ok"}"><div class="rotulo">A receber em aberto</div><div class="valor">${fmtMoney(d.recebiveis_aberto_centavos)}</div><div class="obs">${d.recebiveis_vencidos_qtd} vencida(s)</div></div>
    <div class="card ${d.pagaveis_vencidos_qtd ? "alerta" : "ok"}"><div class="rotulo">A pagar em aberto</div><div class="valor">${fmtMoney(d.pagaveis_aberto_centavos || 0)}</div><div class="obs">${d.pagaveis_vencidos_qtd || 0} vencida(s)</div></div>
    <div class="card info"><div class="rotulo">Estoque (custo)</div><div class="valor">${fmtMoney(d.estoque_valor_centavos)}</div><div class="obs">${d.total_pecas} peças ativas</div></div>
  </div>`;

  html += `<div class="painel" id="painel-celular">
    <div class="painel-topo"><h3>Acessar pelo celular</h3>
      <span class="muted small">O sistema continua neste computador — o celular só mostra a mesma tela</span></div>
    <div id="conteudo-celular"><div class="vazio">Carregando…</div></div>
  </div>`;

  html += `<div class="painel">
    <div class="painel-topo"><h3>⚠ Peças abaixo do estoque mínimo</h3><span class="muted small">Atualiza toda vez que a tela abre</span></div>`;
  if (!d.abaixo_minimo.length) {
    html += '<div class="vazio">Nenhuma peça abaixo do mínimo. Tudo em ordem! </div>';
  } else {
    html += `<div class="tabela-wrap"><table><thead><tr>
      <th>Código</th><th>Descrição</th><th>Marca</th><th class="dir">Estoque</th><th class="dir">Mínimo</th><th class="dir">Comprar</th><th></th>
    </tr></thead><tbody>`;
    d.abaixo_minimo.slice(0, 40).forEach(p => {
      const comprar = Math.max(1, Math.ceil(p.minimo - p.estoque));
      html += `<tr>
        <td class="mono">${esc(p.codigo)}</td>
        <td>${esc(p.descricao)}<div class="muted small">${esc(p.local || "")}</div></td>
        <td>${esc(p.marca)}</td>
        <td class="dir"><span class="tag vermelho">${fmtQtd(p.estoque)}</span></td>
        <td class="dir">${fmtQtd(p.minimo)}</td>
        <td class="dir">${comprar}</td>
        <td class="dir"><button class="btn pequeno" onclick="irPara('estoque')">Entrada</button></td>
      </tr>`;
    });
    html += "</tbody></table></div>";
    if (d.abaixo_minimo.length > 40) html += `<p class="dica">Mostrando 40 de ${d.abaixo_minimo.length}. Veja todas em Relatórios → Peças abaixo do mínimo.</p>`;
  }
  html += `</div>`;

  html += `<div class="painel">
    <div class="painel-topo"><h3>Últimas vendas</h3><button class="btn pequeno" onclick="irPara('fechamento')">Fechamento do dia</button></div>`;
  if (!d.ultimas_vendas.length) {
    html += '<div class="vazio">Nenhuma venda registrada ainda. Comece pela Bancada (F2).</div>';
  } else {
    html += `<table><thead><tr><th>#</th><th>Data</th><th>Cliente</th><th>Pagamento</th><th class="dir">Total</th><th>Situação</th></tr></thead><tbody>`;
    d.ultimas_vendas.forEach(v => {
      html += `<tr class="clicavel" onclick="verVenda(${v.id})">
        <td>#${v.id}</td><td>${dataHoraBR(v.data)}</td><td>${esc(v.cliente || "—")}</td>
        <td>${esc(nomePagamento(v.forma_pagamento))}</td>
        <td class="dir num">${fmtMoney(v.total_centavos)}</td>
        <td>${v.status === "cancelada" ? '<span class="tag vermelho">cancelada</span>' : '<span class="tag verde">concluída</span>'}</td>
      </tr>`;
    });
    html += "</tbody></table>";
  }
  html += "</div>";
  $("#conteudo").innerHTML = html;
  carregarPainelCelular();
};

/* ---------------- 1.0.7: acesso pelo celular ---------------- */

// gerarQR desenha o QR no proprio navegador (biblioteca qrcode.min.js, embutida).
function gerarQR(texto) {
  try {
    if (typeof qrcode !== "function") return "";
    const qr = qrcode(0, "M");
    qr.addData(texto);
    qr.make();
    return qr.createDataURL(8, 2);
  } catch (e) { return ""; }
}
window.gerarQR = gerarQR;

function copiarTexto(txt) {
  const pronto = () => toast("Copiado: " + txt, "ok");
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(txt).then(pronto, () => toast(txt, "info"));
    return;
  }
  const t = document.createElement("textarea");
  t.value = txt; document.body.appendChild(t); t.select();
  try { document.execCommand("copy"); pronto(); } catch (e) { toast(txt, "info"); }
  t.remove();
}
window.copiarTexto = copiarTexto;

async function carregarPainelCelular() {
  const alvo = $("#conteudo-celular");
  if (!alvo) return;
  let r;
  try {
    r = await api("/rede");
  } catch (e) {
    alvo.innerHTML = '<div class="erro-box">' + esc(e.message) + "</div>";
    return;
  }
  const ligado = r.modo === "rede" && r.senha_definida;
  const urls = r.urls || [];
  // se a maquina nao tem endereco de rede detectavel, aproveita o endereco pelo
  // qual esta tela foi aberta (menos quando e o proprio 127.0.0.1)
  const atual = (location.origin || "").replace(/\/$/, "");
  const daRede = atual && !/127\.0\.0\.1|localhost/.test(atual) ? atual : "";
  const principal = urls.length ? urls[0].url : daRede;
  const notaFora = `<p class="dica" style="margin:12px 0 0">Fora da loja (4G) só funciona com uma VPN gratuita ligada no celular e no computador
    — passo a passo no manual, capítulo <strong>Acessar pelo celular</strong>.</p>`;

  if (ligado) {
    alvo.innerHTML = `<div class="celular-grade">
      <div class="celular-qr">
        ${principal ? '<img id="qr-celular" alt="QR Code com o endereço do sistema">' : ""}
        <small>${principal ? "Aponte a câmera do celular" : "Computador sem rede — ligue no Wi-Fi"}</small>
        ${principal && !urls.length ? `<div class="dica">Use o endereço <code>${esc(principal)}</code> no celular.</div>` : ""}
      </div>
      <div class="celular-info">
        <p style="margin:0 0 8px"><strong>Ligado.</strong> No celular:</p>
        <ol style="margin:0 0 10px;padding-left:20px;line-height:1.7">
          <li>Aponte a câmera para o código (ou digite um dos endereços abaixo).</li>
          <li>Entre com a senha que você definiu.</li>
        </ol>
        ${urls.map(u => `<div class="endereco-celular"><span class="muted small" style="min-width:170px">${esc(u.rotulo)}</span>
          <code>${esc(u.url)}</code>
          <button class="btn pequeno" onclick="copiarTexto('${esc(u.url)}')">Copiar</button></div>`).join("")}
        ${!urls.length ? '<div class="aviso-box">O computador não está em nenhuma rede (Wi-Fi ou cabo). Ligue-o na rede da loja para o celular achar o sistema.</div>' : ""}
        <div class="acoes" style="margin-top:12px">
          <button class="btn" onclick="trocarSenhaCelular()">Trocar a senha</button>
          <button class="btn vermelho" onclick="desligarAcessoCelular()">Desligar o acesso pelo celular</button>
        </div>
        <p class="dica" style="margin:10px 0 0">Funciona com o celular na mesma Wi-Fi da loja. Este computador precisa estar ligado e com o AutoPeças aberto.</p>
        ${notaFora}
      </div>
    </div>`;
    const img = $("#qr-celular");
    if (img && principal) img.src = gerarQR(principal);
    return;
  }

  alvo.innerHTML = `<div class="celular-desligado">
      <p style="margin:0 0 6px"><strong>Desligado.</strong> Hoje o sistema só abre neste computador — é a configuração mais segura.</p>
      <p style="margin:0 0 12px">Para usar no celular, escolha uma senha e clique em ligar. O programa passa a atender os celulares da mesma Wi-Fi na hora — sem reiniciar nada.</p>
      <div class="grade g2">
        <div class="campo"><label>Senha de acesso</label>
          <input type="password" id="nova-senha-celular" placeholder="mínimo 4 caracteres" autocomplete="new-password"></div>
        <div class="campo"><label>Repita a senha</label>
          <input type="password" id="nova-senha-celular2" placeholder="repita a senha" autocomplete="new-password"></div>
      </div>
      <button class="btn primario" onclick="ligarAcessoCelular()">Ligar o acesso pelo celular</button>
    </div>
    ${notaFora}`;
}
window.carregarPainelCelular = carregarPainelCelular;

async function aplicarAcessoCelular(corpo) {
  try {
    // O corpo TEM que ir dentro de { corpo: ... }: sem isso o api() monta um GET
    // e a rota (que so aceita POST) responde "404 page not found" — era o erro
    // que aparecia ao clicar em "Ligar o acesso pelo celular".
    const r = await api("/rede/aplicar", { corpo: Object.assign({ reiniciar: true }, corpo) });
    if (r.aviso) toast(r.aviso, "erro");
    toast(r.ativo ? "Acesso pelo celular ligado." : "Acesso pelo celular desligado.", "ok");
    // 1.0.11: o programa passa a atender a rede na hora; nao ha mais reinicio
    // (era ali que o sistema sumia da tela). So recarrega o painel.
    await carregarPainelCelular();
    return true;
  } catch (e) {
    toast(e.message, "erro");
    return false;
  }
}

async function ligarAcessoCelular() {
  const s1 = (($("#nova-senha-celular") || {}).value || "").trim();
  const s2 = (($("#nova-senha-celular2") || {}).value || "").trim();
  if (s1.length < 4) return toast("A senha precisa de pelo menos 4 caracteres.", "aviso");
  if (s1 !== s2) return toast("As duas senhas não são iguais.", "aviso");
  await aplicarAcessoCelular({ ativo: true, senha: s1 });
}
window.ligarAcessoCelular = ligarAcessoCelular;

async function desligarAcessoCelular() {
  confirmar("Desligar o acesso pelo celular? O aparelho (e qualquer outro computador) não vai mais abrir o sistema.",
    () => aplicarAcessoCelular({ ativo: false }), "Desligar");
}
window.desligarAcessoCelular = desligarAcessoCelular;

function trocarSenhaCelular() {
  abrirModal("Trocar a senha do celular",
    `<div class="campo"><label>Nova senha</label>
       <input type="password" id="troca-senha-1" placeholder="mínimo 4 caracteres" autocomplete="new-password"></div>
     <div class="campo"><label>Repita a nova senha</label>
       <input type="password" id="troca-senha-2" placeholder="repita a senha" autocomplete="new-password"></div>
     <p class="dica">Os celulares já conectados continuam entrando até saírem ou até 12 horas de uso. Para cortar todos na hora,
     desligue e ligue o acesso pelo celular.</p>`,
    `<button class="btn" onclick="fecharModal()">Cancelar</button>
     <button class="btn primario" id="btn-trocar-senha">Salvar a nova senha</button>`, "min(520px,96vw)");
  $("#btn-trocar-senha").onclick = async function () {
    const s1 = ($("#troca-senha-1").value || "").trim();
    const s2 = ($("#troca-senha-2").value || "").trim();
    if (s1.length < 4) return toast("A senha precisa de pelo menos 4 caracteres.", "aviso");
    if (s1 !== s2) return toast("As duas senhas não são iguais.", "aviso");
    fecharModal();
    await aplicarAcessoCelular({ ativo: true, senha: s1 });
  };
}
window.trocarSenhaCelular = trocarSenhaCelular;

function nomePagamento(p) {
  return { dinheiro: "Dinheiro", pix: "PIX", cartao: "Cartão", faturado: "Faturado", transferencia: "Transferência" }[p] || p || "";
}

async function fazerBackupRapido() {
  try {
    const r = await api("/backup", { method: "POST" });
    ESTADO.config.ultimo_backup = r.data;
    toast("Backup criado: " + r.arquivo + " (" + (r.tamanho_bytes / 1024).toFixed(0) + " KB)", "ok");
    if (rotaAtual() === "inicio") navegar();
  } catch (e) { toast(e.message, "erro"); }
}
window.fazerBackupRapido = fazerBackupRapido;

/* ================= BANCADA (vendas) ================= */
const CARRINHO = { itens: [], cliente: null, veiculo: { placa: "", modelo: "", ano: "" } };

ROTAS.bancada = async function () {
  CARRINHO.itens = [];
  CARRINHO.cliente = null;
  $("#conteudo").innerHTML = `
  <div class="topo">
    <div><h1>Bancada — venda rápida</h1><p class="sub">Busque a peça pelo número (F2), informe o cliente e finalize (F9)</p></div>
    <div class="acoes"><button class="btn" onclick="irPara('inicio')">Voltar ao início</button></div>
  </div>
  <div class="bancada">
    <div>
      <div class="painel" style="margin-bottom:14px">
        <div class="busca-grande">
          <input type="search" id="busca-bancada" placeholder="Digite o código, parte da descrição, equivalente ou marca…" autocomplete="off">
          <button class="btn" onclick="document.getElementById('busca-bancada').value='';document.getElementById('busca-bancada').focus()">Limpar</button>
        </div>
        <div id="lista-bancada" class="resultados">
          <div class="vazio">Digite para buscar. Use ↑ ↓ e Enter para adicionar ao carrinho, ou clique na peça.</div>
        </div>
      </div>
      <div class="painel">
        <div class="painel-topo"><h3>Carrinho</h3><span class="muted small" id="carrinho-info">0 item(ns)</span></div>
        <div id="carrinho"></div>
      </div>
    </div>
    <div>
      <div class="painel">
        <h3>Cliente e veículo</h3>
        <div class="campo" style="margin-top:12px">
          <label>Cliente (opcional)</label>
          <input type="text" id="venda-cliente" placeholder="Nome do cliente" list="lista-clientes" autocomplete="off">
          <datalist id="lista-clientes"></datalist>
          <div class="dica">Comece a digitar para escolher um cliente já cadastrado.</div>
        </div>
        <div class="grade g3">
          <div class="campo"><label>Placa</label><input type="text" id="venda-placa" maxlength="10" placeholder="ABC1D23"></div>
          <div class="campo"><label>Modelo</label><input type="text" id="venda-modelo" placeholder="Gol 1.0"></div>
          <div class="campo"><label>Ano</label><input type="text" id="venda-ano" placeholder="2015"></div>
        </div>
      </div>
      <div class="painel">
        <h3>Pagamento</h3>
        <div class="campo" style="margin-top:12px">
          <label>Forma de pagamento</label>
          <select id="venda-pagamento">
            <option value="dinheiro">Dinheiro</option>
            <option value="pix">PIX</option>
            <option value="cartao">Cartão</option>
            <option value="faturado">Faturado (a prazo)</option>
            <option value="transferencia">Transferência</option>
          </select>
        </div>
        <div class="grade g2">
          <div class="campo"><label>Valor recebido</label><input type="text" id="venda-recebido" placeholder="0,00" inputmode="decimal"></div>
          <div class="campo" id="campo-venc" style="display:none"><label>Vencimento (dias)</label><input type="number" id="venda-venc" value="30" min="0"></div>
        </div>
        <div class="campo"><label>Desconto na venda (R$)</label><input type="text" id="venda-desconto" placeholder="0,00" inputmode="decimal"></div>
        <div class="campo"><label>Observação</label><input type="text" id="venda-obs" placeholder="Ex.: cliente retira amanhã"></div>
        <div id="totais"></div>
        <button class="btn verde btn-grande" style="width:100%;margin-top:14px;justify-content:center" onclick="finalizarVenda()">Finalizar venda (F9)</button>
      </div>
    </div>
  </div>`;

  campoBuscaPeca("busca-bancada", "lista-bancada", (p) => { adicionarAoCarrinho(p); });

  // teclado: setas + enter
  const inp = $("#busca-bancada");
  inp.addEventListener("keydown", (ev) => {
    const itens = $$("#lista-bancada .res-item");
    const atual = $("#lista-bancada .res-item.sel");
    let idx = itens.indexOf(atual);
    if (ev.key === "ArrowDown") { ev.preventDefault(); idx = Math.min(itens.length - 1, idx + 1); marcar(idx); }
    else if (ev.key === "ArrowUp") { ev.preventDefault(); idx = Math.max(0, idx - 1); marcar(idx); }
    else if (ev.key === "Enter") {
      ev.preventDefault();
      if (atual) atual.click();
      else if (itens.length === 1) itens[0].click();
    }
    function marcar(i) {
      itens.forEach(el => el.classList.remove("sel"));
      if (itens[i]) {
        itens[i].classList.add("sel");
        itens[i].style.background = "#e8f0fb";
        itens[i].scrollIntoView({ block: "nearest" });
      }
      itens.forEach((el, j) => { if (j !== i) el.style.background = ""; });
    }
  });

  // clientes no datalist
  try {
    const c = await api("/clientes?q=");
    $("#lista-clientes").innerHTML = c.itens.map(x => `<option value="${esc(x.nome)}" data-id="${x.id}">${esc(x.telefone || "")}</option>`).join("");
  } catch (e) {}

  $("#venda-pagamento").onchange = () => {
    const fat = $("#venda-pagamento").value === "faturado";
    $("#campo-venc").style.display = fat ? "" : "none";
    $("#venda-recebido").disabled = fat;
    atualizarTotais();
  };
  ["venda-recebido", "venda-desconto"].forEach(id => $("#" + id).addEventListener("input", atualizarTotais));
  $("#venda-cliente").addEventListener("change", () => {
    const opt = $$("#lista-clientes option").find(o => o.value === $("#venda-cliente").value);
    CARRINHO.cliente = opt ? { id: Number(opt.dataset.id), nome: opt.value } : null;
  });

  renderCarrinho();
  setTimeout(() => inp.focus(), 120);
};

function adicionarAoCarrinho(p) {
  const existente = CARRINHO.itens.find(i => i.peca_id === p.id);
  if (existente) {
    existente.quantidade += 1;
  } else {
    CARRINHO.itens.push({
      peca_id: p.id, codigo: p.codigo, descricao: p.descricao, marca: p.marca,
      quantidade: 1, preco_centavos: p.preco_centavos, desconto_centavos: 0,
      estoque: p.estoque, custo: p.custo_centavos
    });
  }
  renderCarrinho();
  const inp = $("#busca-bancada");
  if (inp) { inp.value = ""; inp.focus(); $("#lista-bancada").innerHTML = '<div class="vazio">Peça adicionada. Digite a próxima…</div>'; }
}

function renderCarrinho() {
  const div = $("#carrinho");
  if (!div) return;
  if (!CARRINHO.itens.length) {
    div.innerHTML = '<div class="vazio">Carrinho vazio. Busque uma peça acima.</div>';
    atualizarTotais();
    return;
  }
  let html = `<table class="carrinho-tabela"><thead><tr>
    <th>Peça</th><th style="width:74px" class="dir">Qtd</th><th style="width:104px" class="dir">Preço un.</th>
    <th style="width:92px" class="dir">Desconto</th><th style="width:96px" class="dir">Total</th><th></th></tr></thead><tbody>`;
  CARRINHO.itens.forEach((it, i) => {
    html += `<tr>
      <td><strong>${esc(it.descricao)}</strong>
        <div class="muted small"><span class="mono">${esc(it.codigo)}</span>${it.marca ? " · " + esc(it.marca) : ""} · estoque ${fmtQtd(it.estoque)}${it.quantidade > it.estoque ? ' <span class="tag amarelo">sem estoque</span>' : ""}</div></td>
      <td><input type="number" min="0.01" step="1" value="${it.quantidade}" data-i="${i}" data-campo="qtd"></td>
      <td><input type="text" value="${(it.preco_centavos / 100).toFixed(2).replace(".", ",")}" data-i="${i}" data-campo="preco"></td>
      <td><input type="text" value="${(it.desconto_centavos / 100).toFixed(2).replace(".", ",")}" data-i="${i}" data-campo="desc"></td>
      <td class="dir num">${fmtMoney(it.preco_centavos * it.quantidade - it.desconto_centavos)}</td>
      <td class="dir"><button class="btn pequeno vermelho" data-remover="${i}">✕</button></td>
    </tr>`;
  });
  html += "</tbody></table>";
  div.innerHTML = html;

  $$("#carrinho input").forEach(inp => {
    inp.addEventListener("change", () => {
      const i = Number(inp.dataset.i);
      const campo = inp.dataset.campo;
      if (campo === "qtd") CARRINHO.itens[i].quantidade = Math.max(0.01, parseQtd(inp.value));
      if (campo === "preco") CARRINHO.itens[i].preco_centavos = Math.max(0, parseDinheiro(inp.value));
      if (campo === "desc") CARRINHO.itens[i].desconto_centavos = Math.max(0, parseDinheiro(inp.value));
      // 1.0.10: redesenhar so depois que o navegador terminar o evento "change".
      // Trocar o innerHTML no meio do change (que o Chrome dispara no blur do
      // campo) da erro "node to be removed is no longer a child" e o carrinho
      // ficava na tela com a quantidade velha e o total errado.
      setTimeout(renderCarrinho, 0);
    });
  });
  $$("#carrinho [data-remover]").forEach(b => {
    b.onclick = () => { CARRINHO.itens.splice(Number(b.dataset.remover), 1); renderCarrinho(); };
  });
  $("#carrinho-info").textContent = CARRINHO.itens.length + " item(ns)";
  atualizarTotais();
}

// Dias de vencimento da venda faturada.
// Campo vazio (ou valor invalido) = 30 (prazo padrao do sistema).
// "0" digitado é respeitado: vale 0, e o servidor usa o prazo configurado.
function diasVencimento() {
  const el = $("#venda-venc");
  if (!el) return 30;
  const texto = String(el.value === null || el.value === undefined ? "" : el.value).trim();
  if (texto === "") return 30;
  const n = Number(texto);
  if (!isFinite(n) || n < 0) return 30;
  return Math.trunc(n);
}

function totaisVenda() {
  let subtotal = 0;
  let custo = 0;
  CARRINHO.itens.forEach(i => {
    subtotal += i.preco_centavos * i.quantidade;
    custo += (i.custo || 0) * i.quantidade;
  });
  const descontoLoja = Math.max(0, parseDinheiro($("#venda-desconto") ? $("#venda-desconto").value : 0));
  const descontoItens = CARRINHO.itens.reduce((s, i) => s + i.desconto_centavos, 0);
  const total = Math.max(0, subtotal - descontoItens - descontoLoja);
  return { subtotal, descontoItens, descontoLoja, total, custo };
}

function atualizarTotais() {
  const div = $("#totais");
  if (!div) return;
  const t = totaisVenda();
  const recebido = parseDinheiro($("#venda-recebido") ? $("#venda-recebido").value : "");
  const faturado = $("#venda-pagamento") && $("#venda-pagamento").value === "faturado";
  let trocoHTML = "";
  if (!faturado) {
    const dif = recebido - t.total;
    if (recebido > 0) {
      trocoHTML = dif >= 0
        ? `<div class="troco">Troco: ${fmtMoney(dif)}</div>`
        : `<div class="troco falta">Falta receber: ${fmtMoney(-dif)}</div>`;
    }
  } else {
    trocoHTML = `<div class="dica">Venda faturada: vai para Contas a Receber.</div>`;
  }
  div.innerHTML = `
    <div class="total-linha"><span>Subtotal</span><span>${fmtMoney(t.subtotal)}</span></div>
    ${t.descontoItens ? `<div class="total-linha"><span>Desconto nos itens</span><span>- ${fmtMoney(t.descontoItens)}</span></div>` : ""}
    ${t.descontoLoja ? `<div class="total-linha"><span>Desconto da venda</span><span>- ${fmtMoney(t.descontoLoja)}</span></div>` : ""}
    <div class="total-linha grande"><span>TOTAL</span><span>${fmtMoney(t.total)}</span></div>
    ${trocoHTML}`;
}

async function finalizarVenda() {
  if (!CARRINHO.itens.length) { toast("Adicione pelo menos uma peça ao carrinho.", "aviso"); return; }
  const t = totaisVenda();
  const pagamento = $("#venda-pagamento") ? $("#venda-pagamento").value : "dinheiro";
  const recebido = parseDinheiro($("#venda-recebido") ? $("#venda-recebido").value : "");
  if (pagamento !== "faturado" && recebido > 0 && recebido < t.total) {
    toast("O valor recebido é menor que o total. Confira o troco.", "aviso");
    return;
  }
  const clienteNome = $("#venda-cliente") ? $("#venda-cliente").value.trim() : "";
  const corpo = {
    cliente_id: CARRINHO.cliente && CARRINHO.cliente.nome === clienteNome ? CARRINHO.cliente.id : null,
    cliente_nome: clienteNome,
    veiculo_placa: $("#venda-placa") ? $("#venda-placa").value.trim().toUpperCase() : "",
    veiculo_modelo: $("#venda-modelo") ? $("#venda-modelo").value.trim() : "",
    veiculo_ano: $("#venda-ano") ? $("#venda-ano").value.trim() : "",
    itens: CARRINHO.itens.map(i => ({
      peca_id: i.peca_id, quantidade: i.quantidade, preco_centavos: i.preco_centavos,
      desconto_centavos: i.desconto_centavos
    })),
    desconto_centavos: t.descontoLoja,
    forma_pagamento: pagamento,
    recebido_centavos: recebido,
    vencimento_dias: diasVencimento(),
    observacao: $("#venda-obs") ? $("#venda-obs").value.trim() : ""
  };
  try {
    const venda = await api("/vendas", { corpo });
    CARRINHO.itens = [];
    CARRINHO.cliente = null;
    mostrarRecibo(venda);
    atualizarAlertas();
    // 1.0.10: aviso na hora se a peca ficou com saldo negativo
    if (venda.aviso_estoque && venda.aviso_estoque.length) {
      const nomes = venda.aviso_estoque.map(a => `${a.descricao} (saldo ${fmtQtd(a.saldo)})`).join(", ");
      toast(`Venda gravada. Atenção: ${nomes} ficou sem estoque — dê entrada na compra quando chegar.`, "aviso");
    }
  } catch (e) { toast(e.message, "erro"); }
}
window.finalizarVenda = finalizarVenda;

function mostrarRecibo(v) {
  const linhas = v.itens.map(i => `<div class="linha"><span>${fmtQtd(i.quantidade)}x ${esc(i.descricao)}</span><span>${fmtMoney(i.total_centavos)}</span></div>`).join("");
  const cupom = `<div class="cupom">
    <h3>${esc(ESTADO.config.loja_nome || "Minha Loja")}</h3>
    <div style="text-align:center;font-size:11px">${esc(ESTADO.config.loja_telefone || "")}</div>
    <hr>
    <div>Venda #${v.id} — ${dataHoraBR(v.data)}</div>
    <div>Cliente: ${esc(v.cliente || "—")}</div>
    ${v.veiculo_placa || v.veiculo_modelo ? `<div>Veículo: ${esc(v.veiculo_modelo)} ${esc(v.veiculo_placa)} ${esc(v.veiculo_ano)}</div>` : ""}
    <hr>
    ${linhas}
    <hr>
    <div class="linha"><span>Subtotal</span><span>${fmtMoney(v.subtotal_centavos)}</span></div>
    ${v.desconto_centavos ? `<div class="linha"><span>Desconto</span><span>-${fmtMoney(v.desconto_centavos)}</span></div>` : ""}
    <div class="linha"><strong>TOTAL</strong><strong>${fmtMoney(v.total_centavos)}</strong></div>
    <div class="linha"><span>Pagamento</span><span>${nomePagamento(v.forma_pagamento)}</span></div>
    ${v.troco_centavos ? `<div class="linha"><span>Troco</span><span>${fmtMoney(v.troco_centavos)}</span></div>` : ""}
    ${v.vencimento ? `<div class="linha"><span>Vencimento</span><span>${dataBR(v.vencimento)}</span></div>` : ""}
    <hr><div style="text-align:center;font-size:11px">Obrigado pela preferência!</div>
  </div>`;

  abrirModal("Venda #" + v.id + " concluída",
    `<div class="ok-box">Venda registrada. Estoque já foi baixado${v.forma_pagamento === "faturado" ? " e a conta foi lançada em Contas a Receber" : ""}.</div>
     <div class="grade g2">
       <div><div class="kv"><span>Total</span><strong>${fmtMoney(v.total_centavos)}</strong></div>
         <div class="kv"><span>Pagamento</span><span>${nomePagamento(v.forma_pagamento)}</span></div>
         <div class="kv"><span>Troco</span><span>${fmtMoney(v.troco_centavos)}</span></div>
         <div class="kv"><span>Lucro estimado</span><span>${fmtMoney(v.lucro_centavos)}</span></div></div>
       <div class="cupom" style="border:1px dashed #ccc;padding:10px;background:#fafbfd">${cupom}</div>
     </div>`,
    `<button class="btn" onclick="fecharModal()">Fechar</button>
     <button class="btn" onclick="imprimirHTML(document.querySelector('#modal-corpo .cupom').outerHTML)">Imprimir cupom</button>
     <button class="btn primario" onclick="fecharModal();irPara('bancada')">Nova venda (F2)</button>`, "min(880px,96vw)");
}
window.imprimirHTML = function (html) {
  $("#print-area").innerHTML = html;
  window.print();
  setTimeout(() => { $("#print-area").innerHTML = ""; }, 800);
};

async function verVenda(id) {
  try {
    const v = await api("/vendas/" + id);
    const itens = v.itens.map(i => `<tr><td class="mono">${esc(i.codigo)}</td><td>${esc(i.descricao)}</td>
      <td class="dir">${fmtQtd(i.quantidade)}</td><td class="dir">${fmtMoney(i.preco_centavos)}</td>
      <td class="dir">${fmtMoney(i.total_centavos)}</td></tr>`).join("");
    abrirModal("Venda #" + v.id,
      `<div class="grade g3" style="margin-bottom:14px">
        <div><label>Data</label><div>${dataHoraBR(v.data)}</div></div>
        <div><label>Cliente</label><div>${esc(v.cliente || "—")}</div></div>
        <div><label>Veículo</label><div>${esc([v.veiculo_modelo, v.veiculo_placa, v.veiculo_ano].filter(Boolean).join(" ") || "—")}</div></div>
      </div>
      <table><thead><tr><th>Código</th><th>Peça</th><th class="dir">Qtd</th><th class="dir">Preço</th><th class="dir">Total</th></tr></thead><tbody>${itens}</tbody></table>
      <div style="margin-top:14px">
        <div class="kv"><span>Subtotal</span><span>${fmtMoney(v.subtotal_centavos)}</span></div>
        <div class="kv"><span>Desconto</span><span>${fmtMoney(v.desconto_centavos)}</span></div>
        <div class="kv"><span><strong>Total</strong></span><strong>${fmtMoney(v.total_centavos)}</strong></div>
        <div class="kv"><span>Pagamento</span><span>${nomePagamento(v.forma_pagamento)}${v.vencimento ? " — vence " + dataBR(v.vencimento) : ""}</span></div>
        <div class="kv"><span>Situação</span><span>${esc(v.status)}</span></div>
      </div>`,
      `${v.status !== "cancelada" ? `<button class="btn vermelho" onclick="cancelarVenda(${v.id})">Cancelar venda</button>` : ""}
       <button class="btn" onclick="fecharModal()">Fechar</button>`, "min(720px,96vw)");
  } catch (e) { toast(e.message, "erro"); }
}
window.verVenda = verVenda;

function cancelarVenda(id) {
  confirmar("Cancelar a venda #" + id + "? O estoque das peças será devolvido.", async () => {
    await api("/vendas/" + id + "/cancelar", { method: "POST" });
    toast("Venda cancelada e estoque devolvido.", "ok");
    fecharModal();
    navegar();
  }, "Cancelar venda");
}
window.cancelarVenda = cancelarVenda;

/* ================= PEÇAS ================= */
ROTAS.pecas = async function () {
  $("#conteudo").innerHTML = `
  <div class="topo">
    <div><h1>Cadastro de peças</h1><p class="sub">Busca por descrição, código, código do fabricante, equivalente ou categoria (F3)</p></div>
    <div class="acoes">
      <button class="btn primario" onclick="editarPeca(null)">+ Nova peça</button>
      <button class="btn" onclick="irPara('xml')">Importar XML</button>
    </div>
  </div>
  <div class="painel">
    <div class="grade g3" style="margin-bottom:12px">
      <div class="campo" style="margin:0"><input type="search" id="busca-pecas" placeholder="Buscar peça…" autocomplete="off"></div>
      <div class="campo" style="margin:0"><select id="filtro-baixo"><option value="">Todas as peças</option><option value="1">Só abaixo do mínimo</option></select></div>
      <div class="campo" style="margin:0;display:flex;align-items:center;gap:10px">
        <input type="text" id="filtro-categoria" placeholder="Categoria (opcional)" list="lista-categorias" style="flex:1">
        <datalist id="lista-categorias"></datalist>
        <button class="btn" onclick="carregarPecas()">Buscar</button>
      </div>
    </div>
    <div id="resultado-pecas"><div class="vazio">Carregando…</div></div>
  </div>`;

  $("#busca-pecas").addEventListener("input", debounce(carregarPecas, 180));
  $("#filtro-baixo").addEventListener("change", carregarPecas);
  $("#filtro-categoria").addEventListener("change", carregarPecas);
  await carregarPecas();
  setTimeout(() => $("#busca-pecas").focus(), 100);
};

async function carregarPecas() {
  const q = $("#busca-pecas") ? $("#busca-pecas").value.trim() : "";
  const baixo = $("#filtro-baixo") ? $("#filtro-baixo").value : "";
  const cat = $("#filtro-categoria") ? $("#filtro-categoria").value.trim() : "";
  const alvo = $("#resultado-pecas");
  if (!alvo) return;
  const url = "/pecas?q=" + encodeURIComponent(q) + (baixo ? "&abaixo_minimo=1" : "") +
    (cat ? "&categoria=" + encodeURIComponent(cat) : "") + "&limite=500";
  const r = await api(url);
  // sugere categorias
  try {
    const todas = await api("/pecas?limite=5000");
    const cats = [...new Set(todas.itens.map(p => p.categoria).filter(Boolean))].sort();
    const dl = $("#lista-categorias");
    if (dl) dl.innerHTML = cats.map(c => `<option value="${esc(c)}"></option>`).join("");
  } catch (e) {}

  if (!r.itens.length) {
    alvo.innerHTML = '<div class="vazio">Nenhuma peça encontrada. ' +
      (q ? `Deseja <a href="#" onclick="editarPeca(null);return false">cadastrar uma nova peça</a>?` : "") + "</div>";
    return;
  }
  let html = `<div class="tabela-wrap"><table><thead><tr>
    <th>Código</th><th>Descrição</th><th>Marca / Fab.</th><th>Categoria</th>
    <th class="dir">Estoque</th><th class="dir">Custo</th><th class="dir">Venda</th><th>Local</th><th></th>
  </tr></thead><tbody>`;
  r.itens.forEach(p => {
    html += `<tr class="clicavel" onclick="editarPeca(${p.id})">
      <td class="mono">${esc(p.codigo)}</td>
      <td><strong>${esc(p.descricao)}</strong>
        ${p.equivalentes.length ? `<div class="muted small">equiv.: ${p.equivalentes.slice(0, 4).map(e => esc(e.codigo)).join(", ")}</div>` : ""}</td>
      <td>${esc(p.marca)}<div class="muted small">${esc(p.codigo_fabricante)}</div></td>
      <td>${esc(p.categoria)}</td>
      <td class="dir">${p.abaixo_minimo ? `<span class="tag vermelho">${fmtQtd(p.estoque)}</span>` : fmtQtd(p.estoque)}
        <div class="muted small">mín ${fmtQtd(p.minimo)}</div></td>
      <td class="dir num">${fmtMoney(p.custo_centavos)}</td>
      <td class="dir num"><strong>${fmtMoney(p.preco_centavos)}</strong></td>
      <td class="small">${esc(p.local)}</td>
      <td class="dir" onclick="event.stopPropagation()">
        <button class="btn pequeno" onclick="movEstoqueRapido(${p.id})">Estoque</button>
      </td>
    </tr>`;
  });
  html += `</tbody></table></div><p class="dica">${r.itens.length} peça(s) listada(s). Clique na linha para editar.</p>`;
  alvo.innerHTML = html;
}
window.carregarPecas = carregarPecas;

async function editarPeca(id) {
  let p = { id: 0, codigo: "", codigo_fabricante: "", descricao: "", marca: "", categoria: "", aplicacao: "", custo_centavos: 0,
    preco_centavos: 0, minimo: 1, estoque: 0, local: "", fornecedor_id: null, equivalentes: [], ativo: true };
  if (id) {
    try { p = await api("/pecas/" + id); } catch (e) { toast(e.message, "erro"); return; }
  }
  const forn = await api("/fornecedores");
  const optForn = `<option value="">— sem fornecedor —</option>` +
    forn.itens.map(f => `<option value="${f.id}" ${p.fornecedor_id === f.id ? "selected" : ""}>${esc(f.nome)}</option>`).join("");

  abrirModal(id ? "Editar peça" : "Nova peça", `
    <div class="grade g3">
      <div class="campo"><label>Código da loja *</label><input type="text" id="p-codigo" value="${esc(p.codigo)}" placeholder="Ex.: 1042"></div>
      <div class="campo"><label>Código do fabricante</label><input type="text" id="p-codfab" value="${esc(p.codigo_fabricante)}"></div>
      <div class="campo"><label>Marca</label><input type="text" id="p-marca" value="${esc(p.marca)}"></div>
    </div>
    <div class="campo"><label>Descrição *</label><input type="text" id="p-desc" value="${esc(p.descricao)}" placeholder="Ex.: Filtro de óleo Gol 1.0 8v"></div>
    <div class="campo"><label>Aplicação — veículos compatíveis (a busca acha pelo carro)</label>
      <input type="text" id="p-aplic" value="${esc(p.aplicacao || "")}" placeholder="Ex.: VW GOL 1.0/1.6 08/16; VOYAGE 09/16"></div>
    <div class="grade g3">
      <div class="campo"><label>Categoria</label><input type="text" id="p-cat" value="${esc(p.categoria)}" list="lista-categorias-modal" placeholder="Ex.: Filtros"></div>
      <div class="campo"><label>Local no depósito</label><input type="text" id="p-local" value="${esc(p.local)}" placeholder="Ex.: Corredor 3, prateleira B"></div>
      <div class="campo"><label>Fornecedor</label><select id="p-forn">${optForn}</select></div>
    </div>
    <div class="grade g4">
      <div class="campo"><label>Preço de custo (R$)</label><input type="text" id="p-custo" value="${(p.custo_centavos / 100).toFixed(2).replace(".", ",")}" inputmode="decimal"></div>
      <div class="campo"><label>Preço de venda (R$)</label><input type="text" id="p-preco" value="${(p.preco_centavos / 100).toFixed(2).replace(".", ",")}" inputmode="decimal"></div>
      <div class="campo"><label>Estoque mínimo</label><input type="number" id="p-min" value="${p.minimo}" step="1" min="0"></div>
      <div class="campo"><label>${id ? "Estoque atual (use Estoque para alterar)" : "Estoque inicial"}</label>
        <input type="number" id="p-estoque" value="${p.estoque}" step="1" ${id ? "readonly" : ""}></div>
    </div>
    <div class="campo">
      <label>Códigos equivalentes / OEM (para cruzamento)</label>
      <div style="display:flex;gap:8px">
        <input type="text" id="p-novo-equiv" placeholder="Digite um código e tecle Enter">
        <button class="btn" onclick="adicionarEquiv()">Adicionar</button>
      </div>
      <div class="grid-equivalentes" id="p-equivs"></div>
      <div class="dica">Ex.: códigos de outras marcas, OEM, referências que o cliente costuma informar.</div>
    </div>
    ${id ? `<div class="campo"><label><input type="checkbox" id="p-ativo" ${p.ativo ? "checked" : ""}> Peça ativa (desmarque para inativar sem perder o histórico)</label></div>` : ""}
    <div id="p-historico"></div>
  `,
    `<button class="btn" onclick="fecharModal()">Cancelar</button>
     ${id ? `<button class="btn vermelho" onclick="excluirPeca(${id})">Inativar peça</button>` : ""}
     <button class="btn primario" onclick="salvarPeca(${id})">Salvar peça</button>`);

  window.__equivs = p.equivalentes.map(e => e.codigo);
  renderEquivs();
  $("#p-novo-equiv").addEventListener("keydown", (ev) => { if (ev.key === "Enter") { ev.preventDefault(); adicionarEquiv(); } });
  // sugestão automática de preço de venda
  $("#p-custo").addEventListener("change", () => {
    const custo = parseDinheiro($("#p-custo").value);
    const atual = parseDinheiro($("#p-preco").value);
    if (custo > 0 && (!atual || atual < custo)) {
      $("#p-preco").value = ((custo * 1.6) / 100).toFixed(2).replace(".", ",");
      toast("Sugestão de venda: custo + 60%. Ajuste se quiser.", "aviso");
    }
  });
  if (id) {
    try {
      const h = await api("/pecas/" + id + "/historico");
      if (h.itens.length) {
        $("#p-historico").innerHTML = `<h3 style="margin:18px 0 8px">Últimas movimentações</h3>
          <div class="lista-ignorados"><table><thead><tr><th>Data</th><th>Tipo</th><th class="dir">Qtd</th><th class="dir">Saldo</th><th>Motivo</th></tr></thead><tbody>
          ${h.itens.slice(0, 15).map(m => `<tr><td class="small">${dataHoraBR(m.data)}</td><td>${esc(m.tipo)}</td>
            <td class="dir">${m.quantidade > 0 ? "+" : ""}${fmtQtd(m.quantidade)}</td><td class="dir">${fmtQtd(m.saldo)}</td>
            <td class="small">${esc(m.motivo)}</td></tr>`).join("")}
          </tbody></table></div>`;
      }
    } catch (e) {}
  }
}
window.editarPeca = editarPeca;

function renderEquivs() {
  const div = $("#p-equivs");
  if (!div) return;
  if (!window.__equivs.length) { div.innerHTML = '<span class="muted small">Nenhum código equivalente cadastrado.</span>'; return; }
  div.innerHTML = window.__equivs.map((e, i) =>
    `<span class="chip">${esc(e)}<button onclick="removerEquiv(${i})">✕</button></span>`).join("");
}
function adicionarEquiv() {
  const inp = $("#p-novo-equiv");
  const v = inp.value.trim();
  if (!v) return;
  if (!window.__equivs.some(e => e.toUpperCase() === v.toUpperCase())) window.__equivs.push(v.toUpperCase());
  inp.value = "";
  renderEquivs();
  inp.focus();
}
function removerEquiv(i) { window.__equivs.splice(i, 1); renderEquivs(); }
window.adicionarEquiv = adicionarEquiv;
window.removerEquiv = removerEquiv;

async function salvarPeca(id) {
  const corpo = {
    codigo: $("#p-codigo").value.trim(),
    codigo_fabricante: $("#p-codfab").value.trim(),
    descricao: $("#p-desc").value.trim(),
    marca: $("#p-marca").value.trim(),
    categoria: $("#p-cat").value.trim(),
    aplicacao: $("#p-aplic").value.trim(),
    local: $("#p-local").value.trim(),
    custo_centavos: parseDinheiro($("#p-custo").value),
    preco_centavos: parseDinheiro($("#p-preco").value),
    minimo: parseQtd($("#p-min").value),
    estoque: parseQtd($("#p-estoque").value),
    fornecedor_id: $("#p-forn").value ? Number($("#p-forn").value) : null,
    equivalentes: window.__equivs,
    ativo: $("#p-ativo") ? $("#p-ativo").checked : true
  };
  if (!corpo.descricao) { toast("Informe a descrição da peça.", "aviso"); return; }
  try {
    const p = await api(id ? "/pecas/" + id : "/pecas", { corpo, method: id ? "PUT" : "POST" });
    toast("Peça salva: " + p.codigo + " — " + p.descricao, "ok");
    fecharModal();
    if (rotaAtual() === "pecas") carregarPecas();
    atualizarAlertas();
  } catch (e) { toast(e.message, "erro"); }
}
window.salvarPeca = salvarPeca;

function excluirPeca(id) {
  confirmar("Inativar esta peça? Ela deixa de aparecer nas buscas, mas o histórico é mantido.", async () => {
    await api("/pecas/" + id, { method: "DELETE" });
    toast("Peça inativada.", "ok");
    fecharModal();
    carregarPecas();
  }, "Inativar");
}
window.excluirPeca = excluirPeca;

/* ================= IMPORTAR XML ================= */
ROTAS.xml = async function () {
  $("#conteudo").innerHTML = `
  <div class="topo">
    <div><h1>Importar catálogo XML do fornecedor</h1>
      <p class="sub">Arraste o arquivo ou clique para escolher. Peças já cadastradas são atualizadas (sem duplicar) e as novas são cadastradas.</p></div>
  </div>
  <div class="painel">
    <div class="dropzone" id="dropzone">
      <div class="big"><svg viewBox="0 0 48 48" width="44" height="44" fill="none" stroke="#1557b0" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M24 6v20m0 0-7-7m7 7 7-7"/><path d="M8 30v8a4 4 0 0 0 4 4h24a4 4 0 0 0 4-4v-8"/></svg></div>
      <strong>Arraste o arquivo XML aqui</strong>
      <span class="muted">ou clique para escolher o arquivo do fornecedor (.xml)</span>
      <input type="file" id="arquivo-xml" accept=".xml,text/xml,application/xml" style="display:none">
    </div>
    <div id="nome-arquivo" class="dica"></div>

    <h3 style="margin:20px 0 10px">Como devo tratar este arquivo?</h3>
    <div class="grade g3">
      <div class="campo"><label>O preço do XML é:</label>
        <select id="op-preco-custo">
          <option value="1">Preço de custo (compra)</option>
          <option value="0">Preço de venda</option>
        </select></div>
      <div class="campo"><label>Margem para calcular a venda (%)</label>
        <input type="number" id="op-margem" value="60" min="0" step="1"></div>
      <div class="campo"><label>Fornecedor deste arquivo</label>
        <select id="op-fornecedor"><option value="">— não vincular —</option></select></div>
    </div>
    <div class="grade g2">
      <div class="campo">
        <label><input type="checkbox" id="op-criar" checked> Cadastrar peças novas</label>
        <label><input type="checkbox" id="op-custo" checked> Atualizar preço de custo das existentes</label>
        <label><input type="checkbox" id="op-preco" checked> Atualizar preço de venda das existentes</label>
      </div>
      <div class="campo">
        <label><input type="checkbox" id="op-desc"> Atualizar também a descrição das existentes</label>
        <label><input type="checkbox" id="op-estoque"> Lançar o estoque informado no XML como ENTRADA</label>
        <label><input type="checkbox" id="op-fichas" checked> Fechar fichas de falta que batem com as peças recebidas</label>
      </div>
    </div>
    <div id="bloco-nota"></div>
    <div class="acoes" style="margin-top:10px">
      <button class="btn" id="btn-analisar" disabled onclick="analisarXML()">🔍 Analisar arquivo</button>
      <button class="btn primario" id="btn-importar" disabled onclick="importarXML()">✓ Importar agora</button>
    </div>
    <div id="resultado-xml" style="margin-top:16px"></div>
  </div>`;

  try {
    const f = await api("/fornecedores");
    $("#op-fornecedor").innerHTML = '<option value="">— não vincular —</option>' +
      f.itens.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join("");
  } catch (e) {}

  const dz = $("#dropzone");
  const inp = $("#arquivo-xml");
  dz.onclick = () => inp.click();
  inp.onchange = () => { if (inp.files[0]) { window.__arquivo = inp.files[0]; mostrarArquivo(); } };
  ["dragenter", "dragover"].forEach(ev => dz.addEventListener(ev, (e) => { e.preventDefault(); dz.classList.add("hover"); }));
  ["dragleave", "drop"].forEach(ev => dz.addEventListener(ev, (e) => { e.preventDefault(); dz.classList.remove("hover"); }));
  dz.addEventListener("drop", (e) => {
    const f = e.dataTransfer.files[0];
    if (f) { window.__arquivo = f; mostrarArquivo(); }
  });
};

function mostrarArquivo() {
  const f = window.__arquivo;
  $("#nome-arquivo").innerHTML = `Arquivo escolhido: <strong>${esc(f.name)}</strong> (${(f.size / 1024).toFixed(1)} KB)`;
  $("#btn-analisar").disabled = false;
  $("#btn-importar").disabled = false;
  $("#resultado-xml").innerHTML = "";
  painelDaNota(null);
}
window.mostrarArquivo = mostrarArquivo;

function formOpcoesXML() {
  return {
    margem_percentual: Number($("#op-margem").value || 0),
    preco_xml_eh_custo: $("#op-preco-custo").value === "1",
    atualizar_estoque: $("#op-estoque").checked,
    receber_fichas: $("#op-fichas").checked,
    atualizar_custo: $("#op-custo").checked,
    atualizar_preco: $("#op-preco").checked,
    atualizar_descricao: $("#op-desc").checked,
    criar_novas: $("#op-criar").checked,
    fornecedor_id: $("#op-fornecedor").value ? Number($("#op-fornecedor").value) : null,
    // nota de entrada (1.0.8)
    lancar_conta_pagar: $("#op-conta-pagar") ? $("#op-conta-pagar").checked : null,
    ratear_extras: $("#op-ratear") ? $("#op-ratear").checked : null,
    cadastrar_fornecedor: $("#op-cad-fornecedor") ? $("#op-cad-fornecedor").checked : null,
    vencimento: $("#op-vencimento") ? $("#op-vencimento").value : "",
    prazo_dias: $("#op-prazo") ? Number($("#op-prazo").value || 30) : 30
  };
}

/* Painel que aparece quando o XML é uma nota fiscal de verdade: mostra o
   fornecedor, o valor total e os boletos, e pergunta o que fazer com eles. */
function painelDaNota(nota) {
  const alvo = $("#bloco-nota");
  if (!alvo) return;
  if (!nota) { alvo.innerHTML = ""; return; }
  const dups = nota.duplicatas || [];
  const vencInput = !dups.length || !dups[0].vencimento;
  // 1.0.10: o rotulo do "lancar conta" mostrava o total da nota mesmo quando
  // os boletos somavam outra coisa. Agora mostra a soma dos boletos.
  const somaDups = dups.reduce((t, d) => t + (d.valor_centavos || 0), 0);
  const valorContas = somaDups > 0 ? somaDups : nota.total_centavos;
  const avisos = nota.avisos || [];
  alvo.innerHTML = `
    <div class="painel" style="margin-top:14px;border-left:4px solid #1557b0">
      <div class="painel-topo">
        <h3>📄 Nota do fornecedor encontrada</h3>
        ${nota.ja_importada ? '<span class="tag vermelho">esta nota já foi lançada</span>' : '<span class="tag verde">nota nova</span>'}
      </div>
      <div class="grade g4" style="margin-top:6px">
        <div><div class="muted small">Fornecedor</div><strong>${esc(nota.fornecedor || "—")}</strong>
          <div class="muted small">${esc(nota.cnpj || "")}${nota.fornecedor_cadastrado ? "" : " · não cadastrado"}</div></div>
        <div><div class="muted small">Nota</div><strong>#${esc(nota.numero || "—")}${nota.serie ? "/" + esc(nota.serie) : ""}</strong>
          <div class="muted small">${nota.emitida_em ? "emitida em " + dataBR(nota.emitida_em) : ""}</div></div>
        <div><div class="muted small">Valor total (vNF)</div><strong>${fmtMoney(nota.total_centavos)}</strong>
          <div class="muted small">produtos ${fmtMoney(nota.produtos_centavos)}</div></div>
        <div><div class="muted small">Vem de frete/IPI/ST</div><strong>${fmtMoney(nota.extras_centavos)}</strong>
          <div class="muted small">${nota.extras_centavos > 0 ? "entra no custo da peça" : "nada a ratear"}</div></div>
      </div>
      ${avisos.length ? `<div class="erro-box" style="margin-top:10px"><strong>Confira antes de importar:</strong>
        <ul style="margin:6px 0 0 18px">${avisos.map(a => `<li>${esc(a)}</li>`).join("")}</ul></div>` : ""}
      ${dups.length && dups[0].vencimento ? `<div class="tabela-wrap" style="margin-top:10px;max-height:170px"><table>
        <thead><tr><th>Parcela</th><th>Vencimento</th><th class="dir">Valor</th></tr></thead><tbody>
        ${dups.map((d, i) => `<tr><td>${esc(d.numero || (i + 1))}/${dups.length}</td>
          <td>${d.vencimento ? dataBR(d.vencimento) : "—"}</td>
          <td class="dir num">${fmtMoney(d.valor_centavos)}</td></tr>`).join("")}
        </tbody></table></div>` : ""}
      <div class="grade g2" style="margin-top:12px">
        <div class="campo">
          <label><input type="checkbox" id="op-conta-pagar" checked>
            Lançar ${dups.length > 1 ? "as " + dups.length + " contas" : "a conta"} a pagar (${fmtMoney(valorContas)})</label>
          ${nota.ja_importada ? '<div class="dica" style="color:#c0392b">Esta nota já está lançada: importar de novo atualiza o estoque, mas não cria outra conta a pagar.</div>' : ""}
          <label><input type="checkbox" id="op-ratear" ${nota.extras_centavos > 0 ? "checked" : ""} ${nota.extras_centavos > 0 ? "" : "disabled"}>
            Somar frete/IPI/ST (${fmtMoney(nota.extras_centavos)}) no custo das peças</label>
          ${nota.fornecedor_cadastrado ? "" : `<label><input type="checkbox" id="op-cad-fornecedor" checked>
            Cadastrar o fornecedor "${esc(nota.fornecedor || "")}"</label>`}
        </div>
        ${vencInput ? `<div class="campo">
          <label>Vencimento (a nota não traz o boleto)</label>
          <input type="date" id="op-vencimento" value="${esc(nota.vencimento_sugerido || hoje())}">
          <div class="muted small">Padrão: emissão + prazo</div>
        </div>` : `<div class="campo"><div class="dica">Os vencimentos vêm dos boletos da própria nota.</div></div>`}
      </div>
    </div>`;
}
window.painelDaNota = painelDaNota;

async function analisarXML() {
  if (!window.__arquivo) { toast("Escolha o arquivo XML primeiro.", "aviso"); return; }
  const fd = new FormData();
  fd.append("arquivo", window.__arquivo);
  $("#resultado-xml").innerHTML = '<div class="vazio">Analisando o arquivo…</div>';
  try {
    const resp = await comCarregando(() => fetch("/api/xml/analisar", { method: "POST", body: fd, headers: cabecalhosSeguros() }));
    const r = await resp.json();
    if (!resp.ok) throw new Error(r.erro || "erro ao analisar");
    let html = `<div class="ok-box">Arquivo lido: <strong>${r.total_itens}</strong> item(ns) encontrados — 
      <strong>${r.novos}</strong> seriam cadastrados, <strong>${r.atualizacoes}</strong> atualizados, 
      <strong>${r.ignorados}</strong> ignorados.</div>`;
    painelDaNota(r.nota || null);
    if (r.amostra && r.amostra.length) {
      html += `<div class="lista-ignorados"><table><thead><tr><th>Descrição no XML</th><th>Código</th><th>Fab.</th>
        <th>Marca</th><th class="dir">Preço</th><th class="dir">Estoque</th><th>Ação</th></tr></thead><tbody>` +
        r.amostra.map(a => `<tr><td>${esc(a.descricao)}</td><td class="mono">${esc(a.codigo || "")}</td>
          <td class="mono">${esc(a.codigo_fabricante || "")}</td><td>${esc(a.marca || "")}</td>
          <td class="dir">${esc(a.custo_texto || a.preco_texto || "")}</td><td class="dir">${esc(a.estoque_texto || "")}</td>
          <td><span class="tag ${a.acao === "atualizar" ? "azul" : "verde"}">${a.acao}</span></td></tr>`).join("") +
        `</tbody></table></div><p class="dica">Prévia dos primeiros ${r.amostra.length} itens. Confira e clique em “Importar agora”.</p>`;
    }
    $("#resultado-xml").innerHTML = html;
  } catch (e) {
    $("#resultado-xml").innerHTML = '<div class="erro-box">' + esc(e.message) + "</div>";
  }
}
window.analisarXML = analisarXML;

async function importarXML() {
  if (!window.__arquivo) { toast("Escolha o arquivo XML primeiro.", "aviso"); return; }
  const fd = new FormData();
  fd.append("arquivo", window.__arquivo);
  fd.append("opcoes", JSON.stringify(formOpcoesXML()));
  $("#resultado-xml").innerHTML = '<div class="vazio">Importando… isso pode levar alguns segundos.</div>';
  const btn = $("#btn-importar");
  btn.disabled = true;
  try {
    const resp = await comCarregando(() => fetch("/api/xml/importar", { method: "POST", body: fd, headers: cabecalhosSeguros() }));
    const r = await resp.json();
    if (!resp.ok) throw new Error(r.erro || "erro ao importar");
    let html = `<div class="ok-box">
      <strong>Importação concluída:</strong><br>
      ✓ ${r.cadastradas} peça(s) cadastrada(s)<br>
      ↻ ${r.atualizadas} peça(s) atualizada(s) (sem duplicar)<br>
      → ${r.ignoradas} item(ns) ignorado(s)
      ${r.fichas_fechadas ? `<br>${r.fichas_fechadas} ficha(s) de falta fechada(s) e estoque lançado` : ""}
    </div>`;
    // 1.0.10: avisos da nota (boletos que nao fecham com o total, rateio
    // arredondado). Antes ficavam so no relatorio e o dono da loja nao via.
    if (r.avisos && r.avisos.length) {
      html += `<div class="erro-box" style="margin-top:10px"><strong>Confira antes de pagar:</strong><ul style="margin:6px 0 0 18px">` +
        r.avisos.map(a => `<li>${esc(a)}</li>`).join("") + `</ul></div>`;
    }
    if (r.contas_criadas) {
      html += `<div class="ok-box" style="margin-top:10px">
        💰 <strong>${r.contas_criadas} conta(s) a pagar lançada(s)</strong> — ${fmtMoney(r.contas_total_centavos || 0)}
        ${r.fornecedor_nome ? `<br>Fornecedor: <strong>${esc(r.fornecedor_nome)}</strong>${r.fornecedor_novo ? " (cadastrado agora)" : ""}` : ""}
        ${r.rateado_centavos ? `<br>Frete/IPI/ST somados no custo das peças: ${fmtMoney(r.rateado_centavos)}` : ""}
        <br><a href="#/pagar">Ver contas a pagar</a></div>`;
    } else if (r.nota_ja_existia) {
      html += `<div class="erro-box" style="margin-top:10px">Esta nota já estava lançada: o estoque foi
        atualizado, mas <strong>não</strong> criei outra conta a pagar (seria pagar duas vezes).</div>`;
    }
    if (r.ignoradas_lista && r.ignoradas_lista.length) {
      html += `<h3 style="margin:14px 0 8px">Itens ignorados e o motivo</h3>
        <div class="lista-ignorados"><table><thead><tr><th>Descrição</th><th>Código</th><th>Motivo</th></tr></thead><tbody>` +
        r.ignoradas_lista.slice(0, 300).map(i => `<tr><td>${esc(i.descricao)}</td><td class="mono">${esc(i.codigo)}</td>
          <td><span class="tag amarelo">${esc(i.motivo)}</span></td></tr>`).join("") + `</tbody></table></div>`;
    }
    html += `<div class="acoes" style="margin-top:14px">
      <a class="btn primario" href="#/pecas">Ver peças cadastradas</a>
      <button class="btn" onclick="document.getElementById('arquivo-xml').click()">Importar outro arquivo</button>
      </div>`;
    $("#resultado-xml").innerHTML = html;
    atualizarAlertas();
  } catch (e) {
    $("#resultado-xml").innerHTML = '<div class="erro-box">' + esc(e.message) + "</div>";
  } finally { btn.disabled = false; }
}
window.importarXML = importarXML;

/* ================= NOTA DE COMPRA POR FOTO ================= */
// Le a foto da nota no servidor (OCR offline embutido) e mostra uma tabela
// editavel. Nada entra no estoque antes do usuario conferir e confirmar.
const NOTA = { itens: [], totalNota: 0, veredito: "", fotoUrl: "" };

ROTAS.nota = async function () {
  $("#conteudo").innerHTML = `
  <div class="topo">
    <div><h1>Nota de compra por foto</h1>
      <p class="sub">Fotografe a nota do fornecedor. O sistema lê os itens e mostra a lista para você conferir —
      o estoque só é lançado depois do seu OK.</p></div>
    <div class="acoes"><span class="tag" id="nota-motor">verificando o leitor…</span></div>
  </div>
  <div class="painel">
    <div class="dropzone" id="dropzone-nota">
      <div class="big"><svg viewBox="0 0 48 48" width="44" height="44" fill="none" stroke="#1557b0" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M6 14h9l3-4h12l3 4h9v26H6V14Z"/><circle cx="24" cy="26" r="7"/></svg></div>
      <strong>Arraste a foto da nota aqui</strong>
      <span class="muted">ou clique para escolher a foto (JPG/PNG). Também funciona foto tirada na hora pelo celular.</span>
      <input type="file" id="arquivo-nota" accept="image/*" capture="environment" style="display:none">
    </div>
    <div id="nome-nota" class="dica"></div>
    <div class="dica" style="margin-top:6px">
      Para o leitor acertar: apoie o celular, enquadre a folha inteira, com luz de frente e sem sombra.
      Nota escrita à mão não é lida por foto — nesse caso digite pela tela de Importar XML ou direto na peça.
    </div>
    <div class="acoes" style="margin-top:12px">
      <button class="btn primario" id="btn-ler-nota" disabled onclick="lerNotaFoto()"><svg viewBox="0 0 16 16" width="15" height="15" fill="currentColor" aria-hidden="true" style="vertical-align:-2px;margin-right:4px"><path d="M6 2h4l.8 1.4H14v10.2H2V3.4h3.2L6 2Zm2 3.2a3.1 3.1 0 1 0 0 6.2 3.1 3.1 0 0 0 0-6.2Zm0 1.4a1.7 1.7 0 1 1 0 3.4 1.7 1.7 0 0 1 0-3.4Z"/></svg>Ler a nota</button>
      <button class="btn" id="btn-outra-nota" style="display:none" onclick="document.getElementById('arquivo-nota').click()">Trocar a foto</button>
    </div>
    <div id="resultado-nota" style="margin-top:16px"></div>
  </div>`;

  try {
    const st = await api("/nota-foto/status");
    $("#nota-motor").textContent = st.disponivel ? "leitor pronto: " + st.motor : "leitor indisponível nesta instalação";
    if (!st.disponivel) $("#nota-motor").className = "tag vermelho";
  } catch (e) { $("#nota-motor").textContent = "não consegui verificar o leitor"; }

  const dz = $("#dropzone-nota");
  const inp = $("#arquivo-nota");
  dz.onclick = () => inp.click();
  inp.onchange = () => { if (inp.files[0]) escolherFotoNota(inp.files[0]); };
  ["dragenter", "dragover"].forEach(ev => dz.addEventListener(ev, (e) => { e.preventDefault(); dz.classList.add("hover"); }));
  ["dragleave", "drop"].forEach(ev => dz.addEventListener(ev, (e) => { e.preventDefault(); dz.classList.remove("hover"); }));
  dz.addEventListener("drop", (e) => {
    const f = e.dataTransfer.files[0];
    if (f) escolherFotoNota(f);
  });
};

function escolherFotoNota(f) {
  window.__fotoNota = f;
  if (NOTA.fotoUrl) URL.revokeObjectURL(NOTA.fotoUrl);
  NOTA.fotoUrl = URL.createObjectURL(f);
  $("#nome-nota").innerHTML = `Foto escolhida: <strong>${esc(f.name)}</strong> (${(f.size / 1024).toFixed(0)} KB)`;
  $("#btn-ler-nota").disabled = false;
  $("#btn-outra-nota").style.display = "";
  $("#resultado-nota").innerHTML = "";
}
window.escolherFotoNota = escolherFotoNota;

async function lerNotaFoto() {
  const f = window.__fotoNota;
  if (!f) { toast("Escolha a foto da nota primeiro.", "aviso"); return; }
  const btn = $("#btn-ler-nota");
  btn.disabled = true;
  $("#resultado-nota").innerHTML = '<div class="vazio">Lendo a nota… na primeira vez pode levar alguns segundos (o leitor é preparado).</div>';
  const fd = new FormData();
  fd.append("arquivo", f);
  try {
    const resp = await comCarregando(() => fetch("/api/nota-foto/ler", { method: "POST", body: fd, headers: cabecalhosSeguros() }));
    const r = await resp.json();
    if (!resp.ok) throw new Error(r.erro || "não consegui ler a nota");
    NOTA.itens = (r.itens || []).map((it, i) => ({ ...it, incluir: true, _i: i }));
    NOTA.totalNota = r.total_nota_centavos || 0;
    NOTA.veredito = r.veredito;
    NOTA.resultado = r;
    desenharResultadoNota(r);
  } catch (e) {
    $("#resultado-nota").innerHTML = '<div class="erro-box">' + esc(e.message) + "</div>";
  } finally {
    btn.disabled = false;
  }
}
window.lerNotaFoto = lerNotaFoto;

function desenharResultadoNota(r) {
  const cor = r.veredito === "ok" ? "verde" : r.veredito === "revisar" ? "amarelo" : "vermelho";
  const rotulo = r.veredito === "ok" ? "Nota conferida" : r.veredito === "revisar" ? "Confira antes de importar" : "Não consegui ler";
  let html = `<div class="ok-box" style="border-left:4px solid ${r.veredito === "ok" ? "#1a7f37" : r.veredito === "revisar" ? "#b54708" : "#b42318"}">
    <span class="tag ${cor}">${rotulo}</span>
    <strong>${NOTA.itens.length}</strong> item(ns) lido(s) ·
    soma dos itens <strong>${fmtMoney(r.soma_itens_centavos)}</strong>
    ${r.total_nota_centavos ? `· total impresso na nota <strong>${fmtMoney(r.total_nota_centavos)}</strong>` : "· total da nota não encontrado na foto"}
    <div class="dica" style="margin-top:4px">leitor: ${esc(r.motor || "")} · ${(r.tempo_ms / 1000).toFixed(1)}s${r.qualidade && r.qualidade.inclinacao_graus !== undefined ? ` · inclinação corrigida ${Number(r.qualidade.inclinacao_graus).toFixed(1)}°` : ""}</div>
  </div>`;
  if (r.mensagem) html += `<div class="${r.veredito === "ok" ? "dica" : "erro-box"}" style="margin-top:8px">${esc(r.mensagem)}</div>`;
  if (!NOTA.itens.length) {
    html += `<div class="painel" style="margin-top:12px"><div class="vazio">Dicas para a próxima foto:<br>
      • apoie o celular na mesa (foto de cima, sem tremer)<br>
      • enquadre a nota inteira, incluindo o total<br>
      • luz de frente, sem sombra da mão<br>
      • evite foto muito de lado ou com a folha dobrada</div></div>`;
    html += `<div class="acoes" style="margin-top:10px"><button class="btn primario" onclick="document.getElementById('arquivo-nota').click()"><svg viewBox="0 0 16 16" width="15" height="15" fill="currentColor" aria-hidden="true" style="vertical-align:-2px;margin-right:4px"><path d="M6 2h4l.8 1.4H14v10.2H2V3.4h3.2L6 2Zm2 3.2a3.1 3.1 0 1 0 0 6.2 3.1 3.1 0 0 0 0-6.2Zm0 1.4a1.7 1.7 0 1 1 0 3.4 1.7 1.7 0 0 1 0-3.4Z"/></svg>Tirar outra foto</button></div>`;
    $("#resultado-nota").innerHTML = html;
    return;
  }
  html += `<div class="painel" style="margin-top:12px">
    <h3 style="margin:0 0 8px">Confira cada linha</h3>
    <div class="dica" style="margin-bottom:8px">Linhas em amarelo o sistema não conseguiu conferir pela conta (quantidade × unitário = total). Corrija o que estiver errado — o que você digitar é o que vai ser gravado.</div>
    <table class="tabela-nota"><thead><tr>
      <th style="width:34px"><input type="checkbox" id="nota-todos" checked onchange="marcarTodosNota(this.checked)"></th>
      <th>Código</th><th>Descrição</th><th class="dir" style="width:80px">Qtd</th>
      <th class="dir" style="width:120px">Unitário</th><th class="dir" style="width:120px">Total</th>
      <th style="width:90px">Situação</th></tr></thead><tbody>`;
  NOTA.itens.forEach((it) => {
    const classe = it.conferido ? "" : "linha-revisar";
    html += `<tr class="${classe}" data-item="${it._i}">
      <td><input type="checkbox" ${it.incluir ? "checked" : ""} onchange="marcarItemNota(${it._i}, this.checked)"></td>
      <td><input class="entrada-mini mono" value="${esc(it.codigo || "")}" oninput="editarItemNota(${it._i},'codigo',this.value)"></td>
      <td><input class="entrada-mini largo" value="${esc(it.descricao || "")}" oninput="editarItemNota(${it._i},'descricao',this.value)"></td>
      <td><input class="entrada-mini dir" value="${esc(fmtQtd(it.quantidade))}" onchange="editarItemNota(${it._i},'quantidade',this.value)"></td>
      <td><input class="entrada-mini dir" value="${esc(fmtMoney(it.unitario_centavos).replace('R$ ', ''))}" onchange="editarItemNota(${it._i},'unitario_centavos',this.value)"></td>
      <td><input class="entrada-mini dir" value="${esc(fmtMoney(it.total_centavos).replace('R$ ', ''))}" onchange="editarItemNota(${it._i},'total_centavos',this.value)"></td>
      <td><span class="tag ${it.conferido ? "verde" : "amarelo"}">${it.conferido ? "conferido" : "revisar"}</span></td>
    </tr>`;
  });
  html += `</tbody></table></div>
  <div class="painel" style="margin-top:12px">
    <div class="grade g3">
      <div class="campo"><label>Fornecedor desta nota</label><select id="nota-fornecedor"><option value="">— não vincular —</option></select></div>
      <div class="campo"><label>Margem para o preço de venda (%)</label><input type="number" id="nota-margem" value="60" min="0" step="1"></div>
      <div class="campo"><label>&nbsp;</label>
        <label><input type="checkbox" id="nota-estoque" checked> Lançar entrada no estoque</label></div>
    </div>
    <div class="grade g2">
      <div class="campo">
        <label><input type="checkbox" id="nota-custo" checked> Atualizar o preço de custo das peças existentes</label>
        <label><input type="checkbox" id="nota-criar" checked> Cadastrar as peças que ainda não existem</label>
      </div>
      <div class="campo">
        <label><input type="checkbox" id="nota-fichas" checked> Fechar fichas de falta que batem com as peças</label>
        <label class="dica">O código da nota vira código de fábrica; se não achar, o sistema procura pela descrição igual.</label>
      </div>
    </div>
    <div class="acoes" style="margin-top:10px">
      <button class="btn primario" id="btn-importar-nota" onclick="importarNotaFoto()">✓ Importar os itens marcados</button>
      <button class="btn" onclick="document.getElementById('arquivo-nota').click()">Ler outra foto</button>
    </div>
    <div id="resultado-importacao-nota" style="margin-top:12px"></div>
  </div>`;
  if (r.texto_ocr) {
    html += `<details style="margin-top:12px"><summary class="dica">Ver o texto que o leitor encontrou na foto</summary>
      <pre style="white-space:pre-wrap;font-family:monospace;font-size:12px;background:#f7f8fa;padding:10px;border-radius:8px;max-height:260px;overflow:auto">${esc(r.texto_ocr)}</pre></details>`;
  }
  $("#resultado-nota").innerHTML = html;
  const sel = $("#nota-fornecedor");
  if (sel) {
    api("/fornecedores").then(f => {
      sel.innerHTML = '<option value="">— não vincular —</option>' +
        f.itens.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join("");
    }).catch(() => {});
  }
}
window.desenharResultadoNota = desenharResultadoNota;

function marcarTodosNota(v) { NOTA.itens.forEach(it => it.incluir = v); desenharResultadoNota(NOTA.resultado); }
window.marcarTodosNota = marcarTodosNota;
function marcarItemNota(i, v) { NOTA.itens[i].incluir = v; }
window.marcarItemNota = marcarItemNota;
function editarItemNota(i, campo, valor) {
  const it = NOTA.itens[i];
  const anteriorConferido = it.conferido;
  if (campo === "quantidade") it.quantidade = parseQtd(valor);
  else if (campo === "unitario_centavos") it.unitario_centavos = parseDinheiro(valor);
  else if (campo === "total_centavos") it.total_centavos = parseDinheiro(valor);
  else it[campo] = valor;
  // recalcula a conferencia da linha com o que o usuario digitou
  const conta = Math.round(it.quantidade * it.unitario_centavos);
  it.conferido = it.quantidade > 0 && Math.abs(conta - it.total_centavos) <= 2;
  // total em branco: assume a conta (evita obrigar a digitar duas vezes)
  if (!it.conferido && it.total_centavos === 0 && conta > 0) {
    it.total_centavos = conta;
    it.conferido = true;
  }
  if (anteriorConferido !== it.conferido) atualizarLinhaNota(i);
}
window.editarItemNota = editarItemNota;

// atualizarLinhaNota acerta o selo e a cor da linha sem redesenhar a tabela
// (redesenhar tiraria o foco do campo que a pessoa esta digitando).
function atualizarLinhaNota(i) {
  const it = NOTA.itens[i];
  const linhas = $$(".tabela-nota tbody tr");
  const tr = linhas.filter(t => Number(t.dataset.item) === i)[0];
  if (!tr) return;
  tr.classList.toggle("linha-revisar", !it.conferido);
  const selo = tr.querySelector("td:nth-child(7) .tag");
  if (selo) {
    selo.className = "tag " + (it.conferido ? "verde" : "amarelo");
    selo.textContent = it.conferido ? "conferido" : "revisar";
  }
  const campoTot = tr.querySelector("td:nth-child(6) input");
  if (campoTot && document.activeElement !== campoTot) {
    campoTot.value = fmtMoney(it.total_centavos).replace("R$ ", "");
  }
}
window.atualizarLinhaNota = atualizarLinhaNota;

async function importarNotaFoto() {
  const marcados = NOTA.itens.filter(it => it.incluir);
  if (!marcados.length) { toast("Marque pelo menos um item.", "aviso"); return; }
  const naoConferidos = marcados.filter(it => !it.conferido).length;
  const fazer = async () => {
    const btn = $("#btn-importar-nota");
    btn.disabled = true;
    $("#resultado-importacao-nota").innerHTML = '<div class="vazio">Lançando no estoque…</div>';
    try {
      const r = await api("/nota-foto/importar", { corpo: {
        confirmado: true,
        criar_novas: $("#nota-criar").checked,
        atualizar_custo: $("#nota-custo").checked,
        lancar_estoque: $("#nota-estoque").checked,
        receber_fichas: $("#nota-fichas").checked,
        margem_percentual: Number($("#nota-margem").value || 60),
        fornecedor_id: $("#nota-fornecedor") && $("#nota-fornecedor").value ? Number($("#nota-fornecedor").value) : null,
        total_nota_centavos: NOTA.totalNota,
        itens: marcados.map(it => ({
          codigo: it.codigo || "", descricao: it.descricao || "",
          quantidade: Number(it.quantidade) || 0,
          unitario_centavos: Number(it.unitario_centavos) || 0,
          total_centavos: Number(it.total_centavos) || 0
        }))
      }});
      let html = `<div class="ok-box">Nota lançada: <strong>${r.cadastradas}</strong> peça(s) cadastrada(s),
        <strong>${r.atualizadas}</strong> atualizada(s), <strong>${r.ignoradas}</strong> ignorada(s)
        ${r.fichas_fechadas ? `· <strong>${r.fichas_fechadas}</strong> ficha(s) de falta fechada(s)` : ""}
        · total da compra <strong>${fmtMoney(r.total_compra_centavos)}</strong></div>`;
      if (r.itens && r.itens.some(i => i.acao === "ignorada")) {
        html += `<div class="lista-ignorados"><table><thead><tr><th>Item</th><th>Motivo</th></tr></thead><tbody>` +
          r.itens.filter(i => i.acao === "ignorada").map(i =>
            `<tr><td>${esc(i.descricao || i.codigo)}</td><td>${esc(i.motivo || "")}</td></tr>`).join("") +
          `</tbody></table></div>`;
      }
      html += `<div class="acoes" style="margin-top:10px">
        <button class="btn" onclick="irPara('estoque')">Ver o estoque</button>
        <button class="btn" onclick="irPara('pecas')">Ver as peças</button>
        <button class="btn primario" onclick="irPara('nota')">Ler outra nota</button></div>`;
      $("#resultado-importacao-nota").innerHTML = html;
      toast("Nota importada.", "ok");
      atualizarAlertas && atualizarAlertas();
    } catch (e) {
      $("#resultado-importacao-nota").innerHTML = '<div class="erro-box">' + esc(e.message) + "</div>";
    } finally {
      btn.disabled = false;
    }
  };
  if (naoConferidos > 0) {
    confirmar(`${naoConferidos} linha(s) marcada(s) como "revisar" não fecham a conta quantidade × unitário. Lançar mesmo assim?`, fazer, "Lançar assim");
  } else {
    fazer();
  }
}
window.importarNotaFoto = importarNotaFoto;


/* ---------------- 1.0.7: formulario da tela de senha ---------------- */
document.addEventListener("DOMContentLoaded", function () {
  const form = $("#form-login");
  if (form) {
    form.addEventListener("submit", async function (ev) {
      ev.preventDefault();
      const btn = $("#btn-login");
      const senha = ($("#login-senha") || {}).value || "";
      if (btn) { btn.disabled = true; btn.textContent = "Entrando…"; }
      try { await entrarNoSistema(senha); }
      finally { if (btn) { btn.disabled = false; btn.textContent = "Entrar"; } }
    });
  }
  const ver = $("#login-ver");
  if (ver) ver.onchange = function () {
    const campo = $("#login-senha");
    if (campo) campo.type = ver.checked ? "text" : "password";
  };
});
