# Spike 2 — Custo de extração de RAR sólido

## Desvio em relação ao plano original (§5 item 2)

O plano previa duas amostras: uma conhecida como sólida e uma normal, para
comparação (Step 5). Só havia disponível **uma** amostra real de mercado
(`spikes/rar-cost/testdata/sample-solid.cbr`, ~28.8 MiB / 30 195 907 bytes,
uma release de scan real). Não havia uma segunda amostra não-sólida
disponível, então o Step 5 (execução comparativa) **foi pulado** — não há
dados de comparação sólido vs. não-sólido neste spike.

Além disso, a biblioteca `nwaples/rardecode/v2` na versão resolvida pelo
`go get` (v2.4.1) já não aceita mais senha como segundo argumento posicional
em `OpenReader(path, "")` como o snippet do plano assumia — a API atual usa
opções funcionais variádicas (`rardecode.Option`). O `main.go` foi ajustado
para `rardecode.OpenReader(path)` sem opções (arquivo sem senha). O restante
da lógica do programa segue exatamente o que o plano especificou.

## Achado inesperado: a amostra não é sólida

O pacote expõe um campo `Solid bool` no header de cada entrada
(`rardecode.FileHeader.Solid`), que indica se aquela entrada depende do
stream de descompressão da entrada anterior (compressão sólida real). O
programa de medição foi estendido (além do que o plano pedia) para contar
quantas das entradas decodificadas vêm com `hdr.Solid == true`.

Resultado: **0 de 39 entradas** vieram marcadas como `Solid`. Ou seja, apesar
do nome do arquivo (`sample-solid.cbr`) e da suposição de que releases de
scan tipicamente usam compressão sólida, **esta amostra específica não é uma
release sólida** — cada entrada é comprimida de forma independente. Isso não
foi confirmado antes de rodar o spike; só ficou claro a partir do próprio
resultado da biblioteca.

Consequência prática: não foi possível medir o pior caso genuíno (RAR sólido
real) com a amostra disponível — nem por falta de uma segunda amostra
(Step 5), nem porque a única amostra que temos não é sólida. A medição abaixo
reflete o custo de **decodificação sequencial completa** de um CBR real via
`rardecode` (que é sempre sequencial nesta API, `Next()` + `io.Copy`,
independente de o arquivo ser sólido ou não — a diferença entre sólido e
normal está na possibilidade de acesso aleatório via `Open()`, não na forma
como este spike mede o tempo). Portanto o número de tempo total é uma medida
válida do custo de "abrir e decodificar tudo sequencialmente", mas não isola
o efeito específico da compressão sólida.

## Amostra real (`sample-solid.cbr`)

- arquivo: `spikes/rar-cost/testdata/sample-solid.cbr`
- tamanho: 30 195 907 bytes (~28.8 MiB)
- páginas/entradas: 39
- confirmado sólido pela flag da biblioteca: **não** (0/39 entradas com `Solid=true`)

Três execuções consecutivas (mesma máquina, build local, sem otimizações
especiais):

| execução | tempo até a 1ª entrada | tempo total (39 entradas) |
|----------|------------------------|----------------------------|
| 1        | 49.0091ms              | 545.1332ms                 |
| 2        | 44.0170ms              | 523.8890ms                 |
| 3        | 46.9918ms              | 531.6887ms                 |
| 4        | 48.0901ms              | 548.2958ms                 |

Média aproximada: ~47ms até a 1ª entrada, ~537ms para decodificar as 39
entradas por completo (~13.8ms/página em média, ~53.6 MiB/s de throughput
efetivo de decodificação nesta máquina — 30 195 907 bytes / 0.537s).

## Amostra não-sólida (comparação)

**Não realizado.** Não havia uma segunda amostra `.cbr` não-sólida
disponível para este spike (Step 5 do plano). Não há dados de comparação
sólido vs. não-sólido a registrar. Ironicamente, a única amostra disponível
já se mostrou não-sólida pela própria flag da biblioteca (ver seção acima),
então mesmo que houvesse uma segunda amostra "normal", o contraste
pretendido pelo plano (sólido vs. normal) não pôde ser obtido de forma
alguma neste spike.

## Decisão: GO COM MITIGAÇÃO

Com apenas uma amostra real disponível — e que se revelou não-sólida —, não
dá para afirmar com confiança qual é o custo de abrir um CBR **genuinamente**
sólido e grande (ex.: 100+ MiB, 150+ páginas, típico de algumas releases de
scan). Os números medidos aqui (~537ms para decodificar 28.8 MiB / 39
páginas sequencialmente) são baixos e não seriam perceptíveis como travamento
para o usuário, mas:

1. A extrapolação linear pelo throughput observado (~53.6 MiB/s) sugere que um
   CBR sólido de 100 MiB levaria da ordem de ~1.87s, e um de 200 MiB (mais
   raro, mas existe em releases de scan de alta resolução) da ordem de ~3.73s
   — faixa em que travar a UI sem feedback já é perceptível ao usuário.
2. O spike não testou compressão sólida real; o custo de decodificação
   sólida pode ser diferente (potencialmente mais rápido por reaproveitar
   tabelas/dicionário entre entradas, ou mais lento por não permitir acesso
   aleatório e forçar sempre decodificação completa até a página desejada).
   Sem uma amostra sólida real, essa lacuna permanece.
3. A API confirma a limitação estrutural relevante para arquitetura: para
   entradas com `Solid=true`, `Open()` não é suportado (`ErrSolidOpen`) — só
   `Next()` sequencial funciona. Isso valida a premissa original do risco
   (§5 item 2): CBR sólido real não permite acesso aleatório por página, só
   decodificação sequencial até a página desejada.

**Justificativa da decisão:** os dados reais que temos (ainda que de uma
amostra não-sólida) não mostram um custo alto no caso comum, mas a
extrapolação para arquivos maiores e a confirmação da limitação estrutural
de `Open()` em entradas sólidas justificam mitigação preventiva em vez de
assumir GO simples. NO-GO não se justifica: nada nos dados sugere que RAR
sólido seja inviável de suportar, só que pode precisar de feedback visual em
casos maiores.

## Mitigação recomendada

Na Etapa 3 (implementação do leitor), definir um indicador de progresso
("carregando página X de N" ou spinner) ao abrir um CBR, ativado sempre que:
- o arquivo for detectado como RAR sólido (verificar `hdr.Solid` na primeira
  entrada decodificada após a primeira), **e**
- o tempo estimado de decodificação total ultrapassar um limiar perceptível
  (sugestão inicial: 300-500ms, ajustável após medir com uma amostra
  genuinamente sólida maior quando uma estiver disponível).

Para CBZ (ZIP) e para RAR não-sólido, manter acesso direto sem spinner
obrigatório, já que ambos suportam acesso aleatório por entrada.

**Lacuna a fechar depois:** obter pelo menos uma amostra CBR genuinamente
sólida (confirmável via `hdr.Solid == true`) e grande (100+ MiB) para validar
o pior caso real antes de finalizar o limiar do indicador de progresso.
