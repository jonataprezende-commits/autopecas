import asyncio
from playwright.async_api import async_playwright

B = "http://127.0.0.1:8961"


async def main():
    async with async_playwright() as p:
        nav = await p.chromium.launch()
        pg = await nav.new_page()
        pg.on("requestfailed", lambda r: print("FALHOU:", r.method, r.url, r.failure))
        pg.on("response", lambda r: print("RESP:", r.status, r.request.method, r.url) if "/api/" in r.url else None)
        pg.on("console", lambda m: print("CONSOLE:", m.type, m.text[:200]) if m.type == "error" else None)
        pg.on("pageerror", lambda e: print("PAGEERROR:", e))
        await pg.goto(B + "/", wait_until="networkidle")
        await asyncio.sleep(2)
        await pg.evaluate("location.hash='#/inicio'")
        await asyncio.sleep(2)
        await pg.wait_for_selector("#conteudo-celular", timeout=15000)
        await asyncio.sleep(1)
        if await pg.query_selector("#nova-senha-celular"):
            await pg.fill("#nova-senha-celular", "teste123")
            await pg.fill("#nova-senha-celular2", "teste123")
            await pg.click("text=Ligar o acesso pelo celular")
            await asyncio.sleep(14)
            print("TOASTS:", await pg.eval_on_selector_all(".toast", "els=>els.map(e=>e.innerText)"))
            painel = await pg.query_selector("#conteudo-celular")
            print("PAINEL:", (await pg.inner_text("#conteudo-celular"))[:300] if painel else "sem painel")
            print("BODY:", (await pg.inner_text("body"))[:300])
        else:
            print("painel ja ligado:", (await pg.inner_text("#conteudo-celular"))[:200])
        await pg.screenshot(path="/tmp/clica.png")
        await nav.close()


asyncio.run(main())
