"""Gera a pagina de novidades da versao 1.0.6 e anexa ao Manual-AutoPecas.pdf.

Esta versao NAO tem integracao com o TecDoc (decisao do usuario): o catalogo e
montado com o que a loja ja tem e com a lista do fornecedor.

Uso:  python3 qa/errata_manual_106.py [entrada.pdf] [saida.pdf]
"""
import io
import sys

from pypdf import PdfReader, PdfWriter

from errata_manual_102 import anexar

LINHAS_106 = [
    ("Novidades da versão 1.0.6 — Catálogo que se monta sozinho", 18, True, 0),
    ("", 6, False, 0),
    ("Se a loja não tem planilha nenhuma do fornecedor, o catálogo não precisa começar",
     11, False, 10),
    ("vazio nem ser digitado peça por peça: ele se monta com o que já está dentro do",
     11, False, 2),
    ("sistema.", 11, False, 2),

    ("Botão “Montar com o que já tenho” (tela Catálogo)", 13, True, 18),
    ("Junta duas fontes que a loja já alimenta sem perceber:", 11, False, 4),
    ("   • as PEÇAS CADASTRADAS no estoque — cada uma vira item de catálogo, levando",
     11, False, 2),
    ("     junto a aplicação (veículos) e os códigos equivalentes que ela já tem;", 11, False, 2),
    ("   • as FICHAS DE FALTA feitas pelo botão “Ficha” — elas guardam o código entre",
     11, False, 2),
    ("     colchetes e o veículo que o cliente informou, e voltam para o catálogo.", 11, False, 2),
    ("Rodar de novo só atualiza o que existe, não duplica, e não lança nada no estoque.", 11, False, 2),

    ("Critérios técnicos no item de catálogo", 13, True, 18),
    ("Cada peça do catálogo tem um campo de critérios: rosca, dentes, altura, largura,",
     11, False, 4),
    ("perfil, encaixe. Serve para achar a peça sem saber o código (“120 dentes”,", 11, False, 2),
    ("“rosca 3/4”) e para conferir se a de outra marca é mesmo equivalente.", 11, False, 2),
    ("Na planilha do fornecedor, essa coluna pode vir como “critérios”, “critérios",
     11, False, 2),
    ("técnicos”, “especificações”, “atributos”, “rosca” ou “dentes”. Se o texto tiver",
     11, False, 2),
    ("“;” no meio, o programa entende que é um campo só e não desalinha a linha.", 11, False, 2),

    ("Catálogo grande (lista completa do fornecedor)", 13, True, 18),
    ("A importação passou a ler o arquivo em fluxo, sem limite de 64 MB: uma lista",
     11, False, 4),
    ("de 200 mil linhas entra em cerca de 1 minuto, direto pela tela. A busca é",
     11, False, 2),
    ("instantânea mesmo com o catálogo cheio (a resposta sai em poucos milésimos de",
     11, False, 2),
    ("segundo), inclusive procurando um pedaço do código no meio (“712” acha", 11, False, 2),
    ("“W712/90”).", 11, False, 2),

    ("Sobre o TecDoc (importante)", 13, True, 18),
    ("Esta versão NÃO tem botão nem integração com o TecDoc. A licença do TecDoc",
     11, False, 4),
    ("Catalogue (inclusive o plano gratuito de 1 ano) permite consultar o catálogo no",
     11, False, 2),
    ("site/app, mas não copiar os dados para outro sistema — por isso o AutoPeças não",
     11, False, 2),
    ("importa nada de lá. O catálogo do programa é alimentado pela lista do fornecedor",
     11, False, 2),
    ("e pelo que a própria loja registra (botão “Montar com o que já tenho”, ficha de",
     11, False, 2),
    ("falta e cadastro de peça).", 11, False, 2),

    ("Como atualizar para a 1.0.6 (não perde nada)", 13, True, 18),
    ("1. Encerrar o AutoPeças e fazer backup (Backup e Ajustes > Fazer backup agora).",
     11, False, 4),
    ("2. Rodar o AutoPecas-Setup-1.0.6.exe por cima. O banco é migrado sozinho.", 11, False, 2),
    ("3. Abrir: o rodapé do menu mostra v1.0.6.", 11, False, 2),
    ("4. Tela Catálogo > “Montar com o que já tenho” para começar o catálogo.", 11, False, 2),
    ("", 6, False, 4),
    ("As telas deste manual continuam as da 1.0.0; as telas Nota por Foto e Catálogo", 9, False, 8),
    ("estão descritas nas páginas de novidades das versões 1.0.3 a 1.0.6.", 9, False, 1),
]


def remover_ultimas(entrada, saida, quantas):
    """Recorta as ultimas paginas (as da versao anterior) e grava o PDF base."""
    leitor = PdfReader(entrada)
    escritor = PdfWriter()
    total = len(leitor.pages)
    for i, pagina in enumerate(leitor.pages):
        if i >= total - quantas:
            break
        escritor.add_page(pagina)
    with open(saida, "wb") as f:
        escritor.write(f)
    print(f"base: {total} paginas -> {total - quantas}")


def main():
    entrada = sys.argv[1] if len(sys.argv) > 1 else "/home/user/Manual-AutoPecas.pdf"
    saida = sys.argv[2] if len(sys.argv) > 2 else entrada
    base = "/tmp/manual-base-104.pdf"
    remover_ultimas(entrada, base, 2)   # tira as 2 paginas da 1.0.5
    anexar(base, saida, [LINHAS_106])


if __name__ == "__main__":
    main()
