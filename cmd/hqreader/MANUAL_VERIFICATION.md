# Verificação manual — Etapa 2

**Data:** 2026-09-19
**Cenário:** CBZ sintético de 3 páginas (`page1.jpg`, `page2.jpg`, `page3.jpg`,
cada um só com os bytes `FF D8 FF D9` — JPEG minimamente válido o bastante
pro roteamento, não pra decodificação visual), gerado pelo script Go do
Step 1 do brief. `HQREADER_BOOK` apontando pro `.cbz` gerado.

- [x] `wails3 dev`: primeira página carrega, contador mostra "1 / 3"
- [x] Navegação "Proxima"/"Anterior" funciona e atualiza a imagem
- [x] Índice não ultrapassa os limites (0 a pageCount-1)
- [x] `wails3 build` + binário: mesmo comportamento no build de produção

## Metodologia

Seguindo a mesma técnica validada na Etapa 0 (`spikes/04-multiwindow-verify.ps1`,
`spikes/01-http-route-prod-verify.ps1`): um script PowerShell descartável usa
Windows UI Automation (`System.Windows.Automation`) para localizar a janela
nativa real do processo `hqreader.exe` (dev ou produção, mesmo binário do
ponto de vista da UI), clicar nos botões "Proxima"/"Anterior" via
`InvokePattern`, e ler de volta o texto do contador (`<span id="counter">`)
via um nó `Text` cujo `Name` bate com o padrão `N / M`. Isso evita depender
só de inspeção visual/de código — cada assertão abaixo é sobre o valor
*realmente lido de volta* da janela ao vivo, não uma suposição.

O script (`hqreader-nav-verify.ps1`, mantido fora do repositório — é
descartável, como o próprio brief da Task 6 pede) faz, na ordem:

1. Localiza o processo (`hqreader.exe`) e sua janela top-level.
2. Lê o contador inicial → espera `"1 / 3"`.
3. Clica "Proxima" → espera `"2 / 3"`.
4. Clica "Proxima" de novo → espera `"3 / 3"`.
5. Clica "Proxima" uma TERCEIRA vez (tentando passar do fim) → espera que
   continue `"3 / 3"` (índice não avança além de `pageCount-1`).
6. Clica "Anterior" → espera `"2 / 3"`.
7. Clica "Anterior" mais duas vezes → espera `"1 / 3"` e que uma clicada extra
   não desça abaixo disso (índice não regride abaixo de `0`).

Script completo (para reprodução futura):

```powershell
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

function Find-ByName($parent, $name, $controlType) {
    $nameCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::NameProperty, $name)
    if ($controlType) {
        $typeCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ControlTypeProperty, $controlType)
        $cond = New-Object System.Windows.Automation.AndCondition($nameCond, $typeCond)
    } else {
        $cond = $nameCond
    }
    return $parent.FindFirst([System.Windows.Automation.TreeScope]::Descendants, $cond)
}

function Invoke-ByName($parent, $name) {
    $btn = Find-ByName $parent $name ([System.Windows.Automation.ControlType]::Button)
    if (-not $btn) { throw "button '$name' not found" }
    $pattern = $btn.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern)
    $pattern.Invoke()
}

function Get-CounterText($parent) {
    $typeCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ControlTypeProperty, [System.Windows.Automation.ControlType]::Text)
    $deadline = (Get-Date).AddSeconds(10)
    while ((Get-Date) -lt $deadline) {
        $all = $parent.FindAll([System.Windows.Automation.TreeScope]::Descendants, $typeCond)
        foreach ($el in $all) {
            if ($el.Current.Name -match '^\d+\s*/\s*\d+$') { return $el.Current.Name }
        }
        Start-Sleep -Milliseconds 300
    }
    return $null
}

# ... localiza processo/janela por ProcessName, chama Get-CounterText e
# Invoke-ByName na sequência descrita acima, comparando cada leitura com o
# valor esperado (Assert simples: PASS/FAIL por passo).
```

## Resultado — `wails3 dev`

Comandos executados (a partir de `cmd/hqreader`):

```powershell
$env:HQREADER_BOOK = "<...>\teste.cbz"
wails3 dev
```

O primeiro `wails3 dev` conectou normalmente (sem repetir o problema de
timing já documentado na Etapa 0/Task 1 — não foi necessário retry desta
vez). Log relevante do processo:

```
INFO  AssetServer Info: middleware=true handler=true devServerURL=http://localhost:9245
INFO  Waiting for frontend dev server to start... url=http://localhost:9245
INFO  Retrying...
```

Depois de confirmar a janela nativa `hqreader.exe` de pé, a execução do
script de verificação produziu:

```
=== Locating hqreader.exe process ===
PID: 21296
Found window: hqreader

=== Step A: initial counter (expect '1 / 3') ===
Initial counter text: '1 / 3'
PASS: Initial counter -> '1 / 3'

=== Step B: click Proxima -> expect '2 / 3' ===
PASS: After 1x Proxima -> '2 / 3'

=== Step C: click Proxima again -> expect '3 / 3' ===
PASS: After 2x Proxima -> '3 / 3'

=== Step D: click Proxima a 3rd time (past end) -> expect STILL '3 / 3' (clamped) ===
PASS: After 3x Proxima (clamp at end) -> '3 / 3'

=== Step E: click Anterior -> expect '2 / 3' ===
PASS: After 1x Anterior -> '2 / 3'

=== Step F: click Anterior twice more -> expect '1 / 3', then stays '1 / 3' (clamped at start) ===
PASS: After 2x Anterior -> '1 / 3'
PASS: After 3x Anterior (clamp at start) -> '1 / 3'

=== SUMMARY: 7 passed, 0 failed ===
```

Adicionalmente, o elemento de imagem (`<img id="page" alt="pagina">`) foi
confirmado presente na árvore de acessibilidade (`ControlType.Image`,
`Name='pagina'`) enquanto a janela estava visível — a UI Automation do
WebView2/Chromium não expõe o atributo `src` como uma propriedade legível
diretamente (sem `ValuePattern` suportado nesse nó), então a mudança de
origem da imagem não foi lida byte-a-byte via UIA; ela é inferida com
segurança do fato de que `render()` (a mesma função que atualiza o contador
E define `img.src = /page/{bookID}/{index}` em sequência, sem branch entre
as duas atribuições) executa a cada clique — confirmado pelas 7 leituras
corretas do contador acima. A correção do byte servido em si
(`Content-Type`/`Content-Length`/status 200 vs 404) já tem cobertura direta
nos testes automatizados de `internal/reader/handler_test.go` (Task 3).

**Achado lateral, não um bug do produto:** ao automatizar o lançamento do
`wails3 dev` via `Start-Process` com `-WindowStyle Hidden` (só pra capturar
log em arquivo), a janela nativa do `hqreader.exe` nasceu minimizada, e
nesse estado o WebView2 não expõe nenhum descendente via UI Automation
(`FindAll(Descendants)` retornava 0 nós, incluindo o contador). Restaurar a
janela (`ShowWindow`/`SetForegroundWindow`) resolveu imediatamente — a
árvore de acessibilidade voltou a aparecer por completo. Isso é uma
característica conhecida de WebView2 (não renderiza/expõe acessibilidade
enquanto minimizado) e um artefato de como *este script de verificação* foi
lançado, não um defeito do app: um usuário real abrindo `wails3 dev`
normalmente nunca vê a janela minimizada por padrão.

## Resultado — `wails3 build` (produção)

```powershell
wails3 build
```

Build concluído sem erros:

```
task: [windows:common:frontend:run:npm] npm run build -q
dist/index.html                 0.72 kB
dist/assets/index-DaJ_ZUA7.js  24.60 kB
✓ built in 97ms
task: [windows:build:native] go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui" -o "bin/hqreader.exe"
```

Binário gerado em `cmd/hqreader/bin/hqreader.exe` (11.490.304 bytes). Rodado
diretamente (sem `wails3 dev`, sem proxy do Vite — exercitando
`AssetFileServerFS` sobre o `embed.FS` de `frontend/dist`) com a mesma
variável de ambiente:

```powershell
$env:HQREADER_BOOK = "<...>\teste.cbz"
.\bin\hqreader.exe
```

O mesmo script de verificação, apontado pro processo de produção, produziu
o resultado idêntico ao de dev:

```
=== Locating hqreader.exe process ===
PID: 16108
Found window: hqreader

=== Step A: initial counter (expect '1 / 3') ===
Initial counter text: '1 / 3'
PASS: Initial counter -> '1 / 3'

=== Step B: click Proxima -> expect '2 / 3' ===
PASS: After 1x Proxima -> '2 / 3'

=== Step C: click Proxima again -> expect '3 / 3' ===
PASS: After 2x Proxima -> '3 / 3'

=== Step D: click Proxima a 3rd time (past end) -> expect STILL '3 / 3' (clamped) ===
PASS: After 3x Proxima (clamp at end) -> '3 / 3'

=== Step E: click Anterior -> expect '2 / 3' ===
PASS: After 1x Anterior -> '2 / 3'

=== Step F: click Anterior twice more -> expect '1 / 3', then stays '1 / 3' (clamped at start) ===
PASS: After 2x Anterior -> '1 / 3'
PASS: After 3x Anterior (clamp at start) -> '1 / 3'

=== SUMMARY: 7 passed, 0 failed ===
```

`ControlType.Image` (`Name='pagina'`) também confirmado presente na janela
de produção, mesma observação sobre `src` não legível via `ValuePattern`
descrita acima.

Processo encerrado (`Stop-Process`) depois da confirmação, sem deixar
`hqreader.exe`/`node`/`wails3` residuais.

## Observações

Nenhum desvio do comportamento esperado no fluxo principal (abrir → mostrar
primeira página → navegar próximo/anterior → clamp nos dois extremos), tanto
em `wails3 dev` quanto no binário de produção `wails3 build`. A única
observação registrada é o achado lateral sobre janela minimizada + UI
Automation do WebView2 descrito acima, que é uma característica do ambiente
de automação usado para verificar, não um defeito do app `hqreader`.

Limitação reconhecida: a verificação por UI Automation não conseguiu ler o
atributo `src` do `<img>` diretamente (WebView2/Chromium não expõe
`ValuePattern` nesse nó de imagem). A mudança de origem da imagem a cada
navegação é inferida da mesma função `render()` que também atualiza o
contador (execução sequencial, sem branch entre as duas atribuições) e do
fato de que a leitura de `Current()` (que fornece `bookID`/`pageCount`) só
teve sucesso porque o `.cbz` sintético foi de fato aberto e teve suas 3
páginas contadas corretamente pelo `reader.Service` real — não é uma
suposição de código, é uma consequência observável do valor `"1 / 3"` (e não
um erro) aparecer na tela. A correção byte-a-byte da resposta HTTP
(`Content-Type`, `Content-Length`, status 200/404) já é coberta
diretamente por `internal/reader/handler_test.go` (Task 3).
