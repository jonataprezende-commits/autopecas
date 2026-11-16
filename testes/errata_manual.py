"""Gera uma pagina de errata (novidades da versao 1.0.1) e anexa ao manual.

O manual em PDF foi gerado com as telas da versao 1.0.0 (capturas de tela).
A tela de backup ganhou o campo "Segunda pasta de backup" e o menu Iniciar
ganhou o atalho "Encerrar AutoPecas". Como regerar as capturas exige o
programa rodando no Windows, esta pagina documenta as mudancas e e anexada
ao final do Manual-AutoPecas.pdf.
"""
import io
from pypdf import PdfReader, PdfWriter

# --- escrita manual de uma pagina PDF (Helvetica / WinAnsiEncoding) ---

def esc(txt: str) -> bytes:
    """Escapa texto para string PDF, codificando em cp1252 (WinAnsi)."""
    out = txt.encode("cp1252", "replace")
    return out.replace(b"\\", b"\\\\").replace(b"(", b"\\(").replace(b")", b"\\)")

def montar_pagina(linhas):
    """linhas: lista de (texto, tamanho, negrito, espaco_antes)"""
    y = 800
    partes = []
    for texto, tamanho, negrito, antes in linhas:
        y -= antes
        fonte = b"/F2" if negrito else b"/F1"
        partes.append(b"BT " + fonte + b" " + str(tamanho).encode() + b" Tf 60 " + str(y).encode() + b" Td (" + esc(texto) + b") Tj ET\n")
        y -= tamanho + 6
    conteudo = b"".join(partes)

    objetos = []
    objetos.append(b"<< /Type /Catalog /Pages 2 0 R >>")
    objetos.append(b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
    objetos.append(b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] "
                   b"/Resources << /Font << /F1 4 0 R /F2 5 0 R >> >> /Contents 6 0 R >>")
    objetos.append(b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
    objetos.append(b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
    objetos.append(b"<< /Length " + str(len(conteudo)).encode() + b" >>\nstream\n" + conteudo + b"endstream")

    saida = io.BytesIO()
    saida.write(b"%PDF-1.4\n")
    offsets = []
    for i, obj in enumerate(objetos, start=1):
        offsets.append(saida.tell())
        saida.write(str(i).encode() + b" 0 obj\n" + obj + b"\nendobj\n")
    inicio_xref = saida.tell()
    saida.write(b"xref\n0 " + str(len(objetos) + 1).encode() + b"\n")
    saida.write(b"0000000000 65535 f \n")
    for off in offsets:
        saida.write(("%010d 00000 n \n" % off).encode())
    saida.write(b"trailer\n<< /Size " + str(len(objetos) + 1).encode() + b" /Root 1 0 R >>\n")
    saida.write(b"startxref\n" + str(inicio_xref).encode() + b"\n%%EOF\n")
    return saida.getvalue()


LINHAS = [
    ("Novidades da versao 1.0.1", 20, True, 0),
    ("", 8, False, 0),

    ("Segunda pasta de backup (copia automatica)", 13, True, 10),
    ("Na tela Backup e Ajustes existe agora o campo \"Segunda pasta de backup\".", 11, False, 4),
    ("Preencha com o caminho de um pen drive ou da pasta do Google Drive da loja, por exemplo:", 11, False, 2),
    ("G:\\Meu Drive\\Backup AutoPecas", 11, False, 2),
    ("Depois de cada backup, o sistema copia o mesmo arquivo .db para essa pasta.", 11, False, 2),
    ("Se a pasta nao existir (pen drive desconectado, por exemplo), o backup principal", 11, False, 2),
    ("continua funcionando e aparece um aviso em vermelho na propria tela de Backup.", 11, False, 2),
    ("A tela tambem passou a lembrar a pasta extra configurada.", 11, False, 2),

    ("Atalho \"Encerrar AutoPecas\"", 13, True, 16),
    ("Alem do botao \"Encerrar o sistema\" na tela de Backup, o menu Iniciar agora tem o", 11, False, 4),
    ("atalho \"Encerrar AutoPecas\", que fecha o programa mesmo quando a janela do", 11, False, 2),
    ("navegador ja foi fechada.", 11, False, 2),

    ("Contas a receber de venda cancelada", 13, True, 16),
    ("Quando uma venda faturada que ja tinha sido paga e cancelada, a conta a receber", 11, False, 4),
    ("passa a aparecer como \"estornada\" (antes continuava como \"pago\"). Ela deixa de", 11, False, 2),
    ("entrar em qualquer soma de contas em aberto ou recebidas.", 11, False, 2),

    ("Backup mais seguro", 13, True, 16),
    ("O banco agora grava com sincronizacao completa (synchronous FULL) e o sistema", 11, False, 4),
    ("confere de hora em hora se o backup do dia foi feito.", 11, False, 2),
    ("As telas deste manual foram capturadas na versao 1.0.0: os textos acima", 9, False, 14),
    ("explicam as diferencas da versao 1.0.1.", 9, False, 1),
]

if __name__ == "__main__":
    errata = montar_pagina(LINHAS)
    leitor = PdfReader("/home/user/Manual-AutoPecas.pdf")
    escritor = PdfWriter()
    for pagina in leitor.pages:
        escritor.add_page(pagina)
    escritor.add_page(PdfReader(io.BytesIO(errata)).pages[0])
    with open("/home/user/Manual-AutoPecas.pdf", "wb") as f:
        escritor.write(f)
    print("paginas:", len(escritor.pages))
