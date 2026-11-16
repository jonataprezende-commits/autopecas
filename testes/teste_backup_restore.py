"""Criterio 5 completo: backup de hoje + restauracao em 'computador limpo'."""
import json, os, shutil, subprocess, sys, time, urllib.request, urllib.parse

B = os.environ.get("API_BASE", "http://127.0.0.1:8790/api")
WINE = "/home/user/.cache/wineprefix"
DADOS = f"{WINE}/drive_c/users/user/AppData/Roaming/AutoPecas"
EXE = f"{WINE}/drive_c/users/user/AppData/Local/Programs/AutoPecas/AutoPecas.exe"
PENDRIVE = "/home/user/qa/pendrive"

def req(path, data=None, method=None):
    body = json.dumps(data).encode() if data is not None else None
    h = {"Content-Type": "application/json"} if body else {}
    r = urllib.request.Request(B + path, data=body, headers=h, method=method or ("POST" if body else "GET"))
    with urllib.request.urlopen(r, timeout=30) as resp:
        return json.loads(resp.read().decode())

def esperar_app(seg=40):
    for _ in range(seg):
        try:
            req("/ping")
            return True
        except Exception:
            time.sleep(1)
    return False

def estado():
    pecas = req("/pecas?limite=500")
    return {
        "pecas": pecas["total"],
        "pecas_detalhe": sorted((p["codigo"], p["descricao"], p["estoque"], p["preco_centavos"]) for p in pecas["itens"]),
        "vendas": len(req("/vendas")["itens"]),
        "fichas": len(req("/fichas?status=todas")["itens"]),
        "clientes": len(req("/clientes")["itens"]),
        "fornecedores": len(req("/fornecedores")["itens"]),
        "receber": len(req("/recebiveis?status=todos")["itens"]),
        "estoque_custo": req("/dashboard")["estoque_valor_centavos"],
    }

# garante o sistema aberto (maquina da loja)
try:
    req("/ping")
except Exception:
    env0 = dict(os.environ, WINEPREFIX=WINE, WINEDEBUG="-all", DISPLAY=":80")
    subprocess.Popen(["/usr/lib/wine/wine64", EXE, "-sem-navegador", "-porta", "8790"], env=env0,
                     cwd=os.path.dirname(EXE), stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    assert esperar_app(), "o sistema nao subiu"
    # recria os dados de teste (a maquina da loja tem movimento)
    subprocess.run(["python3", "/home/user/qa/demo_dados.py"],
                   env=dict(os.environ, API_BASE=B), timeout=300, capture_output=True)
    print("   (dados de demonstracao recriados)")

print("=== 1. estado atual (maquina da loja) ===")
antes = estado()
print("  ", {k: v for k, v in antes.items() if k != "pecas_detalhe"})

print("=== 2. backup de hoje (1 clique) ===")
bk = req("/backup", {})
print("   arquivo:", bk["arquivo"], "| tamanho:", round(bk["tamanho_bytes"] / 1024), "KB")
os.makedirs(PENDRIVE, exist_ok=True)
origem = os.path.join(DADOS, "backups", os.path.basename(bk["arquivo"]))
destino = os.path.join(PENDRIVE, "backup-da-loja.db")
shutil.copy(origem, destino)
print("   copiado para o 'pen drive':", destino, round(os.path.getsize(destino)/1024), "KB")

print("=== 3. simulando computador limpo (apaga TODA a pasta de dados) ===")
subprocess.run(["pkill", "-f", "AutoPecas.exe"], capture_output=True)
time.sleep(3)
shutil.rmtree(DADOS, ignore_errors=True)
print("   pasta de dados existe?", os.path.exists(DADOS))

print("=== 4. reinstalando o programa no 'computador novo' (instalador silencioso) ===")
env = dict(os.environ, WINEPREFIX=WINE, WINEDEBUG="-all", DISPLAY=":80")
subprocess.run(["/usr/lib/wine/wine64", "/home/user/dist/AutoPecas-Setup-1.0.0.exe", "/S"], env=env, timeout=180)
print("   programa instalado:", os.path.exists(EXE))

print("=== 5. abrindo o sistema limpo (banco novo, vazio) ===")
proc = subprocess.Popen(["/usr/lib/wine/wine64", EXE, "-sem-navegador", "-porta", "8790"], env=env, cwd=os.path.dirname(EXE),
                        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
assert esperar_app(), "o sistema nao subiu"
vazio = estado()
print("   estado do computador limpo:", {k: v for k, v in vazio.items() if k != "pecas_detalhe"})
assert vazio["pecas"] == 0 and vazio["vendas"] == 0, "o banco novo deveria estar vazio"

print("=== 6. restaurando o backup (enviando o arquivo, como no manual) ===")
out = subprocess.run(["curl", "-s", "-X", "POST", "-F", f"arquivo=@{destino}", B + "/restaurar"],
                     capture_output=True, timeout=120).stdout
print("   resposta:", out.decode()[:200])
depois = estado()
print("   estado depois da restauracao:", {k: v for k, v in depois.items() if k != "pecas_detalhe"})

print("=== 7. comparacao final ===")
igual = antes == depois
print("   dados identicos ao original?", "SIM ✅" if igual else "NAO ❌")
if not igual:
    for k in antes:
        if antes[k] != depois[k]:
            print("   diferenca em", k, "|", str(antes[k])[:120], "!=", str(depois[k])[:120])
    sys.exit(1)
print("\nCRITERIO 5 OK: backup de hoje + restauracao em computador limpo deixou tudo igual.")
proc.terminate()
