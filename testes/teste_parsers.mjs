// Testes dos conversores de dinheiro/quantidade do front (web/app.js).
// Roda com: node qa/teste_parsers.mjs
// Sai com codigo 1 se algum caso falhar (serve para o build automatizado).

import fs from "node:fs";

const src = fs.readFileSync(new URL("../autopecas/web/app.js", import.meta.url), "utf8");
const corte = src.indexOf("function toast(");
const helpers = src.slice(0, corte);
const fabrica = new Function(
  "document", "window", "location", "fetch",
  helpers + "; return { parseDinheiro, parseQtd, fmtMoney, fmtQtd };"
);
const doc = { querySelector: () => null, querySelectorAll: () => [], addEventListener: () => {} };
const h = fabrica(doc, {}, { hash: "" }, () => {});

let falhas = 0;
function igual(nome, obtido, esperado) {
  const ok = obtido === esperado;
  if (!ok) falhas++;
  console.log(`  ${ok ? "[ok]  " : "[FALHOU]"} ${nome}: obtido ${JSON.stringify(obtido)}, esperado ${JSON.stringify(esperado)}`);
}

console.log("parseDinheiro (campo de preço/desconto/custo) — entrada em reais:");
igual('"12,50"', h.parseDinheiro("12,50"), 1250);
igual('"R$ 12,50"', h.parseDinheiro("R$ 12,50"), 1250);
igual('"12.50"', h.parseDinheiro("12.50"), 1250);
igual('"1.234,56"', h.parseDinheiro("1.234,56"), 123456);
igual('"1234"', h.parseDinheiro("1234"), 123400);
// separador de milhar sem centavos: o operador digita R$ 1.234,00
igual('"1.234"', h.parseDinheiro("1.234"), 123400);
igual('"1.000.000"', h.parseDinheiro("1.000.000"), 100000000);
igual('"10.000"', h.parseDinheiro("10.000"), 1000000);

console.log("parseQtd (estoque/quantidade):");
igual('"1.000"', h.parseQtd("1.000"), 1000);
igual('"1,5"', h.parseQtd("1,5"), 1.5);
igual('"10"', h.parseQtd("10"), 10);

console.log("fmtMoney / fmtQtd:");
igual("fmtMoney(123456789)", h.fmtMoney(123456789), "R$ 1.234.567,89");
igual("fmtQtd(2)", h.fmtQtd(2), "2");
igual("fmtQtd(1.5)", h.fmtQtd(1.5), "1,50");

console.log(falhas === 0 ? "\nTODOS OS CASOS OK" : `\n${falhas} caso(s) falharam`);
process.exit(falhas === 0 ? 0 : 1);
