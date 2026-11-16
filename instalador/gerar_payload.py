r"""Compacta o programa e o manual para dentro do instalador (payload/).

Uso (na pasta instalador/):
    python gerar_payload.py
    python gerar_payload.py --programa ..\dist\AutoPecas.exe --manual ..\manual\Manual-AutoPecas.pdf

O deflate e "cru" (sem cabecalho zlib/gzip) porque o instalador Go usa
compress/flate para descomprimir.
"""
import argparse
import os
import zlib

AQUI = os.path.dirname(os.path.abspath(__file__))


def comprimir(origem, destino):
    with open(origem, "rb") as f:
        dados = f.read()
    c = zlib.compressobj(9, zlib.DEFLATED, -15)  # deflate cru (flate do Go)
    saida = c.compress(dados) + c.flush()
    os.makedirs(os.path.dirname(destino), exist_ok=True)
    with open(destino, "wb") as f:
        f.write(saida)
    print(f"{destino}: {len(dados)/1024/1024:.1f} MB -> {len(saida)/1024/1024:.1f} MB")


def main():
    padrao_programa = os.path.join(AQUI, "..", "dist", "AutoPecas.exe")
    padrao_manual = os.path.join(AQUI, "..", "manual", "Manual-AutoPecas.pdf")

    p = argparse.ArgumentParser(description="Gera o payload do instalador.")
    p.add_argument("--programa", default=padrao_programa)
    p.add_argument("--manual", default=padrao_manual)
    p.add_argument("--saida", default=os.path.join(AQUI, "payload"))
    a = p.parse_args()

    for caminho in (a.programa, a.manual):
        if not os.path.isfile(caminho):
            raise SystemExit(f"arquivo nao encontrado: {caminho}")

    comprimir(a.programa, os.path.join(a.saida, "programa.deflate"))
    comprimir(a.manual, os.path.join(a.saida, "manual.deflate"))


if __name__ == "__main__":
    main()
