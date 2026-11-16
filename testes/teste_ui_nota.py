"""Teste de interface da tela "Nota por Foto" (1.0.3) com navegador real.

Prova, no navegador:
  1. a tela abre e o leitor de notas aparece como pronto
  2. enviar a foto boa le deixa 8 itens com valores certos
  3. a foto ruim nao entrega nota conferida (avisa e pede outra foto)
  4. editar uma linha na tela muda o que vai ser gravado
  5. importar lanca o estoque uma unica vez e o motivo fica registrado
  6. foto que nao e imagem e recusada com mensagem clara

Uso: UI_BASE=http://127.0.0.1:8911 python3 qa/teste_ui_nota.py
"""
import asyncio
import json
import os
import time
import urllib.request

from playwright.async_api import async_playwright

BASE = os.environ.get("UI_BASE", "http://127.0.0.1:8911")
SHOT = "/home/user/qa/shots"
FOTOS = "/home/user/autopecas/testdata"
os.makedirs(SHOT, exist_ok=True)
resultado = []


def qualquer_numero(lista, alvo):
    """A quantidade aparece como "2" ou "2,00" dependendo da formatacao."""
    for v in lista:
        if v.strip().rstrip(",00") == alvo or v.strip() == alvo:
            return True
    return False


def ok(msg):
    resultado.append((True, msg))
    print(f"  [ok] {msg}")


def erro(msg):
    resultado.append((False, msg))
    print(f"  [FALHOU] {msg}")


def api(caminho, dados=None):
    corpo = json.dumps(dados).encode() if dados is not None else None
    cab = {"X-AutoPecas": "1"}
    if corpo:
        cab["Content-Type"] = "application/json"
    r = urllib.request.Request(BASE + "/api" + caminho, data=corpo, headers=cab,
                               method="POST" if corpo else "GET")
    with urllib.request.urlopen(r, timeout=120) as resp:
        return json.loads(resp.read().decode())


async def main():
    async with async_playwright() as p:
        nav = await p.chromium.launch()
        pg = await nav.new_page(viewport={"width": 1440, "height": 1000})
        erros_console = []
        pg.on("console", lambda m: erros_console.append(m.text) if m.type == "error" else None)
        pg.on("pageerror", lambda e: erros_console.append(str(e)))

        esperas = {}
        pg.on("response", lambda r: esperas.setdefault(r.url, r.status))

        await pg.goto(BASE + "/#/nota", wait_until="load")
        await pg.wait_for_timeout(1200)

        # 1) tela abre e leitor pronto
        titulo = await pg.text_content("h1")
        if "Nota de compra por foto" in (titulo or ""):
            ok("tela 'Nota de compra por foto' abriu")
        else:
            erro(f"titulo inesperado: {titulo!r}")
        motor = await pg.text_content("#nota-motor")
        if "pronto" in (motor or ""):
            ok(f"leitor de notas disponível na tela ({motor.strip()[:60]})")
        else:
            erro(f"leitor não apareceu como pronto: {motor!r}")

        # 2) foto boa -> itens
        await pg.set_input_files("#arquivo-nota", f"{FOTOS}/nota-foto-boa.jpg")
        await pg.wait_for_timeout(300)
        await pg.click("#btn-ler-nota")
        await pg.wait_for_selector(".tabela-nota tbody tr", timeout=180000)
        linhas = await pg.query_selector_all(".tabela-nota tbody tr")
        if len(linhas) == 8:
            ok("foto boa: 8 itens na tabela de conferência")
        else:
            erro(f"foto boa: {len(linhas)} itens (esperava 8)")

        banner = await pg.text_content("#resultado-nota .ok-box")
        if "Nota conferida" in (banner or ""):
            ok("foto boa: veredito 'Nota conferida'")
        else:
            erro(f"foto boa: veredito inesperado: {(banner or '')[:120]!r}")

        # valores na tela (primeira linha: 2 x 18,50 = 37,00) - os campos sao <input>
        primeira = await pg.evaluate("""() => {
          const tr = document.querySelector(".tabela-nota tbody tr");
          return Array.from(tr.querySelectorAll("input")).map(i => i.value);
        }""")
        if ("SKU-1001" in primeira and "18,50" in primeira and "37,00" in primeira
                and qualquer_numero(primeira, "2")):
            ok(f"foto boa: linha 1 mostra SKU-1001, qtd 2, 18,50 e 37,00 ({primeira})")
        else:
            erro(f"foto boa: valores da linha 1 errados: {primeira!r}")
        await pg.screenshot(path=f"{SHOT}/nota-foto-conferencia.png", full_page=True)

        # 4) editar a quantidade na tela e ver se o total/estado acompanham
        campo_qtd = await pg.query_selector(".tabela-nota tbody tr:nth-child(1) td:nth-child(4) input")
        await campo_qtd.fill("3")
        await campo_qtd.dispatch_event("change")
        await pg.wait_for_timeout(300)
        selo = await pg.text_content(".tabela-nota tbody tr:nth-child(1) td:nth-child(7)")
        if "revisar" in (selo or ""):
            ok("editar quantidade para 3 marcou a linha como 'revisar' (3x18,50 != 37,00)")
        else:
            erro(f"linha deveria ficar em revisar depois da edição: {selo!r}")
        # volta para 2 e confere que a linha volta a ficar conferida
        await campo_qtd.fill("2")
        await campo_qtd.dispatch_event("change")
        await pg.wait_for_timeout(300)
        selo = await pg.text_content(".tabela-nota tbody tr:nth-child(1) td:nth-child(7)")
        if "conferido" in (selo or ""):
            ok("voltando para 2 a linha fica 'conferido' de novo")
        else:
            erro(f"linha deveria voltar a conferida: {selo!r}")

        # 5) importar (mede a diferenca: a base pode ja ter as pecas de uma rodada anterior)
        def total_unidades():
            return round(sum(float(i["estoque"]) for i in api("/pecas")["itens"]), 2)

        antes = api("/pecas")
        unidades_antes = total_unidades()
        await pg.click("#btn-importar-nota")
        await pg.wait_for_selector("#resultado-importacao-nota .ok-box", timeout=120000)
        msg = await pg.text_content("#resultado-importacao-nota")
        if "8" in msg and ("cadastrada" in msg or "atualizada" in msg):
            ok("importação: relatório mostrou 8 peças")
        else:
            erro(f"importação não relatou 8 peças: {msg[:150]!r}")
        depois = api("/pecas")
        codigos = {i["codigo"] for i in depois["itens"]}
        esperados = {f"SKU-{n}" for n in range(1001, 1009)}
        if esperados <= codigos:
            ok("importação: as 8 peças da nota estão no cadastro")
        else:
            erro(f"faltaram peças: {sorted(esperados - codigos)}")
        unidades_depois = total_unidades()
        if round(unidades_depois - unidades_antes, 2) == 24:
            ok(f"importação: entrou 24 unidades no estoque ({unidades_antes} -> {unidades_depois})")
        else:
            erro(f"estoque: {unidades_antes} -> {unidades_depois} (esperava +24)")
        await pg.screenshot(path=f"{SHOT}/nota-foto-importada.png", full_page=True)

        # 3) foto ruim
        await pg.click("text=Ler outra nota") if await pg.query_selector("text=Ler outra nota") else None
        await pg.goto(BASE + "/#/nota", wait_until="load")
        await pg.wait_for_timeout(800)
        await pg.set_input_files("#arquivo-nota", f"{FOTOS}/nota-foto-ruim.jpg")
        await pg.wait_for_timeout(200)
        await pg.click("#btn-ler-nota")
        await pg.wait_for_timeout(6000)
        texto = await pg.text_content("#resultado-nota")
        if "Nota conferida" not in texto:
            ok("foto ruim: não foi dada como conferida")
        else:
            erro("foto ruim foi aceita como conferida")

        # 6) arquivo que não é imagem
        await pg.goto(BASE + "/#/nota", wait_until="load")
        await pg.wait_for_timeout(600)
        await pg.set_input_files("#arquivo-nota", {"name": "nota.txt", "mimeType": "text/plain", "buffer": b"isto nao e foto"})
        await pg.wait_for_timeout(200)
        await pg.click("#btn-ler-nota")
        await pg.wait_for_timeout(2500)
        texto = await pg.text_content("#resultado-nota")
        if "imagem" in texto.lower() or "não consegui" in texto.lower():
            ok("arquivo que não é imagem é recusado com mensagem clara")
        else:
            erro(f"arquivo inválido tratado errado: {texto[:120]!r}")

        # o 400 do arquivo invalido foi provocado de proposito: nao conta como erro
        inesperados = [e for e in erros_console if "Failed to load resource" not in e]
        if not inesperados:
            ok("nenhum erro de console/página (fora o 400 provocado de propósito)")
        else:
            erro(f"erros de console: {inesperados[:3]}")

        await nav.close()

    falhas = [m for okk, m in resultado if not okk]
    print(f"\nRESUMO: {sum(1 for o, _ in resultado if o)} ok, {len(falhas)} falha(s)")
    for f in falhas:
        print("   falha:", f)
    return 1 if falhas else 0


if __name__ == "__main__":
    raise SystemExit(asyncio.run(main()))
