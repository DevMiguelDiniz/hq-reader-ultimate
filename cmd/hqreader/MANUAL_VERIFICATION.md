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
