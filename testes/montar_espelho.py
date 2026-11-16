#!/usr/bin/env python3
"""Monta o zip-espelho do workspace (o que vai para /AutoPecas/workspace/projeto/).

Regras (decididas com o usuário):
  - entra: código (autopecas/), testes (qa/), instalador (installer/, sem payload),
    manual (manual/), dados de demonstração, documentos .md, manual em PDF,
    LEIA-ME e relatório da entrega ATUAL;
  - NÃO entra: banco de loja (autopecas.db*), binários compilados, .exe da entrega,
    caches (.cache, node_modules, __pycache__, payload), tmpwine, qa/shots? (entram),
    demo-dados com banco real.
"""
import os
import zipfile

RAIZ = "/home/user"
SAIDA = os.path.join(RAIZ, "AutoPecas-codigo-fonte.zip")
ENTREGA_ATUAL = "ENTREGA/1.0.11"

EXCLUIR_DIRS = {
    ".cache", ".config", ".local", "__pycache__", "node_modules", "payload",
    "tmpwine", "dist", ".git", ".venv",
}
EXCLUIR_ARQ = {
    "autopecas/autopecas",
    "autopecas.db", "autopecas.db-journal", "autopecas.db-wal", "autopecas.db-shm",
}
EXCLUIR_EXT = {".exe", ".db", ".db-wal", ".db-shm", ".db-journal", ".o", ".a", ".zip"}
INCLUIR_TOP = {
    "autopecas", "qa", "installer", "manual", "demo-dados",
    "BUGS-ENCONTRADOS.md", "CATALOGO-FORNECEDOR.md", "NOTA-FOTO-OCR.md",
    "SUGESTOES-ERP.md", "LEIA-ME-ESPELHO.md", "Manual-AutoPecas.pdf",
    ENTREGA_ATUAL + "/LEIA-ME-PRIMEIRO.txt",
    ENTREGA_ATUAL + "/RELATORIO-DE-TESTES.md",
    ENTREGA_ATUAL + "/SHA-256.txt",
    ENTREGA_ATUAL + "/COMO-CONFERIR.txt",
    ENTREGA_ATUAL + "/catalogo-exemplo.csv",
    ENTREGA_ATUAL + "/exemplos-xml",
}


def quer(nome_rel):
    topo = nome_rel.split("/")[0]
    return any(nome_rel == i or nome_rel.startswith(i + "/") for i in INCLUIR_TOP)


def main():
    arquivos = []
    for base, dirs, files in os.walk(RAIZ):
        dirs[:] = [d for d in dirs if d not in EXCLUIR_DIRS and not d.startswith(".")]
        for f in files:
            caminho = os.path.join(base, f)
            rel = os.path.relpath(caminho, RAIZ)
            if not quer(rel):
                continue
            if rel in EXCLUIR_ARQ or f in EXCLUIR_ARQ or os.path.splitext(f)[1].lower() in EXCLUIR_EXT:
                continue
            arquivos.append(rel)
    arquivos.sort()
    # remove duplicatas do banco de demonstração grande, mantém o resto
    with zipfile.ZipFile(SAIDA, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        for rel in arquivos:
            z.write(os.path.join(RAIZ, rel), rel)
    print(f"{SAIDA}: {os.path.getsize(SAIDA)} bytes, {len(arquivos)} arquivos")


if __name__ == "__main__":
    main()
