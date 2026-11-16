"""Gera a pagina de novidades da versao 1.0.4 (catalogo do fornecedor) e anexa
ao Manual-AutoPecas.pdf, que ja traz as paginas da 1.0.1, 1.0.2 e 1.0.3.

Uso:  python3 qa/errata_manual_104.py [entrada.pdf] [saida.pdf]
"""
import sys

from errata_manual_102 import anexar

# (texto, tamanho, negrito, espaco antes)
LINHAS_104 = [
    ("Novidades da versão 1.0.4 — Catálogo do fornecedor (fim do catálogo de papel)", 18, True, 0),
    ("", 6, False, 0),
    ("O catálogo de papel saiu do balcão. Agora o sistema guarda a lista do fornecedor e", 11, False, 10),
    ("responde na hora: “tem essa peça?”, “para que carro serve?”, “qual é o código?”,", 11, False, 2),
    ("“eu tenho no estoque?”. É a tela Catálogo, no menu da esquerda (atalho F5).", 11, False, 2),

    ("O que a tela faz", 13, True, 18),
    ("Busca rápida do balcão: um campo só. Digite o carro (“gol 2012”), o código", 11, False, 4),
    ("(“W712/90”) ou um equivalente de outra marca (“PSL560”) e o sistema mostra:", 11, False, 2),
    ("   - o que já está na prateleira, com a quantidade em estoque;", 11, False, 2),
    ("   - o que só existe no catálogo, marcado como “não tenho”, com o código.", 11, False, 2),
    ("Aceita o ano em faixa: a aplicação “08/16” responde para 2012 sem ninguém digitar", 11, False, 2),
    ("a faixa inteira. O que casa melhor com o ano aparece primeiro.", 11, False, 2),

    ("Como cadastrar as peças do catálogo", 13, True, 18),
    ("Opção 1 — importar a lista do fornecedor (o jeito rápido):", 11, False, 4),
    ("   Catálogo > Importar lista do fornecedor > escolha o arquivo .csv", 11, False, 2),
    ("   Primeira linha com os títulos:", 11, False, 2),
    ("   codigo;descricao;aplicacao;medida;marca;custo;equivalentes", 10, False, 2),
    ("   Pode ser ponto e vírgula, vírgula ou tabulação. Campo com “;” dentro", 11, False, 2),
    ("   deve ir entre aspas. Planilha salva como CSV pela Excel serve.", 11, False, 2),
    ("   Na pasta de exemplos vai o arquivo catalogo-exemplo.csv para copiar o modelo.", 11, False, 2),
    ("Opção 2 — cadastrar uma peça por vez: Catálogo > + Peça no catálogo.", 11, False, 4),
    ("Reimportar a mesma lista não duplica nada: o que já existe é atualizado.", 11, False, 2),

    ("Ficha de falta direto do catálogo", 13, True, 18),
    ("Achou a peça no catálogo e não tem no estoque? Clique em Ficha. Ela entra na", 11, False, 4),
    ("tela Fichas de Falta já com o código, a descrição e a aplicação preenchidos,", 11, False, 2),
    ("com o nome e o telefone do cliente. Depois é só pedir ao fornecedor.", 11, False, 2),

    ("O que o catálogo NÃO faz", 13, True, 18),
    ("O catálogo não cria peça no estoque e não lança entrada nem saída. Quem mexe no", 11, False, 4),
    ("estoque continua sendo a importação de XML, a nota por foto e a tela Estoque.", 11, False, 2),
    ("É uma lista de consulta: serve para responder o cliente, não para movimentar peça.", 11, False, 2),

    ("Aplicação nas peças que você já tem", 13, True, 18),
    ("A tela de cadastro de peça ganhou o campo Aplicação (veículos compatíveis), e o", 11, False, 4),
    ("XML do fornecedor que traz a tag de aplicação agora guarda essa informação em vez", 11, False, 2),
    ("de descartá-la. Com isso a busca por carro acha também o que está no estoque.", 11, False, 2),

    ("Por que não fotografar o catálogo", 13, True, 18),
    ("Foi medido antes de decidir: foto boa de uma página de catálogo acertou 10 dos 20", 11, False, 4),
    ("códigos e a foto de celular acertou 5 de 20 (códigos viraram “Pa-SO11”, “MEDIDA”", 11, False, 2),
    ("foi lida como “MEDINA”). Código errado no balcão é peça errada na mão do cliente,", 11, False, 2),
    ("então o caminho certo é a lista digital do fornecedor, não a foto do papel.", 11, False, 2),
    ("O texto lido de arquivo (CSV ou XML) acertou 20 de 20 nos mesmos testes.", 11, False, 2),

    ("Como atualizar para a 1.0.4 (não perde nada)", 13, True, 18),
    ("1. Feche o AutoPecas (menu Iniciar > Encerrar AutoPecas, ou o botão Encerrar o sistema).", 11, False, 4),
    ("2. Faça um backup: Backup e Ajustes > Fazer backup agora.", 11, False, 2),
    ("3. Execute o AutoPecas-Setup-1.0.4.exe e instale por cima. O banco é preservado e", 11, False, 2),
    ("   é atualizado sozinho na primeira abertura (entra a tabela do catálogo).", 11, False, 2),
    ("4. Abra o programa: o rodapé do menu deve mostrar v1.0.4.", 11, False, 2),
    ("", 6, False, 4),
    ("As telas deste manual continuam as da 1.0.0: as telas Nota por Foto e Catálogo", 9, False, 8),
    ("estão descritas nas páginas de novidades desta versão e da anterior.", 9, False, 1),
]


def main():
    entrada = sys.argv[1] if len(sys.argv) > 1 else "/home/user/Manual-AutoPecas.pdf"
    saida = sys.argv[2] if len(sys.argv) > 2 else entrada
    anexar(entrada, saida, [LINHAS_104])


if __name__ == "__main__":
    main()
