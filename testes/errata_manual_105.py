"""Gera a pagina de novidades da versao 1.0.5 (criterios tecnicos + TecDoc) e
anexa ao Manual-AutoPecas.pdf.

Uso:  python3 qa/errata_manual_105.py [entrada.pdf] [saida.pdf]
"""
import sys

from errata_manual_102 import anexar

LINHAS_105 = [
    ("Novidades da versão 1.0.5 — Critérios técnicos e consulta ao TecDoc", 18, True, 0),
    ("", 6, False, 0),
    ("Duas melhorias pequenas que resolvem o caso difícil do balcão: quando o cliente",
     11, False, 10),
    ("não sabe o código e quando a peça não é do seu fornecedor de sempre.", 11, False, 2),

    ("1. Critérios técnicos no item do catálogo", 13, True, 18),
    ("Cada peça do catálogo ganhou um campo de critérios: rosca, número de dentes,",
     11, False, 4),
    ("altura, largura, perfil, encaixe, quantidade de pastilhas e o que mais o",
     11, False, 2),
    ("fornecedor informar. Serve para duas coisas:", 11, False, 2),
    ("   - achar a peça sem saber o código: buscar “120 dentes” ou “rosca 3/4”",
     11, False, 2),
    ("     já devolve a correia ou o filtro certo;", 11, False, 2),
    ("   - conferir se a peça de outra marca é mesmo equivalente (mesma rosca,",
     11, False, 2),
    ("     mesma medida), que é o que evita troca e devolução.", 11, False, 2),
    ("Na importação da lista do fornecedor, os critérios podem vir em uma coluna",
     11, False, 4),
    ("chamada “critérios”, “critérios técnicos”, “especificações”, “atributos”,",
     11, False, 2),
    ("“rosca” ou “dentes”. Se o texto tiver “;” no meio, o programa entende que é",
     11, False, 2),
    ("tudo um campo só e não desalinha as colunas seguintes.", 11, False, 2),
    ("A ficha de falta também leva os critérios, para o fornecedor separar a peça",
     11, False, 2),
    ("certa sem ligar de volta para a loja.", 11, False, 2),

    ("2. Botão “TecDoc” na tela do catálogo", 13, True, 18),
    ("O TecDoc Catalogue é o catálogo padrão do mercado. O plano Brasil Gratuito",
     11, False, 4),
    ("dá 1 ano de consulta (1 usuário, sem busca por placa e chassi). Agora cada",
     11, False, 2),
    ("peça do catálogo tem o botão “TecDoc”: ele mostra os dados da peça prontos",
     11, False, 2),
    ("para copiar (código, marca, descrição, aplicação, medida, critérios e",
     11, False, 2),
    ("equivalentes) e abre o portal oficial em outra aba. O código já vai copiado:",
     11, False, 2),
    ("é só dar Ctrl+V na busca do TecDoc.", 11, False, 2),
    ("IMPORTANTE — limites da licença gratuita: ela permite CONSULTAR, não copiar",
     11, False, 4),
    ("os dados para outro sistema (o próprio contrato do TecDoc diz isso). Por isso",
     11, False, 2),
    ("o AutoPeças não importa nada do TecDoc: você consulta lá e o que quiser",
     11, False, 2),
    ("guardar, digita ou importa da sua lista de fornecedor. Se um dia a loja",
     11, False, 2),
    ("contratar a alimentação de dados licenciada, o conector pode ser feito —",
     11, False, 2),
    ("o contato é vendasbrasil@tecalliance.net (TecAlliance do Brasil).", 11, False, 2),

    ("Como atualizar para a 1.0.5 (não perde nada)", 13, True, 18),
    ("1. Encerrar o AutoPeças e fazer backup (Backup e Ajustes > Fazer backup agora).",
     11, False, 4),
    ("2. Rodar o AutoPecas-Setup-1.0.5.exe por cima. O banco é migrado sozinho",
     11, False, 2),
    ("   (entra a coluna de critérios do catálogo; nenhum dado é apagado).", 11, False, 2),
    ("3. Abrir: o rodapé do menu mostra v1.0.5. As peças do catálogo já cadastradas",
     11, False, 2),
    ("   continuam lá — só o campo de critérios vem vazio, para preencher ou",
     11, False, 2),
    ("   reimportar a lista com a coluna nova.", 11, False, 2),
    ("", 6, False, 4),
    ("As telas deste manual continuam as da 1.0.0; as telas Nota por Foto e", 9, False, 8),
    ("Catálogo estão descritas nas páginas de novidades das versões 1.0.3 a 1.0.5.", 9, False, 1),
]


def main():
    entrada = sys.argv[1] if len(sys.argv) > 1 else "/home/user/Manual-AutoPecas.pdf"
    saida = sys.argv[2] if len(sys.argv) > 2 else entrada
    anexar(entrada, saida, [LINHAS_105])


if __name__ == "__main__":
    main()
