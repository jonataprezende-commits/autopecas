#!/usr/bin/env python3
"""Teste de tela das correcoes da 1.0.10.

Cobre o que so aparece NA TELA (o teste de API nao alcanca):

  1. Configuracoes: campo numerico vazio/invalido avisa qual campo e' e NAO
     salva nada quebrado;
  2. Nota do fornecedor: quando os boletos do XML nao fecham com o total da
     nota, o aviso aparece ANTES de importar (e o rotulo mostra a soma dos
     boletos, nao o total da nota);
  3. Venda: quando a peca fica sem estoque (com a trava desligada), a tela
     avisa na hora qual peca ficou negativa.

Uso:  python3 qa/teste_ui_1010.py http://127.0.0.1:PORTA
"""
import json
import sys
import time
import urllib.error
import urllib.request

from playwright.sync_api import sync_playwright

BASE = (sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8940").rstrip("/")
API = BASE + "/api"
falhas = []
oks = 0


def ok(msg):
    global oks
    oks += 1
    print("  OK   " + msg)


def erro(msg):
    falhas.append(msg)
    print("  FALHOU " + msg)


def check(msg, cond, extra=""):
    if cond:
        ok(msg)
    else:
        erro(msg + ((" — " + str(extra)) if extra != "" else ""))


def api(rota, corpo=None, metodo=None):
    cab = {"X-AutoPecas": "1", "Content-Type": "application/json", "Origin": BASE}
    dados = json.dumps(corpo).encode() if corpo is not None else None
    m = metodo or ("GET" if corpo is None else "POST")
    r = urllib.request.Request(API + rota, data=dados, headers=cab, method=m)
    with urllib.request.urlopen(r, timeout=30) as resp:
        return json.loads(resp.read() or b"{}")


XML_BOLETO_ERRADO = """<?xml version="1.0" encoding="utf-8"?>
<nfeProc><NFe><infNFe Id="NFe35260912345678000199550010000099991000099999" versao="4.00">
<ide><nNF>9999</nNF><serie>1</serie><dhEmi>2026-10-01T10:00:00-03:00</dhEmi></ide>
<emit><CNPJ>98765432000188</CNPJ><xNome>FORNECEDOR DO TESTE LTDA</xNome></emit>
<det nItem="1"><prod><cProd>TB1</cProd><xProd>BOMBA DAGUA</xProd><uCom>UN</uCom><qCom>1</qCom>
<vUnCom>100.00</vUnCom><vProd>100.00</vProd></prod></det>
<total><ICMSTot><vProd>100.00</vProd><vNF>500.00</vNF></ICMSTot></total>
<cobr><dup><nDup>001</nDup><dVenc>2026-11-01</dVenc><vDup>200.00</vDup></dup></cobr>
</infNFe></NFe></nfeProc>
"""


def main():
    with sync_playwright() as p:
        nav = p.chromium.launch()
        pg = nav.new_context(viewport={"width": 1440, "height": 950}).new_page()
        erros_js = []
        pg.on("pageerror", lambda e: erros_js.append(str(e)))
        pg.on("console", lambda m: erros_js.append(m.text) if m.type == "error" else None)
        pg.goto(BASE + "/", wait_until="networkidle")
        time.sleep(1.2)

        # ---------- 1) configuracao: numero invalido ----------
        print("[1] Configurações: campo numérico inválido")
        pg.evaluate("location.hash='#/backup'")
        pg.wait_for_selector("#cfg-manter", timeout=15000)
        antes = api("/config")["backup_manter"]
        pg.fill("#cfg-manter", "")
        pg.click("text=Salvar ajustes")
        time.sleep(0.8)
        depois = api("/config")["backup_manter"]
        check("campo vazio não é gravado", depois == antes, (antes, depois))
        corpo_pagina = pg.inner_text("body")
        check("a tela diz qual campo está errado", "Cópias diárias" in corpo_pagina, corpo_pagina[:200])
        pg.fill("#cfg-manter", "2")
        pg.fill("#cfg-manter-horarios", "9999")
        pg.click("text=Salvar ajustes")
        time.sleep(0.8)
        check("valor fora da faixa também não passa", api("/config")["backup_manter_horarios"] != "9999",
              api("/config")["backup_manter_horarios"])
        pg.fill("#cfg-manter-horarios", "3")
        pg.click("text=Salvar ajustes")
        time.sleep(0.8)
        check("valor válido salva", api("/config")["backup_manter_horarios"] == "3",
              api("/config")["backup_manter_horarios"])
        pg.screenshot(path="qa/shots/1010-1-config.png")

        # ---------- 2) nota com boleto que não fecha ----------
        print("[2] Nota do fornecedor: boletos que não fecham com o total")
        pg.evaluate("location.hash='#/xml'")
        time.sleep(1.0)
        # o campo do arquivo e' input[type=file]; envia o XML pela tela
        pg.set_input_files("#arquivo-xml", files=[{
            "name": "nota-boleto-errado.xml",
            "mimeType": "text/xml",
            "buffer": XML_BOLETO_ERRADO.encode("utf-8"),
        }])
        time.sleep(0.6)
        pg.click("#btn-analisar")
        time.sleep(1.5)
        painel = pg.inner_text("#bloco-nota") if pg.query_selector("#bloco-nota") else ""
        check("o painel da nota mostra o aviso", "Confira antes de importar" in painel, painel[:300])
        check("o rótulo mostra a soma dos boletos (200) e não o total da nota (500)",
              "R$ 200,00" in painel, painel[-400:])
        pg.screenshot(path="qa/shots/1010-2-nota-aviso.png")

        # ---------- 3) venda que deixa estoque negativo ----------
        print("[3] Venda sem estoque: aviso na hora")
        api("/config", {"permitir_estoque_negativo": "1"})
        peca = api("/pecas", {"descricao": "Bomba d'agua", "codigo": "BG-" + str(int(time.time())),
                              "preco_centavos": 9000, "custo_centavos": 5000, "estoque": 1})
        pg.evaluate("location.hash='#/bancada'")
        time.sleep(1.2)
        pg.fill("#busca-bancada", peca["codigo"])
        time.sleep(1.2)
        pg.click(f"#lista-bancada .res-item[data-id='{peca['id']}']")
        time.sleep(0.8)
        # quantidade maior que o estoque (1): 3 unidades
        pg.fill("#carrinho input[data-campo='qtd']", "3")
        pg.dispatch_event("#carrinho input[data-campo='qtd']", "change")
        time.sleep(0.6)
        # o total na tela tem que acompanhar a nova quantidade (bug do change)
        linha = pg.inner_text("#carrinho tbody tr")
        check("total do carrinho acompanha a quantidade nova (3 x 90,00)", "270,00" in linha, linha)
        valor_qtd = pg.input_value("#carrinho input[data-campo='qtd']")
        check("quantidade nova aparece na tela do carrinho", valor_qtd.strip() == "3", valor_qtd)
        pg.click("text=Finalizar venda")
        time.sleep(2.0)
        corpo = pg.inner_text("body")
        check("a tela avisa que a peça ficou sem estoque",
              "sem estoque" in corpo.lower() or "estoque negativo" in corpo.lower(), corpo[:400])
        restante = api(f"/pecas/{peca['id']}")["estoque"]
        check("a venda foi gravada mesmo assim (trava desligada)", restante == -2, restante)
        api("/config", {"permitir_estoque_negativo": "0"})
        pg.screenshot(path="qa/shots/1010-3-venda-negativa.png")

        # ---------- 4) console limpo ----------
        print("[4] Console")
        reais = [e for e in erros_js if "favicon" not in e.lower()]
        check("nenhum erro de JavaScript", not reais, reais[:3])
        nav.close()

    print()
    print(f"{oks} OK / {len(falhas)} falha(s)")
    for f in falhas:
        print("  - " + f)
    return 1 if falhas else 0


if __name__ == "__main__":
    sys.exit(main())
