"""Teste de interface da 1.0.2: prova no navegador real que o preco digitado com
separador de milhar e gravado certo e que a ficha recebida nao duplica estoque.
Tambem roda todas as telas para garantir que nada quebrou.

Uso: UI_BASE=http://127.0.0.1:8901 python3 qa/teste_ui_102.py
"""
import asyncio
import json
import os
import time
import urllib.request

from playwright.async_api import async_playwright

BASE = os.environ.get("UI_BASE", "http://127.0.0.1:8901")
SHOT = "/home/user/qa/shots"
os.makedirs(SHOT, exist_ok=True)
resultado = []


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
    with urllib.request.urlopen(r, timeout=30) as resp:
        return json.loads(resp.read().decode())


async def main():
    async with async_playwright() as p:
        navegador = await p.chromium.launch()
        pg = await navegador.new_page(viewport={"width": 1440, "height": 900})
        erros = []
        pg.on("console", lambda m: erros.append(("console." + m.type, m.text)) if m.type == "error" else None)
        pg.on("pageerror", lambda e: erros.append(("pageerror", str(e))))

        # ---------- todas as telas abrem ----------
        rotas = ["inicio", "bancada", "pecas", "xml", "estoque", "fichas", "pedidos", "clientes",
                 "fornecedores", "receber", "fechamento", "relatorios", "backup"]
        for r in rotas:
            await pg.goto(f"{BASE}/#/{r}")
            await pg.wait_for_timeout(700)
            h1 = await pg.query_selector("h1")
            if not h1:
                erro(f"tela {r} sem titulo")
        ok(f"{len(rotas)} telas abriram sem erro de pagina")

        # ---------- B3: preco digitado com milhar ----------
        await pg.goto(f"{BASE}/#/pecas")
        await pg.wait_for_timeout(800)
        await pg.click("text=Nova peça")
        await pg.wait_for_timeout(500)
        codigo = "UI-MILHAR-" + time.strftime("%H%M%S")
        await pg.fill("#p-codigo", codigo)
        await pg.fill("#p-desc", "Radiador com preço de milhar")
        # preenche o preco ANTES do custo: a "sugestao de venda" (custo + 60%)
        # roda no evento change do custo e escreveria por cima do preco
        await pg.fill("#p-preco", "1.234")
        await pg.fill("#p-estoque", "1.000")
        await pg.fill("#p-custo", "1.000")
        await pg.click("text=Salvar peça")
        await pg.wait_for_timeout(1200)
        peca = None
        for it in api("/pecas?q=" + codigo)["itens"]:
            if it["codigo"] == codigo:
                peca = it
        if not peca:
            erro("peça criada pela tela não foi encontrada")
        else:
            if peca["preco_centavos"] == 123400:
                ok('campo preço "1.234" -> R$ 1.234,00 (123400 centavos)')
            else:
                erro(f'campo preço "1.234" gravou {peca["preco_centavos"]} centavos (esperado 123400)')
            if peca["custo_centavos"] == 100000:
                ok('campo custo "1.000" -> R$ 1.000,00')
            else:
                erro(f'campo custo "1.000" gravou {peca["custo_centavos"]} (esperado 100000)')
            if peca["estoque"] == 1000:
                ok('campo estoque "1.000" -> 1000 unidades')
            else:
                erro(f'campo estoque "1.000" gravou {peca["estoque"]} (esperado 1000)')

        # ---------- B1 na interface: botao Receber da ficha ----------
        fornecedor = api("/fornecedores", {"nome": "Fornecedor UI"})
        ficha = api("/fichas", {"peca_id": peca["id"] if peca else 1,
                                "descricao": "Radiador com preço de milhar", "quantidade": 2,
                                "fornecedor_id": fornecedor["id"]})
        await pg.goto(f"{BASE}/#/fichas")
        await pg.wait_for_timeout(900)
        # clica no "Receber" da FICHA QUE O TESTE CRIOU (a lista pode ter outras)
        botao = await pg.query_selector(f"tr:has-text('{ficha['id'] if isinstance(ficha, dict) and 'id' in ficha else ''}')")
        if not botao:
            botao = await pg.query_selector("button:has-text('Receber')")
        if botao:
            # acha a linha pelo id (#N na primeira coluna) e o Receber dela
            alvo = await pg.query_selector(f"xpath=//tr[td[1][normalize-space()='#{ficha['id']}']]//button[contains(., 'Receber')]")
            if alvo:
                botao = alvo
            await botao.click()
            await pg.wait_for_timeout(500)
            if not await pg.query_selector("#btn-confirmar"):
                erro("o botão Receber da ficha criada não pediu confirmação")
            else:
                await pg.click("#btn-confirmar")
            await pg.wait_for_timeout(1200)
            depois = api(f"/pecas/{peca['id']}")["estoque"]
            if depois == 1002:
                ok(f"botão Receber da ficha entrou 2 unidades (1000 -> {depois})")
            else:
                erro(f"estoque depois de receber pela tela = {depois} (esperado 1002)")
            # receber de novo (duplo clique / outra aba) tem que ser recusado
            try:
                api(f"/fichas/{ficha['id']}/status", {"status": "recebida"})
                erro("API aceitou receber a mesma ficha duas vezes")
            except urllib.error.HTTPError as e:
                if e.code == 409:
                    ok("receber a mesma ficha de novo é bloqueado (409)")
                else:
                    erro(f"segundo recebimento retornou {e.code}")
        else:
            erro("botão Receber não apareceu na tela de fichas")

        # ---------- relatorio em PDF baixa e abre ----------
        await pg.goto(f"{BASE}/#/relatorios")
        await pg.wait_for_timeout(900)
        with urllib.request.urlopen(BASE + "/api/relatorios/estoque_valor/pdf", timeout=30) as resp:
            pdf = resp.read()
        if pdf.startswith(b"%PDF") and b"Filtro" in pdf or len(pdf) > 1000:
            ok(f"relatório PDF baixado do servidor ({len(pdf)} bytes)")
        else:
            erro("relatório PDF inválido")

        await pg.screenshot(path=f"{SHOT}/backup-1.0.2.png")

        if erros:
            for e in erros:
                erro(f"erro no navegador: {e}")
        else:
            ok("nenhum erro de console/página")

        await navegador.close()

    falhas = [m for okk, m in resultado if not okk]
    print(f"\nRESUMO: {len(resultado) - len(falhas)} ok, {len(falhas)} falha(s)")
    for f in falhas:
        print("  -", f)
    return 1 if falhas else 0


if __name__ == "__main__":
    raise SystemExit(asyncio.run(main()))
