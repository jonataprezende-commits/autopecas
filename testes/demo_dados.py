"""Popula o sistema com dados de demonstracao (para o manual e para as capturas de tela)."""
import json, os, random, subprocess, urllib.request, urllib.parse, time

B = os.environ.get("API_BASE", "http://127.0.0.1:8902/api")
random.seed(7)

def req(path, data=None, method=None):
    body = json.dumps(data).encode() if data is not None else None
    headers = {"Content-Type": "application/json"} if body else {}
    r = urllib.request.Request(B + path, data=body, headers=headers, method=method or ("POST" if body else "GET"))
    with urllib.request.urlopen(r) as resp:
        return json.loads(resp.read().decode())

def upload(path, arq, opts=None):
    args = ["curl", "-s", "-X", "POST", "-F", f"arquivo=@{arq}"]
    if opts:
        args += ["-F", "opcoes=" + json.dumps(opts)]
    return json.loads(subprocess.run(args + [B + path], capture_output=True).stdout)

# dados da loja
req("/config", {"loja_nome": "Auto Peças São Jorge", "loja_cnpj": "12.345.678/0001-90",
                "loja_telefone": "(21) 2777-1234", "loja_endereco": "Av. Brasil, 1200 - Duque de Caxias/RJ"})

# fornecedores
forns = {}
for nome, contato, fone, mail, obs in [
    ("Distribuidora Carioca de Autopeças", "Sérgio", "(21) 3333-1122", "vendas@cariocapecas.com.br", "Entrega terça e quinta"),
    ("Tecfil Distribuição RJ", "Ana Paula", "(21) 3555-8899", "pedidos@tecfilrj.com.br", "Pedido mínimo R$ 500"),
    ("Fras-le Atacado", "Marcos", "(11) 4004-2020", "atacado@frasle.com", "Prazo 15 dias para itens sob encomenda"),
]:
    r = req("/fornecedores", {"nome": nome, "contato": contato, "telefone": fone, "email": mail, "observacoes": obs})
    forns[nome] = r["id"]

# clientes e veiculos
clientes = {}
for nome, fone, doc, end, veics in [
    ("João Batista Ferreira", "(21) 98888-1122", "123.456.789-00", "Rua das Flores, 45 - Centro",
     [("ABC1D23", "VW Gol 1.0", "2014"), ("RIO2A33", "Fiat Uno 1.0", "2010")]),
    ("Maria Aparecida Souza", "(21) 97777-3344", "987.654.321-00", "Rua Nova, 12 - Jardim Primavera",
     [("KJH4E56", "Chevrolet Onix 1.4", "2016")]),
    ("Oficina do Zé Mecânico", "(21) 99999-5566", "22.333.444/0001-55", "Av. Campos, 890",
     [("LTX7G89", "Honda Civic", "2009"), ("MPA3H77", "Fiat Strada 1.4", "2013")]),
    ("Carlos Eduardo Lima", "(21) 96666-7788", "", "Rua Bela Vista, 9",
     [("FRT8J11", "Toyota Corolla", "2018")]),
]:
    r = req("/clientes", {"nome": nome, "telefone": fone, "documento": doc, "endereco": end})
    clientes[nome] = r["id"]
    for placa, modelo, ano in veics:
        req("/clientes/%d/veiculos" % r["id"], {"placa": placa, "modelo": modelo, "ano": ano})

# importa os dois XMLs de exemplo
upload("/xml/importar", "/home/user/autopecas/exemplos/exemplo-catalogo-fornecedor.xml",
       {"margem_percentual": 65, "preco_xml_eh_custo": True, "fornecedor_id": forns["Tecfil Distribuição RJ"]})
upload("/xml/importar", "/home/user/autopecas/exemplos/exemplo-nfe-fornecedor.xml",
       {"margem_percentual": 55, "preco_xml_eh_custo": True, "fornecedor_id": forns["Distribuidora Carioca de Autopeças"]})

# mais pecas para dar volume (codigos proprios da loja, sem repetir o que veio do XML)
extra = [
    ("1048", "FR-9080", "Disco de freio ventilado Strada 1.4 2011 a 2016", "Fremax", "Freios", 15500, 25900, 2, 1, "Corredor 2 - B3", []),
    ("1050", "SK-3344", "Rolamento de roda dianteiro HB20 2013 a 2018", "SKF", "Rolamentos", 9800, 16900, 4, 6, "Corredor 4 - E1", ["VKBA-789"]),
    ("1051", "CP-5512", "Coxim do motor Celta 1.0 2005 a 2012", "Cofap", "Motor", 5400, 9900, 3, 0, "Corredor 5 - F2", []),
    ("1053", "MG-7701", "Filtro de ar do motor Onix 1.4 2016 a 2019", "Mann", "Filtros", 4200, 7900, 4, 9, "Corredor 1 - A7", []),
    ("1054", "DR-4410", "Sensor de temperatura de agua Palio 1.0 2010", "MTE-Thomson", "Elétrica", 3800, 6900, 2, 3, "Corredor 3 - C2", []),
    ("1055", "TB-9981", "Kit correia dentada + tensor Golf 1.6 2009 a 2013", "Contitech", "Motor", 21000, 34900, 1, 2, "Corredor 1 - A9", ["CT-1058"]),
    ("1056", "LN-3320", "Lanterna traseira esquerda Saveiro 2010 a 2014", "Arteb", "Iluminação", 8900, 14900, 2, 4, "Depósito - D5", []),
]
for cod, fab, desc, marca, cat, custo, preco, minimo, estoque, local, eqs in extra:
    req("/pecas", {"codigo": cod, "codigo_fabricante": fab, "descricao": desc, "marca": marca, "categoria": cat,
                   "custo_centavos": custo, "preco_centavos": preco, "minimo": minimo, "estoque": estoque,
                   "local": local, "equivalentes": eqs})

pecas = {p["descricao"]: p for p in req("/pecas?limite=200")["itens"]}

# vendas dos ultimos dias e hoje
def venda(descricao, qtd, cliente, veiculo, pagamento, desconto=0, recebido_extra=0, dias_atras=0, hora="10:15"):
    p = pecas[descricao]
    data = time.strftime("%Y-%m-%d", time.localtime(time.time() - dias_atras * 86400)) + " " + hora + ":00"
    total = p["preco_centavos"] * qtd - desconto
    body = {"cliente_id": clientes.get(cliente), "cliente_nome": cliente or "", "veiculo_placa": veiculo[0] if veiculo else "",
            "veiculo_modelo": veiculo[1] if veiculo else "", "veiculo_ano": veiculo[2] if veiculo else "",
            "itens": [{"peca_id": p["id"], "quantidade": qtd, "preco_centavos": p["preco_centavos"], "desconto_centavos": desconto}],
            "forma_pagamento": pagamento, "recebido_centavos": (total + recebido_extra) if pagamento != "faturado" else 0,
            "vencimento_dias": 30, "data": data}
    return req("/vendas", body)

venda("Filtro de oleo Gol 1.0 8v 2008 a 2016", 1, "João Batista Ferreira", ("ABC1D23", "VW Gol 1.0", "2014"), "dinheiro", 0, 5000, 3, "09:20")
venda("Pastilha de freio dianteira Onix 1.4 2012 a 2019", 1, "Maria Aparecida Souza", ("KJH4E56", "Chevrolet Onix 1.4", "2016"), "pix", 500, 0, 2, "11:05")
venda("Correia dentada Palio 1.0 8v", 1, None, None, "cartao", 0, 0, 1, "15:40")
venda("Vela de ignicao NGK Gol 1.0 2014", 4, "Oficina do Zé Mecânico", ("LTX7G89", "Honda Civic", "2009"), "faturado", 0, 0, 1, "16:10")
venda("Disco de freio ventilado Strada 1.4 2011 a 2016", 1, "Oficina do Zé Mecânico", ("MPA3H77", "Fiat Strada 1.4", "2013"), "faturado", 0, 0, 0, "09:05")
venda("Amortecedor dianteiro Corolla 2015 a 2019", 2, "Carlos Eduardo Lima", ("FRT8J11", "Toyota Corolla", "2018"), "pix", 0, 0, 0, "10:30")
venda("Filtro de oleo Gol 1.0 8v 2008 a 2016", 2, None, None, "dinheiro", 0, 0, 0, "11:45")
venda("Rolamento de roda dianteiro HB20 2013 a 2018", 1, "João Batista Ferreira", ("RIO2A33", "Fiat Uno 1.0", "2010"), "cartao", 0, 0, 0, "13:20")

# fichas de falta
fichas = [
    ("Coxim do motor Celta 1.0 2005 a 2012", "Coxim do motor Celta 1.0 2005 a 2012", 2, "Carlos Eduardo Lima", "(21) 96666-7788", "Corolla 2018 - FRT8J11", "Distribuidora Carioca de Autopeças", 5400, 12),
    ("Bomba de combustivel Uno 1.0 2010 a 2013", "Bomba de combustivel Uno 1.0 2010 a 2013", 1, "João Batista Ferreira", "(21) 98888-1122", "Uno 1.0 2010 - RIO2A33", "Distribuidora Carioca de Autopeças", 19900, 9),
    ("", "Bandeja de suspensao Civic 2007 a 2011", 1, "Oficina do Zé Mecânico", "(21) 99999-5566", "Civic 2009 - LTX7G89", "Distribuidora Carioca de Autopeças", 28990, 8),
    ("", "Sensor de rotacao do virabrequim HB20 1.0 2015", 1, "Maria Aparecida Souza", "(21) 97777-3344", "HB20 2015 - placa não informada", "Distribuidora Carioca de Autopeças", 12000, 4),
]
ids_fichas = []
for _, desc, qtd, cli, fone, veic, forn, custo, dias in fichas:
    p = pecas.get(desc)
    r = req("/fichas", {"peca_id": p["id"] if p else None, "descricao": desc, "quantidade": qtd,
                        "cliente_id": clientes.get(cli), "cliente_nome": cli, "cliente_telefone": fone,
                        "veiculo": veic, "fornecedor_id": forns[forn], "custo_centavos": custo})
    ids_fichas.append(r["id"])

# uma ficha antiga (para o relatorio de fichas abertas ha muito tempo)
import sqlite3
db = "/home/user/tmpwine/dados/autopecas.db"

print("dados de demonstracao criados")
print("pecas:", req("/pecas?limite=200")["total"])
print("fichas:", len(req("/fichas?status=todas")["itens"]))
print("pedidos:", len(req("/pedidos?status=todos")["itens"]))
