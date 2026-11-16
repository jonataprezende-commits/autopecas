"""Gera a pagina de novidades da versao 1.0.2 e anexa ao Manual-AutoPecas.pdf.

O manual em PDF foi gerado com as telas da versao 1.0.0 e ja tem, no fim,
a pagina de novidades da 1.0.1. Regerar as capturas exigiria o programa
rodando no Windows, entao esta pagina documenta o que mudou na 1.0.2
(as 8 correcoes de defeitos) e como atualizar sem perder dados.

Uso:  python3 qa/errata_manual_102.py [entrada.pdf] [saida.pdf]
"""
import io
import sys

from pypdf import PdfReader, PdfWriter

ALTURA_INICIAL = 800
MARGEM_FINAL = 60


def esc(txt: str) -> bytes:
    """Escapa texto para string PDF, codificando em cp1252 (WinAnsiEncoding)."""
    out = txt.encode("cp1252", "replace")
    return out.replace(b"\\", b"\\\\").replace(b"(", b"\\(").replace(b")", b"\\)")


def montar_pagina(linhas):
    """linhas: lista de (texto, tamanho, negrito, espaco_antes) -> bytes de 1 pagina."""
    y = ALTURA_INICIAL
    partes = []
    for texto, tamanho, negrito, antes in linhas:
        y -= antes
        fonte = b"/F2" if negrito else b"/F1"
        partes.append(
            b"BT " + fonte + b" " + str(tamanho).encode() + b" Tf 60 "
            + str(y).encode() + b" Td (" + esc(texto) + b") Tj ET\n"
        )
        y -= tamanho + 6
    conteudo = b"".join(partes)

    objetos = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] "
        b"/Resources << /Font << /F1 4 0 R /F2 5 0 R >> >> /Contents 6 0 R >>",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>",
        b"<< /Length " + str(len(conteudo)).encode() + b" >>\nstream\n" + conteudo + b"endstream",
    ]

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


# --- conteudo -----------------------------------------------------------
# (texto, tamanho, negrito, espaco antes)
LINHAS = [
    ("Novidades da versão 1.0.2", 20, True, 0),
    ("", 6, False, 0),
    ("Esta versão não muda nenhuma tela e não muda o banco de dados: são correções de", 11, False, 10),
    ("defeitos encontrados numa revisão completa do programa. Seus dados continuam iguais.", 11, False, 2),
    ("Para atualizar, instale por cima da versão antiga (veja o último tópico).", 11, False, 2),

    ("1. Ficha de falta não entra mais duas vezes no estoque", 13, True, 18),
    ("Antes, se alguém clicasse em \"Receber\" duas vezes na mesma ficha de falta (ou recebesse", 11, False, 4),
    ("pela ficha e depois pelo pedido de compra), as peças entravam em dobro no estoque.", 11, False, 2),
    ("Agora o sistema recusa o segundo recebimento com a mensagem:", 11, False, 2),
    ("\"Esta ficha já foi recebida. O estoque não foi lançado de novo.\"", 11, False, 2),

    ("2. Ponto de milhar nos valores (o mais importante do dia a dia)", 13, True, 18),
    ("Antes, digitar 1.234 no preço era lido como R$ 1,23, e 10.000 virava R$ 10,00.", 11, False, 4),
    ("Agora o sistema entende o padrão brasileiro:", 11, False, 2),
    ("   1.234 = mil duzentos e trinta e quatro reais", 11, False, 2),
    ("   1.234,50 = mil duzentos e trinta e quatro reais e cinquenta centavos", 11, False, 2),
    ("   1234,50 = mil duzentos e trinta e quatro reais e cinquenta centavos", 11, False, 2),
    ("   0,50 = cinquenta centavos", 11, False, 2),
    ("Vale para preço de venda, custo, desconto, valor recebido e quantidade em estoque,", 11, False, 2),
    ("tanto digitando na tela quanto importando planilha ou XML. Confira a vírgula:", 11, False, 2),
    ("se houver vírgula, ela é o separador de centavos; o ponto sozinho é milhar.", 11, False, 2),

    ("3. Preço do XML do fornecedor", 13, True, 18),
    ("Muitos fornecedores mandam o XML da nota apenas com a tag vUnCom (valor unitário", 11, False, 4),
    ("comercial). Essa tag não era reconhecida e a peça entrava com preço R$ 0,00.", 11, False, 2),
    ("Agora vUnCom, vUnTrib e vUnitCom são lidas normalmente.", 11, False, 2),

    ("4. Contas a receber não desaparecem mais da lista", 13, True, 18),
    ("Algumas contas (por exemplo, as lançadas sem vínculo com uma venda) ficavam fora da", 11, False, 4),
    ("lista e do total em aberto, sem nenhum aviso. Agora todas aparecem e o total fecha.", 11, False, 2),

    ("5. Acentos nos relatórios em PDF", 13, True, 18),
    ("Todo relatório impresso em PDF saía com acento quebrado (\"óleo\" virava \"Ã³leo\").", 11, False, 4),
    ("Corrigido: os relatórios em PDF agora imprimem português correto.", 11, False, 2),

    ("6. Dias de atraso certos", 13, True, 18),
    ("Uma conta que venceu ontem aparecia como \"2 dias\" de atraso por causa da diferença", 11, False, 4),
    ("de fuso horário. Agora o cálculo é feito por data do calendário.", 11, False, 2),
    ("Vale para contas a receber, para os \"dias em aberto\" das fichas de falta e para", 11, False, 2),
    ("o relatório de recebíveis.", 11, False, 2),

    ("7. Correção interna no relatório de vendas", 13, True, 18),
    ("Vendas sem cliente cadastrado, e pedidos com item sem ficha de falta, podiam deixar", 11, False, 4),
    ("a tela de detalhe incompleta. Corrigido no servidor do programa.", 11, False, 2),

    ("Como atualizar para a 1.0.2 (não perde nada)", 13, True, 20),
    ("1. Feche o AutoPecas (menu Iniciar > Encerrar AutoPecas, ou o botão Encerrar o sistema).", 11, False, 4),
    ("2. Faça um backup: abra o programa e clique em Backup e Ajustes > Fazer backup agora.", 11, False, 2),
    ("   (Se já estiver fechado, copie a pasta AppData\\Roaming\\AutoPecas para um pen drive.)", 11, False, 2),
    ("3. Execute o AutoPecas-Setup-1.0.2.exe e instale por cima. O banco de dados é preservado.", 11, False, 2),
    ("4. Abra o programa: o título deve mostrar \"AutoPeças 1.0.2\".", 11, False, 2),

    ("As telas deste manual foram capturadas na versão 1.0.0: nenhuma tela mudou nas", 9, False, 14),
    ("versões 1.0.1 e 1.0.2, apenas campos e correções descritos nas páginas de novidades.", 9, False, 1),
]


def altura(item):
    texto, tamanho, _negrito, antes = item
    return antes + tamanho + 6


def blocos(linhas):
    """Agrupa as linhas por secao: cada titulo em negrito >=13pt abre um bloco novo."""
    grupos, atual = [], []
    for item in linhas:
        _texto, tamanho, negrito, _antes = item
        if negrito and tamanho >= 13 and atual:
            grupos.append(atual)
            atual = []
        atual.append(item)
    if atual:
        grupos.append(atual)
    return grupos


def paginar(linhas):
    """Quebra as linhas em paginas sem cortar uma secao no meio (quando couber)."""
    paginas, atual = [], []
    y = ALTURA_INICIAL
    for grupo in blocos(linhas):
        total = sum(altura(i) for i in grupo)
        if atual and y - total < MARGEM_FINAL:
            paginas.append(atual)
            atual, y = [], ALTURA_INICIAL
            grupo = [(t, sz, n, 0) if k == 0 else (t, sz, n, a)
                     for k, (t, sz, n, a) in enumerate(grupo)]
        for item in grupo:
            if y - altura(item) < MARGEM_FINAL and atual:  # secao maior que a folha
                paginas.append(atual)
                atual, y = [], ALTURA_INICIAL
                item = (item[0], item[1], item[2], 0)
            atual.append(item)
            y -= altura(item)
    if atual:
        paginas.append(atual)
    return paginas


def main():
    entrada = sys.argv[1] if len(sys.argv) > 1 else "/home/user/Manual-AutoPecas.pdf"
    saida = sys.argv[2] if len(sys.argv) > 2 else entrada

    leitor = PdfReader(entrada)
    escritor = PdfWriter()
    for pagina in leitor.pages:
        escritor.add_page(pagina)
    extras = paginar(LINHAS)
    for linhas_pagina in extras:
        escritor.add_page(PdfReader(io.BytesIO(montar_pagina(linhas_pagina))).pages[0])
    with open(saida, "wb") as f:
        escritor.write(f)
    print("paginas do manual de entrada:", len(leitor.pages))
    print("paginas de novidades 1.0.2 anexadas:", len(extras))
    print("total:", len(escritor.pages))


LINHAS_103 = [
    ("Novidades da versão 1.0.3 — Nota de compra por foto", 19, True, 0),
    ("", 6, False, 0),
    ("Agora dá para fotografar a nota do fornecedor e o sistema lança a compra no estoque.", 11, False, 10),
    ("O leitor de notas funciona sem internet e já vem dentro do programa — não precisa", 11, False, 2),
    ("instalar nada na máquina da loja.", 11, False, 2),

    ("Como usar (o dia a dia)", 13, True, 18),
    ("1. Menu Nota por Foto > arraste a foto da nota (ou clique e escolha o arquivo).", 11, False, 4),
    ("2. Clique em Ler a nota. Em 2 ou 3 segundos aparece a lista dos itens.", 11, False, 2),
    ("3. Confira a lista. Linha em amarelo é a que o sistema NÃO conseguiu conferir.", 11, False, 2),
    ("   Pode corrigir código, descrição, quantidade, unitário e total na própria tela.", 11, False, 2),
    ("4. Escolha o fornecedor (opcional) e clique em Importar os itens marcados.", 11, False, 2),
    ("O estoque só é lançado depois desse clique: nada entra sozinho.", 11, False, 2),

    ("O que o sistema confere sozinho", 13, True, 18),
    ("Cada linha só é marcada como conferida quando quantidade x unitário é igual ao total", 11, False, 4),
    ("daquela linha. No fim, a soma dos itens é comparada com o TOTAL impresso na nota;", 11, False, 2),
    ("se não fechar, aparece a diferença em reais e a nota fica marcada como", 11, False, 2),
    ("CONFIRA ANTES DE IMPORTAR. Isso pega a maioria dos erros de leitura.", 11, False, 2),

    ("Como tirar a foto para o leitor acertar", 13, True, 18),
    ("Apoie o celular na mesa (foto de cima, sem tremer).", 11, False, 4),
    ("Enquadre a folha inteira, incluindo o valor total.", 11, False, 2),
    ("Luz de frente, sem sombra da mão em cima da nota.", 11, False, 2),
    ("Evite foto muito de lado, papel dobrado ou amassado.", 11, False, 2),
    ("Nota escrita à mão NÃO é lida por foto: nesse caso digite a nota ou peça o XML", 11, False, 2),
    ("ao fornecedor (menu Importar XML).", 11, False, 2),

    ("O que foi medido nos testes", 13, True, 18),
    ("Nota impressa fotografada com o celular apoiado: 8 de 8 itens lidos, valores e total", 11, False, 4),
    ("certos, em cerca de 2 segundos.", 11, False, 2),
    ("Foto tirada na mão, com sombra de um lado: também 8 de 8.", 11, False, 2),
    ("Foto torta, escura e desfocada: o sistema não entrega a nota como conferida —", 11, False, 2),
    ("ele avisa que não conseguiu ler e pede outra foto.", 11, False, 2),

    ("Outras mudanças desta versão", 13, True, 18),
    ("O instalador agora tem 20 MB (antes 7 MB) porque carrega o leitor de notas dentro.", 11, False, 4),
    ("A primeira leitura de nota demora alguns segundos a mais: o programa prepara o", 11, False, 2),
    ("leitor na pasta do usuário. Depois disso fica rápido.", 11, False, 2),
    ("Ao atualizar, o programa é instalado por cima; o banco de dados continua o mesmo.", 11, False, 2),
    ("", 6, False, 4),
    ("As telas deste manual continuam as da 1.0.0: esta página descreve a tela nova.", 9, False, 8),
]


def anexar(entrada, saida, paginas):
    leitor = PdfReader(entrada)
    escritor = PdfWriter()
    for pagina in leitor.pages:
        escritor.add_page(pagina)
    extras = []
    for linhas in paginas:
        for grupo in paginar(linhas):
            extras.append(grupo)
    for grupo in extras:
        escritor.add_page(PdfReader(io.BytesIO(montar_pagina(grupo))).pages[0])
    with open(saida, "wb") as f:
        escritor.write(f)
    print("paginas de entrada:", len(leitor.pages), "| anexadas:", len(extras), "| total:", len(escritor.pages))


if __name__ == "__main__" and len(sys.argv) > 3:
    anexar(sys.argv[1], sys.argv[2], [LINHAS_103])
    sys.exit(0)


if __name__ == "__main__":
    main()
