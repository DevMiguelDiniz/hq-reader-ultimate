# Verificação manual — Etapa 2

**Data:** 2026-09-19
**Cenário:** CBZ sintético de 3 páginas (`page1.jpg`, `page2.jpg`, `page3.jpg`,
cada um só com os bytes `FF D8 FF D9` — JPEG minimamente válido o bastante
pro roteamento, não pra decodificação visual), gerado pelo script Go do
Step 1 do brief da Task 6. `HQREADER_BOOK` apontando pro `.cbz` gerado.

- [x] `wails3 dev`: primeira página carrega, contador mostra "1 / 3"
- [x] Navegação "Proxima"/"Anterior" funciona e atualiza a imagem
- [x] Índice não ultrapassa os limites (0 a pageCount-1)
- [x] `wails3 build` + binário: mesmo comportamento no build de produção

## Metodologia e evidência (arquivos reais, não transcrição)

Seguindo o mesmo padrão de rigor já estabelecido na Etapa 0
(`spikes/04-multiwindow-verify.ps1` + `spikes/04-multiwindow-verify-output.txt`,
`spikes/01-http-route-prod-verify.ps1` + `spikes/01-http-route-prod-verify-output.txt`),
esta verificação está registrada em **três arquivos reais, commitados junto
com este documento** — não uma transcrição colada à mão:

- **`cmd/hqreader/verify-navigation.ps1`** — o script completo e
  re-executável de UI Automation (`System.Windows.Automation`). Localiza a
  janela nativa real do processo `hqreader.exe` pelo PID, restaura/foca a
  janela (ver "Achado lateral" abaixo), clica "Proxima"/"Anterior" via
  `InvokePattern`, lê de volta o texto do contador (`<span id="counter">`)
  via um nó `Text` cujo `Name` bate com o padrão `N / M`, e confirma a
  presença do elemento `<img id="page" alt="pagina">` na árvore de
  acessibilidade. Cada uma das 8 asserções imprime `PASS`/`FAIL` com o valor
  real lido, e o script termina com exit code 1 se qualquer uma falhar.
- **`cmd/hqreader/verify-navigation-dev-output.txt`** — saída bruta,
  redirecionada diretamente do PowerShell (`*> arquivo.txt`, sem edição) da
  execução deste script contra o processo `hqreader.exe` rodando via
  `wails3 dev` (PID 12008 nesta rodada).
- **`cmd/hqreader/verify-navigation-prod-output.txt`** — saída bruta
  idêntica, desta vez contra o binário `bin/hqreader.exe` gerado por
  `wails3 build` (PID 13792 nesta rodada), sem `wails3 dev`/proxy do Vite
  envolvido — exercitando `AssetFileServerFS` sobre o `embed.FS` de
  `frontend/dist`.

Ambos os arquivos `-output.txt` terminam em `=== SUMMARY: 8 passed, 0
failed ===`, com as 8 verificações (contador inicial, avanço até o fim,
clamp no fim, retrocesso até o início, clamp no início, e presença do
elemento `<img>`) idênticas entre dev e produção.

## Como reproduzir

Passos completos (incluindo a re-geração do `.cbz` sintético) documentados
no cabeçalho de `cmd/hqreader/verify-navigation.ps1`. Resumo:

```powershell
# 1) gerar um .cbz sintético de 3 páginas (ver Step 1 do brief da Task 6)
# 2) modo dev:
cd cmd/hqreader
$env:HQREADER_BOOK = "<caminho para o .cbz>"
wails3 dev
powershell -NoProfile -ExecutionPolicy Bypass -File verify-navigation.ps1 -ProcessName hqreader

# 3) modo produção:
wails3 build
$env:HQREADER_BOOK = "<caminho para o .cbz>"
Start-Process .\bin\hqreader.exe
powershell -NoProfile -ExecutionPolicy Bypass -File verify-navigation.ps1 -ProcessName hqreader
```

## Achado lateral (não é bug do produto)

Durante a primeira rodada desta verificação, lançar `wails3 dev` via
`Start-Process -WindowStyle Hidden` (só pra capturar log em arquivo em
background) fez a janela nativa `hqreader.exe` nascer minimizada — e nesse
estado o WebView2 não expõe NENHUM descendente via UI Automation
(`FindAll(Descendants)` retornava 0 nós, incluindo o próprio contador).
Restaurar a janela (`ShowWindow`/`SetForegroundWindow`) resolveu
imediatamente. Isso é uma característica conhecida do WebView2/Chromium (não
renderiza/expõe acessibilidade enquanto minimizado), não um defeito do app
`hqreader` — um usuário abrindo `wails3 dev` normalmente nunca vê a janela
minimizada por padrão. `verify-navigation.ps1` agora restaura e foca a
janela ativamente antes de ler qualquer coisa (ver o passo logo após
"Found window" em ambos os `-output.txt`), então esse achado não afeta mais
a confiabilidade do script.

## Observações

Nenhum desvio do comportamento esperado no fluxo principal (abrir → mostrar
primeira página → navegar próximo/anterior → clamp nos dois extremos), tanto
em `wails3 dev` quanto no binário de produção `wails3 build` — ver os dois
arquivos `verify-navigation-*-output.txt` para a evidência bruta completa,
idêntica nos dois modos.

Limitação reconhecida do próprio mecanismo de verificação (não do produto):
UI Automation não consegue ler o atributo `src` do `<img>` diretamente
(WebView2/Chromium não expõe `ValuePattern` nesse nó de imagem), então o
script confirma a presença do elemento `<img>` (Passo G) mas não lê seu
`src` byte-a-byte. A mudança de origem da imagem a cada navegação é uma
consequência direta e não-opcional de `render()` — a mesma função que
atualiza `counter.textContent` também define `img.src =
/page/{bookID}/{index}` na linha seguinte, sem nenhum branch entre as duas
atribuições (ver `cmd/hqreader/frontend/index.html`) — então as 8 leituras
corretas do contador em ambos os `-output.txt` implicam a mesma correção
para `img.src`. A correção byte-a-byte da resposta HTTP em si
(`Content-Type`, `Content-Length`, status 200/404) já tem cobertura direta
em `internal/reader/handler_test.go` (Task 3).

## Etapa 3b — React + Tailwind + shadcn/ui + Zustand

**Data:** 2026-09-22
**Cenário:** mesmo CBZ sintético de 3 páginas, agora com a UI em React chamando
SetCurrentPage no backend a cada navegação (não mais um índice só local).

Script real: `verify-navigation-react.ps1`. Saída bruta:
`verify-navigation-react-dev-output.txt` (wails3 dev, PID 29424 nesta rodada),
`verify-navigation-react-prod-output.txt` (wails3 build, PID 29112 nesta rodada).

- [x] Primeira página carrega, contador "1 / 3"
- [x] Navegação Anterior/Proxima funciona e desabilita nos limites
- [x] Mesmo comportamento em produção (build embutido)
- [x] Nenhum erro observado no console/saída relacionado a SetCurrentPage

**Observações:**

`verify-navigation-react.ps1` é uma adaptação (não uma reescrita do zero) do
`verify-navigation.ps1` da Etapa 2 — reaproveita a lógica de localizar a
janela pelo PID via UI Automation, restaurar/focar a janela, invocar botões
por nome e o esqueleto de PASS/FAIL. Uma investigação real da árvore de UI
Automation da janela React rodando (não suposição) revelou uma diferença de
DOM relevante em relação à Etapa 2 que exigiu ajuste: o contador antigo
(`<span id="counter">1 / 3</span>`) era um único nó de texto, exposto como
um único controle `Text` do UI Automation com `Name='1 / 3'`. O contador
novo é JSX (`<span>{currentPage + 1} / {pageCount}</span>`), e o React
renderiza cada `{expressão}` e cada trecho de string literal como um nó de
texto do DOM separado — a árvore ao vivo confirmou isso: três controles
`Text` irmãos, na ordem do documento, com `Name='1'`, `Name=' / '`,
`Name='3'`, nenhum deles contendo a string completa "N / M". A busca por
nome de botão (`Find-ByName`/`Invoke-ByName`) não precisou de nenhuma
mudança — os botões shadcn/ui `<Button variant="secondary">` continuam
sendo `<button>` reais com `Name` exatamente "Anterior"/"Proxima" na árvore
de acessibilidade. `Get-CounterText` foi reescrita para concatenar todos os
nós `Text` descendentes (na ordem retornada por `FindAll`, que é a ordem do
documento) antes de aplicar a mesma regex `^\d+\s*/\s*\d+$` à string
concatenada, em vez de casar a regex contra um único nó.

O script também adiciona duas verificações que a Etapa 2 não tinha:
confirmar via `IsEnabled` do próprio `AutomationElement` que o botão
"Proxima" fica desabilitado na última página e que "Anterior" fica
desabilitado na primeira — refletindo o atributo HTML `disabled` real
(`disabled={currentPage >= pageCount - 1}` / `disabled={currentPage <= 0}`
em `Reader.tsx`, componente shadcn/ui sobre `@base-ui/react`), não apenas
inferido pelo contador parar de mudar. As 10 verificações (contador inicial, avanço até
o fim, `Proxima.IsEnabled=False` no fim, clamp no fim, retrocesso até o
início, `Anterior.IsEnabled=False` no início, clamp no início, e presença
do elemento `<img>`) são idênticas nos dois arquivos `-output.txt`, com
`=== SUMMARY: 10 passed, 0 failed ===` em ambos e exit code 0 nos dois.

Clicar em um botão já desabilitado via `InvokePattern.Invoke()` levanta uma
exceção COM ("Erro não reconhecido") em vez de silenciosamente não fazer
nada — visível nos dois `-output.txt` no Passo D/F ("clique 3x" nos
extremos). Isso é esperado/aceitável (o próprio UI Automation não permite
invocar um controle com `IsEnabled=False`) e o script já trata esse caso
com `try/catch`, sem contar como falha — o que importa é que o contador
permanece "3 / 3"/"1 / 3" depois, confirmando o clamp do FRONTEND: o
atributo `disabled` do botão já impede o 3º clique de chegar a
`InvokePattern.Invoke()` de fato, e o `return` antecipado de `goToPage` em
`cmd/hqreader/frontend/src/store/readerStore.ts` (quando `index < 0 ||
index >= pageCount`) garante que `SetCurrentPage` nunca é chamado com um
índice fora dos limites. Este script não exercita o clamp de limites do
próprio backend (o `archive.ErrCorrupted` de `internal/reader/reader.go`) —
ele só observa o texto do contador na UI, nunca o estado Go diretamente —,
mas esse clamp tem cobertura direta e separada em
`internal/reader/reader_test.go`.

Nenhum erro relacionado a `SetCurrentPage` apareceu na saída de processo
(stdout/stderr do `wails3 dev`, nem do binário de produção) durante a
navegação real em nenhum dos dois modos — a única linha de erro observada
nos logs brutos de `wails3 dev` (`ERROR task: Failed to run task "run":
exit status 255`) é um artefato esperado de eu ter finalizado o processo
`hqreader.exe` com `Stop-Process -Force` ao fim da verificação, não um erro
do app ou de `SetCurrentPage`.

Mesma limitação reconhecida na Etapa 2 continua valendo aqui: UI Automation
não lê o atributo `src` do `<img>` byte-a-byte (WebView2/Chromium não expõe
`ValuePattern` nesse nó), então o Passo G confirma a presença do elemento
(`ControlType.Image`, `Name='pagina'`, batendo com `alt="pagina"` em
`Reader.tsx`) mas não o `src` renderizado — a correção do `src`
(`/page/{bookID}/{index}`) é a mesma consequência direta de `goToPage`
chamando `SetCurrentPage` e a store atualizando `currentPage`, e a
resposta HTTP em si já tem cobertura em `internal/reader/handler_test.go`.

## Etapa 3c — zoom/pan, modo duplo, atalhos, placeholder de erro, Motion

**Data:** 2026-10-17
**Cenário:** CBZ sintético de 10 páginas (`make-verify-cbz.go`): capa, retratos,
um spread (índice 4) e uma página corrompida (índice 6, bytes que não são PNG).
Cada página não corrompida é uma cor sólida com uma faixa marcadora branca
pura (40 px, centralizada na vertical, largura total) usada para medir a
posição da página em pixels.

Script real: `verify-reader-interactions.ps1`. Saída bruta:
`verify-reader-interactions-dev-output.txt` (wails3 dev, PID 7544),
`verify-reader-interactions-prod-output.txt` (wails3 build, PID 15736).
Resultado: `=== SUMMARY: 112 passed, 0 failed ===` e `=== SUMMARY: 112 passed, 0 failed ===`
(exit code 0 nas duas execuções).

- [x] Navegação por teclado (setas, A/D, Espaço, rajada, Ctrl+→ ignorado, Espaço após clique em botão avança uma página)
- [x] Zoom por teclado (+, =, -, 0) e por roda, com limites 50% e 400%
- [x] Zoom pela roda ancorado no ponto sob o cursor (132% → 179% com o cursor 200 px abaixo da faixa: faixa em 453, esperado 451,8)
- [x] Arrastar move a imagem (eixo vertical), pan limitado ao viewport (limite calculado ±178,9 px a 182%; faixa em 702,5 e 343,5, esperado 702,4 e 344,6)
- [x] Arrastar não faz nada quando a página cabe (100% e 50%) nem no eixo horizontal quando a página ampliada é mais estreita que o viewport
- [x] Redimensionar (maximizar/restaurar) com zoom ativo mantém a imagem visível
- [x] Trocar de página reseta o zoom para 100% (e o pan: a nova página volta centralizada)
- [x] Modo duplo: capa sozinha, pares, spread sozinho, vizinho do spread sozinho, última página sozinha, ida e volta
- [x] Alternar o modo mantém a primeira página da visão e reseta o zoom
- [x] Placeholder de erro (modo único e metade de um par no modo duplo), navegação continua habilitada, "Tentar novamente" funciona sem derrubar a leitura
- [x] Mesmo comportamento em produção (build embutido)

(Marque cada item com `[x]` somente se as linhas `PASS` correspondentes estão nos dois arquivos de saída.)

**Observações:**

- A geometria das páginas é medida em pixels, não pela árvore de UI
  Automation. O dump real da árvore mostrou que o `BoundingRectangle` do
  `Image` `pagina 1` é o retângulo de layout e ignora o `transform` CSS
  (`783;259;354;529` tanto a 100% quanto a 135%, com a página visivelmente
  maior na captura). Por isso o script captura a janela com `PrintWindow` e
  localiza a faixa branca do CBZ; textos, nomes de botões e presença/nome
  das imagens continuam vindo do UI Automation (contador, zoom e mensagem
  de erro são, cada um, um único nó `Text`, confirmado no dump).
- Um "notch" da roda chega ao WebView como `deltaY=100`, então cada notch
  multiplica o zoom por e^0,15 ≈ 1,1618 (3 notches = 157%, 4 = 182%).
- Os dois arquivos de saída diferem apenas na linha `PID:`. Todas as
  verificações e todas as medidas de pixel são iguais em dev e em produção.
- Além do rascunho do plano, o script também verifica: zoom ancorado no
  cursor; arrasto sem efeito a 100% e a 50%; o pan para exatamente no
  limite calculado, nas duas direções; um arrasto extra no limite não move
  a página; a página volta centralizada ao trocar de página; o modo
  alternado reseta um zoom de 115% (não só um zoom que já estava em 100%);
  `Tentar novamente` é clicado com o mouse real e o app continua saudável
  depois.

**Limitações desta verificação:** a transição de Motion (fade de ~150ms) não é
observável por UI Automation; o que se verifica é que, depois de navegar, só
as imagens esperadas existem na árvore (nenhuma imagem antiga presa pela saída
do fade). O efeito do "Tentar novamente" não é observável aqui (a página está
realmente corrompida e o placeholder volta); que o clique real chega ao
`onClick` do botão foi verificado pela execução instrumentada da Task 4
(fix round 1). O comportamento em WebKitGTK/Linux não foi validado (fora de
escopo da etapa, spec §10.6). O script observa a UI (UI Automation) e pixels
capturados da janela; nunca o estado Go diretamente.
