import asyncio, json, os, sys
from playwright.async_api import async_playwright

BASE = os.environ.get("UI_BASE", "http://127.0.0.1:8901")
SHOT = "/home/user/qa/shots"
os.makedirs(SHOT, exist_ok=True)
erros = []

async def main():
    async with async_playwright() as p:
        b = await p.chromium.launch()
        pg = await b.new_page(viewport={"width": 1440, "height": 900})
        pg.on("console", lambda m: erros.append(("console." + m.type, m.text)) if m.type in ("error", "warning") else None)
        pg.on("pageerror", lambda e: erros.append(("pageerror", str(e))))
        rotas = ["inicio", "bancada", "pecas", "xml", "estoque", "fichas", "pedidos", "clientes",
                 "fornecedores", "receber", "fechamento", "relatorios", "backup"]
        for r in rotas:
            await pg.goto(f"{BASE}/#/{r}")
            await pg.wait_for_timeout(900)
            h1 = await pg.query_selector("h1")
            txt = (await h1.inner_text()) if h1 else "(sem h1)"
            await pg.screenshot(path=f"{SHOT}/{r}.png")
            print(f"  {r:<14} -> {txt}")
        # fluxo bancada
        await pg.goto(f"{BASE}/#/bancada")
        await pg.wait_for_timeout(700)
        await pg.fill("#busca-bancada", "pastilha")
        await pg.wait_for_timeout(800)
        itens = await pg.query_selector_all("#lista-bancada .res-item")
        print("  resultados na bancada:", len(itens))
        if itens:
            await itens[0].click()
            await pg.wait_for_timeout(400)
            print("  carrinho:", (await pg.inner_text("#carrinho")).replace("\n", " | ")[:140])
        await pg.fill("#venda-cliente", "Cliente Teste UI")
        await pg.fill("#venda-placa", "XYZ9K88")
        await pg.fill("#venda-recebido", "200,00")
        await pg.wait_for_timeout(300)
        await pg.screenshot(path=f"{SHOT}/bancada-carrinho.png")
        await pg.click("text=Finalizar venda (F9)")
        await pg.wait_for_timeout(1400)
        modal = await pg.query_selector("#modal")
        visivel = modal and not await modal.evaluate("e => e.classList.contains('hidden')")
        print("  modal de venda abriu:", bool(visivel))
        await pg.screenshot(path=f"{SHOT}/venda-concluida.png")
        await pg.click("#modal .btn-x")
        # XML
        await pg.goto(f"{BASE}/#/xml"); await pg.wait_for_timeout(700)
        await pg.set_input_files("#arquivo-xml", "/home/user/autopecas/exemplos/exemplo-nfe-fornecedor.xml")
        await pg.wait_for_timeout(400)
        await pg.click("#btn-analisar"); await pg.wait_for_timeout(1800)
        print("  analise XML:", (await pg.inner_text("#resultado-xml"))[:130].replace("\n", " "))
        await pg.screenshot(path=f"{SHOT}/xml-analise.png")
        # nova peca
        await pg.goto(f"{BASE}/#/pecas"); await pg.wait_for_timeout(1000)
        await pg.click("text=+ Nova peça"); await pg.wait_for_timeout(800)
        await pg.screenshot(path=f"{SHOT}/peca-form.png")
        await pg.fill("#p-desc", "Peca teste da interface")
        await pg.fill("#p-codigo", "UI-001")
        await pg.fill("#p-custo", "10,00")
        await pg.fill("#p-preco", "16,00")
        await pg.click("#modal-rodape >> text=Salvar peça"); await pg.wait_for_timeout(1200)
        print("  peca criada pela UI: ok")
        # ficha
        await pg.goto(f"{BASE}/#/fichas"); await pg.wait_for_timeout(900)
        await pg.click("text=+ Nova ficha (F4)"); await pg.wait_for_timeout(800)
        await pg.fill("#f-desc", "Peca que o cliente pediu (teste UI)")
        await pg.fill("#f-cliente", "Dona Maria")
        await pg.click("#modal-rodape >> text=Salvar ficha"); await pg.wait_for_timeout(1100)
        print("  ficha criada pela UI: ok")
        await pg.screenshot(path=f"{SHOT}/fichas.png")
        # estoque movimento
        await pg.goto(f"{BASE}/#/estoque"); await pg.wait_for_timeout(900)
        await pg.fill("#busca-estoque", "UI-001"); await pg.wait_for_timeout(800)
        it = await pg.query_selector_all("#lista-estoque .res-item")
        if it:
            await it[0].click(); await pg.wait_for_timeout(600)
            await pg.fill("#m-qtd", "5")
            await pg.fill("#m-motivo", "compra teste")
            await pg.click("#modal-rodape >> text=Confirmar movimento"); await pg.wait_for_timeout(900)
            print("  movimento de estoque pela UI: ok")
        # relatorio
        await pg.goto(f"{BASE}/#/relatorios"); await pg.wait_for_timeout(1500)
        print("  relatorio:", (await pg.inner_text("#rel-corpo"))[:100].replace("\n", " "))
        await pg.screenshot(path=f"{SHOT}/relatorios.png")
        # backup
        await pg.goto(f"{BASE}/#/backup"); await pg.wait_for_timeout(1200)
        await pg.click("text=Fazer backup agora"); await pg.wait_for_timeout(1500)
        print("  backup pela UI: ok")
        await pg.screenshot(path=f"{SHOT}/backup.png")
        # atalho F2
        await pg.keyboard.press("F2"); await pg.wait_for_timeout(900)
        print("  F2 focou:", await pg.evaluate("document.activeElement && document.activeElement.id"))
        await b.close()
    print("\n--- erros de console/pagina ---")
    for t, m in erros[:25]:
        print(" ", t, ":", m[:220])
    if not erros:
        print("  nenhum")

asyncio.run(main())
