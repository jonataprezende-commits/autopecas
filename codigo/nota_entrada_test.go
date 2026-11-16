package main

import "testing"

const xmlNFeTeste = `<?xml version="1.0" encoding="UTF-8"?>
<nfeProc xmlns="http://www.portalfiscal.inf.br/nfe">
 <NFe><infNFe Id="NFe35261012345678000199550010000123451000123456" versao="4.00">
  <ide><cUF>35</cUF><nNF>12345</nNF><serie>1</serie><dhEmi>2026-10-20T10:15:00-03:00</dhEmi></ide>
  <emit><CNPJ>12.345.678/0001-99</CNPJ><xNome>DISTRIBUIDORA DE PECAS LTDA</xNome></emit>
  <det nItem="1"><prod><cProd>P-001</cProd><xProd>PASTILHA DE FREIO</xProd><qCom>10</qCom><vUnCom>100.00</vUnCom><vProd>1000.00</vProd></prod></det>
  <det nItem="2"><prod><cProd>P-002</cProd><xProd>DISCO DE FREIO</xProd><qCom>5</qCom><vUnCom>120.00</vUnCom><vProd>600.00</vProd></prod></det>
  <total><ICMSTot><vProd>1600.00</vProd><vFrete>100.00</vFrete><vIPI>50.00</vIPI><vNF>1750.00</vNF></ICMSTot></total>
  <cobr><dup><nDup>001</nDup><dVenc>2026-11-19</dVenc><vDup>583.33</vDup></dup>
        <dup><nDup>002</nDup><dVenc>2026-12-19</dVenc><vDup>583.33</vDup></dup>
        <dup><nDup>003</nDup><dVenc>2027-01-19</dVenc><vDup>583.34</vDup></dup></cobr>
 </infNFe></NFe>
</nfeProc>`

func TestNotaDoXML(t *testing.T) {
	root, err := parseXMLTree([]byte(xmlNFeTeste))
	if err != nil {
		t.Fatal(err)
	}
	n := lerNotaDoXML(root)
	if n.Chave != "35261012345678000199550010000123451000123456" {
		t.Errorf("chave=%q", n.Chave)
	}
	if n.Numero != "12345" || n.Serie != "1" {
		t.Errorf("numero/serie=%q/%q", n.Numero, n.Serie)
	}
	if n.EmitenteNome != "DISTRIBUIDORA DE PECAS LTDA" {
		t.Errorf("nome=%q", n.EmitenteNome)
	}
	if n.EmitenteDoc != "12345678000199" {
		t.Errorf("doc=%q", n.EmitenteDoc)
	}
	if n.EmitidaEm != "2026-10-20" {
		t.Errorf("emissao=%q", n.EmitidaEm)
	}
	if n.TotalCents != 175000 {
		t.Errorf("total=%d", n.TotalCents)
	}
	if n.ProdutosCents != 160000 {
		t.Errorf("produtos=%d", n.ProdutosCents)
	}
	if n.FreteCents != 10000 {
		t.Errorf("frete=%d", n.FreteCents)
	}
	if n.ExtrasCents != 15000 {
		t.Errorf("extras=%d", n.ExtrasCents)
	}
	if len(n.Duplicatas) != 3 {
		t.Fatalf("duplicatas=%d", len(n.Duplicatas))
	}
	if n.Duplicatas[0].Vencimento != "2026-11-19" || n.Duplicatas[0].ValorCents != 58333 {
		t.Errorf("dup0=%+v", n.Duplicatas[0])
	}
	itens, _ := parseCatalogXML([]byte(xmlNFeTeste))
	if len(itens) != 2 {
		t.Errorf("itens=%d", len(itens))
	}
}
