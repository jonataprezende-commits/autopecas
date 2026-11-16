#!/usr/bin/env python3
"""Teste de interface da 1.0.8: a nota do fornecedor vira conta a pagar.

Roda contra um servidor já aberto:
    python3 qa/teste_ui_contas.py http://127.0.0.1:8890 /tmp/pasta-nuvem

O que é conferido (no navegador de verdade, não só na API):
  1. a tela "Contas a Pagar" existe no menu e abre;
  2. importar a nota pelo caminho normal (arrastar/selecionar arquivo) mostra o
     painel "Nota do fornecedor encontrada" com fornecedor, vNF e os 3 boletos;
  3. clicar em "Importar agora" cria as 3 contas e a tela lista os 3 boletos;
  4. a mesma nota importada de novo avisa que já estava lançada (não duplica);
  5. pagar um boleto pela tela muda o status para pago;
  6. criar uma conta manual (aluguel) funciona;
  7. o Início mostra o alerta de boleto vencido;
  8. no celular (390 px) a tela não estoura a largura;
  9. nenhum erro de JavaScript no console.
"""
import json
import sys
import time
from pathlib import Path

from playwright.sync_api import sync_playwright

BASE = (sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8890").rstrip("/")
SHOTS = Path("/home/user/qa/shots")
SHOTS.mkdir(parents=True, exist_ok=True)

OK = 0
FALHAS = []
CONSOLE = []


def check(nome, cond, extra=""):
    global OK
    if cond:
        OK += 1
        print(f"  OK   {nome}")
    else:
        FALHAS.append(nome + (f" — {extra}" if extra else ""))
        print(f"  FALHOU {nome}" + (f" — {extra}" if extra else ""))


XML = """<?xml version="1.0" encoding="UTF-8"?>
<nfeProc xmlns="http://www.portalfiscal.inf.br/nfe">
 <NFe><infNFe Id="NFe35261011222333000144550010000007771000007777">
  <ide><nNF>777</nNF><serie>1</serie><dhEmi>2026-10-25T09:00:00-03:00</dhEmi></ide>
  <emit><CNPJ>11.222.333/0001-44</CNPJ><xNome>PECAS DO ZE DISTRIBUIDORA</xNome></emit>
  <det nItem="1"><prod><cProd>UI-1</cProd><xProd>AMORTECEDOR DIANTEIRO</xProd><qCom>4</qCom><vUnCom>150.00</vUnCom><vProd>600.00</vProd></prod></det>
  <det nItem="2"><prod><cProd>UI-2</cProd><xProd>MOLA HELICOIDAL</xProd><qCom>2</qCom><vUnCom>200.00</vUnCom><vProd>400.00</vProd></prod></det>
  <total><ICMSTot><vProd>1000.00</vProd><vFrete>80.00</vFrete><vNF>1080.00</vNF></ICMSTot></total>
  <cobr><dup><nDup>001</nDup><dVenc>2026-11-24</dVenc><vDup>360.00</vDup></dup>
        <dup><nDup>002</nDup><dVenc>2026-12-24</dVenc><vDup>360.00</vDup></dup>
        <dup><nDup>003</nDup><dVenc>2026-01-10</dVenc><vDup>360.00</vDup></dup></cobr>
 </infNFe></NFe>
</nfeProc>
"""


def texto(pagina, seletor):
    el = pagina.query_selector(seletor)
    return el.inner_text() if el else ""


def ir(pagina, rota):
    pagina.evaluate(f"location.hash = '#/{rota}'")
    pagina.wait_for_timeout(700)


def main():
    arquivo = Path("/tmp/nota-ui-108.xml")
    arquivo.write_text(XML, encoding="utf-8")

    with sync_playwright() as p:
        nav = p.chromium.launch()
        ctx = nav.new_context(viewport={"width": 1440, "height": 900})
        pg = ctx.new_page()
        pg.on("console", lambda m: CONSOLE.append(m.text) if m.type == "error" else None)
        pg.on("pageerror", lambda e: CONSOLE.append(str(e)))
        pg.goto(BASE + "/", wait_until="networkidle")
        time.sleep(1)

        print("\n[1] menu e tela de contas a pagar")
        menu = pg.inner_text("#menu, nav, aside")
        check("menu tem 'Contas a Pagar'", "Contas a Pagar" in menu)
        ir(pg, "pagar")
        h1 = texto(pg, "#conteudo h1")
        check("tela de contas a pagar abriu", "Contas a pagar" in h1, h1)
        check("lista vazia avisa que vem da nota",
              "importa" in pg.inner_text("#lista-pagar").lower(), pg.inner_text("#lista-pagar")[:80])

        print("\n[2] importar a nota mostra o painel da nota")
        ir(pg, "xml")
        pg.set_input_files("#arquivo-xml", str(arquivo))
        pg.wait_for_timeout(400)
        pg.click("#btn-analisar")
        pg.wait_for_selector("#bloco-nota .painel", timeout=15000)
        painel = pg.inner_text("#bloco-nota")
        check("mostra o fornecedor do XML", "PECAS DO ZE DISTRIBUIDORA" in painel)
        check("mostra o valor total da nota", "1.080,00" in painel.replace("\u00a0", " "), painel[:200])
        check("lista os 3 boletos", painel.count("360,00") == 3, painel)
        check("oferece lançar a conta a pagar", pg.is_checked("#op-conta-pagar"))
        check("oferece ratear frete/IPI/ST", pg.is_checked("#op-ratear"))
        check("mostra o frete a ratear (80,00)", "80,00" in painel)
        pg.screenshot(path=str(SHOTS / "contas-1-analise.png"), full_page=True)

        print("\n[3] importar cria as contas a pagar")
        pg.click("#btn-importar")
        pg.wait_for_selector("#resultado-xml .ok-box", timeout=30000)
        time.sleep(1.5)
        res = pg.inner_text("#resultado-xml")
        check("resultado informa as 3 contas", "3 conta(s) a pagar" in res, res[:300])
        check("resultado informa o rateio do frete", "Frete/IPI/ST" in res or "rateado" in res.lower(), res[:400])
        pg.screenshot(path=str(SHOTS / "contas-2-importado.png"), full_page=True)

        ir(pg, "pagar")
        time.sleep(1)
        lista = pg.inner_text("#lista-pagar")
        check("as 3 contas aparecem na tela", lista.count("PECAS DO ZE DISTRIBUIDORA") == 3, lista[:300])
        check("valor do boleto aparece", "360,00" in lista)
        check("boleto vencido em 10/01 aparece como vencida", "vencida" in lista.lower(), lista[:400])
        check("total em aberto somado", "1.080,00" in lista, lista[-200:])
        pg.screenshot(path=str(SHOTS / "contas-3-lista.png"), full_page=True)

        print("\n[4] pagar um boleto pela tela")
        pg.click("button:has-text('Pagar')")
        pg.wait_for_selector("#pagar-valor", timeout=5000)
        pg.fill("#pagar-valor", "360,00")
        pg.click("button:has-text('Confirmar pagamento')")
        pg.wait_for_timeout(1500)
        lista = pg.inner_text("#lista-pagar")
        check("uma conta ficou como paga", lista.count("PECAS DO ZE DISTRIBUIDORA") == 2, lista[:300])
        check("total em aberto caiu para 720,00", "720,00" in lista, lista[-200:])

        print("\n[5] reimportar a mesma nota não duplica a despesa")
        ir(pg, "xml")
        pg.set_input_files("#arquivo-xml", str(arquivo))
        pg.wait_for_timeout(400)
        pg.click("#btn-analisar")
        pg.wait_for_selector("#bloco-nota .painel", timeout=15000)
        check("analisar avisa que a nota já foi lançada",
              "já foi lançada" in pg.inner_text("#bloco-nota").lower(), pg.inner_text("#bloco-nota")[:200])
        pg.click("#btn-importar")
        pg.wait_for_selector("#resultado-xml .erro-box, #resultado-xml .ok-box", timeout=30000)
        time.sleep(1.2)
        res = pg.inner_text("#resultado-xml")
        check("importação avisa que não criou outra conta",
              "já estava lançada" in res.lower(), res[:300])

        print("\n[6] conta manual (aluguel)")
        ir(pg, "pagar")
        pg.wait_for_timeout(600)
        pg.click("button:has-text('+ Nova conta')")
        pg.wait_for_selector("#conta-fornecedor", timeout=5000)
        pg.fill("#conta-fornecedor", "ALUGUEL DO PONTO")
        pg.fill("#conta-valor", "2.500,00")
        pg.fill("#conta-venc", "2026-01-05")
        pg.click("button:has-text('Salvar conta')")
        pg.wait_for_timeout(1500)
        check("conta manual entrou na lista", "ALUGUEL DO PONTO" in pg.inner_text("#lista-pagar"))
        check("conta manual com valor certo", "2.500,00" in pg.inner_text("#lista-pagar"))

        print("\n[7] alerta no Início")
        ir(pg, "inicio")
        time.sleep(1.2)
        corpo = pg.inner_text("#conteudo")
        check("Início avisa dos boletos vencidos", "vencido" in corpo.lower(), corpo[:400])
        check("card A pagar em aberto existe", "a pagar em aberto" in corpo.lower())
        pg.screenshot(path=str(SHOTS / "contas-4-inicio.png"), full_page=True)

        print("\n[8] celular: nada estoura a largura")
        cel = nav.new_context(viewport={"width": 390, "height": 844}, is_mobile=True, has_touch=True)
        pm = cel.new_page()
        pm.on("console", lambda m: CONSOLE.append(m.text) if m.type == "error" else None)
        pm.on("pageerror", lambda e: CONSOLE.append(str(e)))
        pm.goto(BASE + "/", wait_until="networkidle")
        time.sleep(1)
        for rota in ["inicio", "pagar", "xml", "receber"]:
            ir(pm, rota)
            time.sleep(0.8)
            larg = pm.evaluate("Math.max(document.documentElement.scrollWidth, document.body.scrollWidth)")
            check(f"{rota} cabe na largura do celular ({larg}px)", larg <= 392, f"scrollWidth={larg}")
        pm.screenshot(path=str(SHOTS / "contas-5-celular.png"), full_page=True)

        print("\n[9] console")
        reais = [c for c in CONSOLE if "favicon" not in c.lower()]
        check("nenhum erro de JavaScript", not reais, "; ".join(reais[:3]))

        nav.close()

    print(f"\n{OK} OK / {len(FALHAS)} falha(s)")
    if FALHAS:
        for f in FALHAS:
            print("   -", f)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
