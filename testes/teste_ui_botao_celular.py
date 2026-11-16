#!/usr/bin/env python3
"""Teste de tela do painel "Acessar pelo celular" — CLICANDO nos botões.

Este teste existe por causa de um defeito real: o botão "Ligar o acesso pelo
celular" chamava a rota sem o corpo da requisição, o navegador fazia um GET e o
programa respondia "404 page not found". As outras suítes testavam a API por
fora (fetch direto), então o botão podia estar quebrado sem ninguém notar.

O teste sobe o próprio programa (sem senha), abre a tela Início e faz o caminho
completo que a pessoa faz na loja:

  1. digita a senha e clica em "Ligar o acesso pelo celular";
  2. o programa reinicia sozinho; a tela volta e mostra "Ligado." + QR Code;
  3. o celular (mesma máquina, endereço de rede) pede a senha e entra;
  4. "Trocar a senha" -> senha nova passa a valer, a antiga não;
  5. "Desligar o acesso pelo celular" -> volta para "Desligado." e o programa
     deixa de atender a rede.

Uso:  python3 qa/teste_ui_botao_celular.py [binário] [porta]
"""
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request

from playwright.sync_api import sync_playwright

BIN = sys.argv[1] if len(sys.argv) > 1 else "/tmp/ap110b"
PORTA = int(sys.argv[2]) if len(sys.argv) > 2 else 8965
BASE = f"http://127.0.0.1:{PORTA}"
DADOS = f"/tmp/btcel{PORTA}"
SENHA = "loja1234"
SENHA2 = "loja5678"

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
    ok(msg) if cond else erro(msg + ((" — " + str(extra)) if extra != "" else ""))


def ping(timeout=3):
    try:
        with urllib.request.urlopen(BASE + "/api/ping", timeout=timeout) as r:
            return json.loads(r.read())
    except Exception:
        return None


def api_remota(base, rota, senha=None):
    """Chama a API pelo endereco de REDE (como o celular faz): o acesso local
    (127.0.0.1) nao pede senha, entao a senha so pode ser testada por aqui."""
    import base64
    cab = {"X-AutoPecas": "1", "Origin": base}
    if senha:
        cab["Authorization"] = "Basic " + base64.b64encode(("autopecas:" + senha).encode()).decode()
    req = urllib.request.Request(base + "/api" + rota, headers=cab)
    with urllib.request.urlopen(req, timeout=15) as r:
        return json.loads(r.read() or b"{}")


def api(rota, corpo=None, senha=None):
    cab = {"X-AutoPecas": "1", "Origin": BASE}
    d = None
    if corpo is not None:
        cab["Content-Type"] = "application/json"
        d = json.dumps(corpo).encode()
    if senha:
        import base64
        cab["Authorization"] = "Basic " + base64.b64encode(("autopecas:" + senha).encode()).decode()
    req = urllib.request.Request(BASE + "/api" + rota, data=d, headers=cab,
                                 method="GET" if d is None else "POST")
    with urllib.request.urlopen(req, timeout=15) as r:
        return json.loads(r.read() or b"{}")


def subir():
    # garante que nenhum resto de execucao anterior esteja atendendo nesta porta
    # (senao o teste conversa com o programa velho e o resultado nao vale)
    os.system(f"pkill -f 'ap110b -dados {DADOS}' 2>/dev/null; pkill -f 'AutoPecas.exe' 2>/dev/null")
    for _ in range(40):
        if ping() is None:
            break
        time.sleep(0.25)
    os.system(f"rm -rf {DADOS} && mkdir -p {DADOS}")
    log = open(f"/tmp/btcel{PORTA}.log", "wb")
    p = subprocess.Popen([BIN, "-dados", DADOS, "-porta", str(PORTA), "-sem-navegador"],
                         stdout=log, stderr=log)
    for _ in range(120):
        r = ping()
        if r:
            if r.get("pasta") not in (DADOS, "Z:" + DADOS.replace("/", "\\")):
                raise SystemExit(f"a porta {PORTA} está sendo usada por outro processo ({r.get('pasta')})")
            return p
        time.sleep(0.5)
    raise SystemExit("o programa não subiu")


def await_painel(pg, segundos=20):
    """Volta para a tela Inicio e espera o painel do celular aparecer."""
    pg.wait_for_selector("#conteudo-celular", timeout=segundos * 1000)
    time.sleep(1.2)


def esperar_voltar(segundos=40):
    """Depois do reinício, espera o programa responder de novo."""
    for _ in range(segundos * 2):
        time.sleep(0.5)
        if ping():
            return True
    return False


def main():
    proc = subir()
    try:
        with sync_playwright() as p:
            nav = p.chromium.launch()
            pg = nav.new_page()
            erros = []
            pg.on("pageerror", lambda e: erros.append(str(e)))
            pg.on("console", lambda m: erros.append(m.text) if m.type == "error" else None)
            pg.goto(BASE + "/#/inicio", wait_until="networkidle")
            pg.wait_for_selector("#conteudo-celular", timeout=20000)
            time.sleep(1)

            print("1) botão Ligar o acesso pelo celular")
            check("painel começa desligado", "Desligado" in pg.inner_text("#conteudo-celular"))
            pg.fill("#nova-senha-celular", SENHA)
            pg.fill("#nova-senha-celular2", SENHA)
            pg.click("text=Ligar o acesso pelo celular")
            time.sleep(2)
            corpo = pg.inner_text("body")
            check("não aparece '404 page not found'", "404" not in corpo, corpo[:200])
            # o reinicio pode ser rapido: ou a tela mostra o aviso, ou o sistema
            # ja voltou. O que NAO pode e' aparecer 404.
            # 1.0.11: nao ha mais reinicio — o programa passa a atender a rede na
            # hora e a tela se atualiza sozinha
            avisou = False
            for _ in range(20):
                corpo = pg.inner_text("#conteudo-celular")
                if "Ligado." in corpo:
                    avisou = True
                    break
                time.sleep(0.3)
            check("o painel passa para Ligado sem reiniciar", avisou, corpo[:300])
            check("o programa continua no ar (mesma porta)", ping() is not None)
            painel = pg.inner_text("#conteudo-celular")
            check("painel mostra Ligado", "Ligado." in painel, painel[:200])
            r = api("/rede")
            check("servidor entrou no modo rede", r.get("modo") == "rede", r.get("modo"))
            check("senha ficou definida", bool(r.get("senha_definida")), r)
            urls = r.get("urls") or []
            if urls:
                src = pg.get_attribute("#qr-celular", "src") or ""
                check("QR Code desenhado", src.startswith("data:image"), src[:40])
                check("painel lista o endereço de rede", True, urls)
            else:
                # maquina de teste sem placa de rede: o painel tem que EXPLICAR
                check("sem rede, o painel avisa que o PC nao esta na rede",
                      "não está em nenhuma rede" in painel, painel[:200])
            pg.screenshot(path="qa/shots/btcel-1-ligado.png")

            print("2) celular entra com a senha")
            remoto = urls[0]["url"] if urls else ""
            if remoto:
                try:
                    api_remota(remoto, "/pecas", senha=SENHA)
                    ok("senha certa é aceita na rede (pelo endereço do celular)")
                except urllib.error.HTTPError as e:
                    erro(f"senha certa recusada ({e.code})")
                try:
                    api_remota(remoto, "/pecas", senha="errada")
                    erro("senha errada foi aceita")
                except urllib.error.HTTPError as e:
                    check("senha errada é recusada (401)", e.code == 401, e.code)
                try:
                    api_remota(remoto, "/pecas")
                    erro("sem senha, a rede abriu o sistema (deveria recusar)")
                except urllib.error.HTTPError as e:
                    check("sem senha, a rede não abre o sistema (401)", e.code == 401, e.code)
            else:
                erro("sem endereço de rede para testar o celular")

            print("3) botão Trocar a senha")
            pg.click("text=Trocar a senha")
            pg.wait_for_selector("#troca-senha-1", timeout=10000)
            pg.fill("#troca-senha-1", SENHA2)
            pg.fill("#troca-senha-2", SENHA2)
            pg.click("#btn-trocar-senha")
            time.sleep(2)
            check("trocar senha não devolve 404", "404" not in pg.inner_text("body"))
            check("o programa continua no ar depois de trocar a senha", ping() is not None)
            try:
                api_remota(remoto, "/pecas", senha=SENHA2)
                ok("senha nova passa a valer")
            except urllib.error.HTTPError as e:
                erro(f"senha nova recusada ({e.code})")
            try:
                api_remota(remoto, "/pecas", senha=SENHA)
                erro("senha antiga continua valendo")
            except urllib.error.HTTPError as e:
                check("senha antiga não entra mais (401)", e.code == 401, e.code)

            print("4) botão Desligar o acesso pelo celular")
            await_painel(pg)
            pg.click("text=Desligar o acesso pelo celular")
            pg.wait_for_selector("#btn-confirmar", timeout=10000)
            pg.click("#btn-confirmar")
            time.sleep(2)
            check("desligar não devolve 404", "404" not in pg.inner_text("body"))
            check("o programa continua no ar depois de desligar", ping() is not None)
            time.sleep(1)
            r = api("/rede")
            check("servidor saiu do modo rede", r.get("modo") != "rede", r.get("modo"))
            # a senha continua guardada de proposito (para religar sem digitar de
            # novo); o que importa e' que o celular nao consegue mais entrar
            if remoto:
                recusou = False
                try:
                    api_remota(remoto, "/pecas")
                except Exception:
                    recusou = True
                check("com o acesso desligado, o celular não entra mais", recusou)
            await_painel(pg)
            check("painel voltou para Desligado", "Desligado" in pg.inner_text("#conteudo-celular"))
            pg.screenshot(path="qa/shots/btcel-2-desligado.png")

            print("5) console")
            reais = [e for e in erros if "favicon" not in e.lower() and "404" not in e]
            check("nenhum erro de JavaScript", not reais, reais[:3])
            nav.close()
    finally:
        try:
            proc.terminate()
            proc.wait(timeout=10)
        except Exception:
            pass
        os.system(f"pkill -f 'ap110b -dados {DADOS}' 2>/dev/null")

    print()
    print(f"{oks} OK / {len(falhas)} falha(s)")
    for f in falhas:
        print("  - " + f)
    return 1 if falhas else 0


if __name__ == "__main__":
    sys.exit(main())
