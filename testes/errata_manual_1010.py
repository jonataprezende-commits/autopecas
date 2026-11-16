"""Paginas de novidades da versao 1.0.10 (revisao geral: defeitos corrigidos)
anexadas ao Manual-AutoPecas.pdf.

A 1.0.10 nao muda o jeito de usar o programa: e' uma revisao de dentro para
fora (todo o codigo relido, testes automaticos refeitos) mais alguns avisos
novos na tela e uma correcao no carrinho da bancada.

Uso:  python3 qa/errata_manual_1010.py [entrada.pdf] [saida.pdf]
"""
import sys

from errata_manual_102 import anexar

LINHAS_1010_A = [
    ("Novidades da versão 1.0.10 — revisão geral do programa", 18, True, 0),
    ("", 6, False, 0),
    ("Esta versão não muda o seu jeito de trabalhar. O programa inteiro foi", 11, False, 10),
    ("relido de ponta a ponta, com testes automáticos refeitos do zero, para", 11, False, 2),
    ("achar defeitos escondidos. Foram encontrados e corrigidos 20 problemas.", 11, False, 2),
    ("Os que você pode notar no dia a dia estão nas páginas seguintes.", 11, False, 2),

    ("1. Correções que aparecem na tela", 13, True, 18),
    ("   a) Carrinho da bancada: ao mudar a quantidade (ou o preço) de um item", 11, False, 4),
    ("      e sair do campo, o total volta certo. Antes, em alguns casos, a", 11, False, 2),
    ("      linha continuava mostrando o total antigo até recarregar a tela.", 11, False, 2),
    ("   b) Venda que deixa peça sem estoque: ao finalizar, aparece o aviso", 11, False, 2),
    ("      “Venda gravada. Atenção: … ficou sem estoque — dê entrada na compra", 11, False, 2),
    ("      quando chegar”. A venda é gravada (o sistema permite vender sem", 11, False, 2),
    ("      saldo), mas você fica sabendo na hora qual peça está negativa.", 11, False, 2),
    ("   c) Campo numérico errado em Configurações: se você apagar o campo ou", 11, False, 2),
    ("      digitar um valor fora da faixa, o sistema avisa qual campo é e não", 11, False, 2),
    ("      grava nada quebrado (antes podia gravar vazio).", 11, False, 2),
    ("   d) Nota do fornecedor com boleto que não fecha: quando a soma dos", 11, False, 2),
    ("      boletos do XML é diferente do total da nota, o painel mostra o aviso", 11, False, 2),
    ("      “Confira antes de importar” já na análise, com a soma dos boletos.", 11, False, 2),
    ("   e) Ficha marcada como recebida não pode mais ser apagada: o sistema", 11, False, 2),
    ("      responde “esta ficha já foi recebida” em vez de apagar o histórico.", 11, False, 2),
]

LINHAS_1010_B = [
    ("2. Correções que protegem os seus dados", 13, True, 18),
    ("   a) Restaurar backup de outro computador não apaga mais a senha de", 11, False, 4),
    ("      acesso pelo celular (antes a senha da rede voltava para vazia).", 11, False, 2),
    ("   b) Antes de restaurar, o sistema faz uma cópia de segurança. Se essa", 11, False, 2),
    ("      cópia falhar (disco cheio, por exemplo), a restauração é abortada em", 11, False, 2),
    ("      vez de seguir sem rede de proteção.", 11, False, 2),
    ("   c) A faxina de backups apaga pela data que está no nome do arquivo,", 11, False, 2),
    ("      não pela data do disco. Nome de arquivo copiado ou movido não faz", 11, False, 2),
    ("      mais o sistema apagar a cópia errada.", 11, False, 2),
    ("   d) Trocar a senha do sistema derruba na hora quem estava conectado", 11, False, 2),
    ("      pelo celular ou por outra máquina.", 11, False, 2),

    ("3. Correções de contas e de estoque", 13, True, 18),
    ("   a) Venda faturada com vencimento em dias: a data passou a ser contada", 11, False, 4),
    ("      a partir da data da venda (antes podia sair com um dia de erro).", 11, False, 2),
    ("   b) Nota do fornecedor x contas a pagar: quando a soma dos boletos do", 11, False, 2),
    ("      XML não bate com o total da nota, o sistema ajusta a última parcela", 11, False, 2),
    ("      se a diferença for de até 4 centavos (arredondamento) e, se for", 11, False, 2),
    ("      maior, marca ATENÇÃO na conta e avisa na tela. Nunca lança duas", 11, False, 2),
    ("      vezes a mesma nota.", 11, False, 2),
    ("   c) Preço, custo, mínimo e estoque não aceitam mais valor negativo;", 11, False, 2),
    ("      quantidade de item de venda e movimento de estoque têm limite.", 11, False, 2),
    ("   d) Relatório de valor do estoque arredonda em centavos (não sobra mais", 11, False, 2),
    ("      1 centavo de diferença na soma).", 11, False, 2),
    ("   e) CNPJ do fornecedor vai para a coluna “documento” da conta a pagar —", 11, False, 2),
    ("      nunca mais pode virar o número da nota por engano.", 11, False, 2),
]

LINHAS_1010_C = [
    ("4. Testes feitos nesta versão (dentro do programa)", 13, True, 18),
    ("   - 123 testes automáticos do próprio programa (todos passaram);", 10, False, 4),
    ("   - 120 verificações de API, rodadas duas vezes seguidas no mesmo banco;", 10, False, 2),
    ("   - 9 suítes de tela no navegador (bancada, peças, fichas, estoque,", 10, False, 2),
    ("     contas, catálogo, nota, celular, Plano B): 0 falhas;", 10, False, 2),
    ("   - o instalador gerado foi instalado e testado no Windows simulado", 10, False, 2),
    ("     (108 verificações, 0 falhas).", 10, False, 2),
    ("", 6, False, 6),

    ("Como atualizar para a 1.0.10 (não perde nada)", 13, True, 18),
    ("   1. Encerrar o AutoPeças e fazer um backup (Backup e Ajustes -> Fazer", 11, False, 10),
    ("      backup agora).", 11, False, 2),
    ("   2. Rodar o AutoPecas-Setup-1.0.10.exe por cima do que já está", 11, False, 2),
    ("      instalado. O banco é convertido sozinho e o sistema guarda antes uma", 11, False, 2),
    ("      cópia “antes-de-migrar-…”.", 11, False, 2),
    ("   3. Abrir o sistema: o rodapé do menu mostra v1.0.10.", 11, False, 2),
    ("   4. Não é preciso reconfigurar nada: loja, senha, celular, catálogo e", 11, False, 2),
    ("      backups continuam como estavam.", 11, False, 2),
    ("", 6, False, 6),
    ("Se alguma coisa parecer estranha depois de atualizar, faça um backup e", 11, False, 8),
    ("mande a descrição do que aconteceu: todo defeito relatado entra na próxima", 11, False, 2),
    ("revisão.", 11, False, 2),
]


def main():
    entrada = sys.argv[1] if len(sys.argv) > 1 else "/home/user/Manual-AutoPecas.pdf"
    saida = sys.argv[2] if len(sys.argv) > 2 else entrada
    anexar(entrada, saida, [LINHAS_1010_A, LINHAS_1010_B, LINHAS_1010_C])


if __name__ == "__main__":
    main()
