#!/usr/bin/env python3
"""Teste de UI do acesso pelo celular (versao 1.0.7).

Roda contra um AutoPecas aberto em rede:
    AutoPecas.exe -host 0.0.0.0 -senha-rede <SENHA> -porta 8830 -sem-navegador

Uso:
    python3 qa/teste_ui_celular.py http://192.168.0.12:8830 http://127.0.0.1:8830 SENHA

Confere:
  1. tela de senha aparece no celular; senha errada avisa; senha certa entra
  2. barra do topo, gaveta do menu e o titulo mudando de tela
  3. painel "Acessar pelo celular": QR Code, endereco e botao copiar
  4. o computador da loja (127.0.0.1) NAO pede senha e nao mostra a gaveta
  5. a pagina nao estoura a largura do celular (sem rolagem lateral)
  6. tabelas rolam dentro do proprio quadro (nao empurram a tela)
  7. Nota por Foto usa a camera do celular (capture=environment)
  8. sair volta para a tela de senha e bloqueia a lista de pecas
"""
import asyncio
import sys

BASE_CEL = (sys.argv[1] if len(sys.argv) > 1 else "http://192.168.0.12:8830").rstrip("/")
BASE_PC = (sys.argv[2] if len(sys.argv) > 2 else "http://127.0.0.1:8830").rstrip("/")
SENHA = sys.argv[3] if len(sys.argv) > 3 else "teste123"
SHOTS = "/home/user/qa/shots"

ok = []
falhas = []


def checar(nome, condicao, detalhe=""):
    if condicao:
        ok.append(nome)
        print(f"  OK   {nome}")
    else:
        falhas.append(f"{nome} {detalhe}".strip())
        print(f"  FALHA {nome} {detalhe}")


async def sem_rolagem_lateral(pg):
    return await pg.evaluate(
        "() => document.documentElement.scrollWidth <= window.innerWidth + 1")


async def main():
    from playwright.async_api import async_playwright

    async with async_playwright() as p:
        navegador = await p.chromium.launch()

        # ---------------- celular ----------------
        cel = await navegador.new_context(
            viewport={"width": 390, "height": 844}, device_scale_factor=2,
            is_mobile=True, has_touch=True,
            user_agent=("Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) "
                        "AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"))
        pg = await cel.new_page()
        erros_js = []
        pg.on("pageerror", lambda e: erros_js.append(str(e)))
        await pg.goto(BASE_CEL, wait_until="networkidle")
        await pg.wait_for_timeout(700)

        print("1) tela de senha")
        checar("tela de senha aparece no celular", await pg.is_visible("#tela-login"))
        checar("tela do sistema fica escondida ate entrar",
               await pg.evaluate("() => getComputedStyle(document.querySelector('#app')).display") == "none")
        await pg.screenshot(path=f"{SHOTS}/celular-1-senha.png")

        await pg.fill("#login-senha", "senha-errada")
        await pg.click("#btn-login")
        await pg.wait_for_timeout(900)
        checar("senha errada mostra aviso", await pg.is_visible("#login-erro"))
        await pg.screenshot(path=f"{SHOTS}/celular-2-senha-errada.png")

        await pg.fill("#login-senha", SENHA)
        await pg.click("#btn-login")
        await pg.wait_for_timeout(1800)
        checar("senha certa abre o sistema", await pg.is_hidden("#tela-login"))
        checar("nome da loja aparece no menu", (await pg.inner_text("#loja-nome")).strip() != "")
        await pg.screenshot(path=f"{SHOTS}/celular-3-inicio.png", full_page=True)

        print("2) barra do topo e gaveta")
        checar("barra do celular visivel", await pg.is_visible("#topo-mobile"))
        checar("menu lateral comeca fechado",
               await pg.evaluate("() => !document.body.classList.contains('menu-aberto')"))
        await pg.click("#btn-menu")
        await pg.wait_for_timeout(450)
        checar("gaveta abre", await pg.evaluate("() => document.body.classList.contains('menu-aberto')"))
        await pg.screenshot(path=f"{SHOTS}/celular-4-menu.png")
        await pg.mouse.click(370, 600)  # fora da gaveta
        await pg.wait_for_timeout(400)
        checar("gaveta fecha ao tocar fora",
               await pg.evaluate("() => !document.body.classList.contains('menu-aberto')"))

        await pg.click("#btn-menu")
        await pg.wait_for_timeout(300)
        await pg.click('#menu a[data-rota="pecas"]')
        await pg.wait_for_timeout(1200)
        checar("tocar no menu troca de tela", "pecas" in (pg.url.split("#")[-1]))
        checar("titulo do topo acompanha a tela",
               (await pg.inner_text("#topo-titulo")).strip().lower().startswith("peça"))
        checar("gaveta fecha ao escolher uma tela",
               await pg.evaluate("() => !document.body.classList.contains('menu-aberto')"))
        checar("tela de pecas sem rolagem lateral", await sem_rolagem_lateral(pg))
        await pg.screenshot(path=f"{SHOTS}/celular-5-pecas.png", full_page=True)

        print("3) painel Acessar pelo celular")
        await pg.goto(BASE_CEL + "/#/inicio", wait_until="networkidle")
        await pg.wait_for_timeout(1500)
        texto = await pg.inner_text("#conteudo-celular")
        checar("painel mostra que esta ligado", "ligado" in texto.lower())
        checar("painel mostra o endereco da rede", "http://" in texto)
        checar("painel tem QR Code", await pg.query_selector("#qr-celular") is not None)
        src = ""
        qr = await pg.query_selector("#qr-celular")
        if qr:
            src = await qr.get_attribute("src") or ""
        checar("QR Code foi desenhado", src.startswith("data:image"), src[:40])
        await pg.screenshot(path=f"{SHOTS}/celular-6-painel.png", full_page=True)

        print("4) outras telas no celular")
        for rota in ["bancada", "catalogo", "estoque", "fichas", "relatorios", "backup"]:
            await pg.goto(f"{BASE_CEL}/#/{rota}", wait_until="networkidle")
            await pg.wait_for_timeout(1000)
            sem_estouro = await sem_rolagem_lateral(pg)
            checar(f"tela {rota} sem rolagem lateral", sem_estouro)
        await pg.goto(f"{BASE_CEL}/#/bancada", wait_until="networkidle")
        await pg.wait_for_timeout(1200)
        await pg.screenshot(path=f"{SHOTS}/celular-7-bancada.png", full_page=True)

        print("5) Nota por Foto usa a camera")
        await pg.goto(f"{BASE_CEL}/#/nota", wait_until="networkidle")
        await pg.wait_for_timeout(1200)
        captura = await pg.evaluate(
            "() => { const i = document.querySelector('#arquivo-nota'); return i ? i.getAttribute('capture') : 'sem-campo'; }")
        checar("campo de foto pede a camera do celular", captura == "environment", captura)
        await pg.screenshot(path=f"{SHOTS}/celular-8-nota.png", full_page=True)

        print("6) sair do sistema")
        await pg.goto(BASE_CEL + "/#/inicio", wait_until="networkidle")
        await pg.wait_for_timeout(1200)
        checar("botao sair aparece no celular", await pg.is_visible("#btn-sair-topo"))
        await pg.click("#btn-sair-topo")
        await pg.wait_for_timeout(1200)
        checar("sair volta para a tela de senha", await pg.is_visible("#tela-login"))
        bloqueado = await pg.evaluate("""async () => {
            const r = await fetch('/api/pecas', {headers:{'X-AutoPecas':'1'}});
            return r.status;
        }""")
        checar("depois de sair a API bloqueia o celular", bloqueado == 401, f"status {bloqueado}")
        await pg.screenshot(path=f"{SHOTS}/celular-9-saiu.png")
        checar("nenhum erro de JavaScript na tela do celular", not erros_js, "; ".join(erros_js[:3]))
        await cel.close()

        # ---------------- computador da loja ----------------
        print("7) computador da loja continua sem senha")
        pc = await navegador.new_context(viewport={"width": 1440, "height": 900})
        p2 = await pc.new_page()
        erros_pc = []
        p2.on("pageerror", lambda e: erros_pc.append(str(e)))
        await p2.goto(BASE_PC, wait_until="networkidle")
        await p2.wait_for_timeout(1500)
        checar("PC nao pede senha", await p2.is_hidden("#tela-login"))
        checar("PC nao mostra a barra do celular", await p2.is_hidden("#topo-mobile"))
        checar("PC mostra o menu lateral fixo", await p2.is_visible("#sidebar"))
        checar("PC nao mostra o botao sair",
               await p2.evaluate("() => getComputedStyle(document.querySelector('#btn-sair')).display") == "none")
        painel_pc = await p2.inner_text("#conteudo-celular")
        checar("painel do celular aparece tambem no PC", "ligado" in painel_pc.lower())
        await p2.screenshot(path=f"{SHOTS}/celular-10-pc.png")
        checar("nenhum erro de JavaScript no PC", not erros_pc, "; ".join(erros_pc[:3]))
        await pc.close()
        await navegador.close()

    print()
    print(f"{len(ok)} OK / {len(falhas)} falha(s)")
    if falhas:
        for f in falhas:
            print("  - " + f)
        sys.exit(1)


asyncio.run(main())
